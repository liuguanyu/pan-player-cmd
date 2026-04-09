package player

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// SessionState represents the state of a transcode session
type SessionState string

const (
	StateIdle        SessionState = "idle"
	StateTranscoding SessionState = "transcoding"
	StateCompleted   SessionState = "completed"
	StateError       SessionState = "error"
)

// SessionEvent represents an event emitted by a session
type SessionEvent struct {
	Type      string      // "data", "complete", "error", "progress"
	Data      interface{} // event-specific data
	Timestamp time.Time
}

// ProgressData contains progress information
type ProgressData struct {
	Percent   float64
	Timemark  string
	TotalBytes int64
	Duration  float64
}

// DataEvent contains data chunk information
type DataEvent struct {
	Chunk      *bytes.Buffer
	TotalBytes int64
}

// CompleteEvent contains completion information
type CompleteEvent struct {
	TotalBytes int64
	Duration   float64
}

// TranscodeSession manages a single FFmpeg transcode process and memory buffering
type TranscodeSession struct {
	sessionID        string
	sourceURL        string
	startTimeSeconds float64

	mu              sync.RWMutex
	state           SessionState
	totalBytes      int64
	duration        float64
	chunks          []*bytes.Buffer
	mergedBuffer    *bytes.Buffer

	ffmpegCmd       *exec.Cmd
	cancelFunc      context.CancelFunc
	eventChan       chan SessionEvent
	eventListeners  map[string][]func(SessionEvent)

	createdAt       time.Time
	lastAccessedAt  time.Time
	destroyOnce     sync.Once
}

// NewTranscodeSession creates a new transcode session
func NewTranscodeSession(sessionID, sourceURL string, startTimeSeconds float64) *TranscodeSession {
	return &TranscodeSession{
		sessionID:        sessionID,
		sourceURL:        sourceURL,
		startTimeSeconds: startTimeSeconds,
		state:           StateIdle,
		totalBytes:      0,
		duration:        0,
		chunks:          make([]*bytes.Buffer, 0),
		mergedBuffer:    nil,
		eventChan:       make(chan SessionEvent, 100),
		eventListeners:  make(map[string][]func(SessionEvent)),
		createdAt:       time.Now(),
		lastAccessedAt:  time.Now(),
	}
}

// Getters
func (ts *TranscodeSession) SessionID() string     { return ts.sessionID }
func (ts *TranscodeSession) SourceURL() string     { return ts.sourceURL }
func (ts *TranscodeSession) StartTime() float64   { return ts.startTimeSeconds }
func (ts *TranscodeSession) State() SessionState  { return ts.state }
func (ts *TranscodeSession) IsComplete() bool     { return ts.state == StateCompleted }
func (ts *TranscodeSession) TotalBytes() int64    { return ts.totalBytes }
func (ts *TranscodeSession) Duration() float64    { return ts.duration }
func (ts *TranscodeSession) CreatedAt() time.Time { return ts.createdAt }
func (ts *TranscodeSession) LastAccessedAt() time.Time {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.lastAccessedAt
}
func (ts *TranscodeSession) SetLastAccessedAt(t time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.lastAccessedAt = t
}

// Start begins the FFmpeg transcode process
func (ts *TranscodeSession) Start() error {
	ts.mu.Lock()
	if ts.state != StateIdle {
		ts.mu.Unlock()
		return fmt.Errorf("session %s is already in state: %s", ts.sessionID, ts.state)
	}
	ts.state = StateTranscoding
	ts.mu.Unlock()

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	ts.cancelFunc = cancel

	// Build FFmpeg command
	args := ts.buildFFmpegArgs()

	ts.ffmpegCmd = exec.CommandContext(ctx, "ffmpeg", args...)
	ts.ffmpegCmd.Env = []string{"LANG=C", "LC_ALL=C"}

	// Capture stdout (audio data)
	stdout, err := ts.ffmpegCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Capture stderr (progress info)
	stderr, err := ts.ffmpegCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the command
	if err := ts.ffmpegCmd.Start(); err != nil {
		ts.mu.Lock()
		ts.state = StateError
		ts.mu.Unlock()
		return fmt.Errorf("failed to start FFmpeg: %w", err)
	}

	// Start goroutines
	go ts.readStdout(stdout)
	go ts.readStderr(stderr)
	go ts.waitProcess()

	return nil
}

// buildFFmpegArgs constructs FFmpeg command arguments
func (ts *TranscodeSession) buildFFmpegArgs() []string {
	args := []string{
		"-nostdin", // disable interactive input
		"-y",       // overwrite output file
	}

	// Add seek input if specified
	if ts.startTimeSeconds > 0 {
		args = append(args, "-ss", strconv.FormatFloat(ts.startTimeSeconds, 'f', -1, 64))
	}

	// Input
	args = append(args, "-i", ts.sourceURL)

	// Audio codec settings
	args = append(args,
		"-acodec", "flac",
		"-ar", "44100",
		"-ac", "2",
		"-f", "flac",
		"-compression_level", "0", // fastest compression
	)

	// Output to pipe
	args = append(args, "-")
	return args
}

// readStdout reads audio data from FFmpeg stdout
func (ts *TranscodeSession) readStdout(stdout interface{}) {
	reader, ok := stdout.(interface{ Read([]byte) (int, error) })
	if !ok {
		return
	}

	buf := make([]byte, 32768) // 32KB buffer
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := bytes.NewBuffer(buf[:n])

			ts.mu.Lock()
			ts.chunks = append(ts.chunks, chunk)
			ts.totalBytes += int64(n)
			ts.mergedBuffer = nil // clear merged buffer cache
			ts.mu.Unlock()

			ts.emitEvent(SessionEvent{
				Type: "data",
				Data: DataEvent{
					Chunk:      chunk,
					TotalBytes: ts.totalBytes,
				},
				Timestamp: time.Now(),
			})
		}

		if err != nil {
			break
		}
	}
}

// readStderr reads progress info from FFmpeg stderr
func (ts *TranscodeSession) readStderr(stderr interface{}) {
	reader, ok := stderr.(interface{ Read([]byte) (int, error) })
	if !ok {
		return
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		ts.parseProgress(line)
	}
}

// parseProgress parses FFmpeg progress output
func (ts *TranscodeSession) parseProgress(line string) {
	// Parse time=HH:MM:SS.mmm
	timemarkRe := regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)
	if match := timemarkRe.FindStringSubmatch(line); len(match) == 4 {
		hours, _ := strconv.ParseFloat(match[1], 64)
		minutes, _ := strconv.ParseFloat(match[2], 64)
		seconds, _ := strconv.ParseFloat(match[3], 64)
		duration := hours*3600 + minutes*60 + seconds

		ts.mu.Lock()
		ts.duration = duration
		ts.mu.Unlock()

		ts.emitEvent(SessionEvent{
			Type: "progress",
			Data: ProgressData{
				Duration: duration,
			},
			Timestamp: time.Now(),
		})
	}
}

// waitProcess waits for FFmpeg process to complete
func (ts *TranscodeSession) waitProcess() {
	err := ts.ffmpegCmd.Wait()

	ts.mu.Lock()
	if err != nil {
		ts.state = StateError
		ts.mu.Unlock()
		ts.emitEvent(SessionEvent{
			Type: "error",
			Data: err,
			Timestamp: time.Now(),
		})
		return
	}

	ts.state = StateCompleted
	ts.mergePendingChunksLocked()
	totalBytes := ts.totalBytes
	duration := ts.duration
	ts.mu.Unlock()

	ts.emitEvent(SessionEvent{
		Type: "complete",
		Data: CompleteEvent{
			TotalBytes: totalBytes,
			Duration:   duration,
		},
		Timestamp: time.Now(),
	})
}

// GetBufferedData returns buffered data in the specified range
func (ts *TranscodeSession) GetBufferedData(start, end int64) *bytes.Buffer {
	ts.mu.Lock()
	ts.lastAccessedAt = time.Now()

	if start < 0 || start >= ts.totalBytes {
		ts.mu.Unlock()
		return bytes.NewBuffer(nil)
	}

	if end <= 0 || end > ts.totalBytes {
		end = ts.totalBytes
	}

	if start == 0 && int(end) == int(ts.totalBytes) {
		defer ts.mu.Unlock()
		ts.mergePendingChunksLocked()
		return ts.mergedBuffer
	}

	// Get full buffer
	if ts.mergedBuffer == nil || ts.mergedBuffer.Len() != int(ts.totalBytes) {
		ts.mergePendingChunksLocked()
	}

	if ts.mergedBuffer == nil || ts.mergedBuffer.Len() == 0 {
		ts.mu.Unlock()
		return bytes.NewBuffer(nil)
	}

	result := bytes.NewBuffer(ts.mergedBuffer.Bytes()[start:end])
	ts.mu.Unlock()
	return result
}

// GetFullBuffer returns the complete merged buffer
func (ts *TranscodeSession) GetFullBuffer() *bytes.Buffer {
	ts.mu.Lock()
	ts.lastAccessedAt = time.Now()
	ts.mu.Unlock()

	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.mergedBuffer != nil && ts.mergedBuffer.Len() == int(ts.totalBytes) {
		return ts.mergedBuffer
	}

	ts.mergePendingChunksLocked()
	return ts.mergedBuffer
}

// getFullBuffer internal implementation (must be called with lock held)
func (ts *TranscodeSession) getFullBuffer() *bytes.Buffer {
	if ts.mergedBuffer != nil && ts.mergedBuffer.Len() == int(ts.totalBytes) {
		return ts.mergedBuffer
	}

	ts.mergePendingChunksLocked()
	return ts.mergedBuffer
}

// mergePendingChunksLocked merges all chunks into a single buffer (must be called with lock held)
func (ts *TranscodeSession) mergePendingChunksLocked() {
	if len(ts.chunks) == 0 {
		ts.mergedBuffer = bytes.NewBuffer(nil)
		return
	}

	if len(ts.chunks) == 1 {
		ts.mergedBuffer = bytes.NewBuffer(ts.chunks[0].Bytes())
		return
	}

	// Calculate total size
	totalSize := 0
	for _, chunk := range ts.chunks {
		totalSize += chunk.Len()
	}

	// Merge all chunks
	buf := make([]byte, 0, totalSize)
	for _, chunk := range ts.chunks {
		buf = append(buf, chunk.Bytes()...)
	}

	ts.mergedBuffer = bytes.NewBuffer(buf)

	// When completed, replace chunks array with merged buffer
	if ts.state == StateCompleted {
		ts.chunks = []*bytes.Buffer{ts.mergedBuffer}
	}
}

// WaitForData waits until buffer reaches minimum bytes or timeout occurs
func (ts *TranscodeSession) WaitForData(minBytes int64, timeoutMs int) bool {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Check if we already have enough data
	ts.mu.RLock()
	currentBytes := ts.totalBytes
	currentState := ts.state
	ts.mu.RUnlock()

	if currentBytes >= minBytes {
		return true
	}

	if currentState == StateCompleted || currentState == StateError {
		return currentBytes >= minBytes
	}

	// Wait for data event
	done := make(chan bool, 1)
	listener := func(event SessionEvent) {
		if event.Type == "data" {
			if data, ok := event.Data.(DataEvent); ok && data.TotalBytes >= minBytes {
				done <- true
			}
		} else if event.Type == "complete" || event.Type == "error" {
			ts.mu.RLock()
			totalBytes := ts.totalBytes
			ts.mu.RUnlock()
			done <- totalBytes >= minBytes
		}
	}

	ts.On("data", listener)
	ts.On("complete", listener)
	ts.On("error", listener)

	select {
	case result := <-done:
		ts.RemoveListener("data", listener)
		ts.RemoveListener("complete", listener)
		ts.RemoveListener("error", listener)
		return result
	case <-ctx.Done():
		ts.RemoveListener("data", listener)
		ts.RemoveListener("complete", listener)
		ts.RemoveListener("error", listener)
		return false
	}
}

// Destroy terminates FFmpeg process and releases all resources
func (ts *TranscodeSession) Destroy() {
	ts.destroyOnce.Do(func() {
		ts.mu.Lock()
		ts.state = StateIdle
		ts.mu.Unlock()

		// Cancel context
		if ts.cancelFunc != nil {
			ts.cancelFunc()
			ts.cancelFunc = nil
		}

		// Kill FFmpeg process
		if ts.ffmpegCmd != nil && ts.ffmpegCmd.Process != nil {
			ts.ffmpegCmd.Process.Kill()
			ts.ffmpegCmd = nil
		}

		// Release memory
		ts.mu.Lock()
		ts.chunks = nil
		ts.mergedBuffer = nil
		ts.totalBytes = 0
		ts.mu.Unlock()

		// Remove all event listeners
		ts.ClearAllListeners()

		// Close event channel
		close(ts.eventChan)
	})
}

// GetMemoryUsage returns current memory usage in bytes
func (ts *TranscodeSession) GetMemoryUsage() int64 {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.totalBytes
}

// Event handling methods

// On registers an event listener
func (ts *TranscodeSession) On(eventType string, listener func(SessionEvent)) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.eventListeners[eventType] = append(ts.eventListeners[eventType], listener)
}

// RemoveListener removes a specific event listener
func (ts *TranscodeSession) RemoveListener(eventType string, listener func(SessionEvent)) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	listeners := ts.eventListeners[eventType]
	for i, l := range listeners {
		// Compare function pointers
		if fmt.Sprintf("%p", l) == fmt.Sprintf("%p", listener) {
			ts.eventListeners[eventType] = append(listeners[:i], listeners[i+1:]...)
			break
		}
	}
}

// RemoveAllListeners removes all listeners for a specific event type
func (ts *TranscodeSession) RemoveAllListeners(eventType string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	delete(ts.eventListeners, eventType)
}

// ClearAllListeners removes all event listeners for all event types
func (ts *TranscodeSession) ClearAllListeners() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.eventListeners = make(map[string][]func(SessionEvent))
}

// emitEvent emits an event to all registered listeners
func (ts *TranscodeSession) emitEvent(event SessionEvent) {
	ts.mu.RLock()
	listeners := ts.eventListeners[event.Type]
	// Copy listeners map to avoid holding lock during callback
	if listCopy := make([]func(SessionEvent), len(listeners)); len(listeners) > 0 {
		copy(listCopy, listeners)
		ts.mu.RUnlock()

		for _, listener := range listCopy {
			go listener(event)
		}
	} else {
		ts.mu.RUnlock()
	}

	// Also send to event channel
	select {
	case ts.eventChan <- event:
	default:
		// Channel full, drop event
	}
}

// Events returns the event channel
func (ts *TranscodeSession) Events() <-chan SessionEvent {
	return ts.eventChan
}
