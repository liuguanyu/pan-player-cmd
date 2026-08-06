# app.go 拆分方案

## 现状

`internal/tui/app.go` 2581 行，占 `internal/tui/` 包的 85%（总共 3025 行）。包含 6 大类、50+ 个函数/类型，职责混杂。

## 目标

拆分为 6 个文件，按职责清晰分离，**零行为变更，同包内直接复用字段**。

```
internal/tui/
├── app.go           ~200 行  核心结构 + 生命周期 + 消息类型
├── views.go         ~1100 行  全部渲染函数
├── keyhandlers.go   ~600 行   全部按键处理
├── commands.go      ~500 行   全部业务逻辑 + Commands
├── helpers.go       ~100 行   工具函数
├── app_keybinds.go     6 行   保留不动
└── lyrics_handler.go  → 删除（内容归入以上各文件）
```

## 功能清单

### A. 核心生命周期 & 类型定义

| # | 符号 | 行号 | 说明 |
|---|------|------|------|
| A1 | `type App struct` | 25-108 | 应用状态（38 个字段） |
| A2 | `type ViewType` / `const ViewXxx` | 118-132 | 视图枚举（10 个） |
| A3 | `type LyricSearchUI struct` | 110-116 | 歌词搜索 UI 状态 |
| A4 | `NewApp()` | 135-168 | 构造函数 |
| A5 | `Init()` | 171-174 | 初始化 |
| A6 | `Run()` | 1893-1897 | 启动 TUI 程序 |
| A7 | `fullRepaintCmd()` | 1899-1904 | 全屏重绘辅助 |

### B. 消息类型

| # | 类型 | 行号 | 用途 |
|---|------|------|------|
| B1 | `LoginSuccessMsg` | 1907-1909 | 登录成功 |
| B2 | `LoginErrorMsg` | 1911-1913 | 登录失败 |
| B3 | `DeviceCodeMsg` | 1915-1917 | 设备码获取 |
| B4 | `PlaylistsLoadedMsg` | 1919-1921 | 播放列表加载 |
| B5 | `ForceRenderMsg` | 1924 | 强制重渲染 |
| B6 | `TickMsg` | 1927 | 启动屏流光定时器 |
| B7 | `SplashAnimationDoneMsg` | 1930 | 启动屏动画完成 |
| B8 | `LoadingAnimationMsg` | 1933 | 加载动画定时器 |
| B9 | `PlayerUpdateMsg` | 1936 | 播放器状态更新 |
| B10 | `SongChangedMsg` | 1939-1941 | 歌曲切换 |
| B11 | `FilesLoadedMsg` | 1944-1947 | 文件列表加载完成 |
| B12 | `FileSelectionChangedMsg` | 1950-1952 | 文件选择变更 |
| B13 | `FolderFilesLoadedMsg` | 1955-1957 | 文件夹文件加载完成 |

### C. 渲染函数

| # | 函数 | 行号 | 视图 |
|---|------|------|------|
| C1 | `renderSplashView` | 426-441 | 启动屏 |
| C2 | `renderSplashContent` | 444-493 | 启动屏 |
| C3 | `renderLoginView` | 496-554 | 登录 |
| C4 | `renderPlaylistView` | 557-663 | 播放列表 |
| C5 | `renderPlayerView` | 666-779 | 播放器 |
| C6 | `renderProgressBar` | 782-836 | 播放器 |
| C7 | `renderControls` | 839-859 | 播放器 |
| C8 | `renderLyrics` | 862-917 | 播放器 |
| C9 | `renderShimmerText` | 922-972 | 共享效果 |
| C10 | `renderHelpView` | 1028-1121 | 帮助 |
| C11 | `renderInputView` | 1124-1151 | 新建播放列表 |
| C12 | `renderDeleteConfirmView` | 1154-1195 | 删除确认 |
| C13 | `renderRenameView` | 1198-1234 | 重命名 |
| C14 | `renderFileBrowserView` | 1237-1465 | 文件浏览器 |
| C15 | `renderLyricSearchView` | lyrics_handler.go:18-113 | 歌词搜索 |

### D. 按键处理

| # | 函数 | 行号 | 职责 |
|---|------|------|------|
| D1 | `handleKeyPress` | 1468-1841 | 主按键分发（20+ case） |
| D2 | `handleRenameKeyPress` | 1844-1890 | 重命名输入 |
| D3 | `handleInputKeyPress` | 2203-2262 | 新建播放列表输入 |
| D4 | `handleDeleteConfirm` | 2265-2281 | 删除确认 |
| D5 | `handleFileBrowserKeyPress` | 2284-2412 | 文件浏览器 |
| D6 | `handleLyricSearchViewKeyPress` | lyrics_handler.go:116-261 | 歌词搜索 |

### E. 业务逻辑 & Commands

| # | 函数 | 行号 | 领域 |
|---|------|------|------|
| E1 | `checkLogin` | 1960-1979 | 登录 |
| E2 | `startPolling` | 1981-2014 | 登录 |
| E3 | `loadPlaylists` | 2016-2023 | 播放列表 |
| E4 | `startSplashAnimation` | 2026-2030 | 启动屏 |
| E5 | `tick` | 2033-2037 | 启动屏 |
| E6 | `waitForSplash` | 2040-2044 | 启动屏 |
| E7 | `startPlayerUpdateTicker` | 2047-2051 | 播放器 |
| E8 | `resumePlayerUpdates` | 2060-2062 | 播放器 |
| E9 | `loadLyricsForTrack` | 975-1025 | 歌词 |
| E10 | `loadFiles` | 2465-2478 | 文件浏览器 |
| E11 | `tickLoadingAnimation` | 2481-2485 | 文件浏览器 |
| E12 | `addFolderFiles` | 2488-2501 | 文件浏览器 |
| E13 | `cyclePlaybackSpeed` | 2509-2540 | 播放器 |
| E14 | `updateRecentPlaylist` | 2550-2581 | 播放器 |
| E15 | `getSelectedFile` | 2415-2443 | 文件浏览器 |
| E16 | `toggleFileSelection` | 2446-2462 | 文件浏览器 |
| E17 | `computeWindowTitle` | 2067-2091 | 窗口标题 |
| E18 | `updateWindowTitleCmd` | 2095-2102 | 窗口标题 |
| E19 | `resetWindowTitle` | 2105-2108 | 窗口标题 |
| E20 | `handleLyricSearch` | lyrics_handler.go:280-321 | 歌词搜索 |
| E21 | `confirmLyricSelection` | lyrics_handler.go:324-339 | 歌词搜索 |
| E22 | `handleLyricUpload` | lyrics_handler.go:342-372 | 歌词上传 |
| E23 | `uploadLyricsToBaidu` | lyrics_handler.go:375-413 | 歌词上传 |

### F. 工具函数

| # | 函数 | 行号 | 说明 |
|---|------|------|------|
| F1 | `renderShimmerText` | 922-972 | 流光文字效果（无 App 接收者） |
| F2 | `sanitizeTitle` | 2111-2135 | 清理终端标题 |
| F3 | `max` | 2138-2143 | 取最大值 |
| F4 | `min` | 2146-2151 | 取最小值 |
| F5 | `formatTime` | 2154-2158 | 格式化时间 |
| F6 | `formatFileSize` | 2161-2171 | 格式化文件大小 |
| F7 | `generateQRCode` | 2174-2200 | 生成二维码 ASCII |
| F8 | `formatSpeed` | 2543-2548 | 格式化播放倍速 |
| F9 | `extractSongName` | lyrics_handler.go:416-431 | 提取歌曲名 |
| F10 | `showMessage` | lyrics_handler.go:434-438 | 显示消息（App 方法） |
| F11 | `var playbackSpeeds` | 2506 | 倍速选项列表 |

---

## 迁移映射

### app.go（目标 ~200 行）

```
A1-A7: App struct, ViewType, LyricSearchUI, NewApp, Init, View, Run, fullRepaintCmd
B1-B13: 全部消息类型
Update() 分发器（保留在 app.go 作为核心入口）
```

### views.go（目标 ~1100 行）

```
C1-C15: 全部渲染函数
```

### keyhandlers.go（目标 ~600 行）

```
D1-D6: 全部按键处理函数
```

### commands.go（目标 ~500 行）

```
E1-E23: 全部业务逻辑 + tea.Cmd 生成器
```

### helpers.go（目标 ~100 行）

```
F1-F11: 全部工具函数 + 变量
```

### 删除文件

```
lyrics_handler.go → 内容归入 views.go, keyhandlers.go, commands.go, helpers.go
```

---

## 执行注意事项

1. **不改函数签名和逻辑**，纯文件切分
2. **同包内移动**，不引入新 package
3. 拆分后执行 `go build ./...` 确认编译通过
4. 拆分后执行 `gofmt -w .` 确保格式一致
5. 每个文件顶部保留 `package tui` 声明和需要的 import
6. 删除 `lyrics_handler.go` 后，其内部消息类型 `lyricSearchDoneMsg`、`lyricDownloadDoneMsg` 移到 `commands.go`，与生产它们的 `handleLyricSearch`、`confirmLyricSelection` 放在一起