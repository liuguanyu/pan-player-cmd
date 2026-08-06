package tui

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/skip2/go-qrcode"
)

// renderShimmerText 生成带流光效果的文本
// frame: 当前动画帧，用于推进颜色渐变波峰
// spinnerPrefix: 是否在文本前加前导动态旋转字符
func renderShimmerText(text string, frame int, spinnerPrefix bool) string {
	// 前导旋转字符（仿 Claude CLI 效果）
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	// 基础颜色（蓝色系）
	baseColor := "#1E6FD9"
	// 流光波峰（从暗到亮再到暗）
	shimmerWave := []string{
		"#1E6FD9", "#2A86F0", "#45A3FF", "#66B8FF",
		"#8CCBFF", // 波峰中心（最亮）
		"#66B8FF", "#45A3FF", "#2A86F0", "#1E6FD9",
	}

	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}

	// 设定整个循环的周期长度，例如 40 个字符跨度或者文本长度加上波长
	// 这样光束划过文本后会有一段暗色的间隔再出现下一次光束
	cycleLen := n + len(shimmerWave)
	if cycleLen < 30 {
		cycleLen = 30 // 保证一个合理的最小循环周期
	}

	var styledText strings.Builder
	for i, ch := range runes {
		// 计算当前字符在光束循环中的位置
		// 随着 frame 增加，波浪向右移动
		pos := ((i-frame)%cycleLen + cycleLen) % cycleLen

		color := baseColor
		// 如果落在了波峰的范围内
		if pos < len(shimmerWave) {
			color = shimmerWave[pos]
		}

		charStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
		styledText.WriteString(charStyle.Render(string(ch)))
	}

	if spinnerPrefix {
		// 让 spinner 慢一点，frame / 2
		spinnerIdx := ((frame/2)%len(spinnerChars) + len(spinnerChars)) % len(spinnerChars)
		spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#45A3FF")) // 使用中间蓝色
		return spinnerStyle.Render(spinnerChars[spinnerIdx]) + " " + styledText.String()
	}
	return styledText.String()
}

// sanitizeTitle 清理标题文本：去除换行与控制字符，并限制长度，避免破坏终端标题
func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		// 仅保留可打印字符（排除控制字符 0x00-0x1F 与 0x7F），
		// 换行/制表符替换为空格
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7F:
			// 丢弃其他控制字符
		default:
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())

	// 限制长度（按 rune），避免过长的标题
	const maxLen = 80
	runes := []rune(result)
	if len(runes) > maxLen {
		result = string(runes[:maxLen]) + "…"
	}
	return result
}

// max 返回两个整数中的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min 返回两个整数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatTime 格式化时间
func formatTime(seconds float64) string {
	minutes := int(seconds / 60)
	secs := int(seconds) % 60
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// formatFileSize 格式化文件大小
func formatFileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/1024/1024)
	} else {
		return fmt.Sprintf("%.1fGB", float64(bytes)/1024/1024/1024)
	}
}

// generateQRCode 生成二维码 ASCII 字符串
func generateQRCode(content string) string {
	// 生成二维码图片
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "二维码生成失败"
	}

	// 转换为小尺寸 ASCII 字符串，去除边框避免错位
	qr.DisableBorder = true
	qrStr := qr.ToSmallString(false)

	// 确保每行都有回车符，避免显示错位
	lines := strings.Split(qrStr, "\n")
	var result strings.Builder
	for i, line := range lines {
		if line == "" {
			continue
		}
		result.WriteString(line)
		// 如果不是最后一行，添加回车换行
		if i < len(lines)-1 {
			result.WriteString("\r\n")
		}
	}

	return result.String()
}

// formatSpeed 格式化播放倍速
func formatSpeed(speed float64) string {
	if speed == float64(int(speed)) {
		return fmt.Sprintf("%.0fx", speed)
	}
	return fmt.Sprintf("%.2gx", speed)
}

// extractSongName 从文件名中提取歌曲名（去掉扩展名、前导数字和空格）
func extractSongName(filename string) string {
	// 去掉扩展名（使用 path 包处理 Unix 风格路径，兼容 Windows）
	ext := path.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	// 去掉前导数字、点、空格和破折号
	re := regexp.MustCompile(`^[\d\s\.\-\:、]+`)
	name = re.ReplaceAllString(name, "")

	// 去掉前后空格
	name = strings.TrimSpace(name)

	return name
}

// showMessage 显示消息
func (a *App) showMessage(msg string) {
	a.currentMessage = msg
	a.messageTimeout = time.Now().Add(3 * time.Second)
}

// playbackSpeeds 可选的播放倍速列表
var playbackSpeeds = []float64{0.75, 1, 1.25, 1.5, 1.75, 2, 3, 5, 10}
