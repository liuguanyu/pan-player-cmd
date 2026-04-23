package lyrics

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
)

const (
	LRC64HBaseURL = "https://lrc.64h.cn"
)

// LRC64HProvider lrc.64h.cn 歌词提供者
type LRC64HProvider struct {
	client    *resty.Client
	available bool
}

// NewLRC64HProvider 创建新的LRC64H提供者
func NewLRC64HProvider() *LRC64HProvider {
	client := resty.New().
		SetTimeout(15*time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(500*time.Millisecond).
		SetHeader("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	return &LRC64HProvider{
		client:    client,
		available: true, // 默认可用
	}
}

// Name 返回提供者名称
func (p *LRC64HProvider) Name() string {
	return "lrc64h"
}

// IsAvailable 检查提供者是否可用
func (p *LRC64HProvider) IsAvailable() bool {
	return p.available
}

// Search 搜索歌词
func (p *LRC64HProvider) Search(ctx context.Context, keyword string) ([]SearchResult, error) {
	// 1. 构建搜索URL
	encodedKeyword := url.QueryEscape(keyword)
	searchURL := fmt.Sprintf("%s/search/%s", LRC64HBaseURL, encodedKeyword)

	// 2. 发送HTTP请求
	resp, err := p.client.R().
		SetContext(ctx).
		Get(searchURL)
	if err != nil {
		p.available = false
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	// 3. 解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body())))
	if err != nil {
		return nil, fmt.Errorf("parse HTML failed: %w", err)
	}

	// 4. 提取搜索结果
	var results []SearchResult

	// 根据HTML结构，搜索结果在table中
	doc.Find("table tbody tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 6 {
			return
		}

		// 提取"查看详情"链接 - 第6个单元格（索引5）
		detailLink := cells.Eq(5).Find("a")
		href, exists := detailLink.Attr("href")
		if !exists {
			return
		}

		// 提取ID（如 /view/35501.html -> 35501）
		id := extractIDFromURL(href)
		if id == "" {
			return
		}

		// 提取字段 - 根据网页实际结构
		// 第1列: 歌曲信息（包含歌手-歌名）
		songInfo := strings.TrimSpace(cells.Eq(0).Text())
		// 第2列: 转换日期
		// 第3列: 时长
		// 第4列: 下载次数
		// 第5列: 未知
		// 第6列: 操作链接

		// 分离歌手和歌名，格式通常是 "歌手 - 歌名"
		title, artist := splitSongInfo(songInfo)

		// 时长
		duration := strings.TrimSpace(cells.Eq(2).Text())

		// 专辑 - 这里我们没有直接的专辑信息，可以用空字符串
		album := ""

		results = append(results, SearchResult{
			ID:       id,
			Title:    title,
			Artist:   artist,
			Album:    album,
			Duration: duration,
			Source:   p.Name(),
		})
	})

	if len(results) == 0 {
		return nil, fmt.Errorf("no lyrics found for keyword: %s", keyword)
	}

	return results, nil
}

// GetLyric 获取歌词内容
func (p *LRC64HProvider) GetLyric(ctx context.Context, id string) (string, error) {
	// 1. 构建详情页URL
	detailURL := fmt.Sprintf("%s/view/%s.html", LRC64HBaseURL, id)

	// 2. 发送HTTP请求
	resp, err := p.client.R().
		SetContext(ctx).
		Get(detailURL)
	if err != nil {
		return "", fmt.Errorf("get lyric request failed: %w", err)
	}

	body := string(resp.Body())

	// 3. 解析HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse HTML failed: %w", err)
	}

	// 4. 提取歌词内容
	var lrcLines []string
	selectors := []string{
		".lyrics-container .lyrics-text .line",
		".lyrics-container .lyrics-content .line",
		".lyrics-content .line",
		".lyrics-scroll-area .line",
		".lyrics-text .line",
		".line",
	}

	for _, selector := range selectors {
		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			line := strings.TrimSpace(s.Text())
			if line != "" {
				lrcLines = append(lrcLines, line)
			}
		})
		if len(lrcLines) > 0 {
			break
		}
	}

	if len(lrcLines) > 0 {
		return strings.Join(lrcLines, "\n"), nil
	}

	// 5. 回退：从脚本/textarea中提取歌词，兼容页面模板调整
	reList := []*regexp.Regexp{
		regexp.MustCompile(`(?s)lyrics?\s*[:=]\s*["'](.+?)["']`),
		regexp.MustCompile(`(?s)lrc\s*[:=]\s*["'](.+?)["']`),
		regexp.MustCompile(`(?s)<textarea[^>]*>(.*?)</textarea>`),
	}
	for _, re := range reList {
		matches := re.FindStringSubmatch(body)
		if len(matches) < 2 {
			continue
		}
		candidate := htmlUnescapeLyrics(matches[1])
		candidate = normalizeLyricText(candidate)
		if candidate != "" {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no lyrics found on detail page")
}

func htmlUnescapeLyrics(s string) string {
	replacer := strings.NewReplacer(
		`\\n`, "\n",
		`\\r`, "\r",
		`\\t`, "\t",
		`\"`, `"`,
		`\'`, "'",
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"&#10;", "\n",
		"&#13;", "\r",
		"&quot;", `"`,
		"&#39;", "'",
		"&amp;", "&",
	)
	return replacer.Replace(s)
}

func normalizeLyricText(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// extractIDFromURL 从URL中提取ID
// 例如: /view/35501.html -> 35501
func extractIDFromURL(urlStr string) string {
	re := regexp.MustCompile(`/view/(\d+)\.html`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// splitSongInfo 分离歌名和歌手
// 假设格式为 "歌名-歌手"
func splitSongInfo(info string) (title, artist string) {
	parts := strings.SplitN(info, "-", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return info, ""
}
