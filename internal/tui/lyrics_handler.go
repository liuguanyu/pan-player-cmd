package tui

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// renderLyricSearchView 渲染歌词搜索视图
func (a *App) renderLyricSearchView() string {
	var b strings.Builder

	// 标题样式
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(1, 2)

	b.WriteString(titleStyle.Render("🎵 歌词搜索"))
	b.WriteString("\n\n")

	// 显示当前歌曲信息
	state := a.player.GetState()
	if state.CurrentSong != nil {
		songStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06BF54")).
			Padding(0, 2)
		songName := extractSongName(state.CurrentSong.ServerFileName)
		b.WriteString(songStyle.Render(fmt.Sprintf("当前歌曲: %s", songName)))
		b.WriteString("\n\n")
	}

	// 搜索词输入框
	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#333")).
		Padding(0, 2)

	// 显示搜索词，带光标（基于字符索引）
	runes := []rune(a.lyricSearchKeyword)
	cursor := a.lyricSearchCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	// 构建带光标的搜索词显示
	displayKeyword := string(runes[:cursor]) + "█" + string(runes[cursor:])
	b.WriteString(inputStyle.Render("搜索词: " + displayKeyword))
	b.WriteString("\n\n")

	// 如果正在编辑搜索词
	if a.lyricSearchUI.Editing {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA")).Render("输入搜索词 | Enter 搜索 | Esc 返回播放界面"))
		return b.String()
	}

	// 如果没有搜索结果
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

	// 显示搜索结果
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

// handleLyricSearchViewKeyPress 处理歌词搜索视图的按键
func (a *App) handleLyricSearchViewKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 如果正在编辑搜索词
	if a.lyricSearchUI.Editing {
		switch msg.String() {
		case "enter":
			// 执行搜索
			a.lyricSearchUI.Editing = false
			return a, a.handleLyricSearch()
		case "esc":
			// 取消编辑，返回搜索结果（或播放界面）
			a.lyricSearchUI.Editing = false
			if len(a.lyricSearchUI.Results) == 0 {
				a.currentView = ViewPlayer
				a.lyricSearchUI.Visible = false
				a.lyricSearchUI.Results = nil
			}
			return a, nil
		case "left":
			// 光标左移（基于字符索引）
			if a.lyricSearchCursor > 0 {
				a.lyricSearchCursor--
			}
			return a, nil
		case "right":
			// 光标右移（基于字符索引）
			runes := []rune(a.lyricSearchKeyword)
			if a.lyricSearchCursor < len(runes) {
				a.lyricSearchCursor++
			}
			return a, nil
		case "backspace":
			// 删除光标前的字符（基于字符索引）
			if a.lyricSearchCursor > 0 {
				runes := []rune(a.lyricSearchKeyword)
				// 删除光标前的字符
				newRunes := append(runes[:a.lyricSearchCursor-1], runes[a.lyricSearchCursor:]...)
				a.lyricSearchKeyword = string(newRunes)
				a.lyricSearchCursor--
			}
			return a, nil
		case "delete":
			// 删除光标后的字符（基于字符索引）
			runes := []rune(a.lyricSearchKeyword)
			if a.lyricSearchCursor < len(runes) {
				newRunes := append(runes[:a.lyricSearchCursor], runes[a.lyricSearchCursor+1:]...)
				a.lyricSearchKeyword = string(newRunes)
			}
			return a, nil
		default:
			// 处理字符输入（基于字符索引）
			for _, r := range msg.Runes {
				if r >= 32 && r != 127 { // 允许所有Unicode字符（包括中文），只排除控制字符
					runes := []rune(a.lyricSearchKeyword)
					// 在光标位置插入字符
					newRunes := make([]rune, len(runes)+1)
					copy(newRunes, runes[:a.lyricSearchCursor])
					newRunes[a.lyricSearchCursor] = r
					copy(newRunes[a.lyricSearchCursor+1:], runes[a.lyricSearchCursor:])
					a.lyricSearchKeyword = string(newRunes)
					a.lyricSearchCursor++
				}
			}
			return a, nil
		}
	}

	// 不在编辑模式时
	switch msg.String() {
	case "e":
		// 进入编辑模式
		a.lyricSearchUI.Editing = true
		// 如果搜索词为空，使用当前歌曲名
		if a.lyricSearchKeyword == "" {
			state := a.player.GetState()
			if state.CurrentSong != nil {
				a.lyricSearchKeyword = extractSongName(state.CurrentSong.ServerFileName)
			}
		}
		// 初始化光标位置为字符数量
		a.lyricSearchCursor = len([]rune(a.lyricSearchKeyword))
		return a, nil

	case "up":
		if a.lyricSearchUI.SelectedIndex > 0 {
			a.lyricSearchUI.SelectedIndex--
		}
		return a, nil

	case "down":
		if a.lyricSearchUI.SelectedIndex < len(a.lyricSearchUI.Results)-1 {
			a.lyricSearchUI.SelectedIndex++
		}
		return a, nil

	case "enter":
		if len(a.lyricSearchUI.Results) > 0 {
			return a, a.confirmLyricSelection()
		}
		if a.lyricSearchUI.Editing {
			a.lyricSearchUI.Editing = false
			return a, a.handleLyricSearch()
		}
		// 没有结果，进入编辑模式
		a.lyricSearchUI.Editing = true
		if a.lyricSearchKeyword == "" {
			state := a.player.GetState()
			if state.CurrentSong != nil {
				a.lyricSearchKeyword = extractSongName(state.CurrentSong.ServerFileName)
			}
		}
		// 正确设置光标位置（处理中文等多字节字符）
		a.lyricSearchCursor = len([]rune(a.lyricSearchKeyword))
		return a, nil

	case "esc":
		// 返回播放界面
		a.currentView = ViewPlayer
		a.lyricSearchUI.Visible = false
		a.lyricSearchUI.Results = nil
		a.lyricSearchUI.Editing = false
		a.lyricSearchKeyword = ""
		a.lyricSearchCursor = 0
		return a, nil

	case "backspace":
		// 在非编辑模式下也允许退格键删除搜索词（正确处理中文等多字节字符）
		if a.lyricSearchKeyword != "" {
			runes := []rune(a.lyricSearchKeyword)
			if len(runes) > 0 {
				a.lyricSearchKeyword = string(runes[:len(runes)-1])
			}
			// 正确设置光标位置（处理中文等多字节字符）
			a.lyricSearchCursor = len([]rune(a.lyricSearchKeyword))
		}
		return a, nil
	}

	return a, nil
}

// lyricSearchDoneMsg 歌词搜索完成消息
// BubbleTea推荐通过消息机制更新UI状态，避免goroutine直接修改模型
//
//nolint:govet // 用于tea消息传递
type lyricSearchDoneMsg struct {
	keyword string
	results []models.LyricSearchResult
	err     error
}

// lyricDownloadDoneMsg 歌词下载完成消息
type lyricDownloadDoneMsg struct {
	lrcContent string
	err        error
}

// handleLyricSearch 处理歌词搜索
func (a *App) handleLyricSearch() tea.Cmd {
	state := a.player.GetState()
	if state.CurrentSong == nil {
		return nil
	}

	// 如果没有搜索词，使用当前歌曲名并自动进入编辑模式
	if a.lyricSearchKeyword == "" {
		a.lyricSearchKeyword = extractSongName(state.CurrentSong.ServerFileName)
		// 初次进入时自动进入编辑模式
		a.lyricSearchUI.Editing = true
		// 正确设置光标位置（处理中文等多字节字符）
		a.lyricSearchCursor = len([]rune(a.lyricSearchKeyword))
		return nil
	}

	keyword := a.lyricSearchKeyword
	return func() tea.Msg {
		results, err := a.lyricsManager.Search(context.Background(), keyword)
		if err != nil {
			return lyricSearchDoneMsg{keyword: keyword, err: err}
		}

		modelResults := make([]models.LyricSearchResult, len(results))
		for i, r := range results {
			modelResults[i] = models.LyricSearchResult{
				ID:       r.ID,
				Title:    r.Title,
				Artist:   r.Artist,
				Album:    r.Album,
				Duration: r.Duration,
				Source:   r.Source,
			}
		}

		return lyricSearchDoneMsg{
			keyword: keyword,
			results: modelResults,
			err:     nil,
		}
	}
}

// confirmLyricSelection 确认歌词选择并返回异步命令
func (a *App) confirmLyricSelection() tea.Cmd {
	if a.lyricSearchUI.SelectedIndex >= len(a.lyricSearchUI.Results) {
		return nil
	}

	selected := a.lyricSearchUI.Results[a.lyricSearchUI.SelectedIndex]

	// 异步获取歌词详情
	return func() tea.Msg {
		lrcContent, err := a.lyricsManager.GetLyric(context.Background(), selected.Source, selected.ID)
		return lyricDownloadDoneMsg{
			lrcContent: lrcContent,
			err:        err,
		}
	}
}

// handleLyricUpload 处理歌词上传
func (a *App) handleLyricUpload() {
	state := a.player.GetState()
	if state.CurrentSong == nil || len(state.LyricsRaw) == 0 {
		return
	}

	// 构建目标路径（同名.lrc）
	// 使用 path 包（而非 filepath）处理百度网盘的 Unix 风格路径，避免 Windows 路径转换问题
	audioPath := strings.ReplaceAll(state.CurrentSong.Path, "\\", "/")
	ext := path.Ext(audioPath)
	lrcPath := audioPath[:len(audioPath)-len(ext)] + ".lrc"

	// 检查是否已存在
	exists, err := a.api.CheckLRCFileExists(context.Background(), audioPath)
	if err != nil {
		a.showMessage("检查歌词文件失败: " + err.Error())
		return
	}

	if exists != nil {
		// 文件已存在，需要用户确认
		a.showMessage("歌词文件已存在，按 'y' 确认覆盖，按 'c' 取消")
		a.awaitingLyricUploadConfirm = true
		a.uploadTargetPath = lrcPath
		a.uploadLyricsContent = state.LyricsRaw
		return
	}

	// 文件不存在，直接上传
	a.uploadLyricsToBaidu(lrcPath, state.LyricsRaw)
}

// uploadLyricsToBaidu 上传歌词到百度网盘
func (a *App) uploadLyricsToBaidu(targetPath, lrcContent string) {
	a.showMessage("正在上传歌词...")

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "lyrics-*.lrc")
	if err != nil {
		a.showMessage("创建临时文件失败")
		return
	}
	defer os.Remove(tmpFile.Name())

	// 写入歌词内容
	_, err = tmpFile.WriteString(lrcContent)
	tmpFile.Close()
	if err != nil {
		a.showMessage("写入临时文件失败")
		return
	}

	// 上传到百度网盘
	err = a.api.UploadFile(context.Background(), tmpFile.Name(), targetPath)
	if err != nil {
		a.showMessage("上传失败: " + err.Error())
		return
	}

	a.showMessage("歌词已上传至网盘")

	// 更新当前歌曲的LRCPath
	state := a.player.GetState()
	if state.CurrentSong != nil {
		// 创建一个新的PlaylistItem副本，更新LRCPath
		updatedSong := *state.CurrentSong
		updatedSong.LRCPath = targetPath
		// 这里需要更新播放器中的当前歌曲
		// 为简化实现，我们只更新状态
		state.CurrentSong = &updatedSong
	}
}

// extractSongName 从文件名中提取歌曲名（去掉扩展名、前导数字和空格）
func extractSongName(filename string) string {
	// 去掉扩展名（使用 path 包处理 Unix 风格路径，兼容 Windows）
	ext := path.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	// 去掉前导数字、点、空格和破折号
	// 匹配格式如 "01. 周杰伦 - 半兽人" 或 "01 周杰伦 - 半兽人"
	// 使用 \x{3001} 表示中文顿号，或者直接使用字符
	re := regexp.MustCompile(`^[\d\s\.\-\:、]+`)
	name = re.ReplaceAllString(name, "")

	// 去掉前后空格
	name = strings.TrimSpace(name)

	return name
}

// showMessage 显示消息
func (a *App) showMessage(msg string) {
	// 更新消息状态，将在UI上显示
	a.currentMessage = msg
	a.messageTimeout = time.Now().Add(3 * time.Second)
}
