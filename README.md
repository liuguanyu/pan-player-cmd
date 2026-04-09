# Pan Player CMD

基于 Go + Bubble Tea 的百度网盘音乐播放器 TUI 版本

## 功能特性

✅ **百度网盘集成**
- OAuth 设备码授权登录
- 音频文件列表浏览
- 流式下载播放

✅ **播放控制**
- 播放/暂停/停止
- 上一首/下一首
- 音量调节
- 播放速度控制

✅ **播放模式**
- 顺序播放
- 随机播放
- 单曲循环

✅ **歌词支持**
- LRC 歌词解析
- 实时滚动显示
- 歌词时间偏移

✅ **播放列表管理**
- 创建/删除/重命名播放列表
- 拖拽排序
- 最近播放记录

## 技术栈

- **语言**: Go 1.21+
- **TUI 框架**: Bubble Tea (Charm)
- **音频播放**: beep + go-mp3
- **HTTP 客户端**: resty
- **配置管理**: YAML

## 安装

### 前置要求

- Go 1.21 或更高版本

### 快速开始

```bash
# 1. 克隆项目
git clone <repository-url>
cd pan-player-cmd

# 2. 编译
go mod download
go build -o pan-player ./cmd/pan-player

# 3. 运行
./pan-player
```

首次运行会自动创建用户配置文件和数据目录在 `~/.pan-player/`。

### API 凭证

**重要说明**：百度网盘 API 凭证已内置在程序中，无需额外配置。

- 凭证在编译时嵌入到程序内部
- 源码中的 `internal/config/credentials.go` 包含凭证定义
- 出于安全考虑，实际运行时不会暴露明文凭证
- 如需更换凭证，修改源码并重新编译即可

项目参考了 [pan-player](https://github.com/liuguanyu/pan-player) 的凭证管理方式。

## 配置

配置文件位于 `~/.pan-player/config.yaml`，首次运行会自动生成：

```yaml
app:
  name: Pan Player TUI
  version: 1.0.0
  data_dir: ~/.pan-player

player:
  volume: 70
  play_mode: list
  auto_play: false
  show_lyrics: true
  audio_device: default

playlist:
  max_history: 100
  save_on_exit: true
  remember_state: true

ui:
  theme: dark
  language: zh-CN
  show_help: true
  color_lyrics: true
```

用户可以根据需要修改播放器、播放列表和 UI 相关配置。

## 快捷键

### 全局
- `q` / `Ctrl+C`: 退出程序
- `h`: 显示帮助
- `Esc`: 返回上一级

### 播放列表视图
- `↑/↓`: 选择播放列表
- `Enter`: 打开播放列表
- `n`: 新建播放列表
- `d`: 删除播放列表

### 播放器视图
- `Space`: 播放/暂停
- `←`: 上一首
- `→`: 下一首
- `↑`: 增加音量
- `↓`: 减少音量
- `m`: 切换播放模式
- `l`: 显示/隐藏歌词

## 项目结构

```
pan-player-cmd/
├── cmd/
│   └── pan-player/
│       └── main.go           # 主程序入口
├── internal/
│   ├── api/                  # 百度网盘 API
│   │   ├── baidu_client.go
│   │   └── auth.go
│   ├── config/               # 配置管理
│   │   └── config.go
│   ├── lyrics/               # 歌词解析
│   │   └── parser.go
│   ├── models/               # 数据模型
│   │   └── models.go
│   ├── player/               # 播放器核心
│   │   └── player.go
│   ├── playlist/             # 播放列表管理
│   │   └── manager.go
│   └── tui/                  # TUI 界面
│       └── app.go
├── configs/                  # 配置文件模板
├── pkg/
│   └── utils/                # 工具函数
├── go.mod
├── go.sum
└── README.md
```

## 开发计划

### 已完成 ✅
- [x] 项目初始化和基础设施
- [x] 百度网盘 API 集成
- [x] 播放器核心功能
- [x] LRC 歌词解析和显示
- [x] 播放列表管理
- [x] TUI 界面开发
- [x] 快捷键绑定
- [x] 配置和数据持久化

### 待完善 🔨
- [x] 实际音频播放集成 ✅ **已完成**
- [x] 多格式音频支持 ✅ **已完成** (MP3, FLAC, OGG)
- [ ] 音频可视化
- [ ] 搜索功能
- [ ] 文件夹浏览
- [ ] 歌词编辑器
- [ ] 播放历史

## 音频播放

本项目使用以下库实现真实音频播放：

- **beep**: Go 音频库，提供统一的音频处理接口
- **mp3**: MP3 解码器 (github.com/faiface/beep/mp3)
- **flac**: FLAC 解码器 (github.com/faiface/beep/flac)
- **vorbis**: OGG Vorbis 解码器 (github.com/faiface/beep/vorbis)
- **speaker**: 跨平台音频输出

### 支持的音频格式

本项目现已支持多种音频格式的自动识别和解码：

- **MP3** (.mp3) - 主要支持，完全集成
- **WAV** (.wav) - 完全支持，支持 8-bit 和 16-bit PCM
- **FLAC** (.flac) - 无损音频格式支持
- **OGG Vorbis** (.ogg) - 开源音频格式支持

与 pan-player 对齐的格式支持：
- MP3、WAV、FLAC、OGG 已完全支持
- 未来计划支持：M4A、AAC、APE、ALAC 等格式

### 自动格式检测

播放器会自动检测音频文件格式：

1. **文件扩展名检测** - 根据文件后缀快速识别格式
2. **魔数检测** - 通过文件头部的特征字节精确识别格式

### 测试音频播放

```bash
# 测试 MP3 文件播放
go run test_audio.go /path/to/test.mp3

# 测试 FLAC 文件播放
go run test_audio.go /path/to/test.flac

# 测试 OGG 文件播放
go run test_audio.go /path/to/test.ogg
```

## 注意事项

1. **音频播放**: 当前版本的音频播放功能是模拟的，实际播放需要集成系统音频设备
2. **缓存管理**: 播放的歌曲会缓存到本地，定期清理 `~/.pan-player/cache/` 目录
3. **Token 过期**: 百度网盘 Token 有效期为 30 天，过期后需要重新登录

## 参考

本项目参考了 [pan-player](https://github.com/liuguanyu/pan-player) 的设计和实现。

## License

MIT License
