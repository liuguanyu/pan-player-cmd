package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderLyricSearchView 渲染歌词搜索视图
func (a *App) renderLyricSearchView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(1, 2)

	b.WriteString(titleStyle.Render("🎵 歌词搜索"))
	b.WriteString("\n\n")

	state := a.lastSnapshot
	if state.CurrentSong != nil {
		songStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06BF54")).
			Padding(0, 2)
		songName := extractSongName(state.CurrentSong.ServerFileName)
		b.WriteString(songStyle.Render(fmt.Sprintf("当前歌曲: %s", songName)))
		b.WriteString("\n\n")
	}

	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#333")).
		Padding(0, 2)

	runes := []rune(a.lyricSearchKeyword)
	cursor := a.lyricSearchCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	displayKeyword := string(runes[:cursor]) + "█" + string(runes[cursor:])
	b.WriteString(inputStyle.Render("搜索词: " + displayKeyword))
	b.WriteString("\n\n")

	if a.lyricSearchUI.Editing {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA")).Render("输入搜索词 | Enter 搜索 | Esc 返回播放界面"))
		return b.String()
	}

	if len(a.lyricSearchUI.Results) == 0 {
		noResultsStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Italic(true).
			Padding(0, 2)
		b.WriteString(noResultsStyle.Render("未找到歌词，请调整搜索词"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA")).Render("按 Enter 重新搜索 | Esc 返回播放界面"))
		return b.String()
	}

	resultStyle := lipgloss.NewStyle().Padding(0, 2)
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06BF54")).
		Bold(true).
		Padding(0, 2)

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Padding(0, 2).Render(fmt.Sprintf("找到 %d 个结果:", len(a.lyricSearchUI.Results))))
	b.WriteString("\n\n")

	for i, result := range a.lyricSearchUI.Results {
		var line string
		if result.Artist != "" {
			if i == a.lyricSearchUI.SelectedIndex {
				line = selectedStyle.Render(fmt.Sprintf("→ %s - %s [%s]", result.Title, result.Artist, result.Duration))
			} else {
				line = resultStyle.Render(fmt.Sprintf("  %s - %s [%s]", result.Title, result.Artist, result.Duration))
			}
		} else {
			if i == a.lyricSearchUI.SelectedIndex {
				line = selectedStyle.Render(fmt.Sprintf("→ %s [%s]", result.Title, result.Duration))
			} else {
				line = resultStyle.Render(fmt.Sprintf("  %s [%s]", result.Title, result.Duration))
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA")).Render("↑/↓ 选择 | Enter 确认 | E 编辑搜索词 | Esc 返回"))

	return b.String()
}
