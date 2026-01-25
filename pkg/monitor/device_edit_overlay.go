package monitor

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-go-golems/esper/pkg/devices"
)

type deviceEditOverlayConfig struct {
	title  string
	isEdit bool
	entry  devices.DeviceEntry

	// If set, USB serial is fixed and not editable.
	fixedUSBSerial string
}

type deviceEditOverlay struct {
	sz  size
	cfg deviceEditOverlayConfig

	focus int

	usbSerial     string
	nickname      string
	name          string
	description   string
	preferredPath string

	err string
}

func newDeviceEditOverlay(cfg deviceEditOverlayConfig) *deviceEditOverlay {
	o := &deviceEditOverlay{cfg: cfg}
	o.usbSerial = strings.TrimSpace(cfg.entry.USBSerial)
	o.nickname = strings.TrimSpace(cfg.entry.Nickname)
	o.name = strings.TrimSpace(cfg.entry.Name)
	o.description = strings.TrimSpace(cfg.entry.Description)
	o.preferredPath = strings.TrimSpace(cfg.entry.PreferredPath)
	if cfg.fixedUSBSerial != "" {
		o.usbSerial = strings.TrimSpace(cfg.fixedUSBSerial)
	}
	return o
}

func (o *deviceEditOverlay) setSize(sz size) { o.sz = sz }

func (o *deviceEditOverlay) open() {
	o.err = ""
	o.focus = 0
}

func (o *deviceEditOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, overlayOutcome{}
	}

	switch k.Type {
	case tea.KeyEsc:
		return o, nil, overlayOutcome{close: true}
	case tea.KeyTab:
		o.focus = (o.focus + 1) % o.maxFocus()
		return o, nil, overlayOutcome{}
	case tea.KeyShiftTab:
		o.focus = (o.focus + o.maxFocus() - 1) % o.maxFocus()
		return o, nil, overlayOutcome{}
	case tea.KeyEnter:
		if o.focus == o.maxFocus()-1 {
			if err := o.save(); err != nil {
				o.err = err.Error()
				return o, nil, overlayOutcome{}
			}
			return o, nil, overlayOutcome{close: true, forward: devicesRegistryChangedMsg{}}
		}
		// Enter acts as "next field" until buttons.
		o.focus = (o.focus + 1) % o.maxFocus()
		return o, nil, overlayOutcome{}
	}

	switch o.focusField() {
	case "usb":
		if o.cfg.fixedUSBSerial == "" {
			o.usbSerial = updateSingleLineField(o.usbSerial, k)
		}
	case "nick":
		o.nickname = updateSingleLineField(o.nickname, k)
	case "name":
		o.name = updateSingleLineField(o.name, k)
	case "desc":
		o.description = updateSingleLineField(o.description, k)
	case "path":
		o.preferredPath = updateSingleLineField(o.preferredPath, k)
	}

	return o, nil, overlayOutcome{}
}

func (o *deviceEditOverlay) View(st styles) string {
	boxW := max(66, min(96, o.sz.W-6))
	boxH := max(16, min(20, o.sz.H-6))

	innerW := max(0, boxW-st.OverlayBox.GetHorizontalFrameSize())

	title := st.PanelTitle.Render(o.cfg.title)

	usb := fmt.Sprintf("USB Serial: [ %s ]", padOrTrim(o.usbSerial, max(10, innerW-16)))
	if o.cfg.fixedUSBSerial != "" {
		usb = st.Hint.Render("USB Serial: " + o.usbSerial)
	}
	nick := fmt.Sprintf("Nickname:   [ %s ]", padOrTrim(o.nickname, max(10, innerW-16)))
	name := fmt.Sprintf("Name:       [ %s ]", padOrTrim(o.name, max(10, innerW-16)))
	desc := fmt.Sprintf("Description:[ %s ]", padOrTrim(o.description, max(10, innerW-16)))
	pp := fmt.Sprintf("Preferred:  [ %s ]", padOrTrim(o.preferredPath, max(10, innerW-16)))

	if o.focusField() == "usb" && o.cfg.fixedUSBSerial == "" {
		usb = st.SelectedRow.Render(usb)
	}
	if o.focusField() == "nick" {
		nick = st.SelectedRow.Render(nick)
	}
	if o.focusField() == "name" {
		name = st.SelectedRow.Render(name)
	}
	if o.focusField() == "desc" {
		desc = st.SelectedRow.Render(desc)
	}
	if o.focusField() == "path" {
		pp = st.SelectedRow.Render(pp)
	}

	cancel := st.Row.Render("[ Cancel ]")
	save := st.Row.Render("[ Save ]")
	if o.focusField() == "btn" {
		save = st.SelectedRow.Render("[ Save ]")
	} else {
		cancel = st.Hint.Render("[ Cancel ]")
	}
	actions := lipgloss.JoinHorizontal(lipgloss.Top, save, "   ", cancel)
	actions = lipgloss.PlaceHorizontal(innerW, lipgloss.Right, actions)

	hint := st.Hint.Render("Tab Next field   Enter Save   Esc Cancel")
	if o.err != "" {
		hint = st.Hint.Render("Error: " + o.err)
	}

	lines := []string{
		title,
		"",
		usb,
		nick,
		name,
		desc,
		pp,
		"",
		actions,
		"",
		hint,
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return st.OverlayBox.Copy().Width(boxW).Height(boxH).Render(content)
}

func (o *deviceEditOverlay) maxFocus() int {
	// usb, nick, name, desc, path, buttons
	if o.cfg.fixedUSBSerial != "" {
		return 5 // skip usb edit focus
	}
	return 6
}

func (o *deviceEditOverlay) focusField() string {
	if o.cfg.fixedUSBSerial != "" {
		switch o.focus {
		case 0:
			return "nick"
		case 1:
			return "name"
		case 2:
			return "desc"
		case 3:
			return "path"
		default:
			return "btn"
		}
	}

	switch o.focus {
	case 0:
		return "usb"
	case 1:
		return "nick"
	case 2:
		return "name"
	case 3:
		return "desc"
	case 4:
		return "path"
	default:
		return "btn"
	}
}

func (o *deviceEditOverlay) save() error {
	usb := strings.TrimSpace(o.usbSerial)
	nick := strings.TrimSpace(o.nickname)
	if o.cfg.fixedUSBSerial != "" {
		usb = strings.TrimSpace(o.cfg.fixedUSBSerial)
	}
	if usb == "" {
		return fmt.Errorf("missing USB serial")
	}
	if nick == "" {
		return fmt.Errorf("missing nickname")
	}

	reg, _, err := devices.Load()
	if err != nil {
		return err
	}

	entry := devices.DeviceEntry{
		USBSerial:     usb,
		Nickname:      nick,
		Name:          strings.TrimSpace(o.name),
		Description:   strings.TrimSpace(o.description),
		PreferredPath: strings.TrimSpace(o.preferredPath),
	}
	if err := reg.Upsert(entry); err != nil {
		return err
	}
	_, err = devices.Save(reg)
	return err
}
