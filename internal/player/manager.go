package player

import (
	"sync"
	"time"

	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// PlaybackManager 管理播放状态，独立于播放器和解码器。
//
// 并发模型:
//   - 内部 state 是唯一可变状态，所有写操作必须经过 setter 方法，
//     setter 持 stateMutex 写后触发 emitSnapshot 广播快照。
//   - GetState 保留供历史调用方做只读访问(返回指针)，但调用方不得写字段;
//     新代码应使用 Subscribe 订阅不可变快照。
type PlaybackManager struct {
	playerCore *PlayerCore
	state      *models.PlaybackState
	stateMutex sync.RWMutex
	isStream   bool
	// 用于从 decoder 传递时长信息（异步）
	durationChan chan float64

	// 事件订阅:快照广播
	subscribers []chan models.PlaybackState
	subMu       sync.RWMutex
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
			pm.emitSnapshot()
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
	pm.state.CurrentTime = position
	// 只有当动态时长有效时才更新（避免用估算值覆盖已知的真实值）
	if duration > 0 && pm.state.Duration != duration {
		pm.state.Duration = duration
	}
	pm.stateMutex.Unlock()

	// 位置每 tick 都在变,广播一次让 UI 更新进度条
	pm.emitSnapshot()
}

// SetState 整体替换内部状态(用于恢复播放场景)
func (pm *PlaybackManager) SetState(state *models.PlaybackState) {
	pm.stateMutex.Lock()
	*pm.state = *state
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// GetState 返回内部状态指针(兼容历史调用方)。
//
// ⚠️ 仅限只读访问,且调用方必须理解:返回的指针指向内部状态,
// 字段可能被并发 setter 修改。新代码请用 Subscribe 订阅不可变快照。
func (pm *PlaybackManager) GetState() *models.PlaybackState {
	pm.stateMutex.RLock()
	defer pm.stateMutex.RUnlock()
	return pm.state
}

// Subscribe 订阅不可变状态快照。返回的 channel 缓冲为 16,
// 慢消费者会丢帧(非阻塞发送),保证后台 goroutine 不被阻塞。
// 订阅时会立即收到一次当前快照。
func (pm *PlaybackManager) Subscribe() <-chan models.PlaybackState {
	ch := make(chan models.PlaybackState, 16)
	pm.subMu.Lock()
	pm.subscribers = append(pm.subscribers, ch)
	pm.subMu.Unlock()
	// 立即推送当前快照,避免订阅者等待下一次变更
	pm.emitSnapshot()
	return ch
}

// Unsubscribe 移除一个订阅者并关闭其 channel
func (pm *PlaybackManager) Unsubscribe(ch <-chan models.PlaybackState) {
	pm.subMu.Lock()
	defer pm.subMu.Unlock()
	for i, sub := range pm.subscribers {
		// 比较底层 channel 引用是否相同(双向 chan 与 <-chan 底层同一引用)
		if any(sub) == any(ch) {
			close(sub)
			pm.subscribers = append(pm.subscribers[:i], pm.subscribers[i+1:]...)
			return
		}
	}
}

// emitSnapshot 生成不可变快照并广播给所有订阅者。
// 必须在未持有 stateMutex 的情况下调用(内部自行 RLock)。
func (pm *PlaybackManager) emitSnapshot() {
	pm.stateMutex.RLock()
	snapshot := pm.snapshotLocked()
	pm.stateMutex.RUnlock()

	pm.subMu.RLock()
	defer pm.subMu.RUnlock()
	for _, ch := range pm.subscribers {
		select {
		case ch <- snapshot:
		default:
			// 慢消费者:丢弃旧帧,保留最新状态
		}
	}
}

// snapshotLocked 生成 state 的深拷贝快照(必须持有 stateMutex.RLock)。
// 指针/切片字段做深拷贝,确保订阅者拿到的快照不可被后续 setter 修改影响。
func (pm *PlaybackManager) snapshotLocked() models.PlaybackState {
	snap := *pm.state // 值拷贝结构体

	// 深拷贝 CurrentSong 指针
	if pm.state.CurrentSong != nil {
		song := *pm.state.CurrentSong
		snap.CurrentSong = &song
	}

	// 深拷贝切片字段
	if pm.state.LyricsParsed != nil {
		snap.LyricsParsed = append([]models.LyricLine(nil), pm.state.LyricsParsed...)
	}
	if pm.state.Playlists != nil {
		snap.Playlists = append([]models.Playlist(nil), pm.state.Playlists...)
	}
	if pm.state.ShuffleQueue != nil {
		snap.ShuffleQueue = append([]int(nil), pm.state.ShuffleQueue...)
	}
	if pm.state.RecentSongs != nil {
		snap.RecentSongs = append([]models.PlaylistItem(nil), pm.state.RecentSongs...)
	}
	if pm.state.LyricSearchResults != nil {
		snap.LyricSearchResults = append([]models.LyricSearchResult(nil), pm.state.LyricSearchResults...)
	}

	return snap
}

// --- setter 方法:持锁写字段 + 触发快照广播 ---

// SetPlayerCore 设置播放核心
func (pm *PlaybackManager) SetPlayerCore(core *PlayerCore) {
	pm.stateMutex.Lock()
	pm.playerCore = core
	pm.stateMutex.Unlock()
}

// SetIsStream 设置是否为流式格式
func (pm *PlaybackManager) SetIsStream(isStream bool) {
	pm.isStream = isStream
}

// SetDuration 设置时长(通过 channel 异步通知更新)
func (pm *PlaybackManager) SetDuration(duration float64) {
	// 通过 channel 异步通知更新,避免阻塞解码器
	select {
	case pm.durationChan <- duration:
	default: // 非阻塞，避免阻塞解码器
	}
}

// SetIsPlaying 设置播放状态
func (pm *PlaybackManager) SetIsPlaying(playing bool) {
	pm.stateMutex.Lock()
	pm.state.IsPlaying = playing
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetCurrentSong 设置当前歌曲
func (pm *PlaybackManager) SetCurrentSong(song *models.PlaylistItem) {
	pm.stateMutex.Lock()
	pm.state.CurrentSong = song
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetVolume 设置音量
func (pm *PlaybackManager) SetVolume(volume float64) {
	pm.stateMutex.Lock()
	pm.state.Volume = volume
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetPlaybackRate 设置播放倍速
func (pm *PlaybackManager) SetPlaybackRate(rate float64) {
	pm.stateMutex.Lock()
	pm.state.PlaybackRate = rate
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetPlaybackMode 设置播放模式
func (pm *PlaybackManager) SetPlaybackMode(mode models.PlaybackMode) {
	pm.stateMutex.Lock()
	pm.state.PlaybackMode = mode
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetCurrentPlaylistName 设置当前播放列表名
func (pm *PlaybackManager) SetCurrentPlaylistName(name string) {
	pm.stateMutex.Lock()
	pm.state.CurrentPlaylistName = name
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetLyrics 设置歌词内容(raw + 解析结果)及显示开关
func (pm *PlaybackManager) SetLyrics(raw string, parsed []models.LyricLine, show bool) {
	pm.stateMutex.Lock()
	pm.state.LyricsRaw = raw
	pm.state.LyricsParsed = parsed
	pm.state.ShowLyrics = show
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}

// SetShowLyrics 设置歌词显示开关
func (pm *PlaybackManager) SetShowLyrics(show bool) {
	pm.stateMutex.Lock()
	pm.state.ShowLyrics = show
	pm.stateMutex.Unlock()
	pm.emitSnapshot()
}
