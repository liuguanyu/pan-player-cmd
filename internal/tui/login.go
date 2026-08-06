package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuguanyu/pan-player-cmd/internal/api"
	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

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
