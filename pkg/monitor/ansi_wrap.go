package monitor

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func wrapViewportLines(lines []string, w int) []string {
	if len(lines) == 0 || w <= 0 {
		return lines
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl = "\n"
			line = strings.TrimSuffix(line, "\n")
		}

		if ansiVisibleWidth(line) <= w {
			out = append(out, line+nl)
			continue
		}

		rest := line
		for rest != "" {
			head, tail := ansiCutToWidthWithRemainder(rest, w)
			if head == "" {
				// Safety: make progress even with pathological inputs.
				_, sz := utf8.DecodeRuneInString(rest)
				if sz <= 0 || sz > len(rest) {
					sz = 1
				}
				head = rest[:sz]
				tail = rest[sz:]
			}
			if hasANSI(head) {
				head += "\x1b[0m"
			}
			out = append(out, head+"\n")
			rest = tail
		}

		// Restore original newline handling: if the original line had no trailing newline,
		// trim the one we added on the last wrapped segment.
		if nl == "" && len(out) > 0 {
			out[len(out)-1] = strings.TrimSuffix(out[len(out)-1], "\n")
		}
	}
	return out
}

func ansiCutToWidthWithRemainder(s string, w int) (head, tail string) {
	if w <= 0 || s == "" {
		return "", s
	}

	visible := 0
	i := 0
	for i < len(s) {
		// ANSI escape sequence: preserve but don't count width.
		if s[i] == 0x1b {
			// CSI: ESC [
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) {
					b := s[j]
					if b >= 0x40 && b <= 0x7E {
						j++
						break
					}
					j++
				}
				i = j
				continue
			}

			// OSC: ESC ] ... BEL or ESC \
			if i+1 < len(s) && s[i+1] == ']' {
				j := i + 2
				for j < len(s) {
					if s[j] == 0x07 {
						j++
						break
					}
					if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
				continue
			}

			i++
			continue
		}

		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && sz == 1 {
			if visible+1 > w {
				break
			}
			i++
			visible++
			continue
		}

		rw := runewidth.RuneWidth(r)
		if rw < 0 {
			rw = 0
		}
		if visible+rw > w {
			// Avoid infinite loops when a single rune exceeds the width.
			if visible == 0 {
				i += sz
			}
			break
		}
		i += sz
		visible += rw
	}

	head = s[:i]
	tail = s[i:]
	return head, tail
}
