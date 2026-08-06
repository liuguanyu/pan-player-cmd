package app

import (
	"github.com/liuguanyu/pan-player-cmd/internal/api"
	"github.com/liuguanyu/pan-player-cmd/internal/config"
	"github.com/liuguanyu/pan-player-cmd/internal/lyrics"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
	"github.com/liuguanyu/pan-player-cmd/internal/player"
	"github.com/liuguanyu/pan-player-cmd/internal/playlist"
	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// Service 是应用的核心服务层,持有所有业务依赖。
// TUI/CLI/API 等所有消费者通过 Service 访问播放器、网盘、播放列表、歌词等功能,
// 实现界面与业务逻辑的解耦。
type Service struct {
	Player   *player.Player
	API      *api.BaiduPanClient
	Playlist *playlist.Manager
	Lyrics   *lyrics.Manager
	Config   *config.Config
}

// NewService 创建并初始化应用服务。
// 这是所有入口(TUI/CLI)的统一初始化入口。
func NewService(cfg *config.Config) *Service {
	apiClient := api.NewBaiduPanClient(cfg.API.BaiduPan.TokenFile)

	pl := player.NewPlayer(&player.PlayerConfig{
		AudioDevice: cfg.Player.AudioDevice,
		CacheDir:    cfg.App.DataDir + "/cache",
		Speed:       cfg.Player.PlaybackRate,
	}, apiClient)

	plManager := playlist.NewManager(cfg.App.DataDir)

	svc := &Service{
		Player:   pl,
		API:      apiClient,
		Playlist: plManager,
		Lyrics:   lyrics.NewManager(),
		Config:   cfg,
	}

	// 设置歌曲播放回调:更新最近播放记录(这是业务逻辑,不是 TUI 专属)
	pl.SetOnTrackPlay(func(track *models.PlaylistItem) {
		svc.updateRecentPlaylist(track)
	})

	return svc
}

// updateRecentPlaylist 更新最近播放列表(最多保留30首)
func (s *Service) updateRecentPlaylist(track *models.PlaylistItem) {
	recentPlaylist := s.Playlist.GetPlaylist("最近播放")
	if recentPlaylist == nil {
		return
	}

	var recentSongs []*models.PlaylistItem
	if len(recentPlaylist.Items) > 0 {
		for _, item := range recentPlaylist.Items {
			if item.FsID != track.FsID {
				recentSongs = append(recentSongs, item)
			}
		}
	}

	recentSongs = append([]*models.PlaylistItem{track}, recentSongs...)

	if len(recentSongs) > models.MaxRecentSongs {
		recentSongs = recentSongs[:models.MaxRecentSongs]
	}

	if err := s.Playlist.UpdateRecentSongs(recentSongs); err != nil {
		utils.GetLogger().Error("更新最近播放列表失败: %v", err)
	}
}
