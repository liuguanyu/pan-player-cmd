package player

import (
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/speaker"
	"github.com/liuguanyu/pan-player-cmd/internal/analyzer"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// PlayerCore 是播放器的执行引擎，只负责播放控制，不处理数据加载
type PlayerCore struct {
	ctrl       *beep.Ctrl
	volume     *effects.Volume
	streamer   beep.StreamSeekCloser
	format     beep.Format
	isPlaying  bool
	onTrackEnd func()

	// 实时音频分析器
	analyzer *analyzer.RealTimeAnalyzer
	cache    *analyzer.AudioFeatureCache

	// 当前播放音频的 fsID（用于缓存访问）
	currentFsID int64
}

// NewPlayerCore 创建播放器核心
func NewPlayerCore(cache *analyzer.AudioFeatureCache) *PlayerCore {
	return &PlayerCore{
		cache: cache,
	}
}

// SetOnTrackEnd 设置播放结束时的回调函数
func (pc *PlayerCore) SetOnTrackEnd(callback func()) {
	pc.onTrackEnd = callback
}

// Play 开始播放
func (pc *PlayerCore) Play() {
	if pc.ctrl == nil {
		utils.GetLogger().Error("Play() 调用失败: pc.ctrl 为 nil")
		return
	}
	if pc.isPlaying {
		utils.GetLogger().Info("Play() 跳过: 已经在播放中")
		return
	}

	utils.GetLogger().Info("准备开始播放...")
	speaker.Lock()
	pc.ctrl.Paused = false
	speaker.Unlock()

	pc.isPlaying = true
	utils.GetLogger().Info("Play() 完成")
}

// Pause 暂停播放
func (pc *PlayerCore) Pause() {
	if pc.ctrl == nil || !pc.isPlaying {
		return
	}

	speaker.Lock()
	pc.ctrl.Paused = true
	speaker.Unlock()

	pc.isPlaying = false
}

// Stop 停止播放并清理资源
func (pc *PlayerCore) Stop() {
	if pc.ctrl != nil {
		speaker.Lock()
		pc.ctrl.Paused = true
		speaker.Unlock()
	}

	if pc.streamer != nil {
		pc.streamer.Close()
		pc.streamer = nil
		pc.ctrl = nil
	}

	pc.volume = nil
	pc.isPlaying = false

	// 停止分析器
	if pc.analyzer != nil {
		pc.analyzer.Stop()
		pc.analyzer = nil
	}
}

// Seek 跳转到指定位置（秒）
func (pc *PlayerCore) Seek(pos float64) {
	if pc.streamer == nil {
		return
	}

	sample := pc.format.SampleRate.N(time.Duration(pos * float64(time.Second)))
	speaker.Lock()
	pc.streamer.Seek(sample)
	speaker.Unlock()
}

// SetStream 设置新的音频流
func (pc *PlayerCore) SetStream(streamer beep.StreamSeekCloser, format beep.Format) {
	// 停止并清除之前的音频流
	if pc.ctrl != nil {
		speaker.Lock()
		pc.ctrl.Paused = true
		speaker.Unlock()
	}

	// 如果采样率改变，需要重新初始化 speaker
	if pc.format.SampleRate != format.SampleRate {
		utils.GetLogger().Info("检测到采样率变化: %d -> %d，重新初始化 speaker", pc.format.SampleRate, format.SampleRate)
		if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
			utils.GetLogger().Error("重新初始化 speaker 失败: %v", err)
		} else {
			utils.GetLogger().Info("Speaker 重新初始化成功 (采样率: %d)", format.SampleRate)
		}
	}

	pc.streamer = streamer
	pc.format = format

	pc.volume = &effects.Volume{
		Streamer: streamer,
		Base:     2,
		Volume:   0,
		Silent:   false,
	}

	pc.ctrl = &beep.Ctrl{
		Streamer: pc.volume,
		Paused:   true, // 初始设置为暂停状态，Play() 会取消暂停
	}

	// 捕获回调以避免闭包问题
	onTrackEnd := pc.onTrackEnd

	// 关键：将音频流发送到 speaker！使用 beep.Seq 来检测播放结束
	if onTrackEnd != nil {
		speaker.Play(beep.Seq(pc.ctrl, beep.Callback(func() {
			utils.GetLogger().Info("音频流处理完毕，检查是否需要触发自动切歌...")
			// 如果仍是 isPlaying 状态，说明是自然结束的，需要自动切歌
			// (手动 Stop 或 Pause 会改变 isPlaying 状态)
			if pc.isPlaying {
				utils.GetLogger().Info("音频自然播放结束，触发 onTrackEnd 回调")
				// 异步调用以避免在 speaker 的锁或回调中造成死锁
				go onTrackEnd()
			} else {
				utils.GetLogger().Info("音频被手动停止或替换，跳过自动切歌")
			}
		})))
	} else {
		speaker.Play(pc.ctrl)
	}

	pc.isPlaying = false
	utils.GetLogger().Info("SetStream 完成，音频流已发送到 speaker (采样率: %d)", format.SampleRate)

	// 启动实时分析器
	if pc.analyzer != nil {
		pc.analyzer.Stop()
	}
	pc.analyzer = analyzer.NewRealTimeAnalyzer(streamer, format, pc.cache, pc.currentFsID)
	pc.analyzer.Start()
}

// IsPlaying 返回是否正在播放
func (pc *PlayerCore) IsPlaying() bool {
	return pc.isPlaying
}

// GetCurrentPosition 获取当前播放位置（秒）
func (pc *PlayerCore) GetCurrentPosition() float64 {
	if pc.streamer == nil {
		return 0
	}
	return pc.format.SampleRate.D(pc.streamer.Position()).Seconds()
}

// GetDynamicDuration 动态获取音频时长（秒）
// 对于 M4A 流式播放，时长是异步解析的，每次调用都会返回最新值
func (pc *PlayerCore) GetDynamicDuration() float64 {
	if pc.streamer == nil || pc.format.SampleRate == 0 {
		return 0
	}
	// 通过 Len() 获取总采样点数，再转换为时长
	totalSamples := pc.streamer.Len()
	if totalSamples <= 0 {
		return 0
	}
	return pc.format.SampleRate.D(totalSamples).Seconds()
}

// GetFormat 返回音频格式
func (pc *PlayerCore) GetFormat() beep.Format {
	return pc.format
}

// SetVolume 设置音量 (0.0-1.0)
func (pc *PlayerCore) SetVolume(volume float64) {
	if pc.volume == nil {
		return
	}

	// 将 0-1 范围转换为 -5 到 0 的音量值
	// volume = 0 时，Volume = -5（静音）
	// volume = 1 时，Volume = 0（正常音量）
	vol := -5 + volume*5

	speaker.Lock()
	pc.volume.Volume = vol
	speaker.Unlock()
}

// GetIsPlaying 返回是否正在播放（用于状态更新）
func (pc *PlayerCore) GetIsPlaying() bool {
	return pc.isPlaying
}

// GetRealTimeFeatures 获取实时特征通道
func (pc *PlayerCore) GetRealTimeFeatures() <-chan models.RealtimeFeatures {
	if pc.analyzer == nil {
		return nil
	}
	return pc.analyzer.GetFeatureChan()
}
