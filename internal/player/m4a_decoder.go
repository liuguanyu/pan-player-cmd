package player

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/gopxl/beep"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// M4ADecoder M4A 解码器（使用 ffmpeg 流式转码）
type M4ADecoder struct{}

// DecodeFromURL 直接从URL解码 M4A 文件（流式播放）
func (d *M4ADecoder) DecodeFromURL(url string) (beep.StreamSeekCloser, beep.Format, error) {
	// 查找 ffmpeg 可执行文件
	ffmpegPath, err := d.findFFmpeg()
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("ffmpeg not found: %w. M4A playback requires ffmpeg. Please run: ./scripts/get-ffmpeg.sh", err)
	}

	// 对于 M4A 文件，统一使用 FFmpeg 转码播放
	// 不需要探测编码类型，因为所有编码（AAC、MP3、ALAC）都需要用 FFmpeg 提取音频流
	utils.GetLogger().Info("M4A文件，使用FFmpeg转码解码")
	return d.decodeFromURL(url, ffmpegPath)
}

// probeAudioCodec 探测M4A文件的音频编码类型
func (d *M4ADecoder) probeAudioCodec(url string, ffmpegPath string) (string, error) {
	// 使用ffprobe探测音频编码
	cmd := exec.Command(ffmpegPath, "-v", "quiet", "-print_format", "json", "-show_entries", "stream=codec_name", "-select_streams", "a:0", url)

	// 捕获 stderr 用于调试
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	output, err := cmd.Output()
	if err != nil {
		// 获取更详细的错误信息
		if exitErr, ok := err.(*exec.ExitError); ok {
			utils.GetLogger().Error("ffprobe失败: exit code=%d, stderr=%s", exitErr.ExitCode(), stderrBuf.String())
		}
		return "", fmt.Errorf("ffprobe failed: %w", err)
	}

	// 解析JSON输出
	var result struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	if len(result.Streams) == 0 {
		return "", fmt.Errorf("no audio stream found")
	}

	codec := result.Streams[0].CodecName
	utils.GetLogger().Info("M4A探测到音频编码: %s", codec)
	return codec, nil
}

// findFFmpeg 查找 ffmpeg 可执行文件
func (d *M4ADecoder) findFFmpeg() (string, error) {
	// 1. 首先检查程序目录下的 third_party/ffmpeg/ffmpeg
	exePath, err := os.Executable()
	if err == nil {
		localFFmpeg := filepath.Join(filepath.Dir(exePath), "third_party", "ffmpeg", "ffmpeg")
		if _, err := os.Stat(localFFmpeg); err == nil {
			return localFFmpeg, nil
		}
	}

	// 2. 检查当前工作目录下的 third_party/ffmpeg/ffmpeg
	localFFmpeg := filepath.Join("third_party", "ffmpeg", "ffmpeg")
	if _, err := os.Stat(localFFmpeg); err == nil {
		return localFFmpeg, nil
	}

	// 3. 查找系统 PATH 中的 ffmpeg
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("ffmpeg executable not found")
}

// decodeFromURL 直接从HTTP URL读取并解码M4A文件
func (d *M4ADecoder) decodeFromURL(url string, ffmpegPath string) (beep.StreamSeekCloser, beep.Format, error) {
	// 如果 ffmpegPath 为空，自动查找
	if ffmpegPath == "" {
		var err error
		ffmpegPath, err = d.findFFmpeg()
		if err != nil {
			return nil, beep.Format{}, fmt.Errorf("ffmpeg not found: %w. M4A playback requires ffmpeg. Please run: ./scripts/get-ffmpeg.sh", err)
		}
	}

	// 使用FFmpeg直接从HTTP URL读取
	// 这样FFmpeg可以处理MP4容器格式的seek操作，同时保持流式播放
	utils.GetLogger().Info("M4A解码：直接从URL读取: %s (使用 ffmpeg: %s)", url, ffmpegPath)

	// ffmpeg 命令：从URL读取，转码为原始 PCM，输出到 stdout
	// -i <url> : 从HTTP URL读取（FFmpeg支持HTTP流）
	// -f s16le : 输出原始 PCM 格式（16-bit signed little-endian）
	// -ar 44100 : 输出采样率 44.1kHz
	// -ac 2 : 立体声输出
	// -map_metadata -1 : 不输出元数据（避免干扰 progress 解析）
	// -progress pipe:2 : 将进度信息输出到 stderr（关键！用于解析时长）
	cmd := exec.Command(ffmpegPath,
		"-i", url,           // 从URL读取
		"-f", "s16le",       // 原始 PCM 格式
		"-ar", "44100",      // 采样率
		"-ac", "2",          // 立体声
		"-map_metadata", "-1", // 不输出元数据
		"-progress", "pipe:2", // 将进度输出到 stderr（关键！）
		"-",                 // 输出到 stdout
	)

	// 获取 stdout pipe（用于读取转码后的 PCM 数据）
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("failed to create ffmpeg stdout pipe: %w", err)
	}

	// 获取 stderr pipe（用于读取 progress 信息）
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, beep.Format{}, fmt.Errorf("failed to create ffmpeg stderr pipe: %w", err)
	}

	// 启动 ffmpeg
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		utils.GetLogger().Error("Failed to start ffmpeg: %v", err)
		return nil, beep.Format{}, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	utils.GetLogger().Info("FFmpeg process started (pid=%d)", cmd.Process.Pid)

	// 创建一个 PCM 流读取器
	pcmReader := &pcmStreamReader{
		reader:     stdout,
		cmd:        cmd,
		input:      nil, // 没有输入流，直接从URL读取
		stdout:     stdout,
		stderr:     stderr,
		buffer:     make([]byte, 8192), // 缓冲区
		sampleRate: beep.SampleRate(44100),
		state:      PCMStateRunning,
		startTime:  startTime,
		pid:        cmd.Process.Pid,
		tempFile:   "", // 没有临时文件
	}

	// 启动后台 goroutine 读取 stderr 并解析 progress 信息
	go pcmReader.readProgressInfo()

	// 返回一个 StreamSeekCloser（注意：流式播放不支持 seek）
	return pcmReader, beep.Format{
		SampleRate:  beep.SampleRate(44100),
		NumChannels: 2,
		Precision:   2, // 16-bit = 2 bytes
	}, nil
}

// WAVStreamingDecoder WAV流式解码器（使用FFmpeg）
type WAVStreamingDecoder struct{}

// DecodeFromURL 从URL流式解码WAV文件
func (d *WAVStreamingDecoder) DecodeFromURL(url string) (beep.StreamSeekCloser, beep.Format, error) {
	// 查找 ffmpeg 可执行文件
	ffmpegPath, err := (&M4ADecoder{}).findFFmpeg()
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("ffmpeg not found: %w", err)
	}

	// 使用FFmpeg直接从HTTP URL读取并转码为PCM
	utils.GetLogger().Info("WAV流式解码：直接从URL读取: %s", url)

	cmd := exec.Command(ffmpegPath,
		"-i", url,           // 从URL读取
		"-f", "s16le",       // 原始 PCM 格式
		"-ar", "44100",      // 采样率
		"-ac", "2",          // 立体声
		"-map_metadata", "-1", // 不输出元数据
		"-progress", "pipe:2", // 将进度输出到 stderr
		"-",                 // 输出到 stdout
	)

	// 获取 stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("failed to create ffmpeg stdout pipe: %w", err)
	}

	// 获取 stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, beep.Format{}, fmt.Errorf("failed to create ffmpeg stderr pipe: %w", err)
	}

	// 启动 ffmpeg
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		utils.GetLogger().Error("Failed to start ffmpeg: %v", err)
		return nil, beep.Format{}, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	utils.GetLogger().Info("FFmpeg process started (pid=%d)", cmd.Process.Pid)

	// 创建一个 PCM 流读取器
	pcmReader := &pcmStreamReader{
		reader:     stdout,
		cmd:        cmd,
		input:      nil,
		stdout:     stdout,
		stderr:     stderr,
		buffer:     make([]byte, 8192),
		sampleRate: beep.SampleRate(44100),
		state:      PCMStateRunning,
		startTime:  startTime,
		pid:        cmd.Process.Pid,
		tempFile:   "",
	}

	// 启动后台 goroutine 读取 stderr 并解析 progress 信息
	go pcmReader.readProgressInfo()

	// 返回一个 StreamSeekCloser
	return pcmReader, beep.Format{
		SampleRate:  beep.SampleRate(44100),
		NumChannels: 2,
		Precision:   2,
	}, nil
}

// PCMStreamState PCM 流读取器的状态
type PCMStreamState int

const (
	PCMStateIdle PCMStreamState = iota
	PCMStateRunning
	PCMStateCompleted
	PCMStateError
)

func (s PCMStreamState) String() string {
	switch s {
	case PCMStateIdle:
		return "idle"
	case PCMStateRunning:
		return "running"
	case PCMStateCompleted:
		return "completed"
	case PCMStateError:
		return "error"
	default:
		return "unknown"
	}
}

// pcmStreamReader PCM 流读取器（实现 beep.StreamSeekCloser）
type pcmStreamReader struct {
	reader      io.Reader
	cmd         *exec.Cmd
	input       io.ReadCloser
	stdout      io.ReadCloser // 保存 stdout pipe 引用
	stderr      io.ReadCloser // 保存 stderr pipe 引用
	buffer      []byte
	sampleRate  beep.SampleRate
	pos         int
	state       PCMStreamState
	err         error
	startTime   time.Time
	pid         int
	closeOnce   sync.Once
	closeMutex  sync.Mutex
	tempFile    string // 临时文件路径（用于M4A临时文件）

	// duration 相关字段
	duration    time.Duration // 解析得到的音频总时长
	durationMux sync.RWMutex  // 保护 duration 的读写
}

// readProgressInfo 在后台 goroutine 中读取 stderr 并解析 progress 信息
func (s *pcmStreamReader) readProgressInfo() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		// 解析 progress 信息
		s.parseProgressInfo(line)
	}

	// 检查扫描错误
	if err := scanner.Err(); err != nil {
		// stderr 读取错误，但不影响主流程
		utils.GetLogger().Debug("FFmpeg stderr scanner error: %v", err)
	}
}

// parseProgressInfo 解析 ffmpeg 的 progress 输出
// 当使用 -progress pipe:2 时，输出格式为：
// out_time_us=12345678
// out_time=00:00:12.345678
// total_size=1234567
// ...
// progress=continue
// progress=end
// 注意：out_time 是当前播放进度，不是总时长！
// 但当无法获取 Duration 时，我们可以从 total_size 和 bitrate 推算总时长
func (s *pcmStreamReader) parseProgressInfo(line string) {
	// 记录原始行用于调试
	utils.GetLogger().Debug("FFmpeg stderr: %s", line)

	// 1. 尝试从 Duration 行获取总时长（最准确）
	durationRe := regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+\.?\d*)`)
	durationMatches := durationRe.FindStringSubmatch(line)
	if len(durationMatches) == 4 {
		hours, err1 := strconv.ParseFloat(durationMatches[1], 64)
		minutes, err2 := strconv.ParseFloat(durationMatches[2], 64)
		seconds, err3 := strconv.ParseFloat(durationMatches[3], 64)

		if err1 == nil && err2 == nil && err3 == nil {
			totalSeconds := hours*3600 + minutes*60 + seconds

			// 验证时长的合理性
			if totalSeconds > 0.1 && totalSeconds < 36000 {
				duration := time.Duration(totalSeconds * float64(time.Second))

				s.durationMux.Lock()
				s.duration = duration
				s.durationMux.Unlock()

				utils.GetLogger().Info("FFmpeg Duration: 总时长=%v", duration)
			}
		}
		return // 找到 Duration 就直接返回，不再解析其他
	}

	// 2. 如果没有 Duration，尝试从 bitrate= 和 total_size= 推算总时长
	// 格式：bitrate=1383.8kbits/s total_size=1863424
	bitrateRe := regexp.MustCompile(`bitrate=(\d+\.?\d*)kbits/s`)
	sizeRe := regexp.MustCompile(`total_size=(\d+)`)

	bitrateMatches := bitrateRe.FindStringSubmatch(line)
	sizeMatches := sizeRe.FindStringSubmatch(line)

	if len(bitrateMatches) == 2 && len(sizeMatches) == 2 {
		// 解析比特率（转换为 bps）
		bitrateKbps, err4 := strconv.ParseFloat(bitrateMatches[1], 64)
		if err4 == nil {
			bitrateBps := bitrateKbps * 1000

			// 解析总大小
			totalSize, err5 := strconv.ParseInt(sizeMatches[1], 10, 64)
			if err5 == nil && totalSize > 0 {
				// 计算总时长：总大小 / 比特率
				estimatedDuration := float64(totalSize) / (bitrateBps / 8) // 转换为秒

				// 验证估计的时长合理性
				if estimatedDuration > 0.1 && estimatedDuration < 36000 {
					duration := time.Duration(estimatedDuration * float64(time.Second))

					s.durationMux.Lock()
					s.duration = duration
					s.durationMux.Unlock()

					utils.GetLogger().Info("FFmpeg 推算总时长: %v (基于 %d 字节 / %.1f kbps)", duration, totalSize, bitrateKbps)
				}
			}
		}
	}

	// 3. 如果以上都失败，不更新时长（保持默认值）
}

// GetDuration 获取当前解析到的音频时长
func (s *pcmStreamReader) GetDuration() time.Duration {
	s.durationMux.RLock()
	defer s.durationMux.RUnlock()
	dur := s.duration
	utils.GetLogger().Debug("GetDuration() called, returning: %v", dur)
	return dur
}

func (s *pcmStreamReader) Stream(samples [][2]float64) (n int, ok bool) {
	s.closeMutex.Lock()
	if s.state != PCMStateRunning || s.err != nil {
		s.closeMutex.Unlock()
		return 0, false
	}
	s.closeMutex.Unlock()

	// 读取 PCM 数据
	// 每个采样点：2 channels * 2 bytes = 4 bytes
	// 需要读取：len(samples) * 4 bytes
	bytesNeeded := len(samples) * 4
	if len(s.buffer) < bytesNeeded {
		s.buffer = make([]byte, bytesNeeded)
	}

	totalRead := 0
	for totalRead < bytesNeeded {
		nBytes, err := s.reader.Read(s.buffer[totalRead:bytesNeeded])
		totalRead += nBytes
		if err != nil {
			if err == io.EOF {
				s.closeMutex.Lock()
				s.state = PCMStateCompleted
				s.closeMutex.Unlock()
			} else {
				s.closeMutex.Lock()
				s.state = PCMStateError
				s.err = fmt.Errorf("stream read error: %w", err)
				s.closeMutex.Unlock()
			}
			break
		}
	}

	// 转换 PCM 数据为 beep 格式
	numSamples := totalRead / 4
	for i := 0; i < numSamples && i < len(samples); i++ {
		// 读取左声道（16-bit signed little-endian）
		left := int16(s.buffer[i*4]) | int16(s.buffer[i*4+1])<<8
		// 读取右声道
		right := int16(s.buffer[i*4+2]) | int16(s.buffer[i*4+3])<<8

		// 转换为 -1.0 到 1.0 的浮点值
		samples[i][0] = float64(left) / 32768.0
		samples[i][1] = float64(right) / 32768.0
	}

	s.pos += numSamples
	return numSamples, numSamples > 0
}

func (s *pcmStreamReader) Err() error {
	return s.err
}

func (s *pcmStreamReader) Len() int {
	// 优先使用解析到的时长计算总采样点数
	s.durationMux.RLock()
	duration := s.duration
	s.durationMux.RUnlock()

	if duration > 0 {
		// 总采样点数 = 时长（秒） * 采样率
		totalSamples := int(float64(duration.Seconds()) * float64(s.sampleRate))
		utils.GetLogger().Debug("Len() returning calculated samples: %d (duration=%v, sampleRate=%d)", totalSamples, duration, s.sampleRate)
		return totalSamples
	}

	// 如果还没有解析到时长，返回一个临时估算值
	// 等待 FFmpeg 解析完成后，下次调用会返回正确值
	// 估算值：假设音频时长为 5 分钟（300 秒）
	estimatedSamples := int(300 * float64(s.sampleRate))
	utils.GetLogger().Debug("Len() returning estimated samples: %d (duration not yet available)", estimatedSamples)
	return estimatedSamples
}

func (s *pcmStreamReader) Position() int {
	return s.pos
}

func (s *pcmStreamReader) Seek(pos int) error {
	// 流式播放不支持 seek
	return fmt.Errorf("streaming playback does not support seeking")
}

func (s *pcmStreamReader) Close() error {
	var closeErr error

	s.closeOnce.Do(func() {
		utils.GetLogger().Info("Closing PCM stream reader (pid=%d, state=%s)", s.pid, s.state)

		s.closeMutex.Lock()
		s.state = PCMStateIdle
		s.closeMutex.Unlock()

		// 关闭输入流
		if s.input != nil {
			if err := s.input.Close(); err != nil {
				utils.GetLogger().Warn("Failed to close input stream: %v", err)
				closeErr = fmt.Errorf("failed to close input stream: %w", err)
			}
			s.input = nil
		}

		// 关闭 stdout pipe
		if s.stdout != nil {
			if err := s.stdout.Close(); err != nil {
				utils.GetLogger().Warn("Failed to close stdout pipe: %v", err)
				if closeErr == nil {
					closeErr = fmt.Errorf("failed to close stdout pipe: %w", err)
				}
			}
			s.stdout = nil
		}

		// 关闭 stderr pipe
		if s.stderr != nil {
			if err := s.stderr.Close(); err != nil {
				utils.GetLogger().Warn("Failed to close stderr pipe: %v", err)
				if closeErr == nil {
					closeErr = fmt.Errorf("failed to close stderr pipe: %w", err)
				}
			}
			s.stderr = nil
		}

		// 终止 ffmpeg 进程
		if s.cmd != nil && s.cmd.Process != nil {
			utils.GetLogger().Debug("Killing ffmpeg process (pid=%d)", s.pid)

			// 发送 SIGKILL 信号强制终止
			if err := s.cmd.Process.Kill(); err != nil {
				utils.GetLogger().Error("Failed to kill ffmpeg process (pid=%d): %v", s.pid, err)
				if closeErr == nil {
					closeErr = fmt.Errorf("failed to kill ffmpeg process (pid=%d): %w", s.pid, err)
				}
			}

			// 等待进程真正结束
			if err := s.cmd.Wait(); err != nil {
				// 进程已经被 kill，Wait 返回的错误是预期的，不记录为错误
				// 只记录非预期的等待错误
				if s.state != PCMStateIdle {
					utils.GetLogger().Debug("FFmpeg process wait returned error (pid=%d): %v", s.pid, err)
					if closeErr == nil {
						closeErr = fmt.Errorf("ffmpeg process wait error (pid=%d): %w", s.pid, err)
					}
				}
			}

			utils.GetLogger().Info("FFmpeg process terminated (pid=%d, duration=%v)", s.pid, time.Since(s.startTime))
		}

		// 清理临时文件（M4A专用）
		if s.tempFile != "" {
			if err := os.Remove(s.tempFile); err != nil {
				utils.GetLogger().Warn("Failed to remove temp file %s: %v", s.tempFile, err)
			} else {
				utils.GetLogger().Info("Removed temp file: %s", s.tempFile)
			}
			s.tempFile = ""
		}

		// 清空缓冲区
		s.buffer = nil
		s.reader = nil
		s.cmd = nil
	})

	return closeErr
}

// String 返回当前状态的字符串表示（用于调试）
func (s *pcmStreamReader) String() string {
	s.closeMutex.Lock()
	defer s.closeMutex.Unlock()

	duration := time.Since(s.startTime)
	return fmt.Sprintf("pcmStreamReader{state=%s, pid=%d, pos=%d, duration=%v, err=%v}",
		s.state, s.pid, s.pos, duration, s.err)
}
