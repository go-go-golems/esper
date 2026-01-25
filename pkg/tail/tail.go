package tail

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/go-go-golems/esper/pkg/decode"
	"github.com/go-go-golems/esper/pkg/parse"
	"github.com/go-go-golems/esper/pkg/render"
	"github.com/go-go-golems/esper/pkg/serialio"
	"go.bug.st/serial"
)

type Config struct {
	Port            string
	Baud            int
	ElfPath         string
	ToolchainPrefix string

	Timeout time.Duration

	StdinRaw bool

	NoAutoColor bool
	NoBacktrace bool
	NoGDB       bool
	NoCoreDump  bool

	CoreDumpAutoEnter bool
	CoreDumpMute      bool
	NoCoreDumpDecode  bool

	Timestamps bool
	PrefixPort bool

	LogFile   string
	LogAppend bool
	NoStdout  bool
}

func (c Config) validate() error {
	if c.Port == "" {
		return errors.New("missing port")
	}
	if c.Baud <= 0 {
		return fmt.Errorf("invalid baud: %d", c.Baud)
	}
	if c.NoStdout && c.LogFile == "" {
		return errors.New("--no-stdout requires --log-file")
	}
	return nil
}

func Run(ctx context.Context, cfg Config, stdout, stderr io.Writer) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	if cfg.CoreDumpAutoEnter == false && cfg.NoCoreDump {
		// no-op; just a hint that options don’t conflict
	}

	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	out, closeFn, err := newOutputs(cfg, stdout, stderr)
	if err != nil {
		return err
	}
	defer closeFn()

	p, openedPath, err := serialio.Open(serialio.OpenConfig{Port: cfg.Port, Baud: cfg.Baud})
	if err != nil {
		return err
	}
	defer p.Close()
	_ = p.SetReadTimeout(200 * time.Millisecond)

	if cfg.PrefixPort {
		cfg.Port = openedPath
	}

	return runLoop(ctx, cfg, p, out)
}

type outputs struct {
	out *bufio.Writer
	err *bufio.Writer
}

func newOutputs(cfg Config, stdout, stderr io.Writer) (out *outputs, closeFn func(), err error) {
	closeFn = func() {}

	var outW io.Writer = io.Discard
	if !cfg.NoStdout {
		outW = stdout
	}

	var logFile *os.File
	if cfg.LogFile != "" {
		flags := os.O_CREATE | os.O_WRONLY
		if cfg.LogAppend {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, openErr := os.OpenFile(cfg.LogFile, flags, 0o644)
		if openErr != nil {
			return nil, nil, openErr
		}
		logFile = f
		outW = io.MultiWriter(outW, logFile)
		if !cfg.NoStdout {
			// Also tee stderr to the log file so notices don’t get lost.
			stderr = io.MultiWriter(stderr, logFile)
		}
	}

	closeFn = func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}

	return &outputs{
		out: bufio.NewWriterSize(outW, 32*1024),
		err: bufio.NewWriterSize(stderr, 16*1024),
	}, closeFn, nil
}

func runLoop(ctx context.Context, cfg Config, port serial.Port, out *outputs) error {
	if cfg.StdinRaw {
		return runLoopWithStdinRaw(ctx, cfg, port, out)
	}

	var (
		lineSplitter parse.LineSplitter
		autoColor    render.AutoColorer
		gdb          decode.GDBStubDetector
		panicDec     decode.PanicDecoder
		coreDump     decode.CoreDumpDecoder
	)

	autoColor.DisableAutoColor = cfg.NoAutoColor
	panicDec = decode.PanicDecoder{ElfPath: cfg.ElfPath, ToolchainPrefix: cfg.ToolchainPrefix}

	if !cfg.NoCoreDump {
		coreDump.ElfPath = cfg.ElfPath
		if cfg.NoCoreDumpDecode {
			coreDump.ElfPath = ""
		}
	}

	if cfg.CoreDumpAutoEnter == false {
		// explicit false allowed; default will be set by caller
	}

	lastDataAt := time.Now()

	writeNotice := func(s string) {
		_ = writePrefixedBytes(out.err, cfg, []byte(s))
		_ = out.err.Flush()
	}

	writeOut := func(b []byte) {
		_ = writePrefixedBytes(out.out, cfg, b)
		_ = out.out.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			flushTail(cfg, &lineSplitter, &autoColor, writeOut)
			return nil
		default:
		}

		buf := make([]byte, 4096)
		n, err := port.Read(buf)
		if err != nil {
			writeNotice(fmt.Sprintf("--- serial error: %v\n", err))
			continue
		}
		if n <= 0 {
			if time.Since(lastDataAt) > 250*time.Millisecond {
				if tail := lineSplitter.FinalizeTail(); len(tail) > 0 {
					writeOut(autoColor.ColorizeLine(append(tail, '\n')))
				}
			}
			continue
		}
		lastDataAt = time.Now()

		chunk := buf[:n]

		if !cfg.NoGDB {
			if g := gdb.Push(chunk); g != nil {
				writeNotice("--- GDB stub detected\n")
			}
		}

		lines := lineSplitter.Push(chunk)
		for _, line := range lines {
			if !cfg.NoCoreDump {
				events, sendEnter := coreDump.PushLine(line)
				if sendEnter {
					if cfg.CoreDumpAutoEnter {
						_, _ = port.Write([]byte("\n"))
					} else {
						writeNotice("--- Core dump prompt detected; NOT sending Enter (--coredump-auto-enter=false)\n")
					}
				}
				if cfg.CoreDumpMute && coreDump.InProgress() {
					continue
				}
				for _, e := range events {
					writeOut(e)
				}
			}

			if !cfg.NoBacktrace {
				if decoded, ok := panicDec.DecodeBacktraceLine(line); ok && len(decoded) > 0 {
					writeOut(decoded)
				}
			}

			outLine := line
			if !cfg.NoAutoColor {
				outLine = autoColor.ColorizeLine(outLine)
			}
			writeOut(outLine)
		}
	}
}

func runLoopWithStdinRaw(ctx context.Context, cfg Config, port serial.Port, out *outputs) error {
	var (
		lineSplitter parse.LineSplitter
		autoColor    render.AutoColorer
		gdb          decode.GDBStubDetector
		panicDec     decode.PanicDecoder
		coreDump     decode.CoreDumpDecoder
	)

	autoColor.DisableAutoColor = cfg.NoAutoColor
	panicDec = decode.PanicDecoder{ElfPath: cfg.ElfPath, ToolchainPrefix: cfg.ToolchainPrefix}

	if !cfg.NoCoreDump {
		coreDump.ElfPath = cfg.ElfPath
		if cfg.NoCoreDumpDecode {
			coreDump.ElfPath = ""
		}
	}

	writeNotice := func(s string) {
		_ = writePrefixedBytes(out.err, cfg, []byte(s))
		_ = out.err.Flush()
	}

	writeOut := func(b []byte) {
		_ = writePrefixedBytes(out.out, cfg, b)
		_ = out.out.Flush()
	}

	// Put stdin into raw mode when it is a TTY, so keystrokes are delivered as bytes.
	var restore func()
	fd := os.Stdin.Fd()
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("stdin raw: %w", err)
		}
		restore = func() { _ = term.Restore(fd, state) }
	} else {
		restore = func() {}
	}
	defer restore()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Centralize writes to the port to avoid concurrent writes (auto-enter + stdin).
	writeCh := make(chan []byte, 64)
	writeErrCh := make(chan error, 1)
	go func() {
		defer close(writeErrCh)
		for b := range writeCh {
			if len(b) == 0 {
				continue
			}
			if _, err := port.Write(b); err != nil {
				writeErrCh <- err
				return
			}
		}
	}()

	// Read stdin bytes and forward to device.
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				cancel()
				return
			}
			if n <= 0 {
				continue
			}
			for i := 0; i < n; i++ {
				// Ctrl-] exits always.
				if buf[i] == 0x1d {
					cancel()
					return
				}
			}
			select {
			case writeCh <- append([]byte{}, buf[:n]...):
			case <-ctx.Done():
				return
			}
		}
	}()

	lastDataAt := time.Now()

	for {
		select {
		case <-ctx.Done():
			close(writeCh)
			flushTail(cfg, &lineSplitter, &autoColor, writeOut)
			return nil
		case err := <-writeErrCh:
			if err != nil {
				writeNotice(fmt.Sprintf("--- serial write error: %v\n", err))
			}
			close(writeCh)
			return nil
		default:
		}

		buf := make([]byte, 4096)
		n, err := port.Read(buf)
		if err != nil {
			writeNotice(fmt.Sprintf("--- serial error: %v\n", err))
			continue
		}
		if n <= 0 {
			if time.Since(lastDataAt) > 250*time.Millisecond {
				if tail := lineSplitter.FinalizeTail(); len(tail) > 0 {
					writeOut(autoColor.ColorizeLine(append(tail, '\n')))
				}
			}
			continue
		}
		lastDataAt = time.Now()

		chunk := buf[:n]

		if !cfg.NoGDB {
			if g := gdb.Push(chunk); g != nil {
				writeNotice("--- GDB stub detected\n")
			}
		}

		lines := lineSplitter.Push(chunk)
		for _, line := range lines {
			if !cfg.NoCoreDump {
				events, sendEnter := coreDump.PushLine(line)
				if sendEnter {
					if cfg.CoreDumpAutoEnter {
						select {
						case writeCh <- []byte("\n"):
						default:
						}
					} else {
						writeNotice("--- Core dump prompt detected; NOT sending Enter (--coredump-auto-enter=false)\n")
					}
				}
				if cfg.CoreDumpMute && coreDump.InProgress() {
					continue
				}
				for _, e := range events {
					writeOut(e)
				}
			}

			if !cfg.NoBacktrace {
				if decoded, ok := panicDec.DecodeBacktraceLine(line); ok && len(decoded) > 0 {
					writeOut(decoded)
				}
			}

			outLine := line
			if !cfg.NoAutoColor {
				outLine = autoColor.ColorizeLine(outLine)
			}
			writeOut(outLine)
		}
	}
}

func flushTail(cfg Config, splitter *parse.LineSplitter, autoColor *render.AutoColorer, writeOut func([]byte)) {
	if splitter == nil {
		return
	}
	tail := splitter.FinalizeTail()
	if len(tail) == 0 {
		return
	}
	line := append([]byte{}, tail...)
	if !bytes.HasSuffix(line, []byte("\n")) {
		line = append(line, '\n')
	}
	if !cfg.NoAutoColor && autoColor != nil {
		line = autoColor.ColorizeLine(line)
	}
	writeOut(line)
}

func writePrefixedBytes(w *bufio.Writer, cfg Config, b []byte) error {
	if w == nil {
		return nil
	}
	if !cfg.Timestamps && !cfg.PrefixPort {
		_, err := w.Write(b)
		return err
	}

	parts := bytes.SplitAfter(b, []byte("\n"))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		prefix := ""
		if cfg.Timestamps {
			prefix += time.Now().Format("15:04:05") + " "
		}
		if cfg.PrefixPort {
			prefix += cfg.Port + " "
		}
		if prefix != "" {
			if _, err := w.WriteString(prefix); err != nil {
				return err
			}
		}
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}
