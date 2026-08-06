package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// handleKeyPress 处理按键
func (a *App) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 如果在输入视图，优先处理文本输入
	if a.currentView == ViewCreatePlaylist {
		return a.handleInputKeyPress(msg)
	}

	// 如果在删除确认视图，处理确认
	if a.currentView == ViewDeletePlaylist {
		return a.handleDeleteConfirm(msg)
	}

	// 如果在重命名视图，处理输入
	if a.currentView == ViewRenamePlaylist {
		return a.handleRenameKeyPress(msg)
	}

	// 如果在文件浏览器视图，处理文件选择
	if a.currentView == ViewFileBrowser {
		return a.handleFileBrowserKeyPress(msg)
	}

	// 如果在歌词搜索视图，处理歌词搜索
	if a.currentView == ViewLyricSearch {
		return a.handleLyricSearchViewKeyPress(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		// 停止轮询
		if a.pollingCancel != nil {
			a.pollingCancel()
		}
		// 退出前重置终端标题，恢复为终端默认
		return a, tea.Batch(tea.Quit, a.resetWindowTitle())

	case "ctrl+z":
		// 挂起到后台（隐藏/收起 TUI，保持播放）
		// 由于真实的系统 SIGTSTP 会冻结整个进程导致音频停止，
		// 这里通过 ExecProcess 启动一个子 Shell 来"让出"终端，
		// 这样用户可以继续使用终端，而由于父进程并未被系统挂起，音频会继续播放。
		// 用户只需要在 Shell 中执行 exit 或 Ctrl+D 即可回到 TUI。
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "sh"
		}

		// 使用 sh 包装，先打印友好的挂起提示信息，再启动真实 Shell 交互
		wrapperCmd := fmt.Sprintf(`echo "\033[32m▶ Pan Player 已隐藏到后台，音乐继续播放中...\033[0m" && echo "💡 提示：输入 \033[33mexit\033[0m 或按 \033[33mCtrl+D\033[0m 即可恢复播放器界面。\n" && exec %s`, shell)
		cmd := exec.Command("sh", "-c", wrapperCmd)

		return a, tea.ExecProcess(cmd, func(err error) tea.Msg {
			// 恢复后强制重绘
			return ForceRenderMsg{}
		})

	case "h":
		if a.currentView != ViewHelp {
			a.sheepVis.SetVisible(false)
			a.currentView = ViewHelp
		}
		return a, tea.ClearScreen

	case "esc":
		if a.currentView == ViewHelp || a.currentView == ViewPlayer {
			// 如果是从播放器视图返回，保存当前播放状态
			if a.currentView == ViewPlayer {
				a.lastPlaybackState = a.player.GetState()
			}
			a.sheepVis.SetVisible(false)
			a.currentView = ViewPlaylist
			a.inputBuffer = "" // 清空输入缓冲
			// 离开播放界面，恢复默认终端标题
			// 重新加载播放列表以更新"最近播放"
			a.version++
			return a, tea.Batch(a.fullRepaintCmd(), a.loadPlaylists(), a.updateWindowTitleCmd())
		}
		return a, nil

	case "up":
		if a.currentView == ViewPlaylist && a.selectedIndex > 0 {
			a.selectedIndex--
		} else if a.currentView == ViewPlayer {
			volume := a.player.GetState().Volume + 0.1
			if volume > 1 {
				volume = 1
			}
			a.player.SetVolume(volume)
		}
		return a, nil

	case "down":
		if a.currentView == ViewPlaylist && a.selectedIndex < len(a.playlists)-1 {
			a.selectedIndex++
		} else if a.currentView == ViewPlayer {
			volume := a.player.GetState().Volume - 0.1
			if volume < 0 {
				volume = 0
			}
			a.player.SetVolume(volume)
		}
		return a, nil

	case "enter":
		if a.currentView == ViewLogin && a.isLoggedIn {
			a.currentView = ViewPlaylist
		} else if a.currentView == ViewPlaylist && len(a.playlists) > 0 {
			// 根据 selectedIndex 确定要播放的列表
			if a.selectedIndex < len(a.playlists) {
				selectedPlaylist := a.playlists[a.selectedIndex]

				// 检查是否正在播放同一个播放列表
				currentState := a.player.GetState()
				if currentState.IsPlaying &&
					currentState.CurrentSong != nil &&
					currentState.CurrentPlaylistName == selectedPlaylist.Name {
					// 正在播放同一列表，直接切换视图，不中断播放
					a.currentView = ViewPlayer
					a.sheepVis.SetVisible(true)
					return a, tea.Sequence(tea.ClearScreen, a.resumePlayerUpdates())
				}

				if len(selectedPlaylist.Items) > 0 {
					// 设置当前播放列表到 Player
					a.player.SetCurrentPlaylist(selectedPlaylist.Name, selectedPlaylist.Items)

					// 检查是否有保存的播放状态（从上次退出时保存）
					if a.lastPlaybackState != nil && a.lastPlaybackState.CurrentSong != nil {
						// 找到当前播放列表中对应的歌曲
						targetIndex := -1
						for i, item := range selectedPlaylist.Items {
							if item.FsID == a.lastPlaybackState.CurrentSong.FsID {
								targetIndex = i
								break
							}
						}

						if targetIndex >= 0 {
							// 恢复播放状态
							go func() {
								a.player.SetCurrentIndex(targetIndex)
								a.player.LoadTrack(context.Background(), selectedPlaylist.Items[targetIndex])
								// 恢复播放位置
								if a.lastPlaybackState.CurrentTime > 0 {
									time.Sleep(200 * time.Millisecond) // 等待歌曲加载
									a.player.Seek(a.lastPlaybackState.CurrentTime)
								}
								// 恢复播放模式
								a.player.SetPlayMode(a.lastPlaybackState.PlaybackMode)
								// 恢复音量
								a.player.SetVolume(a.lastPlaybackState.Volume)
								// 恢复播放倍速
								a.player.SetSpeed(a.lastPlaybackState.PlaybackRate)
								// 恢复播放状态
								if a.lastPlaybackState.IsPlaying {
									a.player.Play()
								} else {
									a.player.Pause()
								}
								// 加载歌词
								a.loadLyricsForTrack(selectedPlaylist.Items[targetIndex])
							}()
						} else {
							// 歌曲不在列表中，从头开始播放
							go func() {
								state := a.player.GetState()
								var startIndex int
								if state.PlaybackMode == models.PlaybackModeRandom {
									startIndex = a.player.GetShuffleStartIndex()
								} else {
									startIndex = 0
								}
								a.player.SetCurrentIndex(startIndex)
								a.player.LoadTrack(context.Background(), selectedPlaylist.Items[startIndex])
								a.loadLyricsForTrack(selectedPlaylist.Items[startIndex])
							}()
						}
					} else {
						// 没有保存的状态，从头开始播放
						go func() {
							state := a.player.GetState()
							var startIndex int
							if state.PlaybackMode == models.PlaybackModeRandom {
								startIndex = a.player.GetShuffleStartIndex()
							} else {
								startIndex = 0
							}
							a.player.SetCurrentIndex(startIndex)
							a.player.LoadTrack(context.Background(), selectedPlaylist.Items[startIndex])
							a.loadLyricsForTrack(selectedPlaylist.Items[startIndex])
						}()
					}
				}
			}
			a.currentView = ViewPlayer
			a.sheepVis.SetVisible(true)
			// 启动播放器状态更新定时器
			return a, tea.Sequence(tea.ClearScreen, a.resumePlayerUpdates())
		}
		return a, nil
	case " ":
		if a.currentView == ViewPlayer {
			if a.player.IsPlaying() {
				a.player.Pause()
			} else {
				a.player.Play()
			}
		}
		return a, nil

	case "left":
		if a.currentView == ViewPlayer {
			a.player.PlayPrevious()
		}
		return a, nil

	case "right":
		if a.currentView == ViewPlayer {
			a.player.PlayNext()
		}
		return a, nil

	case "l":
		if a.currentView == ViewPlayer {
			// Toggle lyrics visibility
			state := a.player.GetState()
			state.ShowLyrics = !state.ShowLyrics
			// 状态更新会触发 UI 重新渲染
		}
		return a, nil

	case "v":
		if a.currentView == ViewPlayer {
			// Toggle dancing sheep visualization
			if !a.sheepVis.Toggle() {
				a.showMessage(a.sheepVis.UnsupportedReason())
			}
			a.version++
			return a, a.fullRepaintCmd()
		}
		return a, nil

	case "s":
		if a.currentView == ViewPlayer {
			// 切换到歌词搜索视图，清除旧搜索状态，让当前歌曲名自动填入
			a.sheepVis.SetVisible(false)
			a.currentView = ViewLyricSearch
			a.lyricSearchKeyword = ""
			a.lyricSearchCursor = 0
			a.lyricSearchUI.Results = nil
			a.lyricSearchUI.SelectedIndex = 0
			a.version++
			return a, tea.Sequence(tea.ClearScreen, a.handleLyricSearch())
		}
		return a, nil

	case "u":
		if a.currentView == ViewPlayer {
			// 上传歌词到百度网盘
			a.handleLyricUpload()
		}
		return a, nil

	case "y":
		if a.currentView == ViewPlayer && a.awaitingLyricUploadConfirm {
			// 用户确认覆盖歌词文件
			a.awaitingLyricUploadConfirm = false
			a.uploadLyricsToBaidu(a.uploadTargetPath, a.uploadLyricsContent)
			a.uploadTargetPath = ""
			a.uploadLyricsContent = ""
		}
		return a, nil

	case "c":
		if a.currentView == ViewPlayer && a.awaitingLyricUploadConfirm {
			// 用户取消上传
			a.awaitingLyricUploadConfirm = false
			a.uploadTargetPath = ""
			a.uploadLyricsContent = ""
			a.showMessage("已取消上传")
		}
		return a, nil

	case "m":
		if a.currentView == ViewPlayer {
			state := a.player.GetState()
			var newMode models.PlaybackMode
			switch state.PlaybackMode {
			case models.PlaybackModeOrder:
				newMode = models.PlaybackModeRandom
			case models.PlaybackModeRandom:
				newMode = models.PlaybackModeSingle
			default:
				newMode = models.PlaybackModeOrder
			}
			a.player.SetPlayMode(newMode)
		}
		return a, nil

	case ">":
		if a.currentView == ViewPlayer {
			a.cyclePlaybackSpeed()
		}
		return a, nil

	case "p":
		if a.currentView == ViewPlayer {
			a.player.PlayPrevious()
		}
		return a, nil

	case "n":
		if a.currentView == ViewPlayer {
			// 下一曲
			a.player.PlayNext()
		} else if a.currentView == ViewPlaylist {
			// 进入新建播放列表模式
			a.currentView = ViewCreatePlaylist
			a.inputPrompt = "新建播放列表"
			a.inputBuffer = ""
			// 重置文件浏览器状态
			a.currentPath = "/"
			a.files = nil
			a.selectedFiles = nil
			a.fileBrowserIndex = 0
			a.loadingFiles = false
		}
		return a, nil

	case "d":
		if a.currentView == ViewPlaylist && len(a.playlists) > 0 {
			// 进入删除确认模式
			a.currentView = ViewDeletePlaylist
			// 设置当前播放列表为选中的播放列表
			if a.selectedIndex < len(a.playlists) {
				a.currentPlaylist = &a.playlists[a.selectedIndex]
			}
		}
		return a, nil

	case "r":
		if a.currentView == ViewPlaylist && len(a.playlists) > 0 {
			// 进入重命名模式
			a.currentView = ViewRenamePlaylist
			a.inputPrompt = "重命名播放列表"
			a.inputBuffer = ""
			// 设置当前播放列表为选中的播放列表
			if a.selectedIndex < len(a.playlists) {
				a.currentPlaylist = &a.playlists[a.selectedIndex]
			}
		}
		return a, nil

	case "R":
		if a.currentView == ViewPlaylist && len(a.playlists) > 0 {
			// 刷新当前选中的播放列表
			if a.selectedIndex < len(a.playlists) {
				playlistName := a.playlists[a.selectedIndex].Name
				if playlistName != "最近播放" {
					go func() {
						err := a.playlist.RefreshPlaylist(a.api, playlistName)
						if err == nil {
							// 重新加载播放列表
							a.playlist.LoadPlaylists()
							a.playlists = a.playlist.GetPlaylists()
						}
					}()
				}
			}
		}
		return a, nil
	}

	return a, nil
}

// handleRenameKeyPress 处理重命名的按键
func (a *App) handleRenameKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 先检查特定按键
	switch msg.String() {
	case "enter":
		// 确认输入
		if a.inputBuffer != "" && a.selectedIndex < len(a.playlists) {
			// 重命名播放列表
			oldName := a.playlists[a.selectedIndex].Name
			a.playlist.RenamePlaylist(oldName, a.inputBuffer)

			// 重新加载播放列表
			a.playlist.LoadPlaylists()
			a.playlists = a.playlist.GetPlaylists()
		}
		a.currentView = ViewPlaylist
		a.inputBuffer = ""
		return a, nil

	case "backspace":
		// 删除最后一个字符（正确处理中文等多字节字符）
		if len(a.inputBuffer) > 0 {
			runes := []rune(a.inputBuffer)
			if len(runes) > 0 {
				a.inputBuffer = string(runes[:len(runes)-1])
			}
		}
		return a, nil

	case "esc":
		// 取消输入
		a.currentView = ViewPlaylist
		a.inputBuffer = ""
		return a, nil
	}

	// 处理普通字符输入
	switch msg.Type {
	case tea.KeyRunes:
		// 输入字符
		for _, r := range msg.Runes {
			a.inputBuffer += string(r)
		}
		return a, nil
	}

	return a, nil
}

// handleInputKeyPress 处理输入视图的按键
func (a *App) handleInputKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	logger := utils.GetLogger()

	// 先检查特定按键
	switch msg.String() {
	case "enter":
		logger.Info("输入确认: %s", a.inputBuffer)
		// 确认输入
		if a.inputBuffer != "" {
			// 切换到文件浏览器视图
			a.currentView = ViewFileBrowser
			a.currentPath = "/"
			a.fileBrowserIndex = 0
			// 加载根目录文件
			return a, a.loadFiles("/")
		}
		return a, nil

	case "backspace":
		// 删除最后一个字符（正确处理中文等多字节字符）
		if len(a.inputBuffer) > 0 {
			runes := []rune(a.inputBuffer)
			if len(runes) > 0 {
				a.inputBuffer = string(runes[:len(runes)-1])
			}
		}
		a.version++
		return a, func() tea.Msg {
			return ForceRenderMsg{}
		}

	case "esc":
		logger.Info("输入取消")
		// 取消输入
		a.currentView = ViewPlaylist
		a.inputBuffer = ""
		a.inputPrompt = ""
		a.version++
		return a, func() tea.Msg {
			return ForceRenderMsg{}
		}
	}

	// 处理普通字符输入
	switch msg.Type {
	case tea.KeyRunes:
		// 输入字符
		for _, r := range msg.Runes {
			a.inputBuffer += string(r)
			a.version++
		}
		return a, func() tea.Msg {
			return ForceRenderMsg{}
		}
	}

	return a, func() tea.Msg {
		return ForceRenderMsg{}
	}
}

// handleDeleteConfirm 处理删除确认的按键
func (a *App) handleDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "y" && a.currentPlaylist != nil {
		// 确认删除
		a.playlist.RemovePlaylist(a.currentPlaylist.Name)
		a.playlist.LoadPlaylists()
		a.playlists = a.playlist.GetPlaylists()
		if len(a.playlists) > 0 {
			a.currentPlaylist = &a.playlists[0]
			a.selectedIndex = 0
		} else {
			a.currentPlaylist = nil
		}
	}
	// 返回播放列表视图
	a.currentView = ViewPlaylist
	return a, nil
}

// handleFileBrowserKeyPress 处理文件浏览器的按键
func (a *App) handleFileBrowserKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if a.fileBrowserIndex > 0 {
			a.fileBrowserIndex--
		}
		return a, func() tea.Msg {
			return ForceRenderMsg{}
		}

	case "down":
		// 计算可见文件数量
		audioFormats := []string{".mp3", ".m4a", ".flac", ".wav", ".ogg", ".aac", ".wma"}
		count := 0
		for _, file := range a.files {
			if file.Isdir == 1 {
				count++
			} else {
				ext := ""
				if idx := strings.LastIndex(file.ServerFilename, "."); idx > 0 {
					ext = strings.ToLower(file.ServerFilename[idx:])
				}
				for _, format := range audioFormats {
					if ext == format {
						count++
						break
					}
				}
			}
		}
		if a.fileBrowserIndex < count-1 {
			a.fileBrowserIndex++
		}
		return a, func() tea.Msg {
			return ForceRenderMsg{}
		}

	case "enter":
		// 获取当前选中的文件
		currentFile := a.getSelectedFile()
		if currentFile == nil {
			return a, nil
		}

		if currentFile.Isdir == 1 {
			// 如果是文件夹，进入该文件夹
			a.loadingFiles = true
			return a, a.loadFiles(currentFile.Path)
		} else {
			// 如果是文件，切换选中状态
			a.toggleFileSelection(*currentFile)
			return a, nil
		}

	case " ":
		// 切换选中状态
		currentFile := a.getSelectedFile()
		if currentFile != nil && currentFile.Isdir == 0 {
			a.toggleFileSelection(*currentFile)
		}
		return a, nil

	case "backspace":
		// 返回上一级目录
		if a.currentPath != "/" {
			parentPath := "/"
			parts := strings.Split(a.currentPath, "/")
			if len(parts) > 2 {
				parentPath = strings.Join(parts[:len(parts)-1], "/")
			}
			a.loadingFiles = true
			return a, a.loadFiles(parentPath)
		}
		return a, nil

	case "esc":
		// 取消选择，返回播放列表视图
		a.currentView = ViewPlaylist
		a.selectedFiles = nil
		a.files = nil
		a.inputBuffer = ""
		return a, nil

	case "ctrl+s", "s", "S":
		// 保存播放列表
		if len(a.selectedFiles) == 0 {
			// 没有选中文件，不执行保存
			return a, nil
		}
		if a.inputBuffer == "" {
			// 没有输入播放列表名称，不执行保存
			return a, nil
		}

		// 创建播放列表
		err := a.playlist.CreatePlaylist(a.inputBuffer, "")
		if err == nil {
			// 将选中的文件添加到播放列表
			var items []*models.PlaylistItem
			for _, file := range a.selectedFiles {
				items = append(items, &models.PlaylistItem{
					FsID:           file.FsID,
					ServerFileName: file.ServerFilename,
					Path:           file.Path,
					Size:           file.Size,
				})
			}
			a.playlist.AddToPlaylist(a.inputBuffer, items)
		}

		// 返回播放列表视图
		a.currentView = ViewPlaylist
		a.selectedFiles = nil
		a.files = nil
		a.inputBuffer = ""
		return a, a.loadPlaylists()

	case "a", "A":
		// 添加整个文件夹中的音频文件
		currentFile := a.getSelectedFile()
		if currentFile != nil && currentFile.Isdir == 1 {
			// 递归获取文件夹中的所有音频文件
			return a, a.addFolderFiles(currentFile.Path)
		}
		return a, nil
	}

	return a, nil
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
				a.sheepVis.SetVisible(true)
				a.lyricSearchUI.Visible = false
				a.lyricSearchUI.Results = nil
				a.lyricSearchKeyword = ""
				a.lyricSearchCursor = 0
				// 回到播放界面，恢复进度轮询与终端标题
				return a, tea.Sequence(tea.ClearScreen, a.resumePlayerUpdates())
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
		a.sheepVis.SetVisible(true)
		a.lyricSearchUI.Visible = false
		a.lyricSearchUI.Results = nil
		a.lyricSearchUI.Editing = false
		a.lyricSearchKeyword = ""
		a.lyricSearchCursor = 0
		// 回到播放界面，恢复进度轮询与终端标题
		return a, tea.Sequence(tea.ClearScreen, a.resumePlayerUpdates())

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
