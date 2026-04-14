package lyrics

import "context"

// LyricProvider 歌词提供者接口
// 定义统一的接口，方便后续添加更多歌词源
type LyricProvider interface {
	// Search 搜索歌词
	// keyword: 搜索关键词（通常是歌曲名）
	// 返回搜索结果列表
	Search(ctx context.Context, keyword string) ([]SearchResult, error)

	// GetLyric 获取歌词详情
	// id: 歌词唯一标识（不同provider的id格式可能不同）
	// 返回完整的LRC格式歌词内容
	GetLyric(ctx context.Context, id string) (string, error)

	// Name 返回提供者名称
	// 用于标识和日志记录
	Name() string

	// IsAvailable 检查提供者是否可用
	// 可用于健康检查或配置开关
	IsAvailable() bool
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       string // 唯一标识（如 /view/35501.html 中的 "35501"）
	Title    string // 歌曲名
	Artist   string // 歌手
	Album    string // 专辑
	Duration string // 时长
	Source   string // 来源provider名称
}
