package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
	"github.com/liuguanyu/pan-player-cmd/internal/config"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/player"
	"github.com/liuguanyu/pan-player-cmd/internal/playlist"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
	"github.com/liuguanyu/pan-player-cmd/internal/visualizer"
)

// App TUI 应用
type App struct {
	config   *config.Config
	api      *api.BaiduPanClient
	player   player.TuiPlayer
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

	// 终端标题跟踪（记录上一次设置的标题，避免每个 tick 重复发送）
	lastWindowTitle string

	// 版本 ID 用于强制重新渲染
	version int

	// 流光效果状态
	splashText      string
	splashIndex     int
	splashAnimating bool

	// 播放器界面流光动画帧
	shimmerFrame int

	// 播放状态持久化
	lastPlaybackState models.PlaybackState

	// 状态快照订阅(不可变快照,所有渲染只读此副本)
	snapshotSub  <-chan models.PlaybackState
	lastSnapshot models.PlaybackState

	// 歌词管理器
	lyricsManager *lyrics.Manager
	// 歌词搜索UI状态
	lyricSearchUI LyricSearchUI
	// 歌词搜索词
	lyricSearchKeyword string
	// 歌词搜索词光标位置
	lyricSearchCursor int

	// 消息显示状态
	currentMessage string
	messageTimeout time.Time

	// 歌词上传确认
	awaitingLyricUploadConfirm bool
	uploadTargetPath           string
	uploadLyricsContent        string

	// 可视化
	sheepVis *visualizer.ActiveVisualizer
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

	// Create visualizer (may not be supported on all terminals)
	sheepVis := visualizer.NewActiveVisualizer()

	pl := player.NewPlayer(&player.PlayerConfig{
		AudioDevice: cfg.Player.AudioDevice,
		CacheDir:    cfg.App.DataDir + "/cache",
		Speed:       cfg.Player.PlaybackRate,
	}, apiClient)

	if sheepVis.IsSupported() {
		pl.SetTapWrapper(sheepVis.CreateTap)
	}
	plManager := playlist.NewManager(cfg.App.DataDir)

	app := &App{
		config:        cfg,
		api:           apiClient,
		player:        pl,
		playlist:      plManager,
		currentView:   ViewLogin, // 直接进入登录界面
		lyricsManager: lyrics.NewManager(),
		sheepVis:      sheepVis,
	}

	// 设置歌曲播放回调，用于更新最近播放记录
	pl.SetOnTrackPlay(func(track *models.PlaylistItem) {
		app.updateRecentPlaylist(track)
	})

	// 订阅不可变状态快照,渲染只读 lastSnapshot,彻底移除对 GetState 共享指针的依赖
	app.snapshotSub = pl.Subscribe()

	return app
}

// Init 初始化
func (a *App) Init() tea.Cmd {
	// 启动快照消费 + 检查登录状态
	return tea.Batch(a.waitForSnapshot(), a.checkLogin())
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
		a.sheepVis.SetTermHeight(msg.Height)

	case LoginSuccessMsg:
		a.isLoggedIn = true
		a.userInfo = msg.UserInfo
		a.currentView = ViewPlaylist
		a.sheepVis.Start()
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

	case lyricSearchDoneMsg:
		if msg.err != nil {
			utils.GetLogger().Error("歌词搜索失败: %v", msg.err)
			a.lyricSearchUI.Results = nil
		} else if len(msg.results) == 0 {
			utils.GetLogger().Info("未找到歌词: %s", msg.keyword)
			a.lyricSearchUI.Editing = true
			a.lyricSearchUI.Results = nil
		} else {
			a.lyricSearchUI = LyricSearchUI{
				Results:       msg.results,
				SelectedIndex: 0,
				Visible:       true,
				Editing:       false,
			}
		}
		// 强制重新渲染
		a.version++
		return a, nil

	case lyricDownloadDoneMsg:
		// 无论成功失败，都先退出搜索页，避免用户看到"按了回车没反应"
		a.currentView = ViewPlayer
		a.sheepVis.SetVisible(true)
		a.lyricSearchUI.Visible = false
		a.lyricSearchUI.Editing = false

		if msg.err != nil {
			utils.GetLogger().Error("获取歌词失败: %v", msg.err)
			a.version++
			a.showMessage("下载失败: " + msg.err.Error())
			// 回到播放界面，恢复进度轮询与终端标题
			return a, tea.Sequence(tea.ClearScreen, a.resumePlayerUpdates())
		}

		// 解析并显示歌词
		parsed := lyrics.ParseLRC(msg.lrcContent)
		a.player.SetLyrics(msg.lrcContent, parsed.Lines, true)
		a.currentLyrics = parsed.Lines

		// 强制重新渲染
		a.version++
		a.showMessage("歌词已加载，按 'u' 上传至网盘")
		// 回到播放界面，恢复进度轮询与终端标题
		return a, tea.Sequence(tea.ClearScreen, a.resumePlayerUpdates())

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

		// 检查歌曲是否切换（从后台恢复时）
		if a.currentView == ViewPlayer {
			state := a.lastSnapshot
			if state.CurrentSong != nil && state.CurrentSong.FsID != a.lastTrackFsID {
				// 歌曲已切换，更新跟踪ID并加载新歌词
				a.lastTrackFsID = state.CurrentSong.FsID
				go a.loadLyricsForTrack(state.CurrentSong)
			}
		}

		return a, a.updateWindowTitleCmd()

	case PlayerUpdateMsg:
		// 播放器状态更新
		if a.currentView == ViewPlayer {
			// 强制重新渲染以更新播放进度
			a.version++
			a.shimmerFrame++

			// 检查歌曲是否切换
			state := a.lastSnapshot
			if state.CurrentSong != nil && state.CurrentSong.FsID != a.lastTrackFsID {
				// 歌曲已切换，更新跟踪ID并加载新歌词
				a.lastTrackFsID = state.CurrentSong.FsID
				go a.loadLyricsForTrack(state.CurrentSong)
			}

			// 继续接收更新
			return a, a.resumePlayerUpdates()
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
			state := a.lastSnapshot
			if state.CurrentSong != nil && state.CurrentSong.FsID != a.lastTrackFsID {
				// 歌曲已切换，更新跟踪ID并加载新歌词
				a.lastTrackFsID = state.CurrentSong.FsID
				a.currentLyrics = nil // 清空旧歌词
				go a.loadLyricsForTrack(state.CurrentSong)
			}

			return a, a.resumePlayerUpdates()
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

	case SnapshotMsg:
		// 接收播放器不可变快照,存为本地副本,后续渲染只读此副本
		a.lastSnapshot = msg.State
		a.version++
		// 继续消费下一帧快照
		cmds = append(cmds, a.waitForSnapshot())
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

// Run 运行应用
func (a *App) Run() error {
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (a *App) fullRepaintCmd() tea.Cmd {
	return tea.Sequence(
		tea.ClearScreen,
		func() tea.Msg { return ForceRenderMsg{} },
	)
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

// SplashAnimationDoneMsg 流光动画完成消息
type SplashAnimationDoneMsg struct{}

// LoadingAnimationMsg 加载动画消息
type LoadingAnimationMsg struct{}

// PlayerUpdateMsg 播放器状态更新消息
type PlayerUpdateMsg struct{}

// SnapshotMsg 播放器不可变状态快照(来自 Player.Subscribe 事件流)
type SnapshotMsg struct {
	State models.PlaybackState
}

// waitForSnapshot 返回一个命令,从订阅 channel 读取下一帧快照并包装为 SnapshotMsg
func (a *App) waitForSnapshot() tea.Cmd {
	return func() tea.Msg {
		state, ok := <-a.snapshotSub
		if !ok {
			return nil
		}
		return SnapshotMsg{State: state}
	}
}

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
