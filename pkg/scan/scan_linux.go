package scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Options struct {
	All        bool
	PreferByID bool
}

type Port struct {
	Device        string   `json:"device"`
	ByID          string   `json:"by_id,omitempty"`
	VID           string   `json:"vid,omitempty"`
	PID           string   `json:"pid,omitempty"`
	Manufacturer  string   `json:"manufacturer,omitempty"`
	Product       string   `json:"product,omitempty"`
	Serial        string   `json:"serial,omitempty"`
	Score         int      `json:"score"`
	Reasons       []string `json:"reasons,omitempty"`
	PreferredPath string   `json:"preferred_path,omitempty"`
}

func (p Port) VIDPID() string {
	if p.VID == "" && p.PID == "" {
		return "-"
	}
	return strings.ToLower(p.VID) + ":" + strings.ToLower(p.PID)
}

func ScanLinux(ctx context.Context, opts Options) ([]Port, error) {
	_ = ctx
	if runtime.GOOS != "linux" {
		return nil, errors.New("ScanLinux is supported only on linux")
	}

	devs := make(map[string]*Port)

	for _, pat := range []string{"/dev/ttyACM*", "/dev/ttyUSB*"} {
		m, _ := filepath.Glob(pat)
		for _, d := range m {
			devs[d] = &Port{Device: d}
		}
	}

	byID, err := scanByID()
	if err == nil {
		for dev, link := range byID {
			p := devs[dev]
			if p == nil {
				p = &Port{Device: dev}
				devs[dev] = p
			}
			p.ByID = link
		}
	}

	out := make([]Port, 0, len(devs))
	for dev, p := range devs {
		tty := filepath.Base(dev)
		attrs, _ := readUSBAttrs(tty)
		p.VID = attrs.VID
		p.PID = attrs.PID
		p.Manufacturer = attrs.Manufacturer
		p.Product = attrs.Product
		p.Serial = attrs.Serial

		scorePort(p)
		if opts.PreferByID && p.ByID != "" {
			p.PreferredPath = p.ByID
		}

		if !opts.All && p.Score < 50 {
			continue
		}
		out = append(out, *p)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Device < out[j].Device
	})

	return out, nil
}

type usbAttrs struct {
	VID          string
	PID          string
	Manufacturer string
	Product      string
	Serial       string
}

func readUSBAttrs(tty string) (usbAttrs, error) {
	sysDev := filepath.Join("/sys/class/tty", tty, "device")
	start, err := filepath.EvalSymlinks(sysDev)
	if err != nil {
		return usbAttrs{}, err
	}

	dir := start
	for i := 0; i < 12; i++ {
		vidPath := filepath.Join(dir, "idVendor")
		pidPath := filepath.Join(dir, "idProduct")
		if fileExists(vidPath) && fileExists(pidPath) {
			vid, _ := readTrim(vidPath)
			pid, _ := readTrim(pidPath)
			mfr, _ := readTrim(filepath.Join(dir, "manufacturer"))
			prod, _ := readTrim(filepath.Join(dir, "product"))
			ser, _ := readTrim(filepath.Join(dir, "serial"))
			return usbAttrs{VID: vid, PID: pid, Manufacturer: mfr, Product: prod, Serial: ser}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return usbAttrs{}, fmt.Errorf("no usb idVendor/idProduct found for %s", tty)
}

func scanByID() (map[string]string, error) {
	root := "/dev/serial/by-id"
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name())
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		out[target] = p
	}
	return out, nil
}

func scorePort(p *Port) {
	p.Score = 0
	p.Reasons = nil

	// Strong signal: Espressif VID.
	if strings.EqualFold(p.VID, "303a") {
		p.Score += 80
		p.Reasons = append(p.Reasons, "vid=303a (Espressif)")
	}

	// Common by-id names for USB Serial/JTAG.
	if strings.Contains(strings.ToLower(p.ByID), "espressif") {
		p.Score += 40
		p.Reasons = append(p.Reasons, "by-id contains 'Espressif'")
	}

	if strings.Contains(strings.ToLower(p.Product), "usb jtag") || strings.Contains(strings.ToLower(p.Product), "serial debug") {
		p.Score += 30
		p.Reasons = append(p.Reasons, "product looks like USB JTAG serial debug")
	}

	if strings.Contains(strings.ToLower(p.Manufacturer), "espressif") {
		p.Score += 30
		p.Reasons = append(p.Reasons, "manufacturer=Espressif")
	}

	// Bonus for stable path.
	if p.ByID != "" {
		p.Score += 10
	}

	// Clamp.
	if p.Score > 100 {
		p.Score = 100
	}
}

func readTrim(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

