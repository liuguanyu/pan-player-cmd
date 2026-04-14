package player

import (
	"log"
	"sort"
	"sync"
	"time"
)

// SessionManagerConfig holds configuration for session manager
type SessionManagerConfig struct {
	MaxConcurrentSessions int   // Maximum number of concurrent sessions
	MaxTotalMemoryBytes   int64 // Maximum total memory usage in bytes
}

// DefaultSessionManagerConfig returns default configuration
func DefaultSessionManagerConfig() SessionManagerConfig {
	return SessionManagerConfig{
		MaxConcurrentSessions: 3,
		MaxTotalMemoryBytes:   500 * 1024 * 1024, // 500MB
	}
}

// SessionSummary provides summary info about a session
type SessionSummary struct {
	SessionID      string
	SourceURL      string
	State          SessionState
	TotalBytes     int64
	CreatedAt      time.Time
	LastAccessedAt time.Time
}

// SessionManager manages all transcode sessions with LRU caching
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*TranscodeSession
	config   SessionManagerConfig
}

var (
	sessionManagerInstance *SessionManager
	sessionManagerOnce     sync.Once
)

// GetSessionManager returns the singleton instance of SessionManager
func GetSessionManager(config *SessionManagerConfig) *SessionManager {
	sessionManagerOnce.Do(func() {
		cfg := DefaultSessionManagerConfig()
		if config != nil {
			cfg = *config
		}
		sessionManagerInstance = &SessionManager{
			sessions: make(map[string]*TranscodeSession),
			config:   cfg,
		}
		log.Println("[SessionManager] Instance created")
	})
	return sessionManagerInstance
}

// ResetSessionManager resets the singleton instance (for testing only)
func ResetSessionManager() {
	if sessionManagerInstance != nil {
		sessionManagerInstance.DestroyAll()
		sessionManagerInstance = nil
		sessionManagerOnce = sync.Once{}
	}
}

// GetOrCreateSession gets or creates a transcode session
//
// Strategy:
// 1. If session with same ID exists and URL matches, return existing session
// 2. If completed session with same URL exists, reuse that cache
// 3. Otherwise create new session
func (sm *SessionManager) GetOrCreateSession(sessionID, sourceURL string, startTimeSeconds float64) *TranscodeSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 1. Check if session with same ID already exists
	existing, exists := sm.sessions[sessionID]
	if exists && existing.SourceURL() == sourceURL {
		log.Printf("[SessionManager] Reusing existing session: %s", sessionID)
		existing.SetLastAccessedAt(time.Now())
		return existing
	}

	// 2. Check for cached session with same URL (only if no seek offset)
	if startTimeSeconds == 0 {
		cached := sm.findCachedSession(sourceURL)
		if cached != nil {
			log.Printf("[SessionManager] Found cached session for URL, reusing as %s", sessionID)
			cached.SetLastAccessedAt(time.Now())
			// Add new mapping if sessionID doesn't exist
			if !exists {
				sm.sessions[sessionID] = cached
			}
			return cached
		}
	}

	// 3. If exists with different URL, destroy old session
	if exists {
		log.Printf("[SessionManager] Destroying old session with same id but different URL: %s", sessionID)
		sm.destroySessionInternal(sessionID)
	}

	// 4. Enforce limits before creating new session
	sm.enforceMemoryLimits()
	sm.enforceConcurrencyLimits()

	// 5. Create new session
	session := NewTranscodeSession(sessionID, sourceURL, startTimeSeconds)

	// Set up event listeners for logging
	session.On("complete", func(event SessionEvent) {
		if data, ok := event.Data.(CompleteEvent); ok {
			log.Printf("[SessionManager] Session completed: %s, bytes: %d", sessionID, data.TotalBytes)
		}
	})

	session.On("error", func(event SessionEvent) {
		if err, ok := event.Data.(error); ok {
			log.Printf("[SessionManager] Session error: %s: %v", sessionID, err)
		}
	})

	sm.sessions[sessionID] = session
	log.Printf("[SessionManager] Created new session: %s, total sessions: %d", sessionID, len(sm.sessions))

	return session
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) *TranscodeSession {
	sm.mu.RLock()
	session, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return nil
	}

	session.SetLastAccessedAt(time.Now())
	return session
}

// DestroySession destroys a specific session
func (sm *SessionManager) DestroySession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.destroySessionInternal(sessionID)
}

// DestroyAll destroys all sessions (called on app exit)
func (sm *SessionManager) DestroyAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	log.Printf("[SessionManager] Destroying all sessions (%d total)", len(sm.sessions))
	for sessionID := range sm.sessions {
		sm.destroySessionInternal(sessionID)
	}
	sm.sessions = make(map[string]*TranscodeSession)
	log.Println("[SessionManager] All sessions destroyed")
}

// Query methods

// FindCachedSession finds a completed session with the same source URL
func (sm *SessionManager) FindCachedSession(sourceURL string) *TranscodeSession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.findCachedSession(sourceURL)
}

// findCachedSession internal implementation (must be called with lock held)
func (sm *SessionManager) findCachedSession(sourceURL string) *TranscodeSession {
	for _, session := range sm.sessions {
		if session.SourceURL() == sourceURL && session.IsComplete() {
			return session
		}
	}
	return nil
}

// SessionCount returns total number of sessions
func (sm *SessionManager) SessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// GetTotalMemoryUsage returns total memory usage in bytes
func (sm *SessionManager) GetTotalMemoryUsage() int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var total int64
	for _, session := range sm.sessions {
		total += session.GetMemoryUsage()
	}
	return total
}

// GetActiveTranscodingCount returns number of sessions currently transcoding
func (sm *SessionManager) GetActiveTranscodingCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	count := 0
	for _, session := range sm.sessions {
		if session.State() == StateTranscoding {
			count++
		}
	}
	return count
}

// GetSessionsSummary returns summary info for all sessions (for debugging)
func (sm *SessionManager) GetSessionsSummary() []SessionSummary {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	summaries := make([]SessionSummary, 0, len(sm.sessions))
	for id, session := range sm.sessions {
		url := session.SourceURL()
		if len(url) > 80 {
			url = url[:80]
		}
		summaries = append(summaries, SessionSummary{
			SessionID:      id,
			SourceURL:      url,
			State:          session.State(),
			TotalBytes:     session.TotalBytes(),
			CreatedAt:      session.CreatedAt(),
			LastAccessedAt: session.LastAccessedAt(),
		})
	}
	return summaries
}

// Internal methods

// destroySessionInternal destroys a session (must be called with lock held)
func (sm *SessionManager) destroySessionInternal(sessionID string) {
	session, exists := sm.sessions[sessionID]
	if !exists {
		return
	}

	log.Printf("[SessionManager] Destroying session: %s", sessionID)
	session.Destroy()
	delete(sm.sessions, sessionID)
}

// enforceMemoryLimits enforces memory limit using LRU eviction
func (sm *SessionManager) enforceMemoryLimits() {
	// This should be called with lock already held
	totalMemory := sm.getTotalMemoryUsageInternal()

	if totalMemory <= sm.config.MaxTotalMemoryBytes {
		return
	}

	log.Printf("[SessionManager] Memory limit exceeded: %.1fMB / %.1fMB, cleaning up...",
		float64(totalMemory)/1024/1024,
		float64(sm.config.MaxTotalMemoryBytes)/1024/1024)

	// Get inactive sessions sorted by LRU
	inactiveSessions := sm.getInactiveSessionsSortedByLRU()

	for _, id := range inactiveSessions {
		if totalMemory <= sm.config.MaxTotalMemoryBytes {
			break
		}

		session := sm.sessions[id]
		if session != nil {
			freed := session.GetMemoryUsage()
			sm.destroySessionInternal(id)
			totalMemory -= freed
			log.Printf("[SessionManager] Evicted session %s, freed %.1fMB",
				id, float64(freed)/1024/1024)
		}
	}
}

// enforceConcurrencyLimits enforces concurrent session limit
func (sm *SessionManager) enforceConcurrencyLimits() {
	// This should be called with lock already held
	if len(sm.sessions) < sm.config.MaxConcurrentSessions {
		return
	}

	log.Printf("[SessionManager] Concurrency limit reached: %d / %d, cleaning up...",
		len(sm.sessions), sm.config.MaxConcurrentSessions)

	// Get inactive sessions sorted by LRU
	inactiveSessions := sm.getInactiveSessionsSortedByLRU()

	// Number of sessions to free
	toFree := len(sm.sessions) - sm.config.MaxConcurrentSessions + 1

	for i := 0; i < min(toFree, len(inactiveSessions)); i++ {
		sessionID := inactiveSessions[i]
		log.Printf("[SessionManager] Evicting session for concurrency: %s", sessionID)
		sm.destroySessionInternal(sessionID)
	}
}

// getInactiveSessionsSortedByLRU returns inactive sessions sorted by last access time (oldest first)
func (sm *SessionManager) getInactiveSessionsSortedByLRU() []string {
	// This should be called with lock already held
	type sessionInfo struct {
		id           string
		lastAccessed time.Time
	}

	var inactive []sessionInfo
	for id, session := range sm.sessions {
		state := session.State()
		if state == StateCompleted || state == StateError || state == StateIdle {
			inactive = append(inactive, sessionInfo{
				id:           id,
				lastAccessed: session.LastAccessedAt(),
			})
		}
	}

	// Sort by lastAccessedAt ascending (oldest first)
	sort.Slice(inactive, func(i, j int) bool {
		return inactive[i].lastAccessed.Before(inactive[j].lastAccessed)
	})

	// Extract IDs
	result := make([]string, len(inactive))
	for i, info := range inactive {
		result[i] = info.id
	}

	return result
}

// getTotalMemoryUsageInternal returns total memory usage (must be called with lock held)
func (sm *SessionManager) getTotalMemoryUsageInternal() int64 {
	var total int64
	for _, session := range sm.sessions {
		total += session.GetMemoryUsage()
	}
	return total
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
