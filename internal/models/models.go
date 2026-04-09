package models

import "time"

// FileInfo 百度网盘文件信息
type FileInfo struct {
	FsID           int64  `json:"fs_id"`
	Path           string `json:"path"`
	ServerFileName string `json:"server_filename"`
	Size           int64  `json:"size"`
	ServerMtime    int64  `json:"server_mtime"`
	ServerCtime    int64  `json:"server_ctime"`
	LocalMtime     int64  `json:"local_mtime"`
	LocalCtime     int64  `json:"local_ctime"`
	Isdir          int    `json:"isdir"`
	Category       int    `json:"category"`
	MD5            string `json:"md5"`
	DirEmpty       int    `json:"dir_empty"`
	Thumbs         *Thumbs `json:"thumbs,omitempty"`
	Dlink          string `json:"dlink,omitempty"`
}

// Thumbs 缩略图信息
type Thumbs struct {
	Icon   string `json:"icon"`
	URL1   string `json:"url1,omitempty"`
	URL2   string `json:"url2,omitempty"`
	URL3   string `json:"url3,omitempty"`
}

// PlaylistItem 播放列表项目
type PlaylistItem struct {
	FsID           int64   `json:"fs_id"`
	ServerFileName string  `json:"server_filename"`
	Path           string  `json:"path"`
	Size           int64   `json:"size"`
	Category       int    `json:"category"`
	Isdir          int    `json:"isdir"`
	LocalMtime     int64   `json:"local_mtime"`
	ServerMtime    int64   `json:"server_mtime"`
	MD5            string  `json:"md5"`
	AddTime        int64   `json:"add_time"`
	Dlink          string  `json:"dlink,omitempty"`        // 缓存的下载链接
	DlinkExpiresAt  int64   `json:"dlink_expires_at,omitempty"` // 链接过期时间
	Duration       int     `json:"duration,omitempty"` // 歌曲时长（秒）
}

// Track 音频轨道
type Track struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Format  string    `json:"format"`
	MD5     string    `json:"md5"`
	AddedAt time.Time `json:"added_at"`
	LRCPath string    `json:"lrc_path,omitempty"`
}

// Playlist 播放列表
type Playlist struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Items         []*PlaylistItem `json:"items"`
	CreateTime    int64           `json:"create_time"`
	UpdateTime    int64           `json:"update_time"`
}

// PlaybackState 播放状态
type PlaybackState struct {
	IsPlaying      bool             `json:"is_playing"`
	CurrentTime    float64          `json:"current_time"`    // 当前播放位置(秒)
	Duration      float64          `json:"duration"`      // 当前歌曲时长
	Volume         float64          `json:"volume"`         // 音量 0-1
	PlaybackMode   PlaybackMode     `json:"playback_mode"`
	PlaybackRate   float64          `json:"playback_rate"` // 播放速度
	CurrentSong    *PlaylistItem    `json:"current_song"`
	Playlists      []Playlist       `json:"playlists"`
	CurrentPlaylistName string           `json:"current_playlist_name"`

	// 随机播放队列
	ShuffleQueue   []int             `json:"shuffle_queue"`   // 存储随机排列的fs_id列表
	ShuffleIndex   int               `json:"shuffle_index"`     // 当前在洗牌队列中的位置

	// 最近播放
	RecentSongs   []PlaylistItem    `json:"recent_songs"`

	// 歌词
	LyricsRaw      string            `json:"lyrics_raw"`        // 原始歌词
	LyricsParsed  []LyricLine      `json:"lyrics_parsed"`    // 解析后的歌词
	ShowLyrics     bool              `json:"show_lyrics"`
	IsEditingLyrics bool              `json:"is_editing_lyrics"`

	// 音频可视化
	ShowVisualizer bool          `json:"show_visualizer"`
	VisualizationType VisualizerType  `json:"visualization_type"`
}

// PlaybackMode 播放模式
type PlaybackMode string

const (
	PlaybackModeOrder  PlaybackMode = "order"   // 顺序播放
	PlaybackModeRandom PlaybackMode = "random"  // 随机播放
	PlaybackModeSingle PlaybackMode = "single"   // 单曲播放
)

// VisualizerType 可视化类型
type VisualizerType string

const (
	VisualizerParticles VisualizerType = "particles"
	VisualizerBars       VisualizerType = "bars"
	VisualizerWave      VisualizerType = "wave"
	VisualizerSheep     VisualizerType = "sheep"
	VisualizerSheep2    VisualizerType = "sheep2"
	VisualizerNone      VisualizerType = "none"
)

// LyricLine 歌词行
type LyricLine struct {
	ID                string    `json:"id"`
	Time              float64   `json:"time"`      // 时间(秒)
	Text              string    `json:"text"`      // 歌词文本
	IsInterlude       bool      `json:"is_interlude"` // 是否间歇
}

// LRCMetadata LRC 元数据
type LRCMetadata struct {
	Album       string  `json:"album_author"`   // 专辑作者
	By          string  `json:"by"`            // 创建者
	Offset      float64 `json:"offset"`        // 时间偏移
	Re          string  `json:"re"`            // 编辑信息
	Tool        string  `json:"tool"`          // 使用的工具
}

// AuthInfo 认证信息
type AuthInfo struct {
	AccessToken      string     `json:"access_token"`
	RefreshToken     string     `json:"refresh_token"`
	ExpiresAt       int64      `json:"expires_at"`
	Scope           string     `json:"scope"`
	SessionSecret   string     `json:"session_secret"`
	SessionKey      string     `json:"session_key"`
	SessionExpiresAt int64      `json:"session_expires_at"`
	UserID          string     `json:"user_id"`
	Username        string     `json:"username"`
	IsLoggedIn      bool       `json:"is_logged_in"`
}

// UserInfo 用户信息
type UserInfo struct {
	Errno      int    `json:"errno"`
	ErrMsg    string `json:"errmsg,omitempty"`
	BaiduName string `json:"baidu_name,omitempty"`
	NetdiskName string `json:"netdisk_name,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	VipType     int    `json:"vip_type,omitempty"`
	UK          int    `json:"uk,omitempty"`
}

const MaxRecentSongs = 30 // 最大最近播放数量
