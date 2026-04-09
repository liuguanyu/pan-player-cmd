package player

import (
	"sync"
	"time"

	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// PlaybackManager 管理播放状态，独立于播放器和解码器
type PlaybackManager struct {
	playerCore *PlayerCore
	state      *models.PlaybackState
	stateMutex sync.RWMutex
	isStream   bool
	// 用于从 decoder 传递时长信息（异步）
	durationChan chan float64
}

func (pm *PlaybackManager) Start() {
	// 启动进度更新器
	go pm.updatePositionLoop()
}

func (pm *PlaybackManager) updatePositionLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.updatePosition()
		case duration := <-pm.durationChan:
			// 接收来自 decoder 的动态时长更新
			pm.stateMutex.Lock()
			pm.state.Duration = duration
			pm.stateMutex.Unlock()
		}
	}
}

func (pm *PlaybackManager) updatePosition() {
	// ✅ 不持有锁！仅读取当前状态
	if pm.playerCore == nil {
		return
	}

	position := pm.playerCore.GetCurrentPosition()

	// 动态从 streamer 获取最新时长（对于 M4A 流式播放，时长是异步解析的）
	duration := pm.playerCore.GetDynamicDuration()

	pm.stateMutex.Lock()
	defer pm.stateMutex.Unlock()

	pm.state.CurrentTime = position
	// 只有当动态时长有效时才更新（避免用估算值覆盖已知的真实值）
	if duration > 0 {
		pm.state.Duration = duration
	}
}

func (pm *PlaybackManager) SetState(state *models.PlaybackState) {
	pm.stateMutex.Lock()
	defer pm.stateMutex.Unlock()
	*pm.state = *state
}

func (pm *PlaybackManager) GetState() *models.PlaybackState {
	pm.stateMutex.RLock()
	defer pm.stateMutex.RUnlock()
	return pm.state
}

func (pm *PlaybackManager) SetPlayerCore(core *PlayerCore) {
	pm.playerCore = core
}

func (pm *PlaybackManager) SetIsStream(isStream bool) {
	pm.isStream = isStream
}

func (pm *PlaybackManager) SetDuration(duration float64) {
	// 通过 channel 异步通知更新
	select {
	case pm.durationChan <- duration:
	default: // 非阻塞，避免阻塞解码器
	}
}
