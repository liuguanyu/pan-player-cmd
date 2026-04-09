# M4A 转码会话管理和缓存机制

本文档介绍 M4A 转码会话管理和缓存机制的实现，该实现参考了 pan-player 项目中的 TypeScript 实现，并提供相同的特性。

## 概述

转码会话管理系统包含两个核心组件：

1. **TranscodeSession** - 单个转码会话管理
2. **SessionManager** - 全局会话管理器（单例）

## 核心特性

### TranscodeSession

每个 `TranscodeSession` 实例管理一个 FFmpeg 转码进程，提供以下功能：

#### FFmpeg 进程生命周期管理

- 启动 FFmpeg 进程将远程音频源转码为 FLAC 格式
- 支持 seek 功能（通过 `-ss` 参数）
- 自动清理进程资源

#### 内存缓冲管理

- 使用 `bytes.Buffer` 缓冲转码数据
- 支持分段读取：`GetBufferedData(start, end)`
- 支持完整读取：`GetFullBuffer()`
- 自动合并数据块以优化性能

#### 事件机制

支持以下事件类型：

- `data` - 新数据到达
- `complete` - 转码完成
- `error` - 转码错误
- `progress` - 进度更新（包含时长信息）

#### 实时获取时长

从 FFmpeg stderr 解析 `time=HH:MM:SS.mmm` 格式的时间戳，实时更新音频时长。

### SessionManager

全局单例会话管理器，提供以下功能：

#### LRU 缓存策略

- 基于 `lastAccessedAt` 时间戳进行 LRU 淘汰
- 自动清理不活跃的会话

#### 内存限制

- 默认限制：500MB
- 超过限制时自动淘汰最旧的非活跃会话

#### 并发限制

- 默认限制：3个同时转码
- 超过限制时自动淘汰非活跃会话

#### 会话复用

- 相同 URL 的已完成转码可被复用
- 避免重复转码相同的音频文件

## 使用示例

### 基本使用

```go
package main

import (
    "fmt"
    "github.com/liuguanyu/pan-player-cmd/internal/player"
)

func main() {
    // 获取会话管理器实例
    sm := player.GetSessionManager(nil)

    // 创建或获取会话
    session := sm.GetOrCreateSession(
        "song-123",                          // 会话 ID
        "https://example.com/song.m4a",     // 音频源 URL
        0,                                   // 起始时间（秒）
    )

    // 启动转码
    if err := session.Start(); err != nil {
        fmt.Printf("Failed to start session: %v\n", err)
        return
    }

    // 监听事件
    session.On("complete", func(event player.SessionEvent) {
        if data, ok := event.Data.(player.CompleteEvent); ok {
            fmt.Printf("转码完成: %d 字节, 时长: %.1f 秒\n",
                data.TotalBytes, data.Duration)
        }
    })

    session.On("error", func(event player.SessionEvent) {
        if err, ok := event.Data.(error); ok {
            fmt.Printf("转码错误: %v\n", err)
        }
    })

    session.On("progress", func(event player.SessionEvent) {
        if data, ok := event.Data.(player.ProgressData); ok {
            fmt.Printf("进度: %.1f%%, 时长: %.1f 秒\n",
                data.Percent, data.Duration)
        }
    })

    // 等待数据就绪
    if session.WaitForData(1024*1024, 30000) { // 等待 1MB 或 30 秒超时
        // 获取缓冲数据
        buffer := session.GetBufferedData(0, 1024)
        fmt.Printf("获取了 %d 字节\n", buffer.Len())
    }

    // 完成后清理
    defer sm.DestroySession("song-123")
}
```

### 自定义配置

```go
config := &player.SessionManagerConfig{
    MaxConcurrentSessions: 5,                 // 最大并发会话数
    MaxTotalMemoryBytes:   1024 * 1024 * 1024, // 1GB 内存限制
}

sm := player.GetSessionManager(config)
```

### 会话复用

```go
// 第一次请求 - 创建新会话
session1 := sm.GetOrCreateSession("request-1", "https://example.com/song.m4a", 0)
session1.Start()

// 等待完成...
// session1.state = StateCompleted

// 第二次请求相同 URL - 复用已完成的会话
session2 := sm.GetOrCreateSession("request-2", "https://example.com/song.m4a", 0)
// session2 实际指向 session1 的缓存数据
```

### 范围读取

```go
// 读取前 1KB
buffer := session.GetBufferedData(0, 1024)

// 读取中间部分
buffer = session.GetBufferedData(5000, 10000)

// 读取全部
buffer = session.GetFullBuffer()
```

## API 参考

### TranscodeSession

#### 构造函数

```go
func NewTranscodeSession(sessionID, sourceURL string, startTimeSeconds float64) *TranscodeSession
```

#### 主要方法

- `Start() error` - 启动转码进程
- `Destroy()` - 销毁会话，释放所有资源
- `GetBufferedData(start, end int64) *bytes.Buffer` - 获取指定范围的数据
- `GetFullBuffer() *bytes.Buffer` - 获取完整缓冲数据
- `WaitForData(minBytes int64, timeoutMs int) bool` - 等待数据就绪
- `GetMemoryUsage() int64` - 获取当前内存使用量

#### 事件监听

- `On(eventType string, listener func(SessionEvent))` - 注册事件监听器
- `RemoveListener(eventType string, listener func(SessionEvent))` - 移除事件监听器
- `ClearAllListeners()` - 清除所有监听器

#### 属性访问

- `SessionID() string` - 会话 ID
- `SourceURL() string` - 源 URL
- `State() SessionState` - 当前状态
- `IsComplete() bool` - 是否完成
- `TotalBytes() int64` - 总字节数
- `Duration() float64` - 音频时长（秒）

### SessionManager

#### 单例访问

```go
func GetSessionManager(config *SessionManagerConfig) *SessionManager
func ResetSessionManager() // 仅用于测试
```

#### 会话管理

- `GetOrCreateSession(sessionID, sourceURL string, startTimeSeconds float64) *TranscodeSession` - 获取或创建会话
- `GetSession(sessionID string) *TranscodeSession` - 获取指定会话
- `DestroySession(sessionID string)` - 销毁指定会话
- `DestroyAll()` - 销毁所有会话

#### 查询方法

- `SessionCount() int` - 当前会话总数
- `GetTotalMemoryUsage() int64` - 总内存使用量
- `GetActiveTranscodingCount() int` - 正在转码的会话数
- `GetSessionsSummary() []SessionSummary` - 获取会话摘要信息
- `FindCachedSession(sourceURL string) *TranscodeSession` - 查找缓存的会话

## 实现细节

### 并发安全

所有关键操作都使用 `sync.RWMutex` 保护：

- 会话状态管理
- 缓冲区操作
- 事件监听器管理

### 资源清理

- 使用 `sync.Once` 确保资源只清理一次
- `Destroy()` 方法会：
  - 取消上下文
  - 终止 FFmpeg 进程
  - 关闭输出流
  - 释放内存缓冲
  - 移除事件监听器

### FFmpeg 参数

转码使用以下参数：

```bash
ffmpeg -nostdin -y \
  [-ss <start_time>] \           # 可选 seek
  -i <source_url> \
  -acodec flac \                 # FLAC 编码
  -ar 44100 \                    # 44.1kHz 采样率
  -ac 2 \                        # 立体声
  -f flac \                      # FLAC 格式
  -compression_level 0 \         # 最快压缩
  -                              # 输出到 stdout
```

### 内存优化

- 使用 `chunks` 数组收集数据块
- 转码完成后合并为单个 buffer
- 清除 `chunks` 引用以释放内存

## 性能考虑

### 缓冲策略

- 小文件（< 1MB）：立即完整缓冲
- 大文件：流式读取，按需缓冲

### LRU 淘汰

淘汰优先级：

1. 非活跃会话（completed/error/idle）
2. 基于 `lastAccessedAt` 时间戳
3. 最旧的优先淘汰

### 并发控制

- 创建新会话前检查并发限制
- 自动淘汰非活跃会话以腾出空间
- 不会终止正在转码的会话

## 测试

运行测试：

```bash
go test -v ./internal/player/... -run "Test.*Session"
```

测试覆盖：

- 基本会话生命周期
- 缓存复用
- 内存限制
- 并发限制
- 事件机制
- 缓冲管理
- 等待数据

## 与 TypeScript 版本的对应关系

| TypeScript | Go |
|-----------|-----|
| `TranscodeSession` | `TranscodeSession` |
| `SessionManager` | `SessionManager` |
| `EventEmitter` | `eventChan` + `eventListeners` |
| `Buffer.concat()` | `bytes.NewBuffer()` + `append()` |
| `Map<string, TranscodeSession>` | `map[string]*TranscodeSession` |
| `PassThrough` stream | `io.PipeReader/Writer` |

## 注意事项

1. **FFmpeg 依赖**: 必须安装 FFmpeg 并可在 PATH 中访问
2. **内存使用**: 大文件可能占用大量内存，注意配置合适的限制
3. **并发控制**: 建议根据机器性能调整并发限制
4. **会话 ID**: 建议使用文件 ID 或唯一标识符作为会话 ID
5. **清理**: 应用退出时调用 `DestroyAll()` 清理所有会话

## 故障排查

### FFmpeg 未找到

确保 FFmpeg 已安装：

```bash
# macOS
brew install ffmpeg

# Linux
sudo apt-get install ffmpeg
```

### 内存不足

降低内存限制或减少并发数：

```go
config := &player.SessionManagerConfig{
    MaxConcurrentSessions: 2,
    MaxTotalMemoryBytes:   200 * 1024 * 1024, // 200MB
}
```

### 转码超时

检查网络连接和源 URL 可访问性，增加等待超时时间。
