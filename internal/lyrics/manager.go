package lyrics

import (
	"context"
	"fmt"
	"sync"

	"github.com/liuguanyu/pan-player-cmd/internal/utils"
)

// Manager 歌词管理器
// 支持多个歌词源，并提供缓存功能
type Manager struct {
	providers []LyricProvider
	cache     *sync.Map // 简单的内存缓存
}

// NewManager 创建歌词管理器
func NewManager() *Manager {
	m := &Manager{
		cache: &sync.Map{},
	}

	// 注册默认的provider
	// 可以根据配置动态添加
	m.RegisterProvider(NewLRC64HProvider())

	return m
}

// RegisterProvider 注册歌词提供者
func (m *Manager) RegisterProvider(provider LyricProvider) {
	m.providers = append(m.providers, provider)
}

// Search 从所有可用的提供者搜索歌词
// 返回第一个成功的结果
func (m *Manager) Search(ctx context.Context, keyword string) ([]SearchResult, error) {
	// 检查缓存
	if cached, ok := m.cache.Load("search:" + keyword); ok {
		if results, ok := cached.([]SearchResult); ok {
			return results, nil
		}
	}

	// 遍历所有provider
	for _, provider := range m.providers {
		if !provider.IsAvailable() {
			continue
		}

		results, err := provider.Search(ctx, keyword)
		if err != nil {
			// 记录错误但继续尝试下一个provider
			logger := utils.GetLogger()
			logger.Debug("%s search failed: %v", provider.Name(), err)
			continue
		}

		if len(results) > 0 {
			// 缓存结果
			m.cache.Store("search:"+keyword, results)
			return results, nil
		}
	}

	return nil, fmt.Errorf("no lyrics found from any provider")
}

// GetLyric 获取歌词详情
func (m *Manager) GetLyric(ctx context.Context, providerName, id string) (string, error) {
	// 检查缓存
	cacheKey := fmt.Sprintf("lyric:%s:%s", providerName, id)
	if cached, ok := m.cache.Load(cacheKey); ok {
		if lrc, ok := cached.(string); ok {
			return lrc, nil
		}
	}

	// 查找对应的provider
	for _, provider := range m.providers {
		if provider.Name() == providerName {
			lrc, err := provider.GetLyric(ctx, id)
			if err != nil {
				return "", err
			}

			// 缓存结果
			m.cache.Store(cacheKey, lrc)
			return lrc, nil
		}
	}

	return "", fmt.Errorf("provider not found: %s", providerName)
}

// GetProviders 获取所有已注册的提供者名称
func (m *Manager) GetProviders() []string {
	var names []string
	for _, p := range m.providers {
		names = append(names, p.Name())
	}
	return names
}
