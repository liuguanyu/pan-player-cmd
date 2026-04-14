package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
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

	// 显示搜索词，带光标
	keyword := a.lyricSearchKeyword
	cursor := a.lyricSearchCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(keyword) {
		cursor = len(keyword)
	}

	// 构建带光标的搜索词显示
	displayKeyword := keyword[:cursor] + "█" + keyword[cursor:]
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
			if a.lyricSearchCursor > 0 {
				a.lyricSearchCursor--
			}
			return a, nil
		case "right":
			if a.lyricSearchCursor < len(a.lyricSearchKeyword) {
				a.lyricSearchCursor++
			}
			return a, nil
		case "backspace":
			if a.lyricSearchCursor > 0 {
				a.lyricSearchKeyword = a.lyricSearchKeyword[:a.lyricSearchCursor-1] + a.lyricSearchKeyword[a.lyricSearchCursor:]
				a.lyricSearchCursor--
			}
			return a, nil
		case "delete":
			if a.lyricSearchCursor < len(a.lyricSearchKeyword) {
				a.lyricSearchKeyword = a.lyricSearchKeyword[:a.lyricSearchCursor] + a.lyricSearchKeyword[a.lyricSearchCursor+1:]
			}
			return a, nil
		default:
			// 处理字符输入
			for _, r := range msg.Runes {
				if r >= 32 && r <= 126 { // 可打印ASCII字符
					a.lyricSearchKeyword = a.lyricSearchKeyword[:a.lyricSearchCursor] + string(r) + a.lyricSearchKeyword[a.lyricSearchCursor:]
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
		a.lyricSearchCursor = len(a.lyricSearchKeyword)
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
		// 如果在编辑模式，执行搜索并退出编辑模式
		if a.lyricSearchUI.Editing {
			a.lyricSearchUI.Editing = false
			return a, a.handleLyricSearch()
		}
		// 如果有结果，确认选择
		if len(a.lyricSearchUI.Results) > 0 {
			a.confirmLyricSelection()
		} else {
			// 没有结果，进入编辑模式
			a.lyricSearchUI.Editing = true
			if a.lyricSearchKeyword == "" {
				state := a.player.GetState()
				if state.CurrentSong != nil {
					a.lyricSearchKeyword = extractSongName(state.CurrentSong.ServerFileName)
				}
			}
			a.lyricSearchCursor = len(a.lyricSearchKeyword)
		}
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
	}

	return a, nil
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
		a.lyricSearchCursor = len(a.lyricSearchKeyword)
	}

	// 获取日志记录器
	logger := utils.GetLogger()

	// 搜索歌词
	go func() {
		results, err := a.lyricsManager.Search(context.Background(), a.lyricSearchKeyword)
		if err != nil {
			// 错误记录到日志，不在界面显示
			logger.Error("歌词搜索失败: %v", err)
			// 标记为无结果
			a.lyricSearchUI.Results = nil
			return
		}

		if len(results) == 0 {
			// 记录到日志
			logger.Info("未找到歌词: %s", a.lyricSearchKeyword)
			// 自动进入编辑模式让用户调整
			a.lyricSearchUI.Editing = true
			a.lyricSearchUI.Results = nil
			return
		}

		// 转换为models.LyricSearchResult
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

		// 显示搜索结果列表
		a.lyricSearchUI = LyricSearchUI{
			Results:       modelResults,
			SelectedIndex: 0,
			Visible:       true,
			Editing:       false,
		}
	}()

	return nil
}

// confirmLyricSelection 确认歌词选择
func (a *App) confirmLyricSelection() {
	if a.lyricSearchUI.SelectedIndex >= len(a.lyricSearchUI.Results) {
		return
	}

	selected := a.lyricSearchUI.Results[a.lyricSearchUI.SelectedIndex]

	// 获取歌词详情
	lrcContent, err := a.lyricsManager.GetLyric(context.Background(), selected.Source, selected.ID)
	if err != nil {
		a.showMessage("获取歌词失败")
		return
	}

	// 返回播放界面
	a.currentView = ViewPlayer
	a.lyricSearchUI.Visible = false

	// 解析并显示歌词
	parsed := lyrics.ParseLRC(lrcContent)
	state := a.player.GetState()
	state.LyricsRaw = lrcContent
	state.LyricsParsed = parsed.Lines
	a.currentLyrics = parsed.Lines

	// 显示歌词
	state.ShowLyrics = true

	// 提示用户可以上传
	a.showMessage("歌词已加载，按 'u' 上传至网盘")
}

// handleLyricUpload 处理歌词上传
func (a *App) handleLyricUpload() {
	state := a.player.GetState()
	if state.CurrentSong == nil || len(state.LyricsRaw) == 0 {
		return
	}

	// 构建目标路径（同名.lrc）
	audioPath := state.CurrentSong.Path
	ext := filepath.Ext(audioPath)
	lrcPath := audioPath[:len(audioPath)-len(ext)] + ".lrc"

	// 检查是否已存在
	go func() {
		exists, err := a.api.CheckLRCFileExists(context.Background(), audioPath)
		if err == nil && exists != nil {
			// 提示用户是否覆盖
			a.showMessage("歌词文件已存在，是否覆盖？")
			// 这里简化处理，直接覆盖
		}

		// 上传歌词
		a.uploadLyricsToBaidu(lrcPath, state.LyricsRaw)
	}()
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
	// 去掉扩展名
	ext := filepath.Ext(filename)
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

// showMessage 显示消息（简化版本）
func (a *App) showMessage(msg string) {
	// 在实际实现中，这里可以更新一个消息显示状态
	// 为简化实现，这里只是打印到控制台
	fmt.Printf("\n[消息] %s\n", msg)
}
