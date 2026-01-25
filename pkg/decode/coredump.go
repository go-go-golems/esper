package decode

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"
)

var (
	CoreDumpStart  = []byte("================= CORE DUMP START =================")
	CoreDumpEnd    = []byte("================= CORE DUMP END =================")
	CoreDumpPrompt = []byte("Press Enter to print core dump to UART...")
)

type CoreDumpDecoder struct {
	ElfPath string

	state int
	buf   []byte

	lastOk     bool
	lastResult CoreDumpResult
}

func (d *CoreDumpDecoder) BufferedBytes() int {
	return len(d.buf)
}

type CoreDumpResult struct {
	SavedPath string
	RawBytes  int

	DecodedOK bool
	Report    []byte
	DecodeErr string

	HadElf bool
}

func (d *CoreDumpDecoder) LastResult() (CoreDumpResult, bool) {
	if !d.lastOk {
		return CoreDumpResult{}, false
	}
	return d.lastResult, true
}

func (d *CoreDumpDecoder) Abort() {
	d.state = coreDumpIdle
	d.buf = nil
}

const (
	coreDumpIdle = iota
	coreDumpReading
)

func (d *CoreDumpDecoder) InProgress() bool {
	return d.state != coreDumpIdle
}

func (d *CoreDumpDecoder) PushLine(line []byte) (events [][]byte, sendEnter bool) {
	line = bytes.TrimRight(line, "\r\n")
	if bytes.Contains(line, CoreDumpPrompt) {
		// mimic esp_idf_monitor behavior: auto-send Enter to start dump
		return [][]byte{[]byte("--- Core dump prompt detected; sending Enter\n")}, true
	}

	if bytes.Contains(line, CoreDumpStart) {
		d.state = coreDumpReading
		d.buf = nil
		d.lastOk = false
		return [][]byte{[]byte("--- Core dump started (muting output)\n")}, false
	}

	if d.state == coreDumpReading {
		if bytes.Contains(line, CoreDumpEnd) {
			res, decoded := d.decodeOrRaw(time.Now())
			d.lastOk = true
			d.lastResult = res

			events = append(events, decoded...)
			events = append(events, []byte("--- Core dump finished\n"))
			d.state = coreDumpIdle
			d.buf = nil
			return events, false
		}
		d.buf = append(d.buf, line...)
		d.buf = append(d.buf, '\n')
		return nil, false
	}

	return nil, false
}

func (d *CoreDumpDecoder) decodeOrRaw(now time.Time) (CoreDumpResult, [][]byte) {
	res := CoreDumpResult{
		RawBytes: len(d.buf),
		HadElf:   d.ElfPath != "",
	}

	tmp, err := os.CreateTemp("", fmt.Sprintf("esper-coredump-%s-*.b64", now.Format("20060102-150405")))
	if err != nil {
		res.DecodeErr = fmt.Sprintf("tempfile error: %v", err)
		return res, [][]byte{[]byte(fmt.Sprintf("--- Core dump captured; %s\n", res.DecodeErr))}
	}
	res.SavedPath = tmp.Name()
	if _, err := tmp.Write(d.buf); err != nil {
		_ = tmp.Close()
		res.DecodeErr = fmt.Sprintf("write error: %v", err)
		return res, [][]byte{[]byte(fmt.Sprintf("--- Core dump captured; %s (raw at %s)\n", res.DecodeErr, res.SavedPath))}
	}
	_ = tmp.Close()

	if d.ElfPath == "" {
		res.DecodeErr = "no -elf provided"
		return res, [][]byte{[]byte(fmt.Sprintf("--- Core dump captured (no -elf provided; raw saved to %s)\n", res.SavedPath))}
	}

	// Use esp_coredump (Python) for parity in Phase 1.
	py := fmt.Sprintf(
		"import esp_coredump\nc=esp_coredump.CoreDump(core=%q, core_format='b64', prog=%q)\nc.info_corefile()",
		res.SavedPath,
		d.ElfPath,
	)
	cmd := exec.Command("python3", "-c", py)
	out, err := cmd.CombinedOutput()
	if err != nil {
		res.DecodeErr = fmt.Sprintf("%v", err)
		res.Report = out
		return res, [][]byte{
			[]byte(fmt.Sprintf("--- Core dump decode failed (%s); raw saved to %s\n", res.DecodeErr, res.SavedPath)),
		}
	}
	res.DecodedOK = true
	res.Report = out
	return res, [][]byte{
		[]byte(fmt.Sprintf("--- Core dump saved to %s\n", res.SavedPath)),
		[]byte("--- Core dump report:\n"),
		out,
	}
}
