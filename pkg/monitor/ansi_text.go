package monitor

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// Best-effort ANSI escape sequence matcher (CSI + many simple sequences).
// This is used for searching and "does this line have ANSI?" checks.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func hasANSI(s string) bool {
	return ansiRE.MatchString(s)
}

func ansiVisibleWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

func ansiCutToWidth(s string, w int) string {
	if w <= 0 || s == "" {
		return ""
	}

	var out strings.Builder
	out.Grow(len(s))

	visible := 0
	for i := 0; i < len(s); {
		// Preserve ANSI escape sequences without counting them towards visible width.
		if s[i] == 0x1b {
			// CSI: ESC [
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) {
					b := s[j]
					// Final byte of CSI is in 0x40..0x7E (often A-Z/a-z/~).
					if b >= 0x40 && b <= 0x7E {
						j++
						break
					}
					j++
				}
				out.WriteString(s[i:j])
				i = j
				continue
			}

			// OSC: ESC ] ... BEL or ESC \ (best-effort).
			if i+1 < len(s) && s[i+1] == ']' {
				j := i + 2
				for j < len(s) {
					if s[j] == 0x07 { // BEL
						j++
						break
					}
					if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				out.WriteString(s[i:j])
				i = j
				continue
			}

			// Unknown ESC sequence: keep byte.
			out.WriteByte(s[i])
			i++
			continue
		}

		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && sz == 1 {
			// invalid byte; treat as width 1
			if visible+1 > w {
				break
			}
			out.WriteByte(s[i])
			i++
			visible++
			continue
		}

		rw := runewidth.RuneWidth(r)
		if rw < 0 {
			rw = 0
		}
		if visible+rw > w {
			break
		}
		out.WriteString(s[i : i+sz])
		i += sz
		visible += rw
	}
	return out.String()
}

func containsQuery(line, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return false
	}
	return strings.Contains(strings.ToLower(stripANSI(line)), strings.ToLower(query))
}
