package decode

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
		return [][]byte{[]byte("--- Core dump started (muting output)\n")}, false
	}

	if d.state == coreDumpReading {
		if bytes.Contains(line, CoreDumpEnd) {
			// decode
			decoded := d.decodeOrRaw()
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

func (d *CoreDumpDecoder) decodeOrRaw() [][]byte {
	if d.ElfPath == "" {
		return [][]byte{
			[]byte("--- Core dump captured (no -elf provided; raw below)\n"),
			d.buf,
		}
	}

	tmp, err := os.CreateTemp("", "esper-coredump-*.b64")
	if err != nil {
		return [][]byte{
			[]byte(fmt.Sprintf("--- Core dump captured; tempfile error: %v; raw below\n", err)),
			d.buf,
		}
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(d.buf); err != nil {
		tmp.Close()
		return [][]byte{
			[]byte(fmt.Sprintf("--- Core dump captured; write error: %v; raw below\n", err)),
			d.buf,
		}
	}
	_ = tmp.Close()

	// Use esp_coredump (Python) for parity in Phase 1.
	py := fmt.Sprintf(
		"import esp_coredump\nc=esp_coredump.CoreDump(core=%q, core_format='b64', prog=%q)\nc.info_corefile()",
		tmp.Name(),
		d.ElfPath,
	)
	cmd := exec.Command("python3", "-c", py)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return [][]byte{
			[]byte(fmt.Sprintf("--- Core dump decode failed (%v); raw below\n", err)),
			d.buf,
		}
	}
	return [][]byte{[]byte("--- Core dump report:\n"), out}
}

