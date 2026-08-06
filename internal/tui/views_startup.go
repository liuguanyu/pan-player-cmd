package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderSplashView 渲染流光启动视图
func (a *App) renderSplashView() string {
	var b strings.Builder

	// 居中显示
	lines := strings.Split(a.renderSplashContent(), "\n")
	for _, line := range lines {
		padding := (a.width - lipgloss.Width(line)) / 2
		if padding > 0 {
			b.WriteString(strings.Repeat(" ", padding))
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}

	return b.String()
}

// renderSplashContent 渲染流光内容
func (a *App) renderSplashContent() string {
	var b strings.Builder

	// 标题样式
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(2, 4)

	// 副标题样式
	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888")).
		Padding(1, 2)

	// 流光文字 - 使用渐变色彩
	displayText := a.splashText[:a.splashIndex]

	// 创建流光效果 - 每个字符使用不同的颜色
	if len(displayText) > 0 {
		colors := []string{
			"#FF6B6B", "#FF8E53", "#FFC857", "#C9E4CA",
			"#87CEEB", "#B4A7D6", "#FF69B4", "#7D56F4",
		}

		var styledText strings.Builder
		for i, ch := range displayText {
			color := colors[i%len(colors)]
			charStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(color)).
				Bold(true)
			styledText.WriteString(charStyle.Render(string(ch)))
		}

		b.WriteString(titleStyle.Render(styledText.String()))
	} else {
		b.WriteString(titleStyle.Render(""))
	}

	b.WriteString("\r\n\r\n")

	// 加载提示
	if a.splashIndex < len(a.splashText) {
		dots := strings.Repeat(".", (a.splashIndex/3)%4)
		b.WriteString(subtitleStyle.Render("加载中" + dots))
	} else {
		b.WriteString(subtitleStyle.Render("✓ 准备就绪"))
	}

	return b.String()
}

// renderLoginView 渲染登录视图
func (a *App) renderLoginView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(1, 2)

	b.WriteString(titleStyle.Render("Pan Player TUI - 登录"))
	b.WriteString("\r\n\r\n")

	if !a.isLoggedIn {
		if a.loginError != "" {
			errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
			b.WriteString(errorStyle.Render("错误: " + a.loginError))
			b.WriteString("\r\n\r\n")
		}

		if a.deviceAuth != nil {
			infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
			b.WriteString(infoStyle.Render("请使用手机百度网盘APP扫描二维码登录"))
			b.WriteString("\r\n\r\n")

			// 显示二维码（确保从行首开始，不应用样式）
			if a.qrCode != "" {
				b.WriteString(a.qrCode)
				b.WriteString("\r\n\r\n")
			}

			// 显示备用方案
			b.WriteString(infoStyle.Render("无法扫码？使用备用方案："))
			b.WriteString("\r\n")
			b.WriteString(fmt.Sprintf("   访问: %s", a.deviceAuth.VerificationURL))
			b.WriteString("\r\n")
			b.WriteString(fmt.Sprintf("   用户码: %s", a.deviceAuth.UserCode))
			b.WriteString("\r\n\r\n")

			// 显示等待状态
			waitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00"))
			b.WriteString(waitStyle.Render("⏳ 等待授权中..."))
			b.WriteString("\r\n")
		} else {
			infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
			b.WriteString(infoStyle.Render("正在获取设备码，请稍候..."))
			b.WriteString("\r\n")
		}
	} else {
		successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))
		b.WriteString(successStyle.Render("✓ 登录成功！"))
		b.WriteString("\r\n\r\n")
		if a.userInfo != nil {
			b.WriteString(fmt.Sprintf("用户: %s", a.userInfo.BaiduName))
			b.WriteString("\r\n")
		}
		b.WriteString("\r\n按 Enter 进入播放列表...")
	}

	return b.String()
}
