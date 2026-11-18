package tui

import (
	"strings"
	"testing"
)

func TestTabTitleSequenceUsesIconNameOSC(t *testing.T) {
	cases := []struct{ in, want string }{
		{"晴天 — 故事的小黄花", "\x1b]1;晴天 — 故事的小黄花\x07"},
		{"", "\x1b]1;\x07"},
		{"song\x07\x1b]0;pwned\nnext", "\x1b]1;song]0;pwned next\x07"},
		{strings.Repeat("歌", 100), "\x1b]1;" + strings.Repeat("歌", 80) + "…\x07"},
	}
	for _, c := range cases {
		if got := tabTitleSequence(c.in); got != c.want {
			t.Fatalf("tabTitleSequence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// OSC 2 只改窗口标题，OSC 0 会连窗口一起改；tab 标题必须用 OSC 1。
	if seq := tabTitleSequence("x"); !strings.HasPrefix(seq, "\x1b]1;") {
		t.Fatalf("tab title must use OSC 1, got %q", seq)
	}
}
