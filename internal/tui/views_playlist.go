package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderPlaylistView 渲染播放列表视图
func (a *App) renderPlaylistView() string {
	var b strings.Builder

	// 标题区域
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render("播放列表"))
	b.WriteString("\n\n")

	if len(a.playlists) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Italic(true).
			Padding(0, 2)
		b.WriteString(emptyStyle.Render("暂无播放列表，按 'n' 创建新列表"))
	} else {
		// 列表项样式
		normalStyle := lipgloss.NewStyle().Padding(0, 2)
		selectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			Bold(true)

		// 计算可见区域（预留标题、状态栏、帮助栏的空间）
		visibleHeight := a.height - 6
		if visibleHeight < 5 {
			visibleHeight = 5
		}

		// 计算滚动范围，确保选中项可见
		if a.selectedIndex < a.scrollOffset {
			a.scrollOffset = a.selectedIndex
		} else if a.selectedIndex >= a.scrollOffset+visibleHeight {
			a.scrollOffset = a.selectedIndex - visibleHeight + 1
		}

		maxOffset := len(a.playlists) - visibleHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if a.scrollOffset > maxOffset {
			a.scrollOffset = maxOffset
		}
		if a.scrollOffset < 0 {
			a.scrollOffset = 0
		}

		endIndex := a.scrollOffset + visibleHeight
		if endIndex > len(a.playlists) {
			endIndex = len(a.playlists)
		}

		for i := a.scrollOffset; i < endIndex; i++ {
			pl := a.playlists[i]
			var totalSize int64
			for _, item := range pl.Items {
				totalSize += item.Size
			}
			sizeStr := formatFileSize(totalSize)

			var itemLine string
			if i == a.selectedIndex {
				itemLine = selectedStyle.Render(fmt.Sprintf("→ %s  (%d首 · %s)", pl.Name, len(pl.Items), sizeStr))
			} else {
				itemLine = normalStyle.Render(fmt.Sprintf("  %s  (%d首 · %s)", pl.Name, len(pl.Items), sizeStr))
			}
			b.WriteString(itemLine)
			b.WriteString("\n")
		}

		if len(a.playlists) > visibleHeight {
			b.WriteString("\n")
			scrollHint := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
			if a.scrollOffset > 0 || endIndex < len(a.playlists) {
				b.WriteString(scrollHint.Render(fmt.Sprintf("显示 %d-%d / %d", a.scrollOffset+1, endIndex, len(a.playlists))))
				b.WriteString("\n")
			}
		}
	}

	// 状态栏
	b.WriteString("\n")
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	if len(a.playlists) > 0 {
		b.WriteString(statusStyle.Render(fmt.Sprintf("共 %d 个播放列表", len(a.playlists))))
		b.WriteString("\n")
	}

	// 底部提示
	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	b.WriteString(helpStyle.Render(" ↑↓ 选择  |  Enter 打开  |  n 新建  |  r 改名  |  d 删除  |  R 刷新  |  h 帮助  |  q 退出 "))

	return b.String()
}

// renderInputView 渲染输入视图
func (a *App) renderInputView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render(a.inputPrompt))
	b.WriteString("\n\n")

	inputBox := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#333")).
		Padding(0, 2).
		Render(a.inputBuffer + "█")

	b.WriteString(inputBox)
	b.WriteString("\n\n")

	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	b.WriteString(hintStyle.Render("💡 输入播放列表名称，按 Enter 继续，Esc 取消"))

	return b.String()
}

// renderDeleteConfirmView 渲染删除确认视图
func (a *App) renderDeleteConfirmView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF5252")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render("⚠️  确认删除"))
	b.WriteString("\n\n")

	if a.currentPlaylist != nil {
		warnStyle := lipgloss.NewStyle().
			Padding(0, 2)

		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5252")).
			Bold(true)

		countStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888"))

		var content strings.Builder
		content.WriteString("确定要删除播放列表吗？\n\n")
		content.WriteString(nameStyle.Render(fmt.Sprintf("📀 %s", a.currentPlaylist.Name)))
		content.WriteString("\n")
		content.WriteString(countStyle.Render(fmt.Sprintf("共 %d 首歌曲", len(a.currentPlaylist.Items))))

		b.WriteString(warnStyle.Render(content.String()))
		b.WriteString("\n\n")

		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
		b.WriteString(helpStyle.Render("按 Y 确认删除  |  其他键取消"))
	}

	return b.String()
}

// renderRenameView 渲染重命名视图
func (a *App) renderRenameView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render("重命名播放列表"))
	b.WriteString("\n\n")

	if a.selectedIndex < len(a.playlists) {
		currentStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Padding(0, 2)
		b.WriteString(currentStyle.Render(fmt.Sprintf("当前名称: %s", a.playlists[a.selectedIndex].Name)))
		b.WriteString("\n\n")
	}

	inputBox := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#333")).
		Padding(0, 2).
		Render(a.inputBuffer + "█")

	b.WriteString(inputBox)
	b.WriteString("\n\n")

	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	b.WriteString(hintStyle.Render("💡 输入新名称，按 Enter 确认，Esc 取消"))

	return b.String()
}
