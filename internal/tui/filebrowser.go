package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
)

func (a *App) loadFiles(path string) tea.Cmd {
	a.loadingFiles = true
	a.loadingDots = 0
	return tea.Batch(
		a.tickLoadingAnimation(),
		func() tea.Msg {
			files, err := a.svc.API.GetFileList(path, 1, 1000)
			if err != nil {
				return FilesLoadedMsg{Files: nil, Path: path}
			}
			return FilesLoadedMsg{Files: files, Path: path}
		},
	)
}

// tickLoadingAnimation 加载动画定时器
func (a *App) tickLoadingAnimation() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg {
		return LoadingAnimationMsg{}
	})
}

// addFolderFiles 递归添加文件夹中的音频文件
func (a *App) addFolderFiles(folderPath string) tea.Cmd {
	a.loadingFiles = true
	a.loadingDots = 0
	return tea.Batch(
		a.tickLoadingAnimation(),
		func() tea.Msg {
			files, err := a.svc.API.GetAudioFilesRecursive(folderPath)
			if err != nil {
				return FolderFilesLoadedMsg{Files: nil}
			}
			return FolderFilesLoadedMsg{Files: files}
		},
	)
}

// getSelectedFile 获取当前选中的文件
func (a *App) getSelectedFile() *api.FileInfo {
	formats := []string{".mp3", ".m4a", ".flac", ".wav", ".ogg", ".aac", ".wma"}

	index := 0
	for _, file := range a.files {
		if file.Isdir == 1 {
			if index == a.fileBrowserIndex {
				return &file
			}
			index++
		} else {
			ext := ""
			if idx := strings.LastIndex(file.ServerFilename, "."); idx > 0 {
				ext = strings.ToLower(file.ServerFilename[idx:])
			}
			for _, format := range formats {
				if ext == format {
					if index == a.fileBrowserIndex {
						return &file
					}
					index++
					break
				}
			}
		}
	}

	return nil
}

// toggleFileSelection 切换文件选中状态
func (a *App) toggleFileSelection(file api.FileInfo) {
	// 查找文件是否已选中
	found := false
	for i, f := range a.selectedFiles {
		if f.FsID == file.FsID {
			// 已选中，取消选中
			a.selectedFiles = append(a.selectedFiles[:i], a.selectedFiles[i+1:]...)
			found = true
			break
		}
	}

	// 如果未选中，添加到选中列表
	if !found {
		a.selectedFiles = append(a.selectedFiles, file)
	}
}
