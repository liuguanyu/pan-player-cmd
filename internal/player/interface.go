package player

import (
	"context"

	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// MusicPlayer 是播放控制的通用接口,CLI/TUI/API 等所有消费者都依赖它。
// Player 具体类型已实现全部方法,天然满足此接口。
type MusicPlayer interface {
	// 播放控制
	Play()
	Pause()
	Stop()
	Seek(pos float64)
	PlayNext()
	PlayPrevious()
	IsPlaying() bool

	// 队列管理
	LoadTrack(ctx context.Context, track *models.PlaylistItem) error
	SetCurrentPlaylist(name string, items []*models.PlaylistItem)
	SetCurrentSong(track *models.PlaylistItem)
	GetCurrentPlaylist() *models.Playlist
	GetCurrentIndex() int
	SetCurrentIndex(index int)
	SetPlayMode(mode models.PlaybackMode)
	GetShuffleStartIndex() int

	// 音量/速度
	SetVolume(volume float64)
	SetSpeed(speed float64)

	// 状态订阅
	Subscribe() <-chan models.PlaybackState
}

// TuiPlayer 是 TUI 专用的播放接口,在 MusicPlayer 基础上扩展 UI 特有功能。
// SetTapWrapper 不在此接口中,因为它依赖 beep.Streamer 具体类型,
// TUI 侧通过类型断言直接调用 *Player.SetTapWrapper。
type TuiPlayer interface {
	MusicPlayer

	SetShowLyrics(show bool)
	SetLyrics(raw string, parsed []models.LyricLine, show bool)
	SetOnTrackPlay(func(*models.PlaylistItem))
}
