package tui

import (
	"fmt"
	"strings"

	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/visualizer"
)

// renderPlayerView 渲染播放器视图
func (a *App) renderPlayerView() string {
	var b strings.Builder

	state := a.player.GetState()

	// 播放列表信息
	if state.CurrentPlaylistName != "" {
		playlistStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#06BF54"))
		b.WriteString(playlistStyle.Render(fmt.Sprintf("正在播放: %s", state.CurrentPlaylistName)))
		b.WriteString("\n\n")
	}

	// 显示播放列表内容（当前播放项附近5首）
	playlist := a.player.GetCurrentPlaylist()
	if playlist != nil && len(playlist.Items) > 0 {
		currentIndex := a.player.GetCurrentIndex()
		startIndex := max(0, currentIndex-2)
		endIndex := min(len(playlist.Items), currentIndex+3)

		listStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
		currentStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06BF54")).
			Bold(true)

		for i := startIndex; i < endIndex; i++ {
			item := playlist.Items[i]
			fileName := item.ServerFileName
			fileSize := fmt.Sprintf("%.1fMB", float64(item.Size)/1024/1024)

			if i == currentIndex {
				b.WriteString(currentStyle.Render(fmt.Sprintf("→ %d. %s (%s)", i+1, fileName, fileSize)))
			} else {
				b.WriteString(listStyle.Render(fmt.Sprintf("  %d. %s (%s)", i+1, fileName, fileSize)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// 播放模式（放在歌名上面）
	if state.CurrentSong != nil {
		modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
		var modeText string
		switch state.PlaybackMode {
		case models.PlaybackModeOrder:
			modeText = "顺序播放"
		case models.PlaybackModeRandom:
			modeText = "随机播放"
		case models.PlaybackModeSingle:
			modeText = "单曲循环"
		}
		b.WriteString(modeStyle.Render(fmt.Sprintf("模式: %s", modeText)))
		b.WriteString("\n")

		// 网盘文件路径（虚拟目录）
		pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
		if state.CurrentSong.Path != "" {
			b.WriteString(pathStyle.Render(fmt.Sprintf("路径: %s", state.CurrentSong.Path)))
			b.WriteString("\n")
		}

		// 当前歌曲信息
		songText := fmt.Sprintf("正在播放: %s", state.CurrentSong.ServerFileName)
		b.WriteString(renderShimmerText(songText, a.shimmerFrame, false))
		b.WriteString("\n")
	}

	// 进度条（单行，包含播放状态、进度、时间、音量）
	progressBar := a.renderProgressBar(state)
	b.WriteString(progressBar)
	b.WriteString("\n\n")

	// 歌词显示（当前行高亮，显示前后2行）
	if state.ShowLyrics && len(a.currentLyrics) > 0 {
		lyricsView := a.renderLyrics(state.CurrentTime)
		b.WriteString(lyricsView)
		b.WriteString("\n")
	}

	// 可视化（小羊跳舞）— Sixel 图形由后台 goroutine 直接写入 os.Stdout，
	// 绕过 Bubble Tea 渲染管线以避免其行内 diff 破坏 Sixel DCS 序列。
	// 这里只预留空白行，并把当前逻辑行号传给 visualizer，避免用终端底部
	// 绝对定位导致页面切换后旧图残留在非播放页中间。
	if a.sheepVis.Enabled() {
		a.sheepVis.SetRenderRow(strings.Count(b.String(), "\n") + 1)
		for i := 0; i < visualizer.DefaultVisualizerRows; i++ {
			b.WriteString("\n")
		}
	}

	// 消息显示（在快捷键上方）
	if a.currentMessage != "" && time.Now().Before(a.messageTimeout) {
		messageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#06BF54")).Bold(true).Padding(0, 2)
		b.WriteString(messageStyle.Render(a.currentMessage))
		b.WriteString("\n\n")
	}

	// 播放控制提示
	b.WriteString("\n")
	controlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA"))

	// 动态构建快捷键提示 - 只在有歌词时显示 'u' 键
	var shortcutString string
	if state.ShowLyrics && len(a.currentLyrics) > 0 {
		shortcutString = "[空格]暂停/恢复  [n]下一曲  [p]上一曲  [↑/↓]音量  [m]模式  [>]倍速  [l]歌词  [v]可视化  [s]搜索歌词  [u]上传歌词  [Ctrl+Z]后台挂起  [Esc]返回"
	} else {
		shortcutString = "[空格]暂停/恢复  [n]下一曲  [p]上一曲  [↑/↓]音量  [m]模式  [>]倍速  [l]歌词  [v]可视化  [s]搜索歌词  [Ctrl+Z]后台挂起  [Esc]返回"
	}
	b.WriteString(controlStyle.Render(shortcutString))

	return b.String()
}

// renderProgressBar 渲染进度条（单行，包含状态、进度、时间、音量）
func (a *App) renderProgressBar(state *models.PlaybackState) string {
	current := state.CurrentTime
	duration := state.Duration

	// 播放状态文字
	var statusText string
	if state.IsPlaying {
		statusText = "播放中"
	} else {
		statusText = "暂停中"
	}

	// 进度条
	var bar string
	if duration <= 0 {
		bar = strings.Repeat("░", 30)
		speedStr := formatSpeed(state.PlaybackRate)
		return fmt.Sprintf("%s [%s] 00:00 / 00:00 | 音量: %d%% | 倍速: %s",
			statusText, bar, int(state.Volume*100), speedStr)
	}

	// 确保百分比不超过 1
	percentage := current / duration
	if percentage > 1.0 {
		percentage = 1.0
	}
	if percentage < 0 {
		percentage = 0
	}

	// 进度条宽度（固定30个字符宽度）
	barWidth := 30
	filled := int(percentage * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	if empty < 0 {
		empty = 0
	}

	// 使用░字符，已播放部分用绿色，未播放用灰色
	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	remainingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#333"))

	bar = progressStyle.Render(strings.Repeat("░", filled)) + remainingStyle.Render(strings.Repeat("░", empty))

	currentTime := formatTime(current)
	totalTime := formatTime(duration)
	volume := int(state.Volume * 100)

	speedStr := formatSpeed(state.PlaybackRate)
	return fmt.Sprintf("%s [%s] %s / %s | 音量: %d%% | 倍速: %s",
		statusText, bar, currentTime, totalTime, volume, speedStr)
}

// renderControls 渲染播放控制
func (a *App) renderControls(state *models.PlaybackState) string {
	var b strings.Builder

	playIcon := "▶"
	if state.IsPlaying {
		playIcon = "⏸"
	}

	volume := int(state.Volume * 100)
	mode := string(state.PlaybackMode)

	controlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(controlStyle.Render(fmt.Sprintf(" %s 上一首 下一首 | 音量: %d%% | 模式: %s ",
		playIcon, volume, mode)))

	return b.String()
}

// renderLyrics 渲染歌词
func (a *App) renderLyrics(currentTime float64) string {
	var b strings.Builder

	// 如果没有歌词，直接返回
	if a.currentLyrics == nil || len(a.currentLyrics) == 0 {
		return ""
	}

	// 获取当前歌词行
	currentIndex := lyrics.GetCurrentLyricIndex(a.currentLyrics, currentTime)

	// 如果没有找到有效歌词行，显示空
	if currentIndex < 0 {
		return ""
	}

	// 显示当前行及前后几行
	visibleLines := 5
	startIndex := currentIndex - visibleLines/2
	if startIndex < 0 {
		startIndex = 0
	}

	endIndex := startIndex + visibleLines
	if endIndex > len(a.currentLyrics) {
		endIndex = len(a.currentLyrics)
	}

	// 确保 startIndex 在有效范围内
	if startIndex >= len(a.currentLyrics) {
		return ""
	}

	lyricsStyle := lipgloss.NewStyle().Padding(0, 2)

	for i := startIndex; i < endIndex; i++ {
		// 安全检查索引
		if i < 0 || i >= len(a.currentLyrics) {
			continue
		}
		line := a.currentLyrics[i]
		text := line.Text
		if text == "" {
			text = "..."
		}

		if i == currentIndex {
			b.WriteString(renderShimmerText("→ "+text, a.shimmerFrame, false))
		} else {
			b.WriteString(lyricsStyle.Render("  " + text))
		}
		b.WriteString("\n")
	}

	return b.String()
}
