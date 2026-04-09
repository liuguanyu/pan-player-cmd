package lyrics

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/liuguanyu/pan-player-cmd/internal/models"
)

// ParseResult 解析结果
type ParseResult struct {
	Lines    []models.LyricLine
	Metadata models.LRCMetadata
}

// ParseLRC 解析LRC歌词
func ParseLRC(lrcContent string) ParseResult {
	if lrcContent == "" {
		return ParseResult{}
	}

	var lines []models.LyricLine
	var metadata models.LRCMetadata

	lrcLines := strings.Split(lrcContent, "\n")

	// LRC时间标签正则表达式: [mm:ss.xx] 或 [mm:ss]
	timeRegex := regexp.MustCompile(`\[(\d{2}):(\d{2})(?:\.(\d{2,3}))?\]`)

	// LRC元数据标签正则表达式: [ti:title]
	metadataRegex := regexp.MustCompile(`^\[(ti|ar|al|by|offset):(.*)\]$`)

	for _, line := range lrcLines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		// 检查元数据标签
		if matches := metadataRegex.FindStringSubmatch(trimmedLine); matches != nil {
			key := strings.ToLower(matches[1])
			value := strings.TrimSpace(matches[2])

			switch key {
			case "ti":
				metadata.Album = value // ti -> Album (歌曲名)
			case "ar":
				metadata.By = value // ar -> By (歌手名)
			case "al":
				metadata.Album = value // al -> Album (专辑名)
			case "by":
				metadata.By = value
			case "offset":
				if offset, err := strconv.ParseFloat(value, 64); err == nil {
					metadata.Offset = offset / 1000.0 // 转换为秒
				}
			}
			continue
		}

		// 查找所有时间标签
		matches := timeRegex.FindAllStringSubmatch(trimmedLine, -1)

		if len(matches) > 0 {
			// 提取歌词文本（移除所有时间标签）
			text := timeRegex.ReplaceAllString(trimmedLine, "")
			text = strings.TrimSpace(text)

			// 为每个时间标签创建一行歌词
			for _, match := range matches {
				minutes, _ := strconv.Atoi(match[1])
				seconds, _ := strconv.Atoi(match[2])
				milliseconds := 0
				if match[3] != "" {
					msStr := match[3]
					// 补齐到3位
					if len(msStr) == 2 {
						msStr += "0"
					}
					milliseconds, _ = strconv.Atoi(msStr)
				}

				timeSeconds := float64(minutes*60+seconds) + float64(milliseconds)/1000.0

				// 应用offset
				timeSeconds += metadata.Offset

				lines = append(lines, models.LyricLine{
					ID:          generateLyricID(),
					Time:        timeSeconds,
					Text:        text,
					IsInterlude: false,
				})
			}
		} else {
			// 如果没有时间标签，但有文本内容，将其作为纯文本歌词
			// 为纯文本歌词设置一个默认时间值，避免被忽略
			text := strings.TrimSpace(trimmedLine)
			if text != "" {
				// 使用 -1 作为时间戳，表示没有时间信息
				// 在渲染时，按顺序显示这些行
				lines = append(lines, models.LyricLine{
					ID:          generateLyricID(),
					Time:        -1.0,
					Text:        text,
					IsInterlude: false,
				})
			}
		}
	}

	// 按时间排序，但保持纯文本行在最后
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Time == -1.0 && lines[j].Time != -1.0 {
			return false // 纯文本行排在后面
		}
		if lines[i].Time != -1.0 && lines[j].Time == -1.0 {
			return true
		}
		return lines[i].Time < lines[j].Time
	})

	return ParseResult{
		Lines:    lines,
		Metadata: metadata,
	}
}

// GetCurrentLyricIndex 根据当前播放时间获取当前应该显示的歌词行索引
func GetCurrentLyricIndex(lyrics []models.LyricLine, currentTime float64) int {
	if len(lyrics) == 0 {
		return -1
	}

	// 找到最后一个时间小于等于当前时间的歌词行
	currentIndex := -1
	for i, line := range lyrics {
		if line.Time <= currentTime {
			currentIndex = i
		} else {
			break
		}
	}

	return currentIndex
}

// FormatLRCTime 格式化时间为LRC时间标签格式 [mm:ss.xx]
func FormatLRCTime(seconds float64) string {
	minutes := int(seconds / 60)
	secs := int(seconds) % 60
	ms := int((seconds - float64(int(seconds))) * 100)

	return fmt.Sprintf("[%02d:%02d.%02d]", minutes, secs, ms)
}

// generateLyricID 生成唯一ID
func generateLyricID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

// ParsePlainText 从纯文本生成歌词行数组（每行一条歌词，时间初始为-1）
func ParsePlainText(textContent string) []models.LyricLine {
	if textContent == "" {
		return nil
	}

	var lines []models.LyricLine
	textLines := strings.Split(textContent, "\n")

	for _, line := range textLines {
		trimmed := strings.TrimSpace(line)
		lines = append(lines, models.LyricLine{
			ID:          generateLyricID(),
			Time:        -1, // 使用-1表示未设置时间
			Text:        trimmed,
			IsInterlude: false,
		})
	}

	return lines
}

// ParseLRCTimeTag 解析 LRC 时间标签字符串为秒数
func ParseLRCTimeTag(timeStr string) float64 {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return -1
	}

	minutes, err1 := strconv.Atoi(parts[0])
	if err1 != nil {
		return -1
	}

	secParts := strings.Split(parts[1], ".")
	seconds, err2 := strconv.Atoi(secParts[0])
	if err2 != nil {
		return -1
	}

	milliseconds := 0
	if len(secParts) > 1 {
		msStr := secParts[1]
		// 补齐到3位
		if len(msStr) == 2 {
			msStr += "0"
		}
		milliseconds, _ = strconv.Atoi(msStr)
	}

	return float64(minutes*60+seconds) + float64(milliseconds)/1000.0
}

// GenerateLRC 生成LRC文件内容
func GenerateLRC(lyrics []models.LyricLine, metadata *models.LRCMetadata) string {
	var result strings.Builder

	// 写入元数据
	if metadata != nil {
		if metadata.Album != "" {
			result.WriteString(fmt.Sprintf("[ti:%s]\n", metadata.Album))
		}
		if metadata.By != "" {
			result.WriteString(fmt.Sprintf("[by:%s]\n", metadata.By))
		}
		if metadata.Offset != 0 {
			result.WriteString(fmt.Sprintf("[offset:%d]\n", int(metadata.Offset*1000)))
		}
	}

	// 只包含有时间标记的歌词
	var validLyrics []models.LyricLine
	for _, line := range lyrics {
		if line.Time >= 0 {
			validLyrics = append(validLyrics, line)
		}
	}

	// 按时间排序
	sort.Slice(validLyrics, func(i, j int) bool {
		return validLyrics[i].Time < validLyrics[j].Time
	})

	// 写入歌词行
	for _, line := range validLyrics {
		timeTag := FormatLRCTime(line.Time)
		result.WriteString(fmt.Sprintf("%s%s\n", timeTag, line.Text))
	}

	return result.String()
}
