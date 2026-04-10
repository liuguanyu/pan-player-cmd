package analyzer

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/wav"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/vorbis"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// AudioFormat 音频格式枚举
type AudioFormat int

const (
	FormatUnknown AudioFormat = iota
	FormatMP3
	FormatWAV
	FormatFLAC
	FormatOGG
	FormatM4A
	FormatAAC
)

// String 返回格式的字符串表示
func (af AudioFormat) String() string {
	switch {
	case af == FormatMP3:
		return "MP3"
	case af == FormatWAV:
		return "WAV"
	case af == FormatFLAC:
		return "FLAC"
	case af == FormatOGG:
		return "OGG"
	case af == FormatM4A:
		return "M4A"
	case af == FormatAAC:
		return "AAC"
	default:
		return "Unknown"
	}
}

// DetectFormatByFilename 通过文件扩展名检测格式
func DetectFormatByFilename(filename string) AudioFormat {
	switch {
	case len(filename) >= 4 && filename[len(filename)-4:] == ".mp3":
		return FormatMP3
	case len(filename) >= 4 && filename[len(filename)-4:] == ".wav":
		return FormatWAV
	case len(filename) >= 5 && filename[len(filename)-5:] == ".flac":
		return FormatFLAC
	case len(filename) >= 4 && filename[len(filename)-4:] == ".ogg":
		return FormatOGG
	case len(filename) >= 4 && filename[len(filename)-4:] == ".m4a":
		return FormatM4A
	case len(filename) >= 4 && filename[len(filename)-4:] == ".aac":
		return FormatAAC
	}
	return FormatUnknown
}

// DetectFormatByMagic 通过文件魔数检测格式
func DetectFormatByMagic(r io.ReadSeeker) (AudioFormat, error) {
	header := make([]byte, 12)
	n, err := r.Read(header)
	if err != nil {
		return FormatUnknown, err
	}
	if n < 4 {
		return FormatUnknown, fmt.Errorf("file too small")
	}

	// 回到文件开头
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return FormatUnknown, err
	}

	// ID3v2 标记 (MP3)
	if string(header[0:3]) == "ID3" {
		return FormatMP3, nil
	}

	// FLAC 标记
	if string(header[0:4]) == "fLaC" {
		return FormatFLAC, nil
	}

	// OGG 标记
	if string(header[0:4]) == "OggS" {
		return FormatOGG, nil
	}

	// RIFF 标记 (WAV)
	if string(header[0:4]) == "RIFF" && n >= 12 {
		fmtStr := string(header[8:12])
		if fmtStr == "WAVE" {
			return FormatWAV, nil
		}
	}

	// MP3 帧头检测 (帧同步字节: 0xFF 0xE0 - 0xFF 0xFF)
	if header[0] == 0xFF && (header[1]&0xE0) == 0xE0 {
		return FormatMP3, nil
	}

	// M4A 标记 (MP4 容器)
	if n >= 12 && string(header[4:8]) == "ftyp" {
		brand := string(header[8:12])
		if brand == "M4A " || brand == "M4B " || brand == "mp42" || brand == "isom" || brand == "M4V " {
			return FormatM4A, nil
		}
	}

	return FormatUnknown, fmt.Errorf("unknown format")
}

// IsSupported 检查格式是否支持
func IsSupported(format AudioFormat) bool {
	switch format {
	case FormatMP3, FormatWAV, FormatFLAC, FormatOGG, FormatM4A, FormatAAC:
		return true
	}
	return false
}

// GetDecoder 根据格式获取解码器
func GetDecoder(format AudioFormat) (Decoder, error) {
	switch format {
	case FormatMP3:
		return &MP3Decoder{}, nil
	case FormatWAV:
		return &WAVDecoder{}, nil
	case FormatFLAC:
		return &FLACDecoder{}, nil
	case FormatOGG:
		return &OGGDecoder{}, nil
	case FormatM4A, FormatAAC:
		return nil, fmt.Errorf("M4A/AAC requires FFmpeg streaming decoder, not standard decoder")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// Decoder 解码器接口
type Decoder interface {
	Decode(io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error)
}

// MP3Decoder MP3 解码器
type MP3Decoder struct{}

func (d *MP3Decoder) Decode(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
	return mp3.Decode(r)
}

// WAVDecoder WAV 解码器
type WAVDecoder struct{}

func (d *WAVDecoder) Decode(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
	return wav.Decode(r)
}

// FLACDecoder FLAC 解码器
type FLACDecoder struct{}

func (d *FLACDecoder) Decode(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
	return flac.Decode(r)
}

// OGGDecoder OGG 解码器
type OGGDecoder struct{}

func (d *OGGDecoder) Decode(r io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
	return vorbis.Decode(r)
}

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

	// 音频文件的 fsID（用于访问音频缓存）
	fsID int64
}

// NewRealTimeAnalyzer 创建实时分析器
func NewRealTimeAnalyzer(streamer beep.StreamSeekCloser, format beep.Format, cache *AudioFeatureCache, fsID int64) *RealTimeAnalyzer {
	return &RealTimeAnalyzer{
		streamer:    streamer,
		format:      format,
		sampleRate:  format.SampleRate,
		featureChan: make(chan models.RealtimeFeatures, 10),
		stopChan:    make(chan struct{}),
		cache:       cache,
		fsID:        fsID,
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
// 使用缓存的音频文件进行分析，避免干扰播放流
func (rta *RealTimeAnalyzer) readCurrentWindow(currentPos, windowSize float64) []float64 {
	windowSamples := int(rta.sampleRate.N(time.Duration(windowSize * float64(time.Second))))
	if windowSamples <= 0 {
		return nil
	}

	// 使用缓存文件进行分析
	// 缓存文件路径：~/.pan-player/cache/<fsID>.audio
	cacheDir := utils.CacheDir()
	cacheFilePath := filepath.Join(cacheDir, fmt.Sprintf("%d.audio", rta.fsID))

	// 检查缓存文件是否存在
	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil
	}

	// 打开缓存文件
	file, err := os.Open(cacheFilePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	// 使用 beep 解码器解码缓存文件
	// 这需要知道文件格式，我们通过文件扩展名和魔数检测
	formatType := DetectFormatByFilename(cacheFilePath)
	if formatType == FormatUnknown {
		// 尝试通过魔数检测
		file.Seek(0, io.SeekStart)
		if formatType, err = DetectFormatByMagic(file); err != nil {
			return nil
		}
	}

	// 重置文件指针
	file.Seek(0, io.SeekStart)

	// 使用相应的解码器
	decoder, err := GetDecoder(formatType)
	if err != nil {
		return nil
	}

	streamer, format, err := decoder.Decode(file)
	if err != nil {
		return nil
	}
	defer streamer.Close()

	// 获取当前播放位置对应的样本索引
	currentSample := int(rta.sampleRate.N(time.Duration(currentPos * float64(time.Second))))
	windowStart := currentSample
	windowEnd := windowStart + windowSamples

	// 从流中读取窗口数据
	// beep 使用 [][2]float64 格式，每个元素是左右声道
	samples := make([]float64, 0, windowSamples)
	buf := make([][2]float64, 512) // 缓冲区

	for i := 0; i < windowEnd; {
		// 读取一批样本
		n, ok := streamer.Stream(buf)
		if !ok || n == 0 {
			break
		}

		// 处理这批样本
		for j := 0; j < n && i < windowEnd; j, i = j+1, i+1 {
			// 跳过前面的样本
			if i < windowStart {
				continue
			}

			// 平均左右声道
			sample := (buf[j][0] + buf[j][1]) / 2.0
			samples = append(samples, sample)
		}
	}

	return samples
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
