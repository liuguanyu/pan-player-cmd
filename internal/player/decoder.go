package player

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/wav"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/vorbis"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
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
	switch af {
	case FormatMP3:
		return "MP3"
	case FormatWAV:
		return "WAV"
	case FormatFLAC:
		return "FLAC"
	case FormatOGG:
		return "OGG"
	case FormatM4A:
		return "M4A"
	case FormatAAC:
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
	// 读取头部来检测格式
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
	// M4A 文件使用 ISO/IEC 14496-14 (MP4) 容器格式
	// MP4文件结构：[atom size (4字节)] [atom type (4字节)] [brand (4字节)]
	// 对于 ftyp atom: [size] + "ftyp" + [brand]
	// 常见的brand: M4A, M4B, mp42, isom, M4V, etc.
	if n >= 12 && string(header[4:8]) == "ftyp" {
		// 检查品牌标识（位于字节8-12）
		brand := string(header[8:12])
		if brand == "M4A " || brand == "M4B " || brand == "mp42" || brand == "isom" || brand == "M4V " {
			return FormatM4A, nil
		}
	}

	// 注意：AAC 数据流（.aac 文件）没有容器魔数，通常需要其他方式检测

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

// GetDecoder 根据格式获取解码器（仅支持标准格式）
// 注意：M4A/AAC 格式需要 FFmpeg 支持，不能直接用标准解码器
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
		// M4A/AAC 需要用 FFmpeg 转码，不应该走标准解码器路径
		// 返回错误，让调用者使用流式解码路径
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

// AudioDecoder 负责从网络或缓存加载并解码音频，返回结果通过 channel 传递
type AudioDecoder struct {
	apiClient *api.BaiduPanClient
	cacheDir  string
}

// LoadTrack 异步加载音轨，返回 channel，不阻塞调用者
func (ad *AudioDecoder) LoadTrack(ctx context.Context, track *models.PlaylistItem) <-chan result {
	ch := make(chan result, 1)

	go func() {
		defer close(ch)

		downloadURL, err := ad.getDownloadURL(ctx, track)
		if err != nil {
			ch <- result{err: err}
			return
		}

		formatType := DetectFormatByFilename(track.ServerFileName)
		var streamer beep.StreamSeekCloser
		var format beep.Format

		switch {
		case formatType == FormatM4A || formatType == FormatAAC:
			streamer, format, err = ad.loadM4AStraming(ctx, track, downloadURL)
		case formatType == FormatWAV:
			streamer, format, err = ad.loadWAVStreaming(track, downloadURL)
		default:
			streamer, format, err = ad.loadStandardAudio(track, downloadURL, formatType)
		}

		if err != nil {
			ch <- result{err: err}
			return
		}

		ch <- result{streamer: streamer, format: format}
	}()

	return ch
}

// result 是 LoadTrack 的返回结果
type result struct {
	streamer beep.StreamSeekCloser
	format   beep.Format
	err      error
}

// getDownloadURL 获取下载链接
func (ad *AudioDecoder) getDownloadURL(ctx context.Context, track *models.PlaylistItem) (string, error) {
	needNewDlink := track.Dlink == "" || track.DlinkExpiresAt < time.Now().Unix()

	if needNewDlink {
		downloadURL, err := ad.apiClient.GetDownloadLink(ctx, track.FsID)
		if err != nil {
			return "", err
		}
		track.Dlink = downloadURL
		track.DlinkExpiresAt = time.Now().Unix() + 3600 // 1小时后过期
		return downloadURL, nil
	}

	return track.Dlink, nil
}

// getAudioFile 获取音频文件（从缓存或下载）
func (ad *AudioDecoder) getAudioFile(ctx context.Context, fsID int64, downloadURL string) (io.ReadCloser, error) {
	logger := utils.GetLogger()

	// 创建缓存目录
	if err := os.MkdirAll(ad.cacheDir, 0755); err != nil {
		return nil, err
	}

	// 根据格式确定缓存文件名
	cacheFile := filepath.Join(ad.cacheDir, fmt.Sprintf("%d.audio", fsID))

	// 检查缓存文件
	if info, err := os.Stat(cacheFile); err == nil {
		logger.Info("使用缓存文件: %s (大小: %d 字节)", cacheFile, info.Size())
		return os.Open(cacheFile)
	}

	logger.Info("开始下载音频文件: fsID=%d", fsID)

	// 创建缓存文件
	file, err := os.Create(cacheFile)
	if err != nil {
		return nil, err
	}

	// 下载并保存到缓存
	err = ad.apiClient.DownloadFile(ctx, downloadURL, file)
	if err != nil {
		file.Close()
		os.Remove(cacheFile)
		logger.Error("下载失败: %v", err)
		return nil, err
	}

	// 检查下载的文件大小
	if info, err := file.Stat(); err == nil {
		logger.Info("下载完成: %d 字节", info.Size())
	}

	// 关闭文件
	file.Close()

	// 重新打开文件用于读取
	logger.Info("打开缓存文件进行播放: %s", cacheFile)
	return os.Open(cacheFile)
}

// loadStandardAudio 加载标准音频格式（MP3, FLAC, WAV, OGG）
// 注意：M4A/AAC 格式需要 FFmpeg 支持，会尝试使用 FFmpeg 流式解码
func (ad *AudioDecoder) loadStandardAudio(track *models.PlaylistItem, downloadURL string, formatType AudioFormat) (beep.StreamSeekCloser, beep.Format, error) {
	logger := utils.GetLogger()

	audioFile, err := ad.getAudioFile(context.Background(), track.FsID, downloadURL)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("failed to get audio file: %w", err)
	}

	// 检测文件格式
	if formatType == FormatUnknown {
		fileSeeker, ok := audioFile.(io.ReadSeeker)
		if !ok {
			audioFile.Close()
			return nil, beep.Format{}, fmt.Errorf("audio file does not support seeking")
		}
		formatType, err = DetectFormatByMagic(fileSeeker)
		if err != nil {
			audioFile.Close()
			return nil, beep.Format{}, fmt.Errorf("failed to detect audio format: %w", err)
		}
	}

	// 对于 M4A/AAC 格式，统一使用 FFmpeg 流式解码
	if formatType == FormatM4A || formatType == FormatAAC {
		logger.Info("检测到 M4A/AAC 格式，使用 FFmpeg 流式解码")

		// 关闭文件，因为我们要用流式解码
		audioFile.Close()

		// 尝试使用 FFmpeg 流式解码
		decoder := &M4ADecoder{}
		streamer, format, err := decoder.decodeFromURL(downloadURL, "")
		if err == nil {
			// FFmpeg 解码成功
			return streamer, format, nil
		}

		// FFmpeg 解码失败，检查错误原因
		logger.Warn("FFmpeg 流式解码失败: %v", err)

		// 如果是 "ffmpeg not found" 错误，尝试使用系统安装的 ffmpeg
		if strings.Contains(err.Error(), "ffmpeg not found") {
			// 创建一个新的 M4ADecoder 实例，让它自动查找系统 ffmpeg
			decoder = &M4ADecoder{}
			streamer, format, err = decoder.decodeFromURL(downloadURL, "")
			if err == nil {
				return streamer, format, nil
			}
		}

		// 如果仍然失败，返回错误，而不是尝试 MP3 解码器
		// 因为 M4A 容器格式不能用 MP3 解码器直接解码
		return nil, beep.Format{}, fmt.Errorf("failed to decode M4A/AAC: %w. Please install ffmpeg: ./scripts/get-ffmpeg.sh", err)
	}

	// 检查是否支持该格式
	if !IsSupported(formatType) {
		audioFile.Close()
		return nil, beep.Format{}, fmt.Errorf("unsupported audio format: %d", formatType)
	}

	// 解码音频
	decoder, err := GetDecoder(formatType)
	if err != nil {
		audioFile.Close()
		return nil, beep.Format{}, fmt.Errorf("failed to get %d decoder: %w", formatType, err)
	}

	logger.Info("开始解码音频流...")
	streamer, format, err := decoder.Decode(audioFile)
	if err != nil {
		logger.Error("解码失败: %v", err)
		audioFile.Close()
		return nil, beep.Format{}, fmt.Errorf("failed to decode %s: %w", formatType, err)
	}
	logger.Info("解码成功: 采样率=%d, 声道数=%d, 精度=%d", format.SampleRate, format.NumChannels, format.Precision)

	return streamer, format, nil
}

// loadM4AStraming 流式加载 M4A 文件
func (ad *AudioDecoder) loadM4AStraming(ctx context.Context, track *models.PlaylistItem, downloadURL string) (beep.StreamSeekCloser, beep.Format, error) {
	logger := utils.GetLogger()
	logger.Info("开始流式加载 M4A: %s", track.ServerFileName)

	// M4A 文件统一使用 FFmpeg 流式解码
	decoder := &M4ADecoder{}
	streamer, format, err := decoder.DecodeFromURL(downloadURL)
	if err != nil {
		// 流式解码失败
		logger.Error("M4A 流式解码失败: %v", err)
		return nil, beep.Format{}, err
	}

	return streamer, format, nil
}

// loadWAVStreaming 流式加载 WAV 文件
func (ad *AudioDecoder) loadWAVStreaming(track *models.PlaylistItem, downloadURL string) (beep.StreamSeekCloser, beep.Format, error) {
	logger := utils.GetLogger()
	logger.Info("开始流式加载 WAV: %s", track.ServerFileName)

	// 使用WAV流式解码器
	decoder := &WAVStreamingDecoder{}
	streamer, format, err := decoder.DecodeFromURL(downloadURL)
	if err != nil {
		logger.Warn("WAV流式解码失败，降级到标准模式: %v", err)
		// 降级到标准下载模式
		return ad.loadStandardAudio(track, downloadURL, FormatWAV)
	}

	return streamer, format, nil
}