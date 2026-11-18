package visualizer

import (
	"sync"
	"time"
)

// RingBuffer is a lock-free-ish fixed-size circular buffer for stereo audio samples.
// Writer (TapStreamer) and reader (Analyzer) run in different goroutines.
// If the reader falls behind, older samples are silently overwritten.
type RingBuffer struct {
	buf       [][2]float64
	write     int
	count     int
	lastWrite time.Time
	mu        sync.Mutex
}

// NewRingBuffer creates a ring buffer that can hold `capacity` stereo samples.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([][2]float64, capacity),
	}
}

// Write appends samples. If the buffer is full the oldest data is overwritten.
// Returns the number of samples actually written.
func (rb *RingBuffer) Write(samples [][2]float64) int {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := len(samples)
	if len(rb.buf) == 0 {
		return 0
	}
	if n > len(rb.buf) {
		n = len(rb.buf)
		samples = samples[len(samples)-n:]
	}

	for i := 0; i < n; i++ {
		rb.buf[(rb.write+i)%len(rb.buf)] = samples[i]
	}
	rb.write = (rb.write + n) % len(rb.buf)
	rb.count = min(len(rb.buf), rb.count+n)
	if n > 0 {
		rb.lastWrite = time.Now()
	}
	return n
}

// ReadAvailable returns up to `max` most recent samples, oldest first.
// Does NOT consume them — the buffer is a sliding window.
func (rb *RingBuffer) ReadAvailable(max int) [][2]float64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	available := rb.count
	if available > len(rb.buf) {
		available = len(rb.buf)
	}
	if max > 0 && max < available {
		available = max
	}
	if available == 0 {
		return nil
	}

	out := make([][2]float64, available)
	start := (rb.write - available + len(rb.buf)) % len(rb.buf)
	for i := 0; i < available; i++ {
		out[i] = rb.buf[(start+i)%len(rb.buf)]
	}
	return out
}

// Fresh reports whether the audio pipeline has written recently. Sliding-window
// reads alone cannot distinguish silence/end-of-track from an old FFT frame.
func (rb *RingBuffer) Fresh(now time.Time) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return !rb.lastWrite.IsZero() && now.Sub(rb.lastWrite) < 250*time.Millisecond
}

// Available returns how many samples are currently buffered.
func (rb *RingBuffer) Available() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}
