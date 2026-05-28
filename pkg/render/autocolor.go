package render

import "bytes"

type AutoColorer struct {
	DisableAutoColor bool

	trailingColor bool
}

func (c *AutoColorer) ColorizeLine(line []byte) []byte {
	if c.DisableAutoColor {
		return line
	}

	lineStripped, newline := stripLineEnding(line)
	color := autoColorForLine(lineStripped)
	if color == nil {
		if c.trailingColor && len(newline) > 0 {
			// previously started a colored partial line, but this chunk ends the line without recoloring
			out := append([]byte{}, lineStripped...)
			out = append(out, ANSIReset...)
			out = append(out, newline...)
			c.trailingColor = false
			return out
		}
		return line
	}

	if len(newline) > 0 {
		out := append([]byte{}, color...)
		out = append(out, lineStripped...)
		out = append(out, ANSIReset...)
		out = append(out, newline...)
		c.trailingColor = false
		return out
	}

	out := append([]byte{}, color...)
	out = append(out, line...)
	c.trailingColor = true
	return out
}

func stripLineEnding(b []byte) (stripped []byte, newline []byte) {
	stripped = bytes.TrimRight(b, "\r\n")
	return stripped, b[len(stripped):]
}

func autoColorForLine(line []byte) []byte {
	// Mirrors esp_idf_monitor's AUTO_COLOR_REGEX intent:
	// match lines starting with: I|W|E + space + '(' + timestamp-ish + ')'
	if len(line) < 3 {
		return nil
	}
	lvl := line[0]
	if (lvl != 'I' && lvl != 'W' && lvl != 'E') || line[1] != ' ' || line[2] != '(' {
		return nil
	}
	switch lvl {
	case 'I':
		return ANSIGreen
	case 'W':
		return ANSIYellow
	case 'E':
		return ANSIRed
	default:
		return nil
	}
}
