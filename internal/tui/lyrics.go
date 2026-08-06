package tui

import (
	"context"
	"os"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// lyricSearchDoneMsg 歌词搜索完成消息
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

// loadLyricsForTrack 为指定曲目加载歌词
func (a *App) loadLyricsForTrack(track *models.PlaylistItem) {
	logger := utils.GetLogger()
	logger.Info("加载歌词: %s", track.ServerFileName)

	// 重置歌词
	a.currentLyrics = nil

	if a.api == nil || track == nil {
		return
	}

	// 检查同目录下是否存在 LRC 文件
	lrcFile, err := a.api.CheckLRCFileExists(context.Background(), track.Path)
	if err != nil {
		logger.Warn("检查歌词文件失败: %v", err)
		// 不设置任何歌词，让UI不显示歌词区域
		return
	}

	if lrcFile == nil {
		logger.Info("未找到歌词文件")
		// 不设置任何歌词，让UI不显示歌词区域
		return
	}

	logger.Info("找到歌词文件: %s", lrcFile.Path)

	// 下载 LRC 文件内容
	lrcContent, err := a.api.DownloadLRCContent(context.Background(), lrcFile.FsID)
	if err != nil {
		logger.Error("下载歌词文件失败: %v", err)
		// 不设置任何歌词，让UI不显示歌词区域
		return
	}

	// 解析 LRC 内容
	parseResult := lyrics.ParseLRC(lrcContent)
	if len(parseResult.Lines) == 0 {
		logger.Warn("歌词内容为空")
		// 不设置任何歌词，让UI不显示歌词区域
		return
	}

	a.currentLyrics = parseResult.Lines
	logger.Info("成功加载歌词，共 %d 行", len(a.currentLyrics))

	// 显示元数据
	if parseResult.Metadata.Album != "" {
		logger.Info("歌曲信息: %s", parseResult.Metadata.Album)
	}
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
