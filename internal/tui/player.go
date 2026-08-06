package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

func (a *App) loadPlaylists() tea.Cmd {
	return func() tea.Msg {
		if err := a.svc.Playlist.LoadPlaylists(); err == nil {
			return PlaylistsLoadedMsg{Playlists: a.svc.Playlist.GetPlaylists()}
		}
		return nil
	}
}

// startPlayerUpdateTicker 启动播放器状态更新定时器
func (a *App) startPlayerUpdateTicker() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return PlayerUpdateMsg{}
	})
}

// resumePlayerUpdates 返回（重新）进入播放界面所需的命令：启动状态轮询定时器 + 刷新终端标题。
//
// 修复背景：播放器进度/流光/歌词高亮依赖 PlayerUpdateMsg 定时器链，而该链仅在
// currentView == ViewPlayer 时才会自我续期。一旦离开播放界面（例如进入歌词搜索），
// 下一次 PlayerUpdateMsg 命中非播放视图便返回 nil，定时器链随之断裂。若回到播放界面
// 时忘记重启定时器，界面就会"卡住"（进度条不滚动、流光不动、歌词不跟进）。
// 因此任何（重新）进入播放界面的路径都应通过此方法恢复实时更新。
func (a *App) resumePlayerUpdates() tea.Cmd {
	return tea.Batch(a.startPlayerUpdateTicker(), a.updateWindowTitleCmd())
}

// computeWindowTitle 计算当前应设置的终端标题。
// 播放界面且存在当前高亮歌词时，标题为 "歌名 — 歌词行"；
// 没有当前歌词行时退化为歌名；非播放界面或无歌曲时使用默认标题。
func (a *App) computeWindowTitle() string {
	if a.currentView != ViewPlayer {
		return "Pan Player"
	}

	state := a.lastSnapshot
	if state.CurrentSong == nil {
		return "Pan Player"
	}

	songName := extractSongName(state.CurrentSong.ServerFileName)

	// 计算当前高亮歌词行
	line := ""
	if state.ShowLyrics && len(a.currentLyrics) > 0 {
		if idx := lyrics.GetCurrentLyricIndex(a.currentLyrics, state.CurrentTime); idx >= 0 {
			line = strings.TrimSpace(a.currentLyrics[idx].Text)
		}
	}

	if line == "" {
		return sanitizeTitle(songName)
	}
	return sanitizeTitle(songName + " — " + line)
}

// updateWindowTitleCmd 仅在标题发生变化时返回 tea.SetWindowTitle 命令，
// 避免每个 100ms tick 都重复发送相同的标题设置指令。
func (a *App) updateWindowTitleCmd() tea.Cmd {
	title := a.computeWindowTitle()
	if title == a.lastWindowTitle {
		return nil
	}
	a.lastWindowTitle = title
	return tea.SetWindowTitle(title)
}

// resetWindowTitle 重置终端标题（退出时清除标题，恢复为终端默认）
func (a *App) resetWindowTitle() tea.Cmd {
	a.lastWindowTitle = ""
	return tea.SetWindowTitle("")
}

// cyclePlaybackSpeed 切换播放倍速
func (a *App) cyclePlaybackSpeed() {
	state := a.lastSnapshot
	currentSpeed := state.PlaybackRate

	// 找到当前速度在列表中的位置，切换到下一个
	idx := -1
	for i, s := range playbackSpeeds {
		if s == currentSpeed {
			idx = i
			break
		}
	}

	var newSpeed float64
	if idx < 0 || idx >= len(playbackSpeeds)-1 {
		newSpeed = playbackSpeeds[0]
	} else {
		newSpeed = playbackSpeeds[idx+1]
	}

	a.svc.Player.SetSpeed(newSpeed)

	// 保存到配置
	a.svc.Config.Player.PlaybackRate = newSpeed
	if err := a.svc.Config.Save(); err != nil {
		utils.GetLogger().Error("保存播放倍速配置失败: %v", err)
	}

	// 显示倍速切换消息
	speedStr := formatSpeed(newSpeed)
	a.showMessage(fmt.Sprintf("播放倍速: %s", speedStr))
}
