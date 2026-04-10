package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// AudioFeaturesMsg 实时音频特征消息
type AudioFeaturesMsg struct {
	Features models.RealtimeFeatures
}

// renderAudioFeatures 渲染实时音频特征（显示在进度条下方，歌词上方）
func (a *App) renderAudioFeatures() string {
	f := a.realtimeFeatures

	// 如果没有有效特征，不显示
	if f.Timestamp == 0 {
		return ""
	}

	var b strings.Builder

	// 特征样式
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888")).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4"))

	// 人声检测
	vocalText := "🔇 纯音乐"
	if f.HasVocal {
		genderIcon := "👤"
		switch f.Gender {
		case models.GenderMale:
			genderIcon = "♂ 男声"
		case models.GenderFemale:
			genderIcon = "♀ 女声"
		default:
			genderIcon = "👤 人声"
		}
		vocalText = fmt.Sprintf("🎤 %s", genderIcon)
	}

	// 乐器识别
	instrumentText := "🎹 其他"
	switch f.DominantInstr {
	case "piano":
		instrumentText = "🎹 钢琴"
	case "guitar":
		instrumentText = "🎸 吉他"
	case "drums":
		instrumentText = "🥁 鼓"
	case "bass":
		instrumentText = "🎸 贝斯"
	case "vocal":
		instrumentText = "🎤 人声"
	}

	// 和声复杂度（0-5星）
	harmonyStars := int(f.HarmonyLevel * 5)
	if harmonyStars > 5 {
		harmonyStars = 5
	}
	if harmonyStars < 0 {
		harmonyStars = 0
	}
	harmonyText := strings.Repeat("★", harmonyStars) + strings.Repeat("☆", 5-harmonyStars)

	// 能量强度（百分比）
	energyPercent := int(f.EnergyLevel * 100)
	if energyPercent > 100 {
		energyPercent = 100
	}

	// 段落类型
	sectionText := ""
	switch f.CurrentSection {
	case models.SectionIntro:
		sectionText = "🎵 前奏"
	case models.SectionVerse:
		sectionText = "🎵 主歌"
	case models.SectionChorus:
		sectionText = "🎵 副歌"
	case models.SectionBridge:
		sectionText = "🎵 间奏"
	case models.SectionOutro:
		sectionText = "🎵 尾声"
	}

	// 第一行：人声 + 乐器 + 段落
	b.WriteString(labelStyle.Render("特征: "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%s | %s | %s", vocalText, instrumentText, sectionText)))
	b.WriteString("\n")

	// 第二行：和声 + 能量
	b.WriteString(labelStyle.Render("和声: "))
	b.WriteString(valueStyle.Render(harmonyText))
	b.WriteString("  ")
	b.WriteString(labelStyle.Render("能量: "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%d%%", energyPercent)))

	return b.String()
}

// listenAudioFeatures 监听音频特征更新
func (a *App) listenAudioFeatures() tea.Cmd {
	return func() tea.Msg {
		// 如果没有特征通道，返回 nil
		if a.featuresChan == nil {
			return nil
		}

		// 阻塞等待特征更新
		features, ok := <-a.featuresChan
		if !ok {
			// channel 已关闭
			return nil
		}

		return AudioFeaturesMsg{Features: features}
	}
}