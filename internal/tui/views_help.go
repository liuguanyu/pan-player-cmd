package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderHelpView 渲染帮助视图
func (a *App) renderHelpView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(1, 2)

	b.WriteString(titleStyle.Render("帮助 - 快捷键"))
	b.WriteString("\n\n")

	helpStyle := lipgloss.NewStyle().Padding(0, 2)

	shortcuts := []struct {
		key  string
		desc string
	}{
		{"h", "全局-显示帮助"},
		{"q", "全局-退出程序"},
		{"Esc", "全局-返回上一级"},
	}

	b.WriteString(titleStyle.Render("全局快捷键"))
	b.WriteString("\n\n")
	for _, shortcut := range shortcuts {
		b.WriteString(helpStyle.Render(fmt.Sprintf("%-10s %s", shortcut.key, shortcut.desc)))
		b.WriteString("\n")
	}

	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render("播放列表视图"))
	b.WriteString("\n\n")
	shortcuts = []struct {
		key  string
		desc string
	}{
		{"Enter", "打开播放列表"},
		{"n", "创建新播放列表"},
		{"d", "删除播放列表"},
		{"↑/↓", "选择播放列表"},
	}
	for _, shortcut := range shortcuts {
		b.WriteString(helpStyle.Render(fmt.Sprintf("%-10s %s", shortcut.key, shortcut.desc)))
		b.WriteString("\n")
	}

	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render("播放器视图"))
	b.WriteString("\n\n")
	shortcuts = []struct {
		key  string
		desc string
	}{
		{"Space", "播放/暂停"},
		{"←", "上一首"},
		{"→", "下一首"},
		{"↑", "增加音量"},
		{"↓", "减少音量"},
		{"m", "切换播放模式"},
		{"l", "显示/隐藏歌词"},
		{"s", "搜索歌词"},
		{"u", "上传歌词到网盘"},
		{">", "切换播放倍速"},
		{"v", "切换可视化（小羊跳舞/Sixel）"},
	}
	for _, shortcut := range shortcuts {
		b.WriteString(helpStyle.Render(fmt.Sprintf("%-10s %s", shortcut.key, shortcut.desc)))
		b.WriteString("\n")
	}

	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render("文件浏览器视图"))
	b.WriteString("\n\n")
	shortcuts = []struct {
		key  string
		desc string
	}{
		{"Enter", "进入文件夹/选择文件"},
		{"Space", "选择/取消选择文件"},
		{"A", "添加整个文件夹的音频文件"},
		{"Backspace", "返回上一级目录"},
		{"S", "保存并创建播放列表"},
		{"Esc", "取消并返回"},
		{"↑/↓", "选择文件"},
	}
	for _, shortcut := range shortcuts {
		b.WriteString(helpStyle.Render(fmt.Sprintf("%-10s %s", shortcut.key, shortcut.desc)))
		b.WriteString("\n")
	}

	b.WriteString("\n\n按 Esc 返回")

	return b.String()
}
