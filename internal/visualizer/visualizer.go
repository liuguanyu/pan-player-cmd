package visualizer

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gopxl/beep"
)

// ActiveVisualizer tracks thread-safe state shared between the
// visualizer goroutine and the TUI.
type ActiveVisualizer struct {
	sixelOutput string
	clearFrame  string
	enabled     bool
	supported   bool
	reason      string
	mu          sync.RWMutex

	// Terminal dimensions (updated on WindowSizeMsg) for absolute Sixel positioning.
	termHeight int
	// 1-based terminal row where the Sixel image should be drawn. The TUI
	// updates this from renderPlayerView so the image sits directly below lyrics.
	renderRow int

	// Internal
	ringBuf *RingBuffer
	stopCh  chan struct{}
	stopped bool
	visible bool
}

// NewActiveVisualizer creates an ActiveVisualizer. If the terminal
// doesn't support Sixel, supported will be false and reason will explain why.
func NewActiveVisualizer() *ActiveVisualizer {
	supported, reason := SixelCapable()
	return &ActiveVisualizer{
		supported: supported,
		reason:    reason,
		visible:   true,
		clearFrame: SixelEncodeSolid(
			DefaultViewportWidth,
			DefaultViewportHeight,
			ColorTerminalBackground,
		),
	}
}

// IsSupported returns whether Sixel is available.
func (v *ActiveVisualizer) IsSupported() bool {
	return v.supported
}

// UnsupportedReason returns a human-readable explanation for why Sixel is unavailable.
func (v *ActiveVisualizer) UnsupportedReason() string {
	return v.reason
}

// Enabled returns whether visualization should currently occupy/render its area.
// It includes both the user's toggle preference and the current page visibility.
func (v *ActiveVisualizer) Enabled() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.enabled && v.visible
}

// SetTermHeight updates the terminal height used for absolute Sixel positioning.
func (v *ActiveVisualizer) SetTermHeight(h int) {
	v.mu.Lock()
	v.termHeight = h
	v.mu.Unlock()
}

// SetRenderRow updates the 1-based terminal row used for Sixel rendering.
func (v *ActiveVisualizer) SetRenderRow(row int) {
	if row < 1 {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.renderRow != 0 && v.renderRow != row && v.enabled && v.visible {
		v.clearSixelAreaAt(v.renderRow)
	}
	v.renderRow = row
}

// SetVisible controls whether the visualization is rendered on the current page.
// It preserves the user's enabled preference while clearing the terminal area
// when leaving the player view.
func (v *ActiveVisualizer) SetVisible(visible bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.visible == visible {
		return
	}
	v.visible = visible
	if !visible {
		v.clearSixelArea()
	}
}

// Toggle switches visualization on or off.
// Returns false if the terminal doesn't support Sixel.
func (v *ActiveVisualizer) Toggle() bool {
	if !v.supported {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	wasEnabled := v.enabled
	v.enabled = !v.enabled
	// When turning OFF, clear cached output and trigger a clear of the Sixel area.
	if wasEnabled && !v.enabled {
		v.sixelOutput = ""
		v.clearSixelArea()
	}
	return true
}

// clearSixelArea writes spaces to erase any residual Sixel pixels.
// Must be called with mu held.
func (v *ActiveVisualizer) clearSixelArea() {
	row := v.renderRow
	if row < 1 && v.termHeight >= DefaultVisualizerRows {
		row = v.termHeight - DefaultVisualizerRows + 1
	}
	v.clearSixelAreaAt(row)
}

func (v *ActiveVisualizer) clearSixelAreaAt(row int) {
	if row < 1 {
		return
	}
	// Sixel pixels are not ordinary text cells; ANSI EL (\x1b[K) clears text
	// cells but may leave graphic pixels behind in iTerm2. First paint a solid
	// terminal-background Sixel over the old image, then clear the text rows.
	fmt.Fprint(os.Stdout, "\x1b7")
	if v.clearFrame != "" {
		fmt.Fprintf(os.Stdout, "\x1b[%d;1H%s", row, v.clearFrame)
	}
	for i := 0; i < DefaultVisualizerRows+4; i++ {
		fmt.Fprintf(os.Stdout, "\x1b[%d;1H\x1b[K", row+i)
	}
	fmt.Fprint(os.Stdout, "\x1b8")
}

// RenderFrame returns the latest Sixel-encoded frame string, or empty string.
func (v *ActiveVisualizer) RenderFrame() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if !v.enabled || !v.visible {
		return ""
	}
	return v.sixelOutput
}

// CreateTap creates a beep.Streamer tap that feeds the visualizer's audio analysis.
func (v *ActiveVisualizer) CreateTap(inner beep.Streamer) beep.Streamer {
	buf := NewRingBuffer(FFTWindow * 4)
	tap := NewTapStreamer(inner, buf)
	v.mu.Lock()
	v.ringBuf = buf
	v.mu.Unlock()
	return tap
}

// Start begins the visualization loop in a background goroutine.
func (v *ActiveVisualizer) Start() {
	if !v.supported {
		return
	}
	v.mu.Lock()
	if v.stopped {
		v.mu.Unlock()
		return
	}
	if v.ringBuf == nil {
		v.ringBuf = NewRingBuffer(FFTWindow * 4)
	}
	v.stopCh = make(chan struct{})
	v.mu.Unlock()

	go v.runLoop()
}

// Stop shuts down the visualization loop.
func (v *ActiveVisualizer) Stop() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.stopped {
		return
	}
	v.stopped = true
	v.enabled = false
	v.visible = false
	if v.stopCh != nil {
		close(v.stopCh)
	}
}

// --- internal ---

func (v *ActiveVisualizer) runLoop() {
	analyzer := NewAudioAnalyzer()
	state := NewSceneState(ViewportSize{Width: DefaultViewportWidth, Height: DefaultViewportHeight})
	canvas := NewCanvas(DefaultViewportWidth, DefaultViewportHeight)

	ticker := time.NewTicker(time.Duration(AnalysisIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-v.stopCh:
			return
		case <-ticker.C:
		}

		v.mu.RLock()
		active := v.enabled && v.visible
		v.mu.RUnlock()
		if !active {
			continue
		}

		v.mu.RLock()
		currentBuf := v.ringBuf
		v.mu.RUnlock()

		analysis := analyzer.Analyze(currentBuf)
		state.Update(analysis)

		DrawBackground(canvas, state)
		DrawMainSheep(canvas, state, analysis)

		frame := SixelEncode(canvas.Image())

		// Re-check state after the relatively expensive render/encode step.
		// Without this, pressing v/off can clear the area while this goroutine is
		// still encoding a frame, then the stale frame is written after the clear.
		v.mu.Lock()
		activeNow := v.enabled && v.visible
		th := v.termHeight
		row := v.renderRow
		if activeNow {
			v.sixelOutput = frame
		}
		v.mu.Unlock()

		// Sixel DCS 不能放进 Bubble Tea 的 View 输出（标准渲染器的行 diff
		// 会注入 \x1b[K 清行序列，破坏 DCS）。
		// 这里从后台 goroutine 直接写 os.Stdout，绕过 Bubble Tea 渲染管线。
		// 用绝对定位 \x1b[{row};1H 写到当前播放页预留区域，
		// \x1b7/\x1b8 保存/恢复光标，避免切歌/切歌词时位置漂移产生残影。
		if frame != "" && activeNow {
			if row < 1 && th >= DefaultVisualizerRows {
				row = th - DefaultVisualizerRows + 1
			}
			if row >= 1 {
				// If the preferred row would overflow, clamp upward to avoid terminal scrolling.
				if th > 0 && row+DefaultVisualizerRows-1 > th {
					row = th - DefaultVisualizerRows + 1
					if row < 1 {
						row = 1
					}
				}
				fmt.Fprintf(os.Stdout, "\x1b7\x1b[%d;1H%s\x1b8", row, frame)
			}
		}
	}
}
