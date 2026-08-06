package visualizer

import "github.com/gopxl/beep"

// TapStreamer wraps a beep.Streamer and transparently copies every sample
// into a RingBuffer for the visualizer's audio analysis.
//
// Insert it anywhere in the beep processing chain before speaker.Play().
type TapStreamer struct {
	inner beep.Streamer
	buf   *RingBuffer
}

// NewTapStreamer creates a TapStreamer that writes to the given ring buffer.
func NewTapStreamer(inner beep.Streamer, buf *RingBuffer) *TapStreamer {
	return &TapStreamer{inner: inner, buf: buf}
}

// Stream satisfies beep.Streamer. It delegates to the inner streamer and
// copies successfully-read samples to the ring buffer.
func (t *TapStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	n, ok = t.inner.Stream(samples)
	if n > 0 {
		t.buf.Write(samples[:n])
	}
	return
}

// Err satisfies beep.Streamer (no error path; always nil).
func (t *TapStreamer) Err() error {
	return t.inner.Err()
}
