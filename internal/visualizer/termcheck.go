package visualizer

import (
	"os"
	"strconv"
	"strings"
)

// SixelCapable checks whether the current terminal is likely to support Sixel graphics.
// Returns false for terminals known NOT to support it, and a reason string.
func SixelCapable() (bool, string) {
	// Explicit override — skip all detection
	if os.Getenv("FORCE_SIXEL") == "1" {
		return true, ""
	}

	term := os.Getenv("TERM")
	termProg := os.Getenv("TERM_PROGRAM")

	// Known Sixel-capable TERM values
	sixelTerms := map[string]bool{
		"xterm-kitty":   true,
		"wezterm":       true,
		"foot":          true,
		"mlterm":        true,
		"yaft-256color": true,
	}

	if sixelTerms[term] {
		return true, ""
	}

	// iTerm2 — Sixel support added in 3.5.0
	if termProg == "iTerm.app" {
		if !checkMinVersion("TERM_PROGRAM_VERSION", 3, 5) {
			return false, "Sixel requires iTerm2 3.5.0 or newer. Please upgrade iTerm2, or use WezTerm/kitty/foot."
		}
		return true, ""
	}

	// WezTerm
	if termProg == "WezTerm" {
		return true, ""
	}

	// Windows Terminal (Sixel since 1.22)
	if os.Getenv("WT_SESSION") != "" {
		return true, ""
	}

	// tmux — typically blocks Sixel passthrough
	if os.Getenv("TMUX") != "" {
		return false, "Sixel is not supported inside tmux sessions; run outside tmux or use a terminal that supports Sixel passthrough"
	}

	// Kitty (some configs don't set TERM=xterm-kitty)
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true, ""
	}

	return false, "Your terminal does not support Sixel graphics. Please use iTerm2 (3.5+), WezTerm, kitty, foot, or Windows Terminal."
}

// checkMinVersion parses a MAJOR.MINOR.PATCH env var and checks >= given major.minor.
func checkMinVersion(envName string, wantMajor, wantMinor int) bool {
	raw := os.Getenv(envName)
	if raw == "" {
		return false
	}
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if maj > wantMajor {
		return true
	}
	if maj == wantMajor && min >= wantMinor {
		return true
	}
	return false
}
