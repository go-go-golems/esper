package monitor

import "strings"

func searchMatchesForLines(lines []string, query string) []int {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var matches []int
	for i, line := range lines {
		if containsQuery(line, query) {
			matches = append(matches, i)
		}
	}
	return matches
}

func searchNextIndex(cur, n int) int {
	if n <= 0 {
		return 0
	}
	cur++
	if cur >= n {
		cur = 0
	}
	return cur
}

func searchPrevIndex(cur, n int) int {
	if n <= 0 {
		return 0
	}
	cur--
	if cur < 0 {
		cur = n - 1
	}
	return cur
}

// searchJumpTop computes a viewport YOffset that keeps the match visible and
// prefers positioning it near the top third of the viewport.
func searchJumpTop(matchLine, viewportH, totalLines int) int {
	if viewportH <= 0 {
		return clamp(matchLine, 0, max(0, totalLines-1))
	}
	target := matchLine - viewportH/3
	return clamp(target, 0, max(0, totalLines-1))
}
