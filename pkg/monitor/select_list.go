package monitor

// selectList is a tiny reusable core for "select one item from a list" behaviors.
// It intentionally stays UI-framework-agnostic; rendering happens elsewhere.
type selectList struct {
	Selected int
}

func (l *selectList) SetLen(n int) {
	if n <= 0 {
		l.Selected = 0
		return
	}
	l.Selected = clamp(l.Selected, 0, n-1)
}

func (l *selectList) Move(delta int, n int) {
	if n <= 0 {
		l.Selected = 0
		return
	}
	l.Selected = clamp(l.Selected+delta, 0, n-1)
}

func (l selectList) Window(n int, h int) (start int, end int) {
	if n <= 0 || h <= 0 {
		return 0, 0
	}
	if h > n {
		h = n
	}

	start = 0
	if l.Selected >= h {
		start = l.Selected - h + 1
	}
	end = min(n, start+h)
	return start, end
}
