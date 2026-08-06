package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
)

// renderFileBrowserView 渲染文件浏览器视图
func (a *App) renderFileBrowserView() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render(fmt.Sprintf("📂 选择文件 · %s", a.inputBuffer)))
	b.WriteString("\n")

	pathParts := strings.Split(a.currentPath, "/")
	var breadcrumb strings.Builder
	breadcrumb.WriteString("🏠 ")
	for i, part := range pathParts {
		if part == "" {
			continue
		}
		if i > 0 {
			breadcrumb.WriteString(" > ")
		}
		breadcrumb.WriteString(part)
	}
	if a.currentPath == "/" {
		breadcrumb.WriteString("根目录")
	}

	pathStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888")).
		Padding(0, 2)

	b.WriteString(pathStyle.Render(breadcrumb.String()))
	b.WriteString("\n\n")

	if a.loadingFiles {
		loadingColors := []string{"#FF6B6B", "#FFC857", "#4ECDC4", "#7D56F4"}
		dots := strings.Repeat(".", (a.loadingDots)%4+1)

		var loadingBuilder strings.Builder
		loadingText := "正在加载文件" + dots

		for i, ch := range loadingText {
			color := loadingColors[(len(loadingText)-i)%len(loadingColors)]
			charStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(color))
			loadingBuilder.WriteString(charStyle.Render(string(ch)))
		}

		loadingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Padding(2, 2)

		b.WriteString(loadingStyle.Render(loadingBuilder.String()))
		return b.String()
	}

	audioFormats := []string{".mp3", ".m4a", ".flac", ".wav", ".ogg", ".aac", ".wma"}

	var displayFiles []struct {
		file     api.FileInfo
		isAudio  bool
		isFolder bool
		selected bool
	}

	for _, file := range a.files {
		if file.Isdir == 1 {
			selected := false
			for _, f := range a.selectedFiles {
				if f.FsID == file.FsID {
					selected = true
					break
				}
			}
			displayFiles = append(displayFiles, struct {
				file     api.FileInfo
				isAudio  bool
				isFolder bool
				selected bool
			}{file, false, true, selected})
		}
	}

	for _, file := range a.files {
		if file.Isdir == 0 {
			ext := ""
			if idx := strings.LastIndex(file.ServerFilename, "."); idx > 0 {
				ext = strings.ToLower(file.ServerFilename[idx:])
			}
			isAudio := false
			for _, format := range audioFormats {
				if ext == format {
					isAudio = true
					break
				}
			}
			if isAudio {
				selected := false
				for _, f := range a.selectedFiles {
					if f.FsID == file.FsID {
						selected = true
						break
					}
				}
				displayFiles = append(displayFiles, struct {
					file     api.FileInfo
					isAudio  bool
					isFolder bool
					selected bool
				}{file, true, false, selected})
			}
		}
	}

	if len(displayFiles) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Italic(true).
			Padding(2, 2)
		b.WriteString(emptyStyle.Render("此文件夹中没有音频文件"))
	} else {
		normalStyle := lipgloss.NewStyle().Padding(0, 2)
		selectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			Bold(true)
		folderStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4A90E2")).
			Padding(0, 2)
		selectedFolderStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFF")).
			Background(lipgloss.Color("#4A90E2")).
			Padding(0, 2).
			Bold(true)

		visibleCount := a.height - 15
		if visibleCount < 5 {
			visibleCount = 5
		}

		startIndex := 0
		if a.fileBrowserIndex >= visibleCount {
			startIndex = a.fileBrowserIndex - visibleCount/2
		}
		if startIndex > len(displayFiles)-visibleCount {
			startIndex = len(displayFiles) - visibleCount
		}
		if startIndex < 0 {
			startIndex = 0
		}

		endIndex := startIndex + visibleCount
		if endIndex > len(displayFiles) {
			endIndex = len(displayFiles)
		}

		for i := startIndex; i < endIndex; i++ {
			df := displayFiles[i]

			var selectionMark string
			if df.selected {
				selectionMark = "✓ "
			} else if i == a.fileBrowserIndex {
				selectionMark = "→ "
			} else {
				selectionMark = "  "
			}

			sizeStr := formatFileSize(df.file.Size)

			if df.isFolder {
				icon := "📁"
				var line string
				if i == a.fileBrowserIndex {
					line = selectedFolderStyle.Render(fmt.Sprintf("%s%s %s", selectionMark, icon, df.file.ServerFilename))
				} else {
					line = folderStyle.Render(fmt.Sprintf("%s%s %s", selectionMark, icon, df.file.ServerFilename))
				}
				b.WriteString(line)
			} else {
				icon := "🎵"
				var line string
				if i == a.fileBrowserIndex {
					line = selectedStyle.Render(fmt.Sprintf("%s%s %-40s %8s", selectionMark, icon, df.file.ServerFilename, sizeStr))
				} else {
					line = normalStyle.Render(fmt.Sprintf("%s%s %-40s %8s", selectionMark, icon, df.file.ServerFilename, sizeStr))
				}
				b.WriteString(line)
			}

			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	if len(a.selectedFiles) > 0 {
		var totalSize int64
		for _, f := range a.selectedFiles {
			totalSize += f.Size
		}

		selectedStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4CAF50")).
			Bold(true).
			Padding(0, 2)

		b.WriteString(selectedStyle.Render(fmt.Sprintf("✓ 已选择 %d 个文件 · %s", len(a.selectedFiles), formatFileSize(totalSize))))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	b.WriteString(helpStyle.Render(" ↑↓ 选择  |  Enter 进入  |  Space 选择  |  A 全选文件夹  |  S 保存  |  Esc 取消 "))

	return b.String()
}
