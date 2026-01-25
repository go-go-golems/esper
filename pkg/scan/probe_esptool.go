package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type EsptoolProbeOptions struct {
	Port            string
	Baud            int
	ConnectMode     string
	ConnectAttempts int
	After           string // hard_reset | no_reset
}

type EsptoolProbeResult struct {
	OK               bool     `json:"ok"`
	ChipName         string   `json:"chip_name,omitempty"`
	ChipDescription  string   `json:"chip_description,omitempty"`
	SecureDLMode     bool     `json:"secure_download_mode,omitempty"`
	Features         []string `json:"features,omitempty"`
	CrystalMHz       int      `json:"crystal_mhz,omitempty"`
	USBMode          string   `json:"usb_mode,omitempty"`
	MAC              string   `json:"mac,omitempty"`
	Error            string   `json:"error,omitempty"`
	EsptoolLog       string   `json:"esptool_log,omitempty"`
	ImplementationID string   `json:"implementation_id,omitempty"`
}

func ProbeEsptool(ctx context.Context, opts EsptoolProbeOptions) (*EsptoolProbeResult, error) {
	if opts.Port == "" {
		return nil, errors.New("missing port")
	}
	if opts.Baud == 0 {
		opts.Baud = 115200
	}
	if opts.ConnectMode == "" {
		opts.ConnectMode = "default_reset"
	}
	if opts.ConnectAttempts <= 0 {
		opts.ConnectAttempts = 3
	}
	if opts.After == "" {
		opts.After = "hard_reset"
	}
	if opts.After != "hard_reset" && opts.After != "no_reset" {
		return nil, fmt.Errorf("invalid after: %q", opts.After)
	}

	// Note: we intentionally use the installed esptool module (ESP-IDF python env),
	// and run a tiny snippet that outputs only JSON. We suppress stdout/stderr from
	// esptool internals (detect_chip prints "Detecting chip type..." etc).
	py := fmt.Sprintf(`
import contextlib, io, json
from esptool.cmds import detect_chip

out = {"ok": False, "implementation_id": "python-esptool-detect_chip"}
buf = io.StringIO()
try:
  with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(buf):
    esp = detect_chip(
      port=%q,
      baud=%d,
      connect_mode=%q,
      connect_attempts=%d,
      trace_enabled=False,
    )
    out["chip_name"] = getattr(esp, "CHIP_NAME", "")
    out["secure_download_mode"] = bool(getattr(esp, "secure_download_mode", False))
    if not out["secure_download_mode"]:
      try: out["chip_description"] = esp.get_chip_description()
      except Exception: pass
      try: out["features"] = list(esp.get_chip_features())
      except Exception: pass
      try: out["crystal_mhz"] = int(esp.get_crystal_freq())
      except Exception: pass
      try:
        m = esp.get_usb_mode()
        out["usb_mode"] = m if m is not None else ""
      except Exception: pass
      try:
        mac = esp.read_mac()
        if isinstance(mac, (bytes, bytearray)) and len(mac) == 6:
          out["mac"] = ":".join(["%%02x" %% b for b in mac])
      except Exception: pass

    if %q == "hard_reset":
      try: esp.hard_reset()
      except Exception: pass
    try: esp._port.close()
    except Exception: pass
  out["ok"] = True
except Exception as e:
  out["error"] = str(e)
out["esptool_log"] = buf.getvalue()
print(json.dumps(out))
`, opts.Port, opts.Baud, opts.ConnectMode, opts.ConnectAttempts, opts.After)

	cmd := exec.CommandContext(ctx, "python3", "-c", py)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// esptool can take a few seconds; give a reasonable upper bound.
	if deadline, ok := ctx.Deadline(); ok {
		_ = deadline
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd = exec.CommandContext(ctx, "python3", "-c", py)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	rawOut := strings.TrimSpace(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("python3 esptool probe failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if rawOut == "" {
		return nil, fmt.Errorf("python3 esptool probe returned empty output: %s", strings.TrimSpace(stderr.String()))
	}

	var res EsptoolProbeResult
	if jerr := json.Unmarshal([]byte(rawOut), &res); jerr != nil {
		return nil, fmt.Errorf("parse esptool probe json: %w (out=%q stderr=%q)", jerr, rawOut, strings.TrimSpace(stderr.String()))
	}
	// Attach any unexpected stderr output (should be empty, but keep for debugging).
	if strings.TrimSpace(stderr.String()) != "" && res.EsptoolLog == "" {
		res.EsptoolLog = strings.TrimSpace(stderr.String())
	}
	if !res.OK {
		if res.Error == "" {
			res.Error = "unknown error"
		}
		return &res, errors.New(res.Error)
	}
	return &res, nil
}
