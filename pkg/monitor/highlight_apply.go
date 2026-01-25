package monitor

import "strings"

func applyHighlightRules(lines []string, rules []highlightRule) []string {
	if len(lines) == 0 || len(rules) == 0 {
		return lines
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl = "\n"
			line = strings.TrimSuffix(line, "\n")
		}

		stripped := stripANSI(line)
		styled := line
		for _, r := range rules {
			if strings.TrimSpace(r.patternRaw) == "" || r.rx == nil || r.style == highlightStyleNone {
				continue
			}
			if r.rx.MatchString(stripped) {
				styled = highlightStyleFor(r.style).Render(line) + "\x1b[0m"
				break
			}
		}

		out = append(out, styled+nl)
	}
	return out
}
