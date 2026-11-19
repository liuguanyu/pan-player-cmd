package player

import (
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/speaker"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// PlayerCore 是播放器的执行引擎，只负责播放控制，不处理数据加载
type PlayerCore struct {
	ctrl       *beep.Ctrl
	volume     *effects.Volume
	speeder    speedStreamer // 速度控制层（保持音高不变的时间拉伸），包装原始音频流
	streamer   beep.StreamSeekCloser
	format     beep.Format
	isPlaying  bool
	speed      float64 // 当前播放倍速
	onTrackEnd func()
	tapWrapper func(beep.Streamer) beep.Streamer // optional audio tap for visualization
}

// speedStreamer 是支持动态变速的流器接口，wsolaStreamer 和 beep.Resampler 都满足它。
type speedStreamer interface {
	beep.Streamer
	SetSpeed(speed float64)
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

// SetSpeed 设置播放倍速，返回是否实际发生了变更
func (pc *PlayerCore) SetSpeed(speed float64) bool {
	if pc.speed == speed {
		return false
	}
	pc.speed = speed

	if pc.volume == nil {
		return true // 还没有音频流，SetStream 会按 pc.speed 构建链路
	}

	speaker.Lock()
	defer speaker.Unlock()
	if speed == 1.0 {
		// 1x 时旁路速度层，音量直连原始流（零额外延迟/CPU）
		pc.volume.Streamer = beep.Streamer(pc.streamer)
		pc.speeder = nil
		return true
	}
	if pc.speeder == nil {
		total := 0
		if pc.streamer != nil {
			total = pc.streamer.Len()
		}
		pc.speeder = newCascadeSpeedStreamer(beep.Streamer(pc.streamer), total, speed)
	} else {
		pc.speeder.SetSpeed(speed)
	}
	pc.volume.Streamer = beep.Streamer(pc.speeder)
	return true
}

// GetSpeed 获取当前倍速
func (pc *PlayerCore) GetSpeed() float64 {
	return pc.speed
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

	// 构建速度控制层（WSOLA 时间拉伸：变速不变调）。
	// 1x 时旁路速度层，减少额外延迟与 CPU 开销。
	var streamerForVolume beep.Streamer = streamer
	if pc.speed != 1.0 {
		pc.speeder = newCascadeSpeedStreamer(beep.Streamer(streamer), streamer.Len(), pc.speed)
		streamerForVolume = beep.Streamer(pc.speeder)
	} else {
		pc.speeder = nil
	}

	pc.volume = &effects.Volume{
		Streamer: streamerForVolume,
		Base:     2,
		Volume:   0,
		Silent:   false,
	}

	// 确定送入 ctrl 的流：如果有 tapWrapper 则包裹一层（tap 位于音量之后，
	// 可视化看到的是最终要播放的声音）
	streamerForCtrl := beep.Streamer(pc.volume)
	if pc.tapWrapper != nil {
		streamerForCtrl = pc.tapWrapper(streamerForCtrl)
	}

	pc.ctrl = &beep.Ctrl{
		Streamer: streamerForCtrl,
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

// SetTapWrapper sets an optional audio tap wrapper for visualization.
func (pc *PlayerCore) SetTapWrapper(wrapper func(beep.Streamer) beep.Streamer) {
	pc.tapWrapper = wrapper
}
