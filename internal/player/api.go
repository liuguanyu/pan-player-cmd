package player

import (
	"context"
	cryptorand "crypto/rand"
	"math/big"
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
	loadingCancel   context.CancelFunc         // 用于取消正在进行的加载
	loadingMu       sync.Mutex                 // 保护 loadingCancel
	onTrackPlay     func(*models.PlaylistItem) // 歌曲开始播放时的回调函数

	// 随机播放相关
	shuffledIndices []int // 洗牌后的索引序列
	shufflePosition int   // 当前在洗牌序列中的位置
}

// PlayerConfig 播放器配置
type PlayerConfig struct {
	AudioDevice string
	CacheDir    string
	SampleRate  int
	Speed       float64 // 播放倍速
}

// NewPlayer 创建新的播放器
func NewPlayer(cfg *PlayerConfig, apiClient *api.BaiduPanClient) *Player {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 44100
	}
	if cfg.Speed <= 0 {
		cfg.Speed = 1.0
	}

	// 创建 PlayerCore 实例（只创建一次）
	playerCore := &PlayerCore{speed: cfg.Speed}

	p := &Player{
		apiClient: apiClient,
		cacheDir:  cfg.CacheDir,
		stopChan:  make(chan struct{}),
		core:      playerCore,
		decoder:   &AudioDecoder{apiClient: apiClient, cacheDir: cfg.CacheDir},
		manager: &PlaybackManager{
			playerCore: playerCore,
			state: &models.PlaybackState{
				Volume:       0.6,                      // 默认音量60%
				PlaybackMode: models.PlaybackModeOrder, // 默认顺序播放
				PlaybackRate: cfg.Speed,                // 初始化播放倍速
				ShowLyrics:   true,                     // 默认显示歌词
			},
			durationChan: make(chan float64, 10), // 初始化 durationChan
		},
	}

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

// SetSpeed 设置播放倍速
func (p *Player) SetSpeed(speed float64) {
	// 限制速度在合理范围 [0.5, 10.0]
	if speed < 0.5 {
		speed = 0.5
	}
	if speed > 10.0 {
		speed = 10.0
	}

	// 更新播放状态中的速度
	state := p.manager.GetState()
	state.PlaybackRate = speed

	// 设置核心播放器的速度
	p.core.SetSpeed(speed)
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
		// 顺序播放：播到最后一首后循环到第一首
		if currentIndex < 0 || currentIndex >= len(items)-1 {
			// 已经是最后一首，循环到第一首
			currentIndex = 0
		} else {
			currentIndex++
		}
	case models.PlaybackModeRandom:
		// 随机播放：使用稳定的洗牌算法
		currentIndex = p.getShuffleNext()
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
		// 顺序播放：播到第一首后循环到最后一首
		if currentIndex <= 0 {
			// 已经是第一首，循环到最后一首
			currentIndex = len(items) - 1
		} else {
			currentIndex--
		}
	case models.PlaybackModeRandom:
		// 随机播放：使用稳定的洗牌算法
		currentIndex = p.getShufflePrevious()
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
	// 重置洗牌序列
	p.shuffledIndices = nil
	p.shufflePosition = 0

	// 如果是随机播放模式，先生成洗牌顺序
	playbackMode := p.manager.GetState().PlaybackMode
	if playbackMode == models.PlaybackModeRandom {
		p.mu.Unlock()
		p.generateShuffleOrder()
		p.mu.Lock()
	}
	p.mu.Unlock()

	// 更新播放状态中的播放列表名称
	state := p.manager.GetState()
	state.CurrentPlaylistName = name
}

// SetPlayMode 设置播放模式
func (p *Player) SetPlayMode(mode models.PlaybackMode) {
	state := p.manager.GetState()
	oldMode := state.PlaybackMode
	state.PlaybackMode = mode

	// 如果切换到随机播放模式，重新生成洗牌序列
	if mode == models.PlaybackModeRandom && oldMode != mode {
		p.generateShuffleOrder()
		utils.GetLogger().Info("切换到随机播放模式，重新生成洗牌序列")
	}
}

// GetCurrentIndex 获取当前播放索引
func (p *Player) GetCurrentIndex() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentIndex
}

// GetShuffleStartIndex 获取随机播放的起始索引
// 用于在随机模式下开始播放时，从洗牌序列中获取一个随机位置
func (p *Player) GetShuffleStartIndex() int {
	if p.currentPlaylist == nil || len(p.currentPlaylist.Items) == 0 {
		return 0
	}
	if p.shuffledIndices == nil || len(p.shuffledIndices) != len(p.currentPlaylist.Items) {
		p.generateShuffleOrder()
	}
	// 随机选择洗牌序列中的一个位置作为起始
	startPos := rand.Intn(len(p.shuffledIndices))
	return p.shuffledIndices[startPos]
}

// SetCurrentIndex 设置当前播放索引
func (p *Player) SetCurrentIndex(index int) {
	p.mu.Lock()
	p.currentIndex = index
	p.mu.Unlock()
}

// generateShuffleOrder 使用 Fisher-Yates 洗牌算法 + 密码学安全随机数生成器生成随机播放序列
// 确保每首歌在一个周期内恰好播放一次
func (p *Player) generateShuffleOrder() {
	if p.currentPlaylist == nil || len(p.currentPlaylist.Items) == 0 {
		return
	}

	size := len(p.currentPlaylist.Items)
	p.shuffledIndices = make([]int, size)
	for i := 0; i < size; i++ {
		p.shuffledIndices[i] = i
	}

	// Fisher-Yates 洗牌算法
	for i := size - 1; i > 0; i-- {
		// 使用密码学安全的随机数生成器
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			// 如果密码学随机数生成失败，回退到普通随机数
			utils.GetLogger().Warn("密码学随机数生成失败，回退到普通随机数: %v", err)
			j := rand.Intn(i + 1)
			p.shuffledIndices[i], p.shuffledIndices[j] = p.shuffledIndices[j], p.shuffledIndices[i]
		} else {
			j := int(n.Int64())
			p.shuffledIndices[i], p.shuffledIndices[j] = p.shuffledIndices[j], p.shuffledIndices[i]
		}
	}

	// 将当前正在播放的歌曲放到洗牌序列的第一个位置
	if p.currentIndex >= 0 && p.currentIndex < size {
		currentIndexInShuffle := -1
		for i, idx := range p.shuffledIndices {
			if idx == p.currentIndex {
				currentIndexInShuffle = i
				break
			}
		}
		if currentIndexInShuffle > 0 {
			p.shuffledIndices[0], p.shuffledIndices[currentIndexInShuffle] = p.shuffledIndices[currentIndexInShuffle], p.shuffledIndices[0]
		}
	}
	p.shufflePosition = 0

	utils.GetLogger().Info("生成洗牌序列，共 %d 首歌曲，当前位置: %d", size, p.shufflePosition)
}

// getShuffleNext 获取随机播放的下一首歌曲索引
func (p *Player) getShuffleNext() int {
	if p.currentPlaylist == nil || len(p.currentPlaylist.Items) == 0 {
		return 0
	}

	// 如果洗牌序列为空或长度不匹配，重新生成
	if p.shuffledIndices == nil || len(p.shuffledIndices) != len(p.currentPlaylist.Items) {
		p.generateShuffleOrder()
	}

	p.shufflePosition++

	// 如果播完一轮，重新洗牌
	if p.shufflePosition >= len(p.shuffledIndices) {
		// 记录最后一首歌曲
		lastPlayed := p.shuffledIndices[len(p.shuffledIndices)-1]

		// 重新洗牌
		p.generateShuffleOrder()

		// 避免首尾衔接重复：如果新洗牌序列的第一首是上一轮的最后一首，则交换
		if len(p.shuffledIndices) > 1 && p.shuffledIndices[0] == lastPlayed {
			// 随机选择一个位置（除了第一个位置）进行交换
			swapPos := 1 + rand.Intn(len(p.shuffledIndices)-1)
			p.shuffledIndices[0], p.shuffledIndices[swapPos] = p.shuffledIndices[swapPos], p.shuffledIndices[0]
		}

		p.shufflePosition = 0
	}

	return p.shuffledIndices[p.shufflePosition]
}

// getShufflePrevious 获取随机播放的上一首歌曲索引
func (p *Player) getShufflePrevious() int {
	if p.currentPlaylist == nil || len(p.currentPlaylist.Items) == 0 {
		return 0
	}

	// 如果洗牌序列为空或长度不匹配，重新生成
	if p.shuffledIndices == nil || len(p.shuffledIndices) != len(p.currentPlaylist.Items) {
		p.generateShuffleOrder()
	}

	// 如果已经是洗牌序列的第一首，则不变
	if p.shufflePosition > 0 {
		p.shufflePosition--
	}

	return p.shuffledIndices[p.shufflePosition]
}

