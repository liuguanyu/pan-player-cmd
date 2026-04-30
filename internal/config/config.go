package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App      AppConfig      `yaml:"app"`
	API      APIConfig      `yaml:"api"`
	Player   PlayerConfig   `yaml:"player"`
	Playlist PlaylistConfig `yaml:"playlist"`
	UI       UIConfig       `yaml:"ui"`
}

type AppConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	DataDir string `yaml:"data_dir"`
}

type APIConfig struct {
	BaiduPan BaiduPanConfig `yaml:"baidu_pan"`
}

type BaiduPanConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURI  string `yaml:"redirect_uri"`
	TokenFile    string `yaml:"token_file"`
}

type PlayerConfig struct {
	Volume       int     `yaml:"volume"`    // 0-100
	PlayMode     string  `yaml:"play_mode"` // single, list, random
	AutoPlay     bool    `yaml:"auto_play"`
	ShowLyrics   bool    `yaml:"show_lyrics"`
	AudioDevice  string  `yaml:"audio_device"`
	PlaybackRate float64 `yaml:"playback_rate"` // 播放倍速，如 1.0, 1.5, 2.0
}

type PlaylistConfig struct {
	MaxHistory    int  `yaml:"max_history"`
	SaveOnExit    bool `yaml:"save_on_exit"`
	RememberState bool `yaml:"remember_state"`
}

type UIConfig struct {
	Theme       string `yaml:"theme"`    // dark, light
	Language    string `yaml:"language"` // zh-CN, en-US
	ShowHelp    bool   `yaml:"show_help"`
	ColorLyrics bool   `yaml:"color_lyrics"`
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".pan-player")

	return &Config{
		App: AppConfig{
			Name:    "Pan Player TUI",
			Version: "1.0.0",
			DataDir: dataDir,
		},
		API: APIConfig{
			BaiduPan: BaiduPanConfig{
				// 这些凭证会在 main.go 中从 credentials.yaml 加载并注入
				ClientID:     "",
				ClientSecret: "",
				RedirectURI:  "oob",
				TokenFile:    filepath.Join(dataDir, "token.json"),
			},
		},
		Player: PlayerConfig{
			Volume:       70,
			PlayMode:     "list",
			AutoPlay:     false,
			ShowLyrics:   true,
			AudioDevice:  "default",
			PlaybackRate: 1.0,
		},
		Playlist: PlaylistConfig{
			MaxHistory:    100,
			SaveOnExit:    true,
			RememberState: true,
		},
		UI: UIConfig{
			Theme:       "dark",
			Language:    "zh-CN",
			ShowHelp:    true,
			ColorLyrics: true,
		},
	}
}

func LoadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".pan-player", "config.yaml")
	data, err := os.ReadFile(configPath)

	// 如果配置文件不存在或读取失败，使用默认配置
	if err != nil {
		cfg := DefaultConfig()
		// 确保数据目录存在
		if err := os.MkdirAll(cfg.App.DataDir, 0755); err != nil {
			return nil, err
		}
		// 保存默认配置，方便用户查看和修改
		if err := cfg.Save(); err != nil {
			// 保存失败也不影响使用，返回默认配置即可
		}
		return cfg, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		// 配置文件解析失败，使用默认配置
		return DefaultConfig(), nil
	}

	return &cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(c.App.DataDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(c.App.DataDir, "config.yaml")
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
