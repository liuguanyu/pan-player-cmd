package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/config"
	"github.com/liuguanyu/pan-player-cmd/internal/tui"
)

func main() {
	// 加载内置凭证
	creds, err := config.LoadBaiduCredentials()
	if err != nil {
		log.Fatal("Failed to load credentials:", err)
		os.Exit(1)
	}

	// 加载应用配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config: %v, using defaults", err)
		cfg = config.DefaultConfig()
	}

	// 将凭证注入到配置中
	cfg.API.BaiduPan.ClientID = creds.ClientID
	cfg.API.BaiduPan.ClientSecret = creds.ClientSecret
	cfg.API.BaiduPan.RedirectURI = creds.RedirectURI

	// 启动 TUI 应用（使用新架构）
	app := tui.NewApp(cfg)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running program:", err)
		os.Exit(1)
	}
}
