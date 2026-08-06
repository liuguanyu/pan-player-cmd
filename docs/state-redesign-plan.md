# PlaybackState 并发安全重构方案:状态机 + 事件总线

## 目标

解决 `PlaybackManager.GetState()` 返回共享指针被多 goroutine 并发读写导致的数据竞争问题。

## 问题回顾

- `PlaybackManager.GetState()` 返回 `*models.PlaybackState` 指针,锁只保护取指针,不保护字段读写。
- 三条并发路径冲突:
  - 路径A: `updatePositionLoop` 每 100ms 写 `CurrentTime`/`Duration`(持锁)。
  - 路径B: `LoadTrack` goroutine 写 `CurrentSong`/`IsPlaying`(无锁)。
  - 路径C: TUI 渲染 + 键盘 读/写 `LyricsParsed`/`Volume` 等(无锁)。

## 方案:不可变快照 + 事件广播

### 核心思想

Player 不再暴露可变 `GetState()`,改为发布不可变状态快照。消费者(TUI)订阅事件流,收到快照后用本地副本渲染,彻底消除共享可变状态。

### 关键组件

1. **StateSnapshot**:不可变的状态快照(值类型,字段全为值类型或不可变引用)。从 `models.PlaybackState` 改造,指针类字段(`CurrentSong`/`LyricsParsed`)改为值拷贝。
2. **EventBus**:`Player.Subscribe() <-chan StateSnapshot`。内部 goroutine 持锁生成快照广播。
3. **Player 内部**:所有 setter(`SetVolume`/`SetSpeed`/`SetPlayMode`/`LoadTrack` 回调等)改为持锁更新内部 state,然后发布新快照。
4. **TUI App**:订阅快照,收到后存为本地 `lastSnapshot` 字段,渲染只读本地副本。不再调用 `GetState()`。
5. **Bubble Tea 集成**:收到 snapshot 后包装成 `tea.Msg`(如 `SnapshotMsg{State StateSnapshot}`),走标准 Update 分发。

### 改造范围

- `internal/models/models.go`:`PlaybackState` 字段从指针/slice 改为值类型或约定不可变。
- `internal/player/manager.go`:移除 `GetState`,新增 `Subscribe` + `emitSnapshot`。
- `internal/player/api.go`:`LoadTrack`/`Play`/`Pause`/`SetVolume` 等改用内部 setter + 发布。
- `internal/tui/app.go` 及子文件:订阅 snapshot,移除所有 `a.player.GetState()` 调用。

### 兼容性

- 一次性切换,不分阶段兼容(单仓库、单人项目,无需灰度)。
- 保留 `PlaybackState` 名字,内部实现变不可变。
- 更新路径:Player 构造时启动 publisher goroutine,`Subscribe` 返回带缓冲 channel。

### 回归测试计划

- `go build -race ./...` 必须无 race 报告。
- 手动验证:切歌、Seek、调音量、切播放模式、歌词加载并发场景。
- 为 `generateShuffleOrder`/`PlayNext` 等纯函数补表驱动测试。

### 风险

- channel 缓冲满会丢快照:用足够大缓冲(如 16)+ 非阻塞发送,慢消费者容忍。
- TUI 已有 `PlayerUpdateMsg` 机制,迁移成本可控。
- 改动面大,需一次完成不可中途编译。
