package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	BaiduOAuthBase = "https://openapi.baidu.com/oauth/2.0"
)

// OAuthDeviceAuth 设备码授权响应
type OAuthDeviceAuth struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// OAuthTokenResponse 令牌响应
type OAuthTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	SessionKey       string `json:"session_key"`
	SessionSecret    string `json:"session_secret"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// GetDeviceCode 获取设备码
func (c *BaiduPanClient) GetDeviceCode(clientID, clientSecret string) (*OAuthDeviceAuth, error) {
	url := fmt.Sprintf("%s/device/code", BaiduOAuthBase)

	resp, err := c.client.R().
		SetQueryParams(map[string]string{
			"client_id":    clientID,
			"response_type": "device_code",
			"scope":        "basic,netdisk",
		}).
		SetHeader("User-Agent", "pan.baidu.com").
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("failed to get device code: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("API error: %d", resp.StatusCode())
	}

	var auth OAuthDeviceAuth
	if err := json.Unmarshal(resp.Body(), &auth); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if auth.UserCode == "" {
		return nil, fmt.Errorf("invalid device code response")
	}

	return &auth, nil
}

// WaitForAuth 等待用户授权完成（轮询）
func (c *BaiduPanClient) WaitForAuth(ctx context.Context, clientID, clientSecret, deviceCode string, initialInterval time.Duration, onProgress func()) (*OAuthTokenResponse, error) {
	url := fmt.Sprintf("%s/token", BaiduOAuthBase)

	// 动态调整轮询间隔
	currentInterval := initialInterval
	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-ticker.C:
			resp, err := c.client.R().
				SetQueryParams(map[string]string{
					"grant_type":    "device_token",
					"code":          deviceCode,
					"client_id":     clientID,
					"client_secret": clientSecret,
				}).
				SetHeader("User-Agent", "pan.baidu.com").
				Get(url)

			if err != nil {
				return nil, fmt.Errorf("polling failed: %w", err)
			}

			var tokenResp OAuthTokenResponse
			if err := json.Unmarshal(resp.Body(), &tokenResp); err != nil {
				return nil, fmt.Errorf("failed to parse response: %w", err)
			}

			// 授权成功
			if tokenResp.AccessToken != "" {
				return &tokenResp, nil
			}

			// 授权中
			if tokenResp.Error == "authorization_pending" {
				onProgress()
				continue
			}

			// 授权过期
			if tokenResp.Error == "expired_token" {
				return nil, fmt.Errorf("device code expired")
			}

			// 授权被拒绝
			if tokenResp.Error == "access_denied" {
				return nil, fmt.Errorf("access denied")
			}

			// 轮询太快，需要降低频率
			if tokenResp.Error == "slow_down" {
				// 增加轮询间隔以避免继续触发 slow_down
				currentInterval = currentInterval * 2
				if currentInterval > 30*time.Second {
					currentInterval = 30 * time.Second
				}
				ticker.Reset(currentInterval)
				continue
			}

			// 其他错误
			if tokenResp.Error != "" {
				return nil, fmt.Errorf("authorization error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
			}
		}
	}
}

// ParseTokenFromURL 从回调URL中解析令牌
func ParseTokenFromURL(callbackURL string) (*TokenInfo, error) {
	parsedURL, err := url.Parse(callbackURL)
	if err != nil {
		return nil, err
	}

	fragment := parsedURL.Fragment
	if fragment == "" {
		return nil, fmt.Errorf("no fragment in URL")
	}

	values, err := url.ParseQuery(fragment)
	if err != nil {
		return nil, err
	}

	accessToken := values.Get("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("no access_token in URL")
	}

	return &TokenInfo{
		AccessToken:  accessToken,
		RefreshToken: values.Get("refresh_token"),
		ExpiresIn:    parseInt(values.Get("expires_in"), 0),
	}, nil
}

// ParseTokenFromOOB 从OOB回调中解析令牌
func ParseTokenFromOOB(callbackString string) (*TokenInfo, error) {
	// 用户在浏览器授权后,页面会显示一个字符串
	// 格式类似: access_token=xxx&refresh_token=yyy&expires_in=zzz
	if !strings.Contains(callbackString, "access_token=") {
		return nil, fmt.Errorf("invalid callback string")
	}

	values, err := url.ParseQuery(callbackString)
	if err != nil {
		return nil, err
	}

	accessToken := values.Get("access_token")
	if accessToken == "" {
		return nil, fmt.Errorf("no access_token in callback")
	}

	return &TokenInfo{
		AccessToken:  accessToken,
		RefreshToken: values.Get("refresh_token"),
		ExpiresIn:    parseInt(values.Get("expires_in"), 0),
	}, nil
}

func parseInt(s string, def int) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	if result == 0 {
		return def
	}
	return result
}
