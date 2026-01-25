package devices

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	currentVersion = 1
)

type Registry struct {
	Version int           `json:"version"`
	Devices []DeviceEntry `json:"devices"`
}

type DeviceEntry struct {
	USBSerial     string `json:"usb_serial"`
	Nickname      string `json:"nickname"`
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	PreferredPath string `json:"preferred_path,omitempty"`
}

func ConfigPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return filepath.Join(v, "esper", "devices.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "esper", "devices.json"), nil
}

func Load() (*Registry, string, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, "", err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Registry{Version: currentVersion}, path, nil
		}
		return nil, path, err
	}

	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, path, fmt.Errorf("parse devices registry %q: %w", path, err)
	}
	if r.Version == 0 {
		r.Version = currentVersion
	}
	return &r, path, nil
}

func Save(r *Registry) (string, error) {
	if r == nil {
		return "", errors.New("nil registry")
	}
	if r.Version == 0 {
		r.Version = currentVersion
	}

	path, err := ConfigPath()
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return path, err
	}

	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return path, err
	}
	b = append(b, '\n')

	if err := writeAtomic(path, b, 0o600); err != nil {
		return path, err
	}
	return path, nil
}

func (r *Registry) FindByUSBSerial(usbSerial string) *DeviceEntry {
	usbSerial = normalizeUSBSerial(usbSerial)
	if usbSerial == "" {
		return nil
	}
	for i := range r.Devices {
		if normalizeUSBSerial(r.Devices[i].USBSerial) == usbSerial {
			return &r.Devices[i]
		}
	}
	return nil
}

func (r *Registry) FindByNickname(nickname string) *DeviceEntry {
	nickname = normalizeNickname(nickname)
	if nickname == "" {
		return nil
	}
	for i := range r.Devices {
		if normalizeNickname(r.Devices[i].Nickname) == nickname {
			return &r.Devices[i]
		}
	}
	return nil
}

func (r *Registry) Upsert(e DeviceEntry) error {
	e.USBSerial = normalizeUSBSerial(e.USBSerial)
	e.Nickname = normalizeNickname(e.Nickname)
	if e.USBSerial == "" {
		return errors.New("missing usb_serial")
	}
	if e.Nickname == "" {
		return errors.New("missing nickname")
	}

	for _, other := range r.Devices {
		if normalizeUSBSerial(other.USBSerial) == e.USBSerial {
			continue
		}
		if normalizeNickname(other.Nickname) == e.Nickname {
			return fmt.Errorf("nickname %q already in use by usb_serial=%q", e.Nickname, other.USBSerial)
		}
	}

	if existing := r.FindByUSBSerial(e.USBSerial); existing != nil {
		*existing = e
		return nil
	}
	r.Devices = append(r.Devices, e)
	return nil
}

func (r *Registry) RemoveByUSBSerial(usbSerial string) bool {
	usbSerial = normalizeUSBSerial(usbSerial)
	if usbSerial == "" {
		return false
	}
	out := r.Devices[:0]
	removed := false
	for _, d := range r.Devices {
		if normalizeUSBSerial(d.USBSerial) == usbSerial {
			removed = true
			continue
		}
		out = append(out, d)
	}
	r.Devices = out
	return removed
}

func normalizeNickname(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeUSBSerial(s string) string {
	return strings.TrimSpace(s)
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".devices.json.tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
