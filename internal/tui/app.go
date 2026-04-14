package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
	"github.com/liuguanyu/pan-player-cmd/internal/config"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/player"
	"github.com/liuguanyu/pan-player-cmd/internal/playlist"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
	"github.com/skip2/go-qrcode"
)

// App TUI 应用
type App struct {
	config   *config.Config
	api      *api.BaiduPanClient
	player   *player.Player
	playlist *playlist.Manager

	// UI 状态
	currentView ViewType
	width       int
	height      int
	ready       bool

	// 播放列表状态
	playlists       []models.Playlist
	currentPlaylist *models.Playlist
	selectedIndex   int
	scrollOffset    int // 播放列表滚动偏移量

	// 登录状态
	isLoggedIn    bool
	deviceAuth    *api.OAuthDeviceAuth
	qrCode        string
	pollingCancel context.CancelFunc
	userInfo      *models.UserInfo
	loginError    string

	// 输入状态
	inputBuffer   string
	inputPrompt   string
	inputCallback func(string) tea.Cmd

	// 歌词状态
	currentLyrics []models.LyricLine
	lyricsOffset  int

	// 文件浏览状态
	currentPath      string
	files            []api.FileInfo
	selectedFiles    []api.FileInfo
	fileBrowserIndex int
	loadingFiles     bool
	loadingDots      int // 加载动画的点的数量

	// 当前歌曲跟踪（用于检测歌曲切换）
	lastTrackFsID int64

	// 版本 ID 用于强制重新渲染
	version int

	// 流光效果状态
	splashText      string
	splashIndex     int
	splashAnimating bool

	// 播放器界面流光动画帧
	shimmerFrame int

	// 播放状态持久化
	lastPlaybackState *models.PlaybackState

	// 歌词管理器
	lyricsManager *lyrics.Manager
	// 歌词搜索UI状态
	lyricSearchUI LyricSearchUI
	// 歌词搜索词
	lyricSearchKeyword string
	// 歌词搜索词光标位置
	lyricSearchCursor int
}

// LyricSearchUI 歌词搜索UI状态
type LyricSearchUI struct {
	Results       []models.LyricSearchResult
	SelectedIndex int
	Visible       bool
	Editing       bool // 是否正在编辑搜索词
}

// ViewType 视图类型
type ViewType int

const (
	ViewLogin ViewType = iota
	ViewPlaylist
	ViewPlayer
	ViewHelp
	ViewCreatePlaylist
	ViewDeletePlaylist
	ViewRenamePlaylist
	ViewFileBrowser
	ViewSplash
	ViewLyricSearch
)

// NewApp 创建新的 TUI 应用
func NewApp(cfg *config.Config) *App {
	apiClient := api.NewBaiduPanClient(cfg.API.BaiduPan.TokenFile)
	pl := player.NewPlayer(&player.PlayerConfig{
		AudioDevice: cfg.Player.AudioDevice,
		CacheDir:    cfg.App.DataDir + "/cache",
	}, apiClient)
	plManager := playlist.NewManager(cfg.App.DataDir)

	app := &App{
		config:         cfg,
		api:            apiClient,
		player:         pl,
		playlist:       plManager,
		currentView:    ViewLogin, // 直接进入登录界面
		lyricsManager:  lyrics.NewManager(),
	}

	// 设置歌曲播放回调，用于更新最近播放记录
	pl.SetOnTrackPlay(func(track *models.PlaylistItem) {
		app.updateRecentPlaylist(track)
	})

	return app
}

// Init 初始化
func (a *App) Init() tea.Cmd {
	// 检查登录状态，如果已登录则直接进入播放列表
	return a.checkLogin()
}

// Update 更新状态
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.ready = true

	case LoginSuccessMsg:
		a.isLoggedIn = true
		a.userInfo = msg.UserInfo
		a.currentView = ViewPlaylist
		// 登录成功后加载播放列表
		return a, a.loadPlaylists()

	case LoginErrorMsg:
		a.loginError = msg.Error
		return a, nil

	case DeviceCodeMsg:
		a.deviceAuth = msg.DeviceAuth
		// 生成二维码
		qrURL := fmt.Sprintf("%s?code=%s", msg.DeviceAuth.VerificationURL, msg.DeviceAuth.UserCode)
		a.qrCode = generateQRCode(qrURL)
		// 开始轮询，使用百度API返回的interval值，默认5秒
		interval := 5 * time.Second
		if msg.DeviceAuth.Interval > 0 {
			interval = time.Duration(msg.DeviceAuth.Interval) * time.Second
		}
		return a, a.startPolling(msg.DeviceAuth.DeviceCode, interval)
	
	case PlaylistsLoadedMsg:
		a.playlists = msg.Playlists
		// 不要在这里设置 currentPlaylist，保留之前的选择
		// 如果没有选中项且列表不为空，初始化 selectedIndex
		if len(a.playlists) > 0 && a.selectedIndex >= len(a.playlists) {
			a.selectedIndex = 0
		}
		// 重置滚动偏移，因为播放列表顺序可能已改变（"最近播放"添加到首位）
		a.scrollOffset = 0
		// 确保选中的项可见
		if len(a.playlists) > 0 {
			visibleHeight := a.height - 6
			if visibleHeight < 5 {
				visibleHeight = 5
			}
			if a.selectedIndex >= visibleHeight {
				a.scrollOffset = a.selectedIndex - visibleHeight + 1
			}
		}
		return a, nil
	case ForceRenderMsg:
		// 强制重新渲染
		a.version++
		return a, nil

	case PlayerUpdateMsg:
		// 播放器状态更新
		if a.currentView == ViewPlayer {
			// 强制重新渲染以更新播放进度
			a.version++
			a.shimmerFrame++

			// 检查歌曲是否切换
			state := a.player.GetState()
			if state.CurrentSong != nil && state.CurrentSong.FsID != a.lastTrackFsID {
				// 歌曲已切换，更新跟踪ID并加载新歌词
				a.lastTrackFsID = state.CurrentSong.FsID
				go a.loadLyricsForTrack(state.CurrentSong)
			}

			// 继续接收更新
			return a, a.startPlayerUpdateTicker()
		}
		return a, nil

	case FilesLoadedMsg:
		a.files = msg.Files
		a.currentPath = msg.Path
		a.loadingFiles = false
		a.loadingDots = 0
		// 重置选择索引
		a.fileBrowserIndex = 0
		return a, nil

	case FolderFilesLoadedMsg:
		// 添加文件夹中的所有音频文件到已选择列表
		for _, file := range msg.Files {
			found := false
			for _, f := range a.selectedFiles {
				if f.FsID == file.FsID {
					found = true
					break
				}
			}
			if !found {
				a.selectedFiles = append(a.selectedFiles, file)
			}
		}
		a.loadingFiles = false
		a.loadingDots = 0
		return a, nil

	case TickMsg:
		// 流光效果定时器
		if a.currentView == ViewSplash && a.splashAnimating {
			if a.splashIndex < len(a.splashText) {
				a.splashIndex++
				return a, a.tick()
			} else {
				// 动画完成，等待一段时间后进入登录视图
				a.splashAnimating = false
				return a, a.waitForSplash()
			}
		}

		// 播放器状态更新
		if a.currentView == ViewPlayer {
			// 强制重新渲染以更新播放进度
			a.version++
			a.shimmerFrame++

			// 检查歌曲是否切换
			state := a.player.GetState()
			if state.CurrentSong != nil && state.CurrentSong.FsID != a.lastTrackFsID {
				// 歌曲已切换，更新跟踪ID并加载新歌词
				a.lastTrackFsID = state.CurrentSong.FsID
				a.currentLyrics = nil // 清空旧歌词
				go a.loadLyricsForTrack(state.CurrentSong)
			}

			return a, a.startPlayerUpdateTicker()
		}

	case SplashAnimationDoneMsg:
		// 流光动画完成，切换到登录视图
		a.currentView = ViewLogin
		return a, a.checkLogin()

	case LoadingAnimationMsg:
		// 更新加载动画
		if a.loadingFiles {
			a.loadingDots++
			return a, a.tickLoadingAnimation()
		}
	}

	return a, tea.Batch(cmds...)
}

// View 渲染视图
func (a *App) View() string {
	if !a.ready {
		return "Loading..."
	}

	switch a.currentView {
	case ViewLogin:
		return a.renderLoginView()
	case ViewPlaylist:
		return a.renderPlaylistView()
	case ViewPlayer:
		return a.renderPlayerView()
	case ViewHelp:
		return a.renderHelpView()
	case ViewCreatePlaylist:
		return a.renderInputView()
	case ViewDeletePlaylist:
		return a.renderDeleteConfirmView()
	case ViewRenamePlaylist:
		return a.renderRenameView()
	case ViewFileBrowser:
		return a.renderFileBrowserView()
	case ViewSplash:
		return a.renderSplashView()
	case ViewLyricSearch:
		return a.renderLyricSearchView()
	default:
		return "Unknown view"
	}
}

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
		// 标题：2行，状态栏：2行，帮助栏：2行，共预留6行
		visibleHeight := a.height - 6
		if visibleHeight < 5 {
			visibleHeight = 5 // 最少显示5个
		}

		// 计算滚动范围，确保选中项可见
		if a.selectedIndex < a.scrollOffset {
			a.scrollOffset = a.selectedIndex
		} else if a.selectedIndex >= a.scrollOffset+visibleHeight {
			a.scrollOffset = a.selectedIndex - visibleHeight + 1
		}

		// 确保scrollOffset不超出范围
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

		// 只渲染可见区域的列表项
		endIndex := a.scrollOffset + visibleHeight
		if endIndex > len(a.playlists) {
			endIndex = len(a.playlists)
		}

		for i := a.scrollOffset; i < endIndex; i++ {
			pl := a.playlists[i]
			// 计算总大小
			var totalSize int64
			for _, item := range pl.Items {
				totalSize += item.Size
			}
			sizeStr := formatFileSize(totalSize)

			// 列表项内容
			var itemLine string
			if i == a.selectedIndex {
				// 选中项：使用箭头和背景色
				itemLine = selectedStyle.Render(fmt.Sprintf("→ %s  (%d首 · %s)", pl.Name, len(pl.Items), sizeStr))
			} else {
				// 未选中项
				itemLine = normalStyle.Render(fmt.Sprintf("  %s  (%d首 · %s)", pl.Name, len(pl.Items), sizeStr))
			}
			b.WriteString(itemLine)
			b.WriteString("\n")
		}

		// 如果有更多项，显示提示
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

// renderPlayerView 渲染播放器视图
func (a *App) renderPlayerView() string {
	var b strings.Builder

	state := a.player.GetState()

	// 播放列表信息
	if state.CurrentPlaylistName != "" {
		playlistStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#06BF54"))
		b.WriteString(playlistStyle.Render(fmt.Sprintf("正在播放: %s", state.CurrentPlaylistName)))
		b.WriteString("\n\n")
	}

	// 显示播放列表内容（当前播放项附近5首）
	playlist := a.player.GetCurrentPlaylist()
	if playlist != nil && len(playlist.Items) > 0 {
		currentIndex := a.player.GetCurrentIndex()
		startIndex := max(0, currentIndex-2)
		endIndex := min(len(playlist.Items), currentIndex+3)

		listStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
		currentStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#06BF54")).
			Bold(true)

		for i := startIndex; i < endIndex; i++ {
			item := playlist.Items[i]
			fileName := item.ServerFileName
			fileSize := fmt.Sprintf("%.1fMB", float64(item.Size)/1024/1024)

			if i == currentIndex {
				b.WriteString(currentStyle.Render(fmt.Sprintf("→ %d. %s (%s)", i+1, fileName, fileSize)))
			} else {
				b.WriteString(listStyle.Render(fmt.Sprintf("  %d. %s (%s)", i+1, fileName, fileSize)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// 播放模式（放在歌名上面）
	if state.CurrentSong != nil {
		modeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
		var modeText string
		switch state.PlaybackMode {
		case models.PlaybackModeOrder:
			modeText = "顺序播放"
		case models.PlaybackModeRandom:
			modeText = "随机播放"
		case models.PlaybackModeSingle:
			modeText = "单曲循环"
		}
		b.WriteString(modeStyle.Render(fmt.Sprintf("模式: %s", modeText)))
		b.WriteString("\n")

		// 网盘文件路径（虚拟目录）
		pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
		if state.CurrentSong.Path != "" {
			b.WriteString(pathStyle.Render(fmt.Sprintf("路径: %s", state.CurrentSong.Path)))
			b.WriteString("\n")
		}

		// 当前歌曲信息
		songText := fmt.Sprintf("正在播放: %s", state.CurrentSong.ServerFileName)
		b.WriteString(renderShimmerText(songText, a.shimmerFrame, false))
		b.WriteString("\n")
	}

	// 进度条（单行，包含播放状态、进度、时间、音量）
	progressBar := a.renderProgressBar(state)
	b.WriteString(progressBar)
	b.WriteString("\n\n")

	// 歌词显示（当前行高亮，显示前后2行）
	if state.ShowLyrics && len(a.currentLyrics) > 0 {
		lyricsView := a.renderLyrics(state.CurrentTime)
		b.WriteString(lyricsView)
		b.WriteString("\n")
	}

	// 播放控制提示
	b.WriteString("\n")
	controlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAA"))

	// 动态构建快捷键提示 - 只在有歌词时显示 'u' 键
	var shortcutString string
	if state.ShowLyrics && len(a.currentLyrics) > 0 {
		shortcutString = "[空格]暂停/恢复  [n]下一曲  [p]上一曲  [↑/↓]音量  [m]模式  [l]歌词  [s]搜索歌词  [u]上传歌词  [Ctrl+Z]后台挂起  [Esc]返回"
	} else {
		shortcutString = "[空格]暂停/恢复  [n]下一曲  [p]上一曲  [↑/↓]音量  [m]模式  [l]歌词  [s]搜索歌词  [Ctrl+Z]后台挂起  [Esc]返回"
	}
	b.WriteString(controlStyle.Render(shortcutString))

	return b.String()
}

// renderProgressBar 渲染进度条（单行，包含状态、进度、时间、音量）
func (a *App) renderProgressBar(state *models.PlaybackState) string {
	current := state.CurrentTime
	duration := state.Duration

	// 播放状态文字
	var statusText string
	if state.IsPlaying {
		statusText = "播放中"
	} else {
		statusText = "暂停中"
	}

	// 进度条
	var bar string
	if duration <= 0 {
		bar = strings.Repeat("░", 30)
		return fmt.Sprintf("%s [%s] 00:00 / 00:00 | 音量: %d%%",
			statusText, bar, int(state.Volume*100))
	}

	// 确保百分比不超过 1
	percentage := current / duration
	if percentage > 1.0 {
		percentage = 1.0
	}
	if percentage < 0 {
		percentage = 0
	}

	// 进度条宽度（固定30个字符宽度）
	barWidth := 30
	filled := int(percentage * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	if empty < 0 {
		empty = 0
	}

	// 使用░字符，已播放部分用绿色，未播放用灰色
	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	remainingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#333"))

	bar = progressStyle.Render(strings.Repeat("░", filled)) + remainingStyle.Render(strings.Repeat("░", empty))

	currentTime := formatTime(current)
	totalTime := formatTime(duration)
	volume := int(state.Volume * 100)

	return fmt.Sprintf("%s [%s] %s / %s | 音量: %d%%",
		statusText, bar, currentTime, totalTime, volume)
}

// renderControls 渲染播放控制
func (a *App) renderControls(state *models.PlaybackState) string {
	var b strings.Builder

	playIcon := "▶"
	if state.IsPlaying {
		playIcon = "⏸"
	}

	volume := int(state.Volume * 100)
	mode := string(state.PlaybackMode)

	controlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(controlStyle.Render(fmt.Sprintf(" %s 上一首 下一首 | 音量: %d%% | 模式: %s ",
		playIcon, volume, mode)))

	return b.String()
}

// renderLyrics 渲染歌词
func (a *App) renderLyrics(currentTime float64) string {
	var b strings.Builder

	// 如果没有歌词，直接返回
	if a.currentLyrics == nil || len(a.currentLyrics) == 0 {
		return ""
	}

	// 获取当前歌词行
	currentIndex := lyrics.GetCurrentLyricIndex(a.currentLyrics, currentTime)

	// 如果没有找到有效歌词行，显示空
	if currentIndex < 0 {
		return ""
	}

	// 显示当前行及前后几行
	visibleLines := 5
	startIndex := currentIndex - visibleLines/2
	if startIndex < 0 {
		startIndex = 0
	}

	endIndex := startIndex + visibleLines
	if endIndex > len(a.currentLyrics) {
		endIndex = len(a.currentLyrics)
	}

	// 确保 startIndex 在有效范围内
	if startIndex >= len(a.currentLyrics) {
		return ""
	}

	lyricsStyle := lipgloss.NewStyle().Padding(0, 2)

	for i := startIndex; i < endIndex; i++ {
		// 安全检查索引
		if i < 0 || i >= len(a.currentLyrics) {
			continue
		}
		line := a.currentLyrics[i]
		text := line.Text
		if text == "" {
			text = "..."
		}

		if i == currentIndex {
			b.WriteString(renderShimmerText("→ "+text, a.shimmerFrame, false))
		} else {
			b.WriteString(lyricsStyle.Render("  " + text))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderShimmerText 生成带流光效果的文本
// frame: 当前动画帧，用于推进颜色渐变波峰
// spinnerPrefix: 是否在文本前加前导动态旋转字符
func renderShimmerText(text string, frame int, spinnerPrefix bool) string {
	// 前导旋转字符（仿 Claude CLI 效果）
	// 使用十字星或者常见加载字符
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	// 基础颜色（蓝色系）
	baseColor := "#1E6FD9"
	// 流光波峰（从暗到亮再到暗）
	shimmerWave := []string{
		"#1E6FD9", "#2A86F0", "#45A3FF", "#66B8FF",
		"#8CCBFF", // 波峰中心（最亮）
		"#66B8FF", "#45A3FF", "#2A86F0", "#1E6FD9",
	}

	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}

	// 设定整个循环的周期长度，例如 40 个字符跨度或者文本长度加上波长
	// 这样光束划过文本后会有一段暗色的间隔再出现下一次光束
	cycleLen := n + len(shimmerWave)
	if cycleLen < 30 {
		cycleLen = 30 // 保证一个合理的最小循环周期
	}

	var styledText strings.Builder
	for i, ch := range runes {
		// 计算当前字符在光束循环中的位置
		// 随着 frame 增加，波浪向右移动
		pos := ((i-frame)%cycleLen + cycleLen) % cycleLen

		color := baseColor
		// 如果落在了波峰的范围内
		if pos < len(shimmerWave) {
			color = shimmerWave[pos]
		}

		charStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
		styledText.WriteString(charStyle.Render(string(ch)))
	}

	if spinnerPrefix {
		// 让 spinner 慢一点，frame / 2
		spinnerIdx := ((frame/2)%len(spinnerChars) + len(spinnerChars)) % len(spinnerChars)
		spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#45A3FF")) // 使用中间蓝色
		return spinnerStyle.Render(spinnerChars[spinnerIdx]) + " " + styledText.String()
	}
	return styledText.String()
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

// renderInputView 渲染输入视图
func (a *App) renderInputView() string {
	var b strings.Builder

	// 标题区域
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render(a.inputPrompt))
	b.WriteString("\n\n")

	// 输入框
	inputBox := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#333")).
		Padding(0, 2).
		Render(a.inputBuffer + "█")

	b.WriteString(inputBox)
	b.WriteString("\n\n")

	// 提示信息
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	b.WriteString(hintStyle.Render("💡 输入播放列表名称，按 Enter 继续，Esc 取消"))

	return b.String()
}

// renderDeleteConfirmView 渲染删除确认视图
func (a *App) renderDeleteConfirmView() string {
	var b strings.Builder

	// 警告标题
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF5252")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render("⚠️  确认删除"))
	b.WriteString("\n\n")

	if a.currentPlaylist != nil {
		// 提示信息
		warnStyle := lipgloss.NewStyle().
			Padding(0, 2)

		// 列表名称样式
		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5252")).
			Bold(true)

		// 歌曲数量样式
		countStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888"))

		var content strings.Builder
		content.WriteString("确定要删除播放列表吗？\n\n")
		content.WriteString(nameStyle.Render(fmt.Sprintf("📀 %s", a.currentPlaylist.Name)))
		content.WriteString("\n")
		content.WriteString(countStyle.Render(fmt.Sprintf("共 %d 首歌曲", len(a.currentPlaylist.Items))))

		b.WriteString(warnStyle.Render(content.String()))
		b.WriteString("\n\n")

		// 操作提示
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
		b.WriteString(helpStyle.Render("按 Y 确认删除  |  其他键取消"))
	}

	return b.String()
}

// renderRenameView 渲染重命名视图
func (a *App) renderRenameView() string {
	var b strings.Builder

	// 标题区域
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render("重命名播放列表"))
	b.WriteString("\n\n")

	// 当前播放列表名称 - 根据 selectedIndex 显示
	if a.selectedIndex < len(a.playlists) {
		currentStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Padding(0, 2)
		b.WriteString(currentStyle.Render(fmt.Sprintf("当前名称: %s", a.playlists[a.selectedIndex].Name)))
		b.WriteString("\n\n")
	}

	// 输入框
	inputBox := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("#333")).
		Padding(0, 2).
		Render(a.inputBuffer + "█")

	b.WriteString(inputBox)
	b.WriteString("\n\n")

	// 提示信息
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))
	b.WriteString(hintStyle.Render("💡 输入新名称，按 Enter 确认，Esc 取消"))

	return b.String()
}

// renderFileBrowserView 渲染文件浏览器视图
func (a *App) renderFileBrowserView() string {
	var b strings.Builder

	// 标题区域
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		Padding(0, 2)

	b.WriteString(titleStyle.Render(fmt.Sprintf("📂 选择文件 · %s", a.inputBuffer)))
	b.WriteString("\n")

	// 当前路径（面包屑样式）
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
		// 加载动画
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

	// 过滤音频文件和文件夹
	audioFormats := []string{".mp3", ".m4a", ".flac", ".wav", ".ogg", ".aac", ".wma"}

	var displayFiles []struct {
		file     api.FileInfo
		isAudio  bool
		isFolder bool
		selected bool
	}

	// 先添加文件夹
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

	// 然后添加音频文件
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
		// 样式定义
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

		visibleCount := a.height - 15 // 保留空间给标题、路径和提示
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

			// 选中标记
			var selectionMark string
			if df.selected {
				selectionMark = "✓ "
			} else if i == a.fileBrowserIndex {
				selectionMark = "→ "
			} else {
				selectionMark = "  "
			}

			// 文件大小
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

	// 已选择的文件统计
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

	// 底部提示
	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666"))
	b.WriteString(helpStyle.Render(" ↑↓ 选择  |  Enter 进入  |  Space 选择  |  A 全选文件夹  |  S 保存  |  Esc 取消 "))

	return b.String()
}

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
		return a, tea.Quit

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
			a.currentView = ViewHelp
		}
		return a, nil

	case "esc":
		if a.currentView == ViewHelp || a.currentView == ViewPlayer {
			// 如果是从播放器视图返回，保存当前播放状态
			if a.currentView == ViewPlayer {
				a.lastPlaybackState = a.player.GetState()
			}
			a.currentView = ViewPlaylist
			a.inputBuffer = "" // 清空输入缓冲
			// 重新加载播放列表以更新"最近播放"
			return a, a.loadPlaylists()
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
				return a, a.startPlayerUpdateTicker()
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
								a.player.SetCurrentIndex(0)
								a.player.LoadTrack(context.Background(), selectedPlaylist.Items[0])
								a.loadLyricsForTrack(selectedPlaylist.Items[0])
							}()
						}
					} else {
						// 没有保存的状态，从头开始播放
						go func() {
							a.player.SetCurrentIndex(0)
							a.player.LoadTrack(context.Background(), selectedPlaylist.Items[0])
							a.loadLyricsForTrack(selectedPlaylist.Items[0])
						}()
					}
				}
			}
			a.currentView = ViewPlayer
			// 启动播放器状态更新定时器
			return a, a.startPlayerUpdateTicker()
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

	case "s":
		if a.currentView == ViewPlayer {
			// 切换到歌词搜索视图
			a.currentView = ViewLyricSearch
			return a, a.handleLyricSearch()
		}
		return a, nil

	case "u":
		if a.currentView == ViewPlayer {
			// 上传歌词到百度网盘
			a.handleLyricUpload()
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
		// 删除最后一个字符
		if len(a.inputBuffer) > 0 {
			a.inputBuffer = a.inputBuffer[:len(a.inputBuffer)-1]
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

// Run 运行应用
func (a *App) Run() error {
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// Messages
type LoginSuccessMsg struct {
	UserInfo *models.UserInfo
}

type LoginErrorMsg struct {
	Error string
}

type DeviceCodeMsg struct {
	DeviceAuth *api.OAuthDeviceAuth
}

type PlaylistsLoadedMsg struct {
	Playlists []models.Playlist
}

// ForceRenderMsg 强制重新渲染消息
type ForceRenderMsg struct{}

// TickMsg 定时器消息
type TickMsg struct{}

// SplashAnimationDone 流光动画完成消息
type SplashAnimationDoneMsg struct{}

// LoadingAnimationMsg 加载动画消息
type LoadingAnimationMsg struct{}

// PlayerUpdateMsg 播放器状态更新消息
type PlayerUpdateMsg struct{}

// SongChangedMsg 歌曲切换消息
type SongChangedMsg struct {
	Track *models.PlaylistItem
}

// FilesLoadedMsg 文件列表加载完成消息
type FilesLoadedMsg struct {
	Files []api.FileInfo
	Path  string
}

// FileSelectionChangedMsg 文件选择变化消息
type FileSelectionChangedMsg struct {
	Selected []api.FileInfo
}

// FolderFilesLoadedMsg 文件夹文件加载完成消息
type FolderFilesLoadedMsg struct {
	Files []api.FileInfo
}

// Commands
func (a *App) checkLogin() tea.Cmd {
	return func() tea.Msg {
		// 检查是否已登录
		if err := a.api.LoadToken(); err == nil {
			// 已登录，获取用户信息
			return LoginSuccessMsg{UserInfo: &models.UserInfo{BaiduName: "用户"}}
		}

		// 未登录，获取设备码
		deviceAuth, err := a.api.GetDeviceCode(
			a.config.API.BaiduPan.ClientID,
			a.config.API.BaiduPan.ClientSecret,
		)
		if err != nil {
			return LoginErrorMsg{Error: err.Error()}
		}

		return DeviceCodeMsg{DeviceAuth: deviceAuth}
	}
}

func (a *App) startPolling(deviceCode string, interval time.Duration) tea.Cmd {
	return func() tea.Msg {
		// 创建可取消的上下文
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		a.pollingCancel = cancel

		tokenResp, err := a.api.WaitForAuth(
			ctx,
			a.config.API.BaiduPan.ClientID,
			a.config.API.BaiduPan.ClientSecret,
			deviceCode,
			interval,
			func() {
				// 轮询进度回调（可用于更新 UI）
			},
		)

		if err != nil {
			return LoginErrorMsg{Error: err.Error()}
		}

		// 保存令牌
		tokenInfo := &api.TokenInfo{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			ExpiresIn:    tokenResp.ExpiresIn,
		}
		if err := a.api.SaveToken(tokenInfo); err != nil {
			return LoginErrorMsg{Error: "保存令牌失败: " + err.Error()}
		}

		return LoginSuccessMsg{UserInfo: &models.UserInfo{BaiduName: "用户"}}
	}
}

func (a *App) loadPlaylists() tea.Cmd {
	return func() tea.Msg {
		if err := a.playlist.LoadPlaylists(); err == nil {
			return PlaylistsLoadedMsg{Playlists: a.playlist.GetPlaylists()}
		}
		return nil
	}
}

// startSplashAnimation 开始流光动画
func (a *App) startSplashAnimation() tea.Cmd {
	a.splashAnimating = true
	a.splashIndex = 0
	return a.tick()
}

// tick 定时器
func (a *App) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// waitForSplash 等待流光动画结束
func (a *App) waitForSplash() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return SplashAnimationDoneMsg{}
	})
}

// startPlayerUpdateTicker 启动播放器状态更新定时器
func (a *App) startPlayerUpdateTicker() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return PlayerUpdateMsg{}
	})
}

// max 返回两个整数中的最大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min 返回两个整数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatTime 格式化时间
func formatTime(seconds float64) string {
	minutes := int(seconds / 60)
	secs := int(seconds) % 60
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// formatFileSize 格式化文件大小
func formatFileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/1024/1024)
	} else {
		return fmt.Sprintf("%.1fGB", float64(bytes)/1024/1024/1024)
	}
}

// generateQRCode 生成二维码 ASCII 字符串
func generateQRCode(content string) string {
	// 生成二维码图片
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "二维码生成失败"
	}

	// 转换为小尺寸 ASCII 字符串，去除边框避免错位
	qr.DisableBorder = true
	qrStr := qr.ToSmallString(false)

	// 确保每行都有回车符，避免显示错位
	lines := strings.Split(qrStr, "\n")
	var result strings.Builder
	for i, line := range lines {
		if line == "" {
			continue
		}
		result.WriteString(line)
		// 如果不是最后一行，添加回车换行
		if i < len(lines)-1 {
			result.WriteString("\r\n")
		}
	}

	return result.String()
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
		// 删除最后一个字符
		if len(a.inputBuffer) > 0 {
			a.inputBuffer = a.inputBuffer[:len(a.inputBuffer)-1]
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

// getSelectedFile 获取当前选中的文件
func (a *App) getSelectedFile() *api.FileInfo {
	audioFormats := []string{".mp3", ".m4a", ".flac", ".wav", ".ogg", ".aac", ".wma"}

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
			for _, format := range audioFormats {
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

// loadFiles 加载文件列表
func (a *App) loadFiles(path string) tea.Cmd {
	a.loadingFiles = true
	a.loadingDots = 0
	return tea.Batch(
		a.tickLoadingAnimation(),
		func() tea.Msg {
			files, err := a.api.GetFileList(path, 1, 1000)
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
			files, err := a.api.GetAudioFilesRecursive(folderPath)
			if err != nil {
				return FolderFilesLoadedMsg{Files: nil}
			}
			return FolderFilesLoadedMsg{Files: files}
		},
	)
}

// updateRecentPlaylist 更新最近播放列表
func (a *App) updateRecentPlaylist(track *models.PlaylistItem) {
	// 获取最近播放列表
	recentPlaylist := a.playlist.GetPlaylist("最近播放")
	if recentPlaylist == nil {
		return
	}

	// 创建新的最近播放列表（最多保留30首）
	var recentSongs []*models.PlaylistItem
	if len(recentPlaylist.Items) > 0 {
		// 将现有歌曲复制到新列表，但移除当前歌曲（如果存在）
		for _, item := range recentPlaylist.Items {
			if item.FsID != track.FsID {
				recentSongs = append(recentSongs, item)
			}
		}
	}

	// 将新歌曲添加到最前面
	recentSongs = append([]*models.PlaylistItem{track}, recentSongs...)

	// 限制最多30首
	if len(recentSongs) > 30 {
		recentSongs = recentSongs[:30]
	}

	// 更新最近播放列表
	err := a.playlist.UpdateRecentSongs(recentSongs)
	if err != nil {
		utils.GetLogger().Error("更新最近播放列表失败: %v", err)
	}
}
