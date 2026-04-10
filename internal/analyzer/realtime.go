package analyzer

import (
	"math"
	"time"

	"github.com/gopxl/beep"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// RealTimeAnalyzer 实时音频特征分析器
type RealTimeAnalyzer struct {
	streamer    beep.StreamSeekCloser
	format      beep.Format
	sampleRate  beep.SampleRate

	// 实时特征通道
	featureChan chan models.RealtimeFeatures

	// 控制通道
	stopChan    chan struct{}
	isRunning   bool

	// 缓存引用
	cache *AudioFeatureCache
}

// NewRealTimeAnalyzer 创建实时分析器
func NewRealTimeAnalyzer(streamer beep.StreamSeekCloser, format beep.Format, cache *AudioFeatureCache) *RealTimeAnalyzer {
	return &RealTimeAnalyzer{
		streamer:    streamer,
		format:      format,
		sampleRate:  format.SampleRate,
		featureChan: make(chan models.RealtimeFeatures, 10),
		stopChan:    make(chan struct{}),
		cache:       cache,
	}
}

// Start 启动实时分析（goroutine）
func (rta *RealTimeAnalyzer) Start() {
	if rta.isRunning {
		return
	}
	rta.isRunning = true

	go rta.analyzeLoop()
}

// Stop 停止分析
func (rta *RealTimeAnalyzer) Stop() {
	if !rta.isRunning {
		return
	}
	close(rta.stopChan)
	rta.isRunning = false
}

// analyzeLoop 分析循环（后台运行）
func (rta *RealTimeAnalyzer) analyzeLoop() {
	// 每秒更新一次
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rta.stopChan:
			return

		case <-ticker.C:
			// 获取当前播放位置
			currentPos := rta.format.SampleRate.D(rta.streamer.Position()).Seconds()

			// 读取当前窗口的音频数据（2秒窗口）
			samples := rta.readCurrentWindow(currentPos, 2.0)
			if len(samples) == 0 {
				continue
			}

			// 实时特征（简单版本）
			features := rta.analyzeFeatures(samples, currentPos)

			// 发送结果
			select {
			case rta.featureChan <- features:
			default:
				// channel 满了，丢弃
			}
		}
	}
}

// readCurrentWindow 读取当前窗口的音频样本
func (rta *RealTimeAnalyzer) readCurrentWindow(currentPos, windowSize float64) []float64 {
	windowSamples := int(rta.sampleRate.N(time.Duration(windowSize * float64(time.Second))))
	if windowSamples <= 0 {
		return nil
	}

	stereo := make([][2]float64, windowSamples)
	savedPos := rta.streamer.Position()

	startPos := rta.sampleRate.N(time.Duration(currentPos * float64(time.Second)))
	if startPos < 0 {
		startPos = 0
	}

	rta.streamer.Seek(startPos)
	n, ok := rta.streamer.Stream(stereo)
	if !ok || n == 0 {
		rta.streamer.Seek(savedPos)
		return nil
	}

	rta.streamer.Seek(savedPos)

	// 转为单声道
	mono := make([]float64, n)
	for i := 0; i < n; i++ {
		mono[i] = (stereo[i][0] + stereo[i][1]) / 2.0
	}

	return mono
}

// analyzeFeatures 分析音频特征
func (rta *RealTimeAnalyzer) analyzeFeatures(samples []float64, currentPos float64) models.RealtimeFeatures {
	// 简化版特征分析（基于能量和频谱）

	// 1. 计算能量
	energy := rta.calculateEnergy(samples)

	// 2. 检测人声（基于频谱分布）
	vocal := rta.detectVocal(samples)

	// 3. 性别预测（基于基频估计）
	gender := rta.predictGender(samples, vocal)

	// 4. 乐器识别（基于频谱模式）
	instrument := rta.classifyInstrument(samples)

	// 5. 和声复杂度（基于谐波）
	harmony := rta.calculateHarmony(samples)

	// 6. 段落类型（基于当前时间）
	section := rta.getCurrentSection(currentPos)

	return models.RealtimeFeatures{
		HasVocal:       vocal,
		Gender:         gender,
		DominantInstr:  instrument,
		HarmonyLevel:   harmony,
		EnergyLevel:    energy,
		CurrentSection: section,
		Timestamp:      currentPos,
	}
}

// calculateEnergy 计算音频能量（0-1）
func (rta *RealTimeAnalyzer) calculateEnergy(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sum float64
	for _, s := range samples {
		sum += s * s
	}

	avg := sum / float64(len(samples))
	// 将能量映射到 0-1 范围
	return math.Min(1.0, avg*10)
}

// detectVocal 检测人声
func (rta *RealTimeAnalyzer) detectVocal(samples []float64) bool {
	if len(samples) < 100 {
		return false
	}

	// 采样点的绝对值标准差作为代表
	var sum, sumSquares float64
	for _, s := range samples {
		sum += math.Abs(s)
		sumSquares += s * s
	}

	mean := sum / float64(len(samples))
	variance := sumSquares/float64(len(samples)) - mean*mean
	stdDev := math.Sqrt(math.Max(0, variance))

	// 如果标准差大，表示有动态变化（人声）
	return stdDev > 0.1
}

// predictGender 预测性别
func (rta *RealTimeAnalyzer) predictGender(samples []float64, hasVocal bool) string {
	if !hasVocal {
		return models.GenderNeutral
	}

	// 简化：基于能量分布
	var lowFreq, highFreq float64
	for i, s := range samples {
		if i%2 == 0 { // 模拟低频成分
			lowFreq += math.Abs(s)
		}
		if i%3 == 0 { // 模拟高频成分
			highFreq += math.Abs(s)
		}
	}

	if lowFreq > highFreq*1.5 {
		return models.GenderMale
	}
	if highFreq > lowFreq*1.5 {
		return models.GenderFemale
	}
	return models.GenderNeutral
}

// classifyInstrument 乐器识别
func (rta *RealTimeAnalyzer) classifyInstrument(samples []float64) string {
	if len(samples) == 0 {
		return "unknown"
	}

	// 简化：基于能量分布
	var low, mid, high float64
	for i, s := range samples {
		if i%4 == 0 { // 低频
			low += math.Abs(s)
		} else if i%3 == 0 { // 中频
			mid += math.Abs(s)
		} else { // 高频
			high += math.Abs(s)
		}
	}

	// 基于相对能量判断
	if low > mid*1.5 && high < mid*0.3 {
		return "bass"
	}
	if mid > low*1.5 && mid > high*2 {
		return "vocal"
	}
	if high > mid*1.5 {
		return "piano"
	}
	if low > high*2 && high > mid*0.5 {
		return "drums"
	}
	return "other"
}

// calculateHarmony 计算和声复杂度
func (rta *RealTimeAnalyzer) calculateHarmony(samples []float64) float64 {
	if len(samples) < 100 {
		return 0
	}

	// 简化：如果能量变化大，表示有和声
	var prev float64
	var changes int
	for _, s := range samples {
		if prev > 0 && math.Abs(s-prev) > 0.1 {
			changes++
		}
		prev = s
	}

	return math.Min(1.0, float64(changes)/float64(len(samples))*2)
}

// getCurrentSection 获取当前段落
func (rta *RealTimeAnalyzer) getCurrentSection(currentPos float64) string {
	// 基于时间的简单分段
	if currentPos < 15 {
		return models.SectionIntro
	} else if currentPos < 60 {
		return models.SectionVerse
	} else if currentPos < 120 {
		return models.SectionChorus
	} else if currentPos < 180 {
		return models.SectionBridge
	} else {
		return models.SectionOutro
	}
}

// GetFeatureChan 获取特征通道
func (rta *RealTimeAnalyzer) GetFeatureChan() <-chan models.RealtimeFeatures {
	return rta.featureChan
}

// GetFinalFeatures 获取最终特征（用于缓存）
func (rta *RealTimeAnalyzer) GetFinalFeatures() *models.AudioFeatures {
	// 在这里实现完整的特征分析（需要更多处理）
	return &models.AudioFeatures{}
}
