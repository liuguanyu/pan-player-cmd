package player

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// Player 是对外统一接口，组合所有模块
type Player struct {
	core            *PlayerCore
	decoder         *AudioDecoder
	manager         *PlaybackManager
	apiClient       *api.BaiduPanClient
	cacheDir        string
	stopChan        chan struct{}
	mu              sync.RWMutex
	currentTrack    *models.PlaylistItem
	currentPlaylist *models.Playlist
	currentIndex    int
	loadingCancel   context.CancelFunc // 用于取消正在进行的加载
	loadingMu       sync.Mutex         // 保护 loadingCancel
	onTrackPlay     func(*models.PlaylistItem) // 歌曲开始播放时的回调函数
}

// PlayerConfig 播放器配置
type PlayerConfig struct {
	AudioDevice string
	CacheDir    string
	SampleRate  int
}

// NewPlayer 创建新的播放器
func NewPlayer(cfg *PlayerConfig, apiClient *api.BaiduPanClient) *Player {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 44100
	}

	// 创建 PlayerCore 实例（只创建一次）
	playerCore := &PlayerCore{}

	p := &Player{
		apiClient: apiClient,
		cacheDir:  cfg.CacheDir,
		stopChan:  make(chan struct{}),
		core:      playerCore,
		decoder:   &AudioDecoder{apiClient: apiClient, cacheDir: cfg.CacheDir},
	}

	// 使用 NewPlaybackManager 创建播放管理器
	p.manager = NewPlaybackManager()
	// 设置 PlayerCore 引用
	p.manager.SetPlayerCore(playerCore)

	// 初始化扬声器 - 必须在任何播放操作之前完成
	logger := utils.GetLogger()
	sr := beep.SampleRate(cfg.SampleRate)
	if err := speaker.Init(sr, sr.N(time.Second/10)); err != nil {
		// 如果初始化失败，记录错误
		logger.Error("Speaker初始化失败: %v", err)
		logger.Error("这可能导致无法播放音频")
	} else {
		logger.Info("Speaker初始化成功 (采样率: %d)", sr)
	}

	// 设置播放结束回调，实现自动切歌
	p.core.SetOnTrackEnd(func() {
		logger.Info("播放结束，根据播放模式自动切歌")
		p.PlayNext()
	})

	// 启动播放状态和进度更新器（独立goroutine）
	p.manager.Start()

	return p
}

// SetOnTrackPlay 设置歌曲开始播放时的回调函数
func (p *Player) SetOnTrackPlay(callback func(*models.PlaylistItem)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onTrackPlay = callback
}

// SetCurrentFsID 设置当前播放音频的 fsID
func (p *Player) SetCurrentFsID(fsID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.core.currentFsID = fsID
}

// LoadTrack 非阻塞加载音轨
func (p *Player) LoadTrack(ctx context.Context, track *models.PlaylistItem) error {
	// 先停止当前播放，避免多个音频流同时播放
	p.Stop()

	// 取消之前的加载任务
	p.loadingMu.Lock()
	if p.loadingCancel != nil {
		utils.GetLogger().Info("取消之前的加载任务")
		p.loadingCancel()
	}
	// 创建新的取消函数
	ctx, cancel := context.WithCancel(ctx)
	p.loadingCancel = cancel
	p.loadingMu.Unlock()

	p.mu.Lock()
	p.currentTrack = track
	// 设置当前播放音频的 fsID（用于缓存访问）
	p.core.currentFsID = track.FsID
	p.mu.Unlock()

	// 更新播放状态中的当前歌曲
	state := p.manager.GetState()
	state.CurrentSong = track

	// 标记为播放中，这样加载完成后会自动播放
	state.IsPlaying = true

	ch := p.decoder.LoadTrack(ctx, track)
	go func() {
		res := <-ch

		// 检查是否被取消
		select {
		case <-ctx.Done():
			utils.GetLogger().Info("加载任务已取消: %s", track.ServerFileName)
			return
		default:
		}

		if res.err != nil {
			utils.GetLogger().Error("加载音轨失败: %v", res.err)
			// 加载失败，标记为暂停
			state := p.manager.GetState()
			state.IsPlaying = false
			return
		}

		// 再次检查是否被取消（在设置流之前）
		select {
		case <-ctx.Done():
			utils.GetLogger().Info("加载任务已取消（在设置流前）: %s", track.ServerFileName)
			// 关闭未使用的流
			if res.streamer != nil {
				res.streamer.Close()
			}
			return
		default:
		}

		p.mu.Lock()
		utils.GetLogger().Info("解码成功，开始设置流")
		p.core.SetStream(res.streamer, res.format)
		p.manager.SetPlayerCore(p.core)
		p.manager.SetIsStream(true) // M4A/WAV 是流式格式

		// 更新播放状态的时长
		if res.format.SampleRate > 0 {
			duration := res.format.SampleRate.D(res.streamer.Len()).Seconds()
			p.manager.SetDuration(duration)
			utils.GetLogger().Info("音频时长: %.2f秒", duration)
		}

		utils.GetLogger().Info("准备调用 Play()")
		// 自动开始播放
		p.core.Play()
		utils.GetLogger().Info("Play() 调用完成")
		state := p.manager.GetState()
		state.IsPlaying = true
		p.mu.Unlock()

		// 触发歌曲开始播放的回调
		p.mu.RLock()
		if p.onTrackPlay != nil {
			go p.onTrackPlay(track)
		}
		p.mu.RUnlock()

		utils.GetLogger().Info("音频加载完成，自动开始播放")
	}()

	utils.GetLogger().Info("LoadTrack 完成: %s", track.ServerFileName)
	return nil
}

// GetState 获取播放状态（安全，无锁读取）
func (p *Player) GetState() *models.PlaybackState {
	return p.manager.GetState()
}

// Play 开始播放
func (p *Player) Play() {
	p.core.Play()
	// 更新播放状态
	state := p.manager.GetState()
	state.IsPlaying = true
}

// Pause 暂停播放
func (p *Player) Pause() {
	p.core.Pause()
	// 更新播放状态
	state := p.manager.GetState()
	state.IsPlaying = false
}

// Stop 停止播放并清理资源
func (p *Player) Stop() {
	p.core.Stop()
}

// Seek 跳转到指定位置（秒）
func (p *Player) Seek(pos float64) {
	p.core.Seek(pos)
}

// SetVolume 设置音量
func (p *Player) SetVolume(volume float64) {
	// 限制音量在0-1之间
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}

	// 更新播放状态中的音量
	state := p.manager.GetState()
	state.Volume = volume

	// 设置核心播放器的音量
	p.core.SetVolume(volume)
}

// PlayNext 播放下一首
func (p *Player) PlayNext() {
	if p.currentPlaylist == nil || len(p.currentPlaylist.Items) == 0 {
		return
	}

	p.mu.RLock()
	currentIndex := p.currentIndex
	items := p.currentPlaylist.Items
	playbackMode := p.manager.GetState().PlaybackMode
	p.mu.RUnlock()

	// 根据播放模式计算下一曲
	switch playbackMode {
	case models.PlaybackModeOrder:
		// 顺序播放：播到最后一首后停止
		if currentIndex < 0 || currentIndex >= len(items)-1 {
			// 已经是最后一首，不自动播放下一首
			p.mu.Lock()
			p.currentIndex = currentIndex
			p.mu.Unlock()
			return
		}
		currentIndex++
	case models.PlaybackModeRandom:
		// 随机播放：在整个列表中随机选择
		currentIndex = rand.Intn(len(items))
	case models.PlaybackModeSingle:
		// 单曲循环：重新播放当前曲目
		if currentIndex < 0 || currentIndex >= len(items) {
			currentIndex = 0
		}
		// 保持 currentIndex 不变，直接重新加载
	default:
		// 默认顺序播放
		if currentIndex < 0 || currentIndex >= len(items)-1 {
			currentIndex = 0
		} else {
			currentIndex++
		}
	}

	// 停止当前播放
	p.Stop()

	p.SetCurrentIndex(currentIndex)
	p.LoadTrack(context.Background(), items[currentIndex])
}

// PlayPrevious 播放上一首
func (p *Player) PlayPrevious() {
	if p.currentPlaylist == nil || len(p.currentPlaylist.Items) == 0 {
		return
	}

	p.mu.RLock()
	currentIndex := p.currentIndex
	items := p.currentPlaylist.Items
	playbackMode := p.manager.GetState().PlaybackMode
	p.mu.RUnlock()

	// 根据播放模式计算上一曲
	switch playbackMode {
	case models.PlaybackModeOrder:
		// 顺序播放：播到第一首后停止
		if currentIndex <= 0 {
			// 已经是第一首，不自动播放上一首
			p.mu.Lock()
			p.currentIndex = currentIndex
			p.mu.Unlock()
			return
		}
		currentIndex--
	case models.PlaybackModeRandom:
		// 随机播放：在整个列表中随机选择
		currentIndex = rand.Intn(len(items))
	case models.PlaybackModeSingle:
		// 单曲循环：重新播放当前曲目
		if currentIndex < 0 || currentIndex >= len(items) {
			currentIndex = 0
		}
		// 保持 currentIndex 不变，直接重新加载
	default:
		// 默认顺序播放
		if currentIndex <= 0 {
			currentIndex = len(items) - 1
		} else {
			currentIndex--
		}
	}

	// 停止当前播放
	p.Stop()

	p.SetCurrentIndex(currentIndex)
	p.LoadTrack(context.Background(), items[currentIndex])
}

// IsPlaying 是否正在播放
func (p *Player) IsPlaying() bool {
	return p.core.IsPlaying()
}

// GetCurrentPlaylist 获取当前播放列表
func (p *Player) GetCurrentPlaylist() *models.Playlist {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentPlaylist
}

// SetCurrentSong 设置当前曲目（用于恢复播放状态）
func (p *Player) SetCurrentSong(track *models.PlaylistItem) {
	p.mu.Lock()
	p.currentTrack = track
	// 设置当前播放音频的 fsID（用于缓存访问）
	p.core.currentFsID = track.FsID
	p.mu.Unlock()

	// 更新播放状态中的当前歌曲
	state := p.manager.GetState()
	state.CurrentSong = track
}

// SetCurrentPlaylist 设置当前播放列表
func (p *Player) SetCurrentPlaylist(name string, items []*models.PlaylistItem) {
	// 停止当前播放
	p.Stop()

	p.mu.Lock()
	p.currentPlaylist = &models.Playlist{
		Name:  name,
		Items: items,
	}
	p.currentIndex = -1
	p.mu.Unlock()

	// 更新播放状态中的播放列表名称
	state := p.manager.GetState()
	state.CurrentPlaylistName = name
}

// SetPlayMode 设置播放模式
func (p *Player) SetPlayMode(mode models.PlaybackMode) {
	state := p.manager.GetState()
	state.PlaybackMode = mode
}

// GetCurrentIndex 获取当前播放索引
func (p *Player) GetCurrentIndex() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentIndex
}

// SetCurrentIndex 设置当前播放索引
func (p *Player) SetCurrentIndex(index int) {
	p.mu.Lock()
	p.currentIndex = index
	p.mu.Unlock()
}

// GetRealTimeFeatures 获取实时音频特征通道
func (p *Player) GetRealTimeFeatures() <-chan models.RealtimeFeatures {
	return p.core.GetRealTimeFeatures()
}
