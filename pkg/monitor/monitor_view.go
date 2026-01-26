package monitor

// Type definitions, newMonitorModel, and setSize moved to monitor_model.go

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

func (m monitorModel) Update(msg tea.Msg, curMode mode) (monitorModel, tea.Cmd, monitorAction) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		// Global disconnect (matches UX spec).
		if t.String() == "ctrl+d" {
			return m, nil, monitorAction{kind: monitorActionDisconnect, reason: "user disconnect"}
		}

		// Core dump capture mode: disable input and shortcuts; allow abort.
		if m.coredump.InProgress() {
			if t.Type == tea.KeyCtrlC {
				m.coredump.Abort()
				m.coreDumpStartedAt = time.Time{}
				m.addEvent("coredump", "Core dump capture aborted", "User aborted core dump capture (Ctrl-C).")
				m.setToast("Core dump capture aborted", 2*time.Second)
				return m, nil, monitorAction{}
			}
			return m, nil, monitorAction{}
		}

		if t.String() == "ctrl+t" {
			if curMode == modeHost {
				// UX spec wants ctrl+t t for command palette, but ctrl+t alone exits host mode.
				// We implement this as a short prefix window: if the next key is 't' quickly, open palette;
				// otherwise, fall back to exiting host mode.
				m.ctrlTPending = true
				m.ctrlTPendingID++
				id := m.ctrlTPendingID
				return m, ctrlTPrefixTimeoutCmd(id, 350*time.Millisecond), monitorAction{}
			}
			m.follow = false
			m.hostFocus = hostFocusViewport
			return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeHost}
		}

		if curMode == modeHost {
			if m.searchActive {
				switch t.Type {
				case tea.KeyEsc:
					m.closeSearch()
					return m, nil, monitorAction{}
				case tea.KeyEnter:
					m.searchApplyJump()
					return m, nil, monitorAction{}
				}

				switch t.String() {
				case "n", "ctrl+n":
					m.searchComputeMatches()
					if len(m.searchMatches) == 0 {
						m.setToast("no matches", 2*time.Second)
						return m, nil, monitorAction{}
					}
					m.searchCur = searchNextIndex(m.searchCur, len(m.searchMatches))
					return m, nil, monitorAction{}
				case "N", "ctrl+p":
					m.searchComputeMatches()
					if len(m.searchMatches) == 0 {
						m.setToast("no matches", 2*time.Second)
						return m, nil, monitorAction{}
					}
					m.searchCur = searchPrevIndex(m.searchCur, len(m.searchMatches))
					return m, nil, monitorAction{}
				}

				prevQuery := m.searchInput.Value()
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(t)
				m.searchQuery = m.searchInput.Value()
				if m.searchQuery != prevQuery {
					m.searchCur = 0
				}
				m.searchComputeMatches()
				return m, cmd, monitorAction{}
			}

			if m.ctrlTPending {
				switch t.Type {
				case tea.KeyEsc:
					m.ctrlTPending = false
					m.follow = true
					m.showInspector = false
					m.hostFocus = hostFocusViewport
					return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
				}
				if t.String() == "t" {
					// ctrl+t t opens command palette.
					m.ctrlTPending = false
					return m, nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newPaletteOverlay()}
				}
				// During the ctrl+t prefix window, ignore other host shortcuts.
				return m, nil, monitorAction{}
			}

			// HOST mode shortcuts (wireframe parity).
			switch t.String() {
			case "ctrl+r":
				return m, nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newResetConfirmOverlay()}
			case "ctrl+s":
				if err := m.toggleSessionLogging(); err != nil {
					m.setToast(fmt.Sprintf("log: %v", err), 3*time.Second)
				}
				return m, nil, monitorAction{}
			case "ctrl+l":
				m.out = ""
				m.log = nil
				m.viewport.SetContent("")
				m.events = nil
				m.eventList.Selected = 0
				m.setToast("viewport cleared", 2*time.Second)
				return m, nil, monitorAction{}
			case "w":
				m.wrap = !m.wrap
				m.refreshViewportContent()
				if m.wrap {
					m.setToast("wrap: ON", 2*time.Second)
				} else {
					m.setToast("wrap: OFF", 2*time.Second)
				}
				return m, nil, monitorAction{}
			}

			// Host-mode overlays.
			switch t.String() {
			case "/":
				m.openSearch()
				return m, nil, monitorAction{}
			case "f":
				return m, nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newFilterOverlay(m.filterCfg)}
			}

			// Exit to device mode.
			if t.Type == tea.KeyEsc {
				if m.searchActive {
					m.closeSearch()
					return m, nil, monitorAction{}
				}
				m.ctrlTPending = false
				m.follow = true
				m.showInspector = false
				m.hostFocus = hostFocusViewport
				return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
			}

			// Host-mode inspector toggle.
			if t.String() == "i" {
				m.showInspector = !m.showInspector
				if !m.showInspector {
					m.hostFocus = hostFocusViewport
				}
				m.setSize(m.sz)
				return m, nil, monitorAction{}
			}

			if m.showInspector && t.Type == tea.KeyTab {
				if m.hostFocus == hostFocusViewport {
					m.hostFocus = hostFocusInspector
				} else {
					m.hostFocus = hostFocusViewport
				}
				return m, nil, monitorAction{}
			}

			switch t.Type {
			case tea.KeyPgUp:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineUp(10)
				} else {
					m.eventList.Move(-10, len(m.events))
				}
			case tea.KeyPgDown:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineDown(10)
				} else {
					m.eventList.Move(10, len(m.events))
				}
			case tea.KeyUp:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineUp(1)
				} else {
					m.eventList.Move(-1, len(m.events))
				}
			case tea.KeyDown:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineDown(1)
				} else {
					m.eventList.Move(1, len(m.events))
				}
			case tea.KeyHome:
				if m.hostFocus == hostFocusViewport {
					m.viewport.GotoTop()
				} else {
					m.eventList.Selected = 0
					m.eventList.SetLen(len(m.events))
				}
			case tea.KeyEnd:
				if m.hostFocus == hostFocusViewport {
					m.viewport.GotoBottom()
				} else {
					m.eventList.Selected = max(0, len(m.events)-1)
					m.eventList.SetLen(len(m.events))
				}
			}

			if m.showInspector && m.hostFocus == hostFocusInspector && (t.Type == tea.KeyEnter || t.String() == "enter") {
				i := m.eventList.Selected
				if i >= 0 && i < len(m.events) {
					return m, nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newInspectorDetailOverlay(m.events[i], i)}
				}
				return m, nil, monitorAction{}
			}

			if t.String() == "G" || t.String() == "g" {
				m.follow = true
				m.viewport.GotoBottom()
				return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
			}
			return m, nil, monitorAction{}
		}

		// Device mode: line-buffered send (Enter submits line, other keys ignored for now).
		if t.Type == tea.KeyEnter {
			line := m.input.Value()
			m.input.SetValue("")
			if m.session != nil {
				_ = m.session.WriteLine(line)
			}
			return m, nil, monitorAction{}
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(t)
		return m, cmd, monitorAction{}

	case filterSetMsg:
		m.filterCfg = t.cfg
		m.refreshViewportContent()
		return m, nil, monitorAction{}

	case paletteExecMsg:
		cmd, act := m.execPalette(t.kind)
		return m, cmd, act

	case inspectorDetailCopyTextMsg:
		text := strings.TrimSpace(t.text)
		if text == "" {
			m.setToast("nothing to copy", 2*time.Second)
			return m, nil, monitorAction{}
		}
		if err := copyToClipboard(text); err != nil {
			m.setToast(fmt.Sprintf("copy failed: %v", err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		m.setToast("copied", 2*time.Second)
		return m, nil, monitorAction{}

	case inspectorDetailCopyFileMsg:
		path := strings.TrimSpace(t.path)
		if path == "" {
			m.setToast("nothing to copy", 2*time.Second)
			return m, nil, monitorAction{}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			m.setToast(fmt.Sprintf("read failed: %v", err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		if len(b) > 512*1024 {
			m.setToast(fmt.Sprintf("too large to copy (%d bytes)", len(b)), 3*time.Second)
			return m, nil, monitorAction{}
		}
		if err := copyToClipboard(string(b)); err != nil {
			m.setToast(fmt.Sprintf("copy failed: %v", err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		m.setToast("copied", 2*time.Second)
		return m, nil, monitorAction{}

	case inspectorDetailSaveTextMsg:
		text := strings.TrimSpace(t.text)
		if text == "" {
			m.setToast("nothing to save", 2*time.Second)
			return m, nil, monitorAction{}
		}
		prefix := "esper-inspector"
		switch t.label {
		case "report":
			prefix = "esper-coredump-report"
		}
		path, err := makeTimestampedPath(prefix, "txt", t.at)
		if err != nil {
			m.setToast(fmt.Sprintf("save failed: %v", err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			m.setToast(fmt.Sprintf("save failed: %v", err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		m.setToast("saved to: "+path, 4*time.Second)
		return m, nil, monitorAction{}

	case inspectorDetailJumpToLogMsg:
		anchor := strings.TrimSpace(t.anchor)
		if anchor == "" {
			m.setToast("no anchor", 2*time.Second)
			return m, nil, monitorAction{}
		}
		lines := m.filteredLines()
		lineIdx := findFirstLineContaining(lines, anchor)
		if lineIdx < 0 {
			m.setToast("anchor not found", 2*time.Second)
			return m, nil, monitorAction{}
		}
		m.follow = false
		m.hostFocus = hostFocusViewport
		m.viewport.YOffset = searchJumpTop(lineIdx, m.viewport.Height, len(lines))
		m.setToast("jumped to log", 2*time.Second)
		return m, nil, monitorAction{}

	case inspectorDetailNextEventMsg:
		if len(m.events) == 0 {
			m.setToast("no events", 2*time.Second)
			return m, nil, monitorAction{}
		}
		i := clamp(t.fromIndex, 0, len(m.events)-1)
		next := i + 1
		if next >= len(m.events) {
			next = 0
		}
		m.eventList.Selected = next
		m.eventList.SetLen(len(m.events))
		return m, nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newInspectorDetailOverlay(m.events[next], next)}

	case resetDeviceMsg:
		if curMode != modeHost {
			return m, nil, monitorAction{}
		}
		return m, m.resetDeviceCmd(), monitorAction{}

	case resetResultMsg:
		if t.err != nil {
			m.addEvent("reset", "Reset failed", fmt.Sprintf("Reset failed: %v", t.err))
			m.setToast(fmt.Sprintf("Reset failed: %v", t.err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		m.addEvent("reset", "Reset sent", "Sent reset pulse to device.")
		m.setToast("Reset sent", 2*time.Second)
		return m, nil, monitorAction{}

	case sendBreakResultMsg:
		if t.err != nil {
			m.addEvent("break", "Send break failed", fmt.Sprintf("Send break failed: %v", t.err))
			m.setToast(fmt.Sprintf("Send break failed: %v", t.err), 3*time.Second)
			return m, nil, monitorAction{}
		}
		m.addEvent("break", "Send break", "Sent a serial break (palette execution).")
		m.setToast("Sent break", 2*time.Second)
		return m, nil, monitorAction{}

	case serialChunkMsg:
		if len(t.b) == 0 {
			return m, m.readSerialCmd(), monitorAction{}
		}
		m.lastDataAt = time.Now()

		if g := m.gdb.Push(t.b); g != nil {
			m.append([]byte("--- GDB stub detected\n"))
			m.addEvent("gdb", "GDB stub detected", string(g.Payload))
			m.setToast("GDB stub detected (HOST mode: press i for inspector)", 3*time.Second)
		}

		lines := m.lineSplitter.Push(t.b)
		for _, line := range lines {
			wasInProgress := m.coredump.InProgress()
			events, sendEnter := m.coredump.PushLine(line)
			nowInProgress := m.coredump.InProgress()
			m.appendCoreDumpLogEvents(events)
			if sendEnter && m.session != nil {
				_, _ = m.session.port.Write([]byte("\n"))
			}

			if sendEnter {
				m.addEvent("coredump", "Core dump prompt detected", "Detected core dump prompt; sending Enter to start dump.")
				m.setToast("Core dump prompt detected; sending Enter", 2*time.Second)
			}

			if !wasInProgress && nowInProgress {
				m.coreDumpStartedAt = time.Now()
				m.addEvent("coredump", "Core dump capture started", "Started core dump capture (output muted).")
				m.setToast("Core dump capture started", 2*time.Second)
			}
			if wasInProgress && !nowInProgress {
				m.coreDumpStartedAt = time.Time{}
				body := "Core dump captured."
				if res, ok := m.coredump.LastResult(); ok {
					status := "Captured"
					if res.DecodedOK {
						status = "✓ Captured and decoded successfully"
					} else if res.HadElf {
						status = "⚠ Captured but decode failed"
					} else {
						status = "⚠ Captured (no -elf provided)"
					}
					body = fmt.Sprintf("Status: %s\nSize: %d bytes\nSaved to: %s", status, res.RawBytes, res.SavedPath)
					if !res.DecodedOK && res.DecodeErr != "" {
						body += "\nDecode error: " + res.DecodeErr
					}
					if strings.TrimSpace(string(res.Report)) != "" {
						body += "\n\n" + strings.TrimSpace(string(res.Report))
					}
				} else if len(events) > 0 {
					body = strings.TrimSpace(string(bytes.Join(events, nil)))
				}
				m.addEvent("coredump", "Core dump report", body)
				m.setToast("Core dump captured (HOST mode: press i for inspector)", 3*time.Second)
			}

			if nowInProgress {
				// suppress normal output while buffering
				continue
			}

			// panic backtrace decode is opportunistic: if the line contains Backtrace:, emit extra decoded lines.
			if decoded, ok := m.panic.DecodeBacktraceLine(line); ok && len(decoded) > 0 {
				raw := strings.TrimSpace(string(bytes.TrimRight(line, "\r\n")))
				body := fmt.Sprintf("Raw Backtrace:\n%s\n\nDecoded Frames:\n%s", raw, strings.TrimSpace(string(decoded)))
				m.append(decoded)
				m.addEvent("panic", "Backtrace decoded", body)
				m.setToast("Backtrace decoded (HOST mode: press i for inspector)", 3*time.Second)
			}

			m.append(m.autoColor.ColorizeLine(line))
		}

		m.refreshViewportContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.readSerialCmd(), monitorAction{}

	case serialErrMsg:
		m.append([]byte(fmt.Sprintf("--- serial error: %v\n", t.err)))
		m.refreshViewportContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.readSerialCmd(), monitorAction{}

	case tickMsg:
		m.now = t.t
		if m.toastText != "" && !m.toastUntil.IsZero() && m.now.After(m.toastUntil) {
			m.toastText = ""
			m.toastUntil = time.Time{}
		}
		if m.ctrlTPending && m.ctrlTPendingID != 0 && m.now.After(m.toastUntil) {
			// no-op; ctrl-t timeout handled by ctrlTPrefixTimeoutMsg
		}
		// finalize tail if idle
		if time.Since(m.lastDataAt) > 250*time.Millisecond {
			if tail := m.lineSplitter.FinalizeTail(); len(tail) > 0 {
				m.append(m.autoColor.ColorizeLine(append(tail, '\n')))
			}
		}
		m.refreshViewportContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.tickCmd(), monitorAction{}

	case ctrlTPrefixTimeoutMsg:
		if curMode == modeHost && m.ctrlTPending && t.id == m.ctrlTPendingID {
			m.ctrlTPending = false
			m.follow = true
			m.showInspector = false
			m.hostFocus = hostFocusViewport
			return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
		}
		return m, nil, monitorAction{}

	default:
		return m, nil, monitorAction{}
	}
}

func (m monitorModel) View(st styles, sz size, curMode mode) string {
	if sz.W < 30 || sz.H < 6 {
		return st.Hint.Render("Terminal too small.")
	}

	titleText := truncate.StringWithTail(m.renderTitle(), uint(sz.W), "…")
	title := padOrTrim(st.TitleBar.Render(titleText), sz.W)

	statusText := truncate.StringWithTail(m.renderStatus(curMode), uint(sz.W), "…")
	status := padOrTrim(st.StatusBar.Render(statusText), sz.W)
	if curMode == modeHost && m.searchActive {
		status = padOrTrim(st.StatusBar.Render(strings.Repeat("─", sz.W)), sz.W)
	}

	bodyH := max(1, sz.H-3)
	vw := max(1, m.viewportWidthFor(sz))
	vp := m.viewport
	vp.Width = vw
	vp.Height = bodyH
	body := vp.View()
	body = lipgloss.NewStyle().Width(vw).Height(bodyH).Render(body)

	main := body
	if curMode == modeHost && m.showInspector && sz.W >= 100 {
		panelW := max(32, min(54, sz.W/3))
		main = lipgloss.JoinHorizontal(
			lipgloss.Top,
			lipgloss.NewStyle().Width(sz.W-panelW-1).Render(body),
			lipgloss.NewStyle().Width(panelW).Render(m.renderInspectorPanel(st, size{W: panelW, H: bodyH})),
		)
	}

	footer := ""
	if m.coredump.InProgress() {
		footer = padOrTrim(st.Hint.Render("(input disabled during capture)"), sz.W)
	} else if curMode == modeHost {
		if m.searchActive {
			footer = padOrTrim(m.renderSearchBar(sz.W), sz.W)
		} else {
			footerText := "Ctrl-T: DEVICE   Ctrl-T T: commands   PgUp/PgDn scroll   G: resume follow   / search   f filter   i inspector"
			if m.showInspector {
				footerText += "   Tab: focus"
			}
			if m.toastText != "" {
				footerText += "   " + m.toastText
			}
			footerText = truncate.StringWithTail(footerText, uint(sz.W), "…")
			footer = padOrTrim(footerText, sz.W)
		}
	} else {
		field := padOrTrim(m.input.View(), max(0, sz.W-4))
		footerLine := "> [" + field + "]"
		footerLine = truncate.StringWithTail(footerLine, uint(sz.W), "…")
		footer = padOrTrim(footerLine, sz.W)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, main, status, footer)
	if m.coredump.InProgress() {
		content = renderOverlayOver(st, sz, content, m.renderCoreDumpProgressBox(st, sz))
	}
	return content
}

func (m monitorModel) renderCoreDumpProgressBox(st styles, sz size) string {
	w := max(44, min(62, sz.W-8))
	h := max(11, min(15, sz.H-6))

	innerW := max(0, w-st.OverlayBox.GetHorizontalFrameSize())
	innerH := max(0, h-st.OverlayBox.GetVerticalFrameSize())

	elapsed := time.Since(m.coreDumpStartedAt).Round(100 * time.Millisecond)
	if m.coreDumpStartedAt.IsZero() {
		elapsed = 0
	}
	buf := m.coredump.BufferedBytes()

	bufStr := fmt.Sprintf("%d bytes", buf)
	if buf >= 1024*1024 {
		bufStr = fmt.Sprintf("%.2f MiB", float64(buf)/(1024.0*1024.0))
	} else if buf >= 1024 {
		bufStr = fmt.Sprintf("%.1f KiB", float64(buf)/1024.0)
	}

	// Best-effort progress for UX parity: core dumps are commonly ~64 KiB.
	const defaultTarget = 64 * 1024
	pct := float64(buf) / float64(defaultTarget)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	barW := max(10, innerW-8)
	filled := int(pct * float64(barW))
	if filled < 0 {
		filled = 0
	}
	if filled > barW {
		filled = barW
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", max(0, barW-filled))
	pctStr := fmt.Sprintf("%d%%", int(pct*100.0+0.5))

	title := st.PanelTitle.Render("CORE DUMP CAPTURE IN PROGRESS")
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		"Receiving core dump data...",
		fmt.Sprintf("Buffered: %s", bufStr),
		fmt.Sprintf("Elapsed:  %s", elapsed),
		"",
		fmt.Sprintf("%s  %s", bar, pctStr),
		st.Hint.Render("Normal output is MUTED during capture."),
		st.Hint.Render("Core dump will auto-decode when complete."),
	)

	content := lipgloss.NewStyle().
		Width(innerW).
		Height(innerH).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body))
	return st.OverlayBox.Width(w).Height(h).Render(content)
}

func (m monitorModel) renderTitle() string {
	elfOK := "—"
	if m.cfg.ElfPath != "" {
		elfOK = "✓"
	}
	port := m.cfg.Port
	if m.session != nil && m.session.portPath != "" {
		port = m.session.portPath
	}
	return fmt.Sprintf("esper ── Connected: %s ── %d ── ELF: %s ──", port, m.cfg.Baud, elfOK)
}

func (m monitorModel) renderStatus(curMode mode) string {
	if m.coredump.InProgress() {
		buf := m.coredump.BufferedBytes()
		bufStr := fmt.Sprintf("%d bytes", buf)
		if buf >= 1024*1024 {
			bufStr = fmt.Sprintf("%.2f MiB", float64(buf)/(1024.0*1024.0))
		} else if buf >= 1024 {
			bufStr = fmt.Sprintf("%.1f KiB", float64(buf)/1024.0)
		}
		return fmt.Sprintf("Mode: CAPTURE │ Follow: — │ Capture: %s │ Press Ctrl-C to abort │ %s",
			bufStr,
			m.now.Format("15:04:05"),
		)
	}

	modeStr := "DEVICE"
	if curMode == modeHost {
		modeStr = "HOST"
	}
	followStr := "OFF"
	if m.follow {
		followStr = "ON"
	}
	capture := "—"
	if m.coredump.InProgress() {
		capture = "CORE"
	}
	buf := fmt.Sprintf("%dK/1M", len(m.out)/1024)
	ins := "OFF"
	if m.showInspector && curMode == modeHost {
		ins = "ON"
	}
	return fmt.Sprintf("Mode: %s │ Follow: %s │ Inspect: %s │ Capture: %s │ Filter: %s │ Search: %s │ Buf: %s │ %s",
		modeStr,
		followStr,
		ins,
		capture,
		m.filterSummary(),
		m.searchSummary(),
		buf,
		m.now.Format("15:04:05"),
	)
}

func (m monitorModel) filterSummary() string {
	cfg := m.filterCfg
	def := defaultFilterConfig()
	enabled := cfg.levelE != def.levelE ||
		cfg.levelW != def.levelW ||
		cfg.levelI != def.levelI ||
		cfg.levelD != def.levelD ||
		cfg.levelV != def.levelV ||
		strings.TrimSpace(cfg.includeRaw) != "" ||
		strings.TrimSpace(cfg.excludeRaw) != "" ||
		len(cfg.rules) > 0
	if !enabled {
		return "—"
	}
	parts := []string{}
	if cfg.levelE != def.levelE || cfg.levelW != def.levelW || cfg.levelI != def.levelI || cfg.levelD != def.levelD || cfg.levelV != def.levelV {
		lv := ""
		if cfg.levelE {
			lv += "E"
		}
		if cfg.levelW {
			lv += "W"
		}
		if cfg.levelI {
			lv += "I"
		}
		if cfg.levelD {
			lv += "D"
		}
		if cfg.levelV {
			lv += "V"
		}
		if lv == "" {
			lv = "∅"
		}
		parts = append(parts, "lvl:"+lv)
	}
	if strings.TrimSpace(cfg.includeRaw) != "" {
		parts = append(parts, "inc:"+padOrTrim(cfg.includeRaw, 16))
	}
	if strings.TrimSpace(cfg.excludeRaw) != "" {
		parts = append(parts, "exc:"+padOrTrim(cfg.excludeRaw, 16))
	}
	if len(cfg.rules) > 0 {
		parts = append(parts, fmt.Sprintf("hl:%d", len(cfg.rules)))
	}
	if len(parts) == 0 {
		return "ON"
	}
	return strings.Join(parts, " ")
}

func (m monitorModel) searchSummary() string {
	q := strings.TrimSpace(m.searchQuery)
	if q == "" || !m.searchActive {
		return "—"
	}
	return "/" + padOrTrim(q, 18)
}

func (m *monitorModel) append(b []byte) {
	s := string(b)
	m.out += s
	for _, line := range splitKeepNewline(s) {
		if line == "" {
			continue
		}
		m.log = append(m.log, line)
	}
	const maxLines = 4000
	if len(m.log) > maxLines {
		m.log = append([]string{}, m.log[len(m.log)-maxLines:]...)
	}
	// Keep a bounded rolling buffer to avoid unbounded memory growth.
	const maxBytes = 1 << 20  // 1 MiB
	const keepBytes = 1 << 19 // 512 KiB
	if len(m.out) > maxBytes {
		m.out = m.out[len(m.out)-keepBytes:]
	}

	m.writeSessionLog(b)
}

func (m *monitorModel) appendCoreDumpLogEvents(events [][]byte) {
	if len(events) == 0 {
		return
	}

	// Keep core dump log annotations small and scannable; avoid dumping the decoded report into the main viewport.
	var out []byte
	for _, ev := range events {
		if len(ev) == 0 {
			continue
		}
		if !bytes.HasPrefix(ev, []byte("---")) {
			continue
		}
		// Stop before report content.
		if bytes.HasPrefix(ev, []byte("--- Core dump report")) {
			break
		}
		out = append(out, ev...)
		if len(out) > 8*1024 {
			break
		}
	}
	if len(out) > 0 {
		m.append(out)
	}
}

func findFirstLineContaining(lines []string, needle string) int {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return -1
	}
	for i, line := range lines {
		if strings.Contains(stripANSI(line), needle) {
			return i
		}
	}
	return -1
}

func (m monitorModel) readSerialCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil || m.session.port == nil {
			return serialErrMsg{err: fmt.Errorf("not connected")}
		}
		buf := make([]byte, 4096)
		n, err := m.session.port.Read(buf)
		if err != nil {
			return serialErrMsg{err: err}
		}
		if n <= 0 {
			return serialChunkMsg{b: nil}
		}
		return serialChunkMsg{b: append([]byte{}, buf[:n]...)}
	}
}

func (m monitorModel) tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{t: t}
	})
}

func (m *monitorModel) addEvent(kind, title, body string) {
	if body == "" {
		body = title
	}
	m.events = append(m.events, monitorEvent{
		At:    time.Now(),
		Kind:  kind,
		Title: title,
		Body:  body,
	})
	const maxEvents = 200
	if len(m.events) > maxEvents {
		m.events = append([]monitorEvent{}, m.events[len(m.events)-maxEvents:]...)
		m.eventList.SetLen(len(m.events))
	}
	if len(m.events) == 1 {
		m.eventList.Selected = 0
	}
}

func (m *monitorModel) setToast(text string, d time.Duration) {
	if d <= 0 {
		m.toastText = ""
		m.toastUntil = time.Time{}
		return
	}
	m.toastText = text
	m.toastUntil = time.Now().Add(d)
}

func (m monitorModel) renderInspectorPanel(st styles, sz size) string {
	if sz.W < 10 || sz.H < 4 {
		return ""
	}
	panelInnerW := max(0, sz.W-st.Panel.GetHorizontalBorderSize())
	panelInnerH := max(0, sz.H-st.Panel.GetVerticalBorderSize())
	if len(m.events) == 0 {
		return st.Panel.Width(panelInnerW).Height(panelInnerH).Render(st.Hint.Render("No events yet."))
	}

	header := st.PanelTitle.Render("Inspector")
	focus := "view"
	if m.hostFocus == hostFocusInspector {
		focus = "inspector"
	}
	sub := st.Hint.Render(fmt.Sprintf("focus: %s  events:%d", focus, len(m.events)))

	listH := max(3, panelInnerH-6)
	start, end := m.eventList.Window(len(m.events), listH)

	var rows []string
	for i := start; i < end; i++ {
		e := m.events[i]
		prefix := fmt.Sprintf("%s %-7s ", e.At.Format("15:04:05"), e.Kind)
		line := padOrTrim(prefix+e.Title, max(0, panelInnerW-2))
		if i == m.eventList.Selected {
			line = st.SelectedRow.Render(line)
		} else {
			line = st.Row.Render(line)
		}
		rows = append(rows, line)
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}

	body := stringsJoinVertical(rows)

	detail := ""
	if m.eventList.Selected >= 0 && m.eventList.Selected < len(m.events) {
		detail = m.events[m.eventList.Selected].Body
	}
	detail = padOrTrim(detail, max(0, panelInnerW-2))

	content := lipgloss.JoinVertical(lipgloss.Left, header, sub, "", body, "", detail)
	return st.Panel.Width(panelInnerW).Height(panelInnerH).Render(content)
}

// viewportWidthFor moved to monitor_model.go

// stringsJoinVertical and splitKeepNewline moved to ui_helpers.go

func (m *monitorModel) refreshViewportContent() {
	baseLines := m.filteredLines()
	if m.searchActive {
		m.searchMatches = searchMatchesForLines(baseLines, m.searchQuery)
		if len(m.searchMatches) == 0 {
			m.searchCur = 0
		} else if m.searchCur >= len(m.searchMatches) {
			m.searchCur = 0
		}
	}

	lines := applyHighlightRules(baseLines, m.filterCfg.rules)
	if m.wrap && !m.searchActive {
		lines = wrapViewportLines(lines, m.viewportWidthFor(m.sz))
	}
	if m.searchActive {
		lines = m.decorateSearchLines(lines, m.viewportWidthFor(m.sz))
	} else {
		m.searchMatches = nil
		m.searchCur = 0
	}

	m.viewport.SetContent(strings.Join(lines, ""))
}

func (m monitorModel) filteredLines() []string {
	if len(m.log) == 0 {
		return nil
	}

	cfg := m.filterCfg
	def := defaultFilterConfig()
	enabled := cfg.levelE != def.levelE ||
		cfg.levelW != def.levelW ||
		cfg.levelI != def.levelI ||
		cfg.levelD != def.levelD ||
		cfg.levelV != def.levelV ||
		cfg.include != nil ||
		cfg.exclude != nil
	if !enabled {
		return m.log
	}

	var out []string
	for _, line := range m.log {
		stripped := stripANSI(line)

		// Level filter (ESP-IDF-ish prefix: I|W|E + space + '(').
		if len(stripped) >= 3 {
			lvl := stripped[0]
			if stripped[1] == ' ' && stripped[2] == '(' {
				switch lvl {
				case 'E':
					if !cfg.levelE {
						continue
					}
				case 'W':
					if !cfg.levelW {
						continue
					}
				case 'I':
					if !cfg.levelI {
						continue
					}
				case 'D':
					if !cfg.levelD {
						continue
					}
				case 'V':
					if !cfg.levelV {
						continue
					}
				}
			}
		}

		if cfg.include != nil && !cfg.include.MatchString(stripped) {
			continue
		}
		if cfg.exclude != nil && cfg.exclude.MatchString(stripped) {
			continue
		}

		out = append(out, line)
	}
	return out
}

// splitLinesN moved to ui_helpers.go

func (m *monitorModel) searchApplyJump() {
	m.searchComputeMatches()
	if len(m.searchMatches) == 0 {
		m.setToast("no matches", 2*time.Second)
		return
	}
	m.searchCur = clamp(m.searchCur, 0, len(m.searchMatches)-1)
	lines := m.filteredLines()
	lineIdx := m.searchMatches[m.searchCur]
	m.viewport.YOffset = searchJumpTop(lineIdx, m.viewport.Height, len(lines))
}

func (m *monitorModel) searchApplyNext() {
	m.searchComputeMatches()
	if len(m.searchMatches) == 0 {
		m.setToast("no matches", 2*time.Second)
		return
	}
	m.searchCur = searchNextIndex(m.searchCur, len(m.searchMatches))
}

func (m *monitorModel) searchApplyPrev() {
	m.searchComputeMatches()
	if len(m.searchMatches) == 0 {
		m.setToast("no matches", 2*time.Second)
		return
	}
	m.searchCur = searchPrevIndex(m.searchCur, len(m.searchMatches))
}

func (m *monitorModel) searchComputeMatches() {
	query := strings.TrimSpace(m.searchQuery)
	if query == "" {
		m.searchMatches = nil
		m.searchCur = 0
		return
	}

	lines := m.filteredLines()
	m.searchMatches = searchMatchesForLines(lines, query)
	if len(m.searchMatches) == 0 {
		m.searchCur = 0
		return
	}
	if m.searchCur >= len(m.searchMatches) {
		m.searchCur = 0
	}
}

func (m *monitorModel) execPalette(kind paletteCommandKind) (tea.Cmd, monitorAction) {
	switch kind {
	case cmdOpenSearch:
		m.openSearch()
		return nil, monitorAction{}
	case cmdOpenFilter:
		return nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newFilterOverlay(m.filterCfg)}
	case cmdToggleInspector:
		m.showInspector = !m.showInspector
		m.setSize(m.sz)
		return nil, monitorAction{}
	case cmdToggleWrap:
		m.wrap = !m.wrap
		m.refreshViewportContent()
		if m.wrap {
			m.setToast("wrap: ON", 2*time.Second)
		} else {
			m.setToast("wrap: OFF", 2*time.Second)
		}
		return nil, monitorAction{}
	case cmdResetDevice:
		return nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newResetConfirmOverlay()}
	case cmdSendBreak:
		return m.sendBreakCmd(), monitorAction{}
	case cmdDisconnect:
		return nil, monitorAction{kind: monitorActionDisconnect, reason: "disconnect"}
	case cmdClearViewport:
		m.out = ""
		m.log = nil
		m.viewport.SetContent("")
		m.events = nil
		m.eventList.Selected = 0
		return nil, monitorAction{}
	case cmdToggleSessionLog:
		if err := m.toggleSessionLogging(); err != nil {
			m.setToast(fmt.Sprintf("log: %v", err), 3*time.Second)
		}
		return nil, monitorAction{}
	case cmdShowHelp:
		return nil, monitorAction{kind: monitorActionOpenOverlay, overlay: newHelpOverlay()}
	case cmdQuit:
		return nil, monitorAction{kind: monitorActionQuit}
	default:
		return nil, monitorAction{}
	}
}

func (m *monitorModel) openSearch() {
	m.searchActive = true
	m.searchInput.SetValue(m.searchQuery)
	m.searchInput.Focus()
	m.searchCur = 0
	m.searchComputeMatches()
	m.refreshViewportContent()
}

func (m *monitorModel) closeSearch() {
	m.searchActive = false
	m.searchInput.Blur()
	// Keep searchQuery around so reopening resumes the last query, but clear
	// match decorations immediately.
	m.refreshViewportContent()
}

func (m monitorModel) renderSearchBar(w int) string {
	q := m.searchInput.Value()
	matches := "—"
	qTrim := strings.TrimSpace(q)
	if qTrim != "" {
		if len(m.searchMatches) == 0 {
			matches = "0 matches"
		} else {
			matches = fmt.Sprintf("match %d/%d", clamp(m.searchCur, 0, len(m.searchMatches)-1)+1, len(m.searchMatches))
		}
	}
	hint := "n:next N:prev  Enter:jump  Esc:close"
	if m.toastText != "" {
		hint += "   " + m.toastText
	}

	prefix := "Search: ["
	suffix := "]"
	sep := " │ "
	right := suffix + sep + matches + sep + hint

	queryW := w - ansiVisibleWidth(prefix) - ansiVisibleWidth(right)
	if queryW < 1 {
		queryW = 1
	}

	// Update input width so cursor rendering stays stable.
	m.searchInput.Width = queryW
	field := padOrTrim(m.searchInput.View(), queryW)

	line := prefix + field + right
	return truncate.StringWithTail(line, uint(w), "…")
}

func (m monitorModel) decorateSearchLines(lines []string, width int) []string {
	q := strings.TrimSpace(m.searchQuery)
	if q == "" || len(lines) == 0 || width <= 0 || len(m.searchMatches) == 0 {
		return lines
	}

	total := len(m.searchMatches)
	curOrd := clamp(m.searchCur, 0, total-1) + 1
	orderByLine := make(map[int]int, total)
	for i, li := range m.searchMatches {
		orderByLine[li] = i + 1
	}

	markerStyle := lipgloss.NewStyle().Faint(true)
	curMarkerStyle := lipgloss.NewStyle().Bold(true)
	highlightStyle := lipgloss.NewStyle().Reverse(true)

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl = "\n"
			line = strings.TrimSuffix(line, "\n")
		}

		if ord, ok := orderByLine[i]; ok {
			marker := fmt.Sprintf("← MATCH %d/%d", ord, total)
			if ansiVisibleWidth(marker) > width-2 {
				marker = "← MATCH"
			}
			if ord == curOrd {
				marker = curMarkerStyle.Render(marker)
			} else {
				marker = markerStyle.Render(marker)
			}

			leftW := width - 1 - ansiVisibleWidth(marker)
			if leftW < 0 {
				leftW = 0
			}

			left := line
			// Best-effort substring highlight: only attempt when the line has no ANSI of its own.
			if !hasANSI(left) {
				left = highlightPlainSubstring(left, q, highlightStyle)
			}

			left = ansiCutToWidth(left, leftW)
			pad := leftW - ansiVisibleWidth(left)
			if pad < 0 {
				pad = 0
			}

			// Reset styles before padding/marker so we don't "inherit" log colors.
			left = left + "\x1b[0m" + strings.Repeat(" ", pad) + " " + marker
			out = append(out, left+nl)
			continue
		}

		out = append(out, line+nl)
	}
	return out
}

func highlightPlainSubstring(s, query string, st lipgloss.Style) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return s
	}
	// Best-effort: only ASCII queries (avoids tricky Unicode case-fold length changes).
	for _, r := range query {
		if r > 127 {
			return s
		}
	}

	ls := strings.ToLower(s)
	lq := strings.ToLower(query)
	var out strings.Builder
	out.Grow(len(s))

	i := 0
	for {
		j := strings.Index(ls[i:], lq)
		if j < 0 {
			out.WriteString(s[i:])
			break
		}
		j += i
		out.WriteString(s[i:j])
		out.WriteString(st.Render(s[j : j+len(query)]))
		i = j + len(query)
	}
	return out.String()
}

func (m monitorModel) resetDeviceCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil {
			return resetResultMsg{err: fmt.Errorf("not connected")}
		}
		return resetResultMsg{err: m.session.ResetPulse()}
	}
}

func (m monitorModel) sendBreakCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil {
			return sendBreakResultMsg{err: fmt.Errorf("not connected")}
		}
		return sendBreakResultMsg{err: m.session.SendBreak(250 * time.Millisecond)}
	}
}

func (m *monitorModel) toggleSessionLogging() error {
	if m.sessionLogOn {
		m.closeSessionLogging()
		m.setToast("log: OFF", 2*time.Second)
		return nil
	}

	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "esper")
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return mkErr
	}
	path := filepath.Join(dir, fmt.Sprintf("session-%s.log", time.Now().Format("20060102-150405")))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	m.sessionLogOn = true
	m.sessionLogFile = f
	m.sessionLogPath = path
	m.setToast("log: ON ("+filepath.Base(path)+")", 3*time.Second)
	return nil
}

func (m *monitorModel) closeSessionLogging() {
	if m.sessionLogFile != nil {
		_ = m.sessionLogFile.Close()
	}
	m.sessionLogOn = false
	m.sessionLogFile = nil
	m.sessionLogPath = ""
}

func (m *monitorModel) writeSessionLog(b []byte) {
	if !m.sessionLogOn || m.sessionLogFile == nil || len(b) == 0 {
		return
	}
	if _, err := m.sessionLogFile.Write(b); err != nil {
		// If the file goes bad (disk full, etc), stop logging to avoid repeated errors.
		m.closeSessionLogging()
		m.setToast(fmt.Sprintf("log write failed: %v", err), 3*time.Second)
	}
}
