package decode

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var backtraceRE = regexp.MustCompile(`0x[0-9a-fA-F]{8}`)

type PanicDecoder struct {
	ElfPath         string
	ToolchainPrefix string
}

func (d *PanicDecoder) DecodeBacktraceLine(line []byte) ([]byte, bool) {
	// Common ESP-IDF format: "Backtrace: 0x....:0x.... 0x....:0x...."
	if !bytes.Contains(line, []byte("Backtrace:")) {
		return nil, false
	}
	addrs := backtraceRE.FindAll(line, -1)
	if len(addrs) == 0 {
		return []byte("--- Backtrace: no addresses found\n"), true
	}

	if d.ElfPath == "" || d.ToolchainPrefix == "" {
		return []byte(fmt.Sprintf("--- Backtrace PCs: %s\n", strings.Join(bytesToStrings(addrs), " "))), true
	}

	// decode each PC address (the first address of each pair). Heuristic: take every other match.
	var pcs [][]byte
	for i := 0; i < len(addrs); i += 2 {
		pcs = append(pcs, addrs[i])
	}
	out := &bytes.Buffer{}
	out.WriteString("--- Decoded backtrace (addr2line):\n")
	for _, pc := range pcs {
		s, err := addr2line(d.ToolchainPrefix, d.ElfPath, string(pc))
		if err != nil {
			out.WriteString(fmt.Sprintf("---   %s: %v\n", pc, err))
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
			out.WriteString("---   ")
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), true
}

func addr2line(toolchainPrefix, elfPath, addr string) (string, error) {
	cmd := exec.Command(toolchainPrefix+"addr2line", "-pfiaC", "-e", elfPath, addr)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", cmd.String(), err, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

func bytesToStrings(bss [][]byte) []string {
	out := make([]string, 0, len(bss))
	for _, b := range bss {
		out = append(out, string(b))
	}
	return out
}
