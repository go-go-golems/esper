package monitor

import "testing"

func TestSearchMatchesForLines(t *testing.T) {
	lines := []string{
		"I (12) wifi_init: start\n",
		"\x1b[31mE\x1b[0m (34) http: timeout\n",
		"noise\n",
		"\x1b[32mI\x1b[0m (56) WiFi: connected\n",
	}

	got := searchMatchesForLines(lines, "wifi")
	if len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Fatalf("matches=%v, want [0 3]", got)
	}

	got = searchMatchesForLines(lines, "   ")
	if got != nil {
		t.Fatalf("empty query matches=%v, want nil", got)
	}
}

func TestSearchNextPrevIndexWrap(t *testing.T) {
	if got := searchNextIndex(0, 0); got != 0 {
		t.Fatalf("next(0,0)=%d, want 0", got)
	}
	if got := searchPrevIndex(0, 0); got != 0 {
		t.Fatalf("prev(0,0)=%d, want 0", got)
	}

	if got := searchNextIndex(4, 5); got != 0 {
		t.Fatalf("next(4,5)=%d, want 0", got)
	}
	if got := searchPrevIndex(0, 5); got != 4 {
		t.Fatalf("prev(0,5)=%d, want 4", got)
	}
}

func TestSearchJumpTop(t *testing.T) {
	// Prefer top-third.
	if got := searchJumpTop(10, 9, 100); got != 7 {
		t.Fatalf("jump(10,h=9)=%d, want 7", got)
	}
	// Clamp at top.
	if got := searchJumpTop(1, 9, 100); got != 0 {
		t.Fatalf("jump(1,h=9)=%d, want 0", got)
	}
	// Clamp at bottom.
	if got := searchJumpTop(99, 9, 100); got != 96 {
		t.Fatalf("jump(99,h=9)=%d, want 96", got)
	}
}
