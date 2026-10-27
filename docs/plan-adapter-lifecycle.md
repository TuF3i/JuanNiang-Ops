# 完善 OneBot11 Adapter 实施计划

> 状态：已批准实施（2026-10-27）
> 项目：JuanNiang-Ops（Go 1.27，gorm+PostgreSQL，lxzan/gws 正向 WS）
> 范围：仅 OneBot11；不接 cmd/main 入口；HTTP API（internal/api）不在本次范围
>
> 已确认决策：
> ① `Inbound()` 恢复统一消息模型 `common.Message`（在 onebot.Adapter 内完成转换）
> ② `onebot_configs` 单条配置，AdapterRuntime 持有单实例，重建 = Stop 旧实例 + NewAdapter + Start

---

## 一、onebot 包：事件接收完善

位置：`internal/adapter/onebot/`

### 1.1 adapter.go

| 改动 | 说明 |
|---|---|
| `inbound chan UnionEvent` → `chan common.Message` | `Inbound() <-chan common.Message`，符合 AGENT.md 契约 |
| 新增 `notices chan UnionEvent`（缓冲 64）+ `Notices() <-chan UnionEvent` | 承载 notice（戳一戳等）与 lifecycle（机器人上下线）事件，原始 JSON 透传，业务层自行解析 `PokeEvent` / `LifeCycleEvent`；meta 心跳仍只刷新 `lastBeat`，不入管道 |
| 恢复 `onMessageEvent` 转换（原为注释代码） | `MessageEvent` → `common.Message`：`convertSegments` 接线、Platform=Onebot、ChatID=群号/QQ号、Sender=Card\|\|Nickname、MentionedSelf、`Original=json.RawMessage(data)`；selfID 为 0 时用连接绑定的 peer selfID 兜底。删除全部注释掉的旧实现 |
| `onNoticeEvent` → 推 `notices` | 不再进 inbound |
| `onMetaEvent` 的 lifecycle 事件 → 推 `notices` | EventType=`MetaEventType` |
| `Start()` | 同时建立 notices 管道 |
| `Stop()` | 第 6 步扩展为 `close(inbound)` + `close(notices)`；**Stop 六步顺序不动**，写侧一律走既有 `select { <-done / ch<-v }` 模式 |
| `GetCurrentState()` | `PeerList` 由未导出的 `map[string]*peer` 改为导出快照 `map[string]PeerState`（原实现泄漏未导出类型，外部无法使用） |

### 1.2 modules.go

- 新增导出结构 `PeerState{Session, SelfID, LastBeat, RemoteAddr string}`
- `CurrentState.PeerList` 类型替换为 `map[string]PeerState`
- 修正 `CurrentStatus` 注释笔误（实际类型名为 `CurrentState`）

### 1.3 adapter_test.go（重建测试）

约定：包内测试、真实 gws 客户端端到端、所有等待带超时（`waitFor` / `recvInbound` / `waitPeer` 模式）。

用例清单：
1. Start / Stop / 再 Start（重启可用）
2. 错误 Token → 鉴权拒绝
3. message 事件 → Inbound 收到字段正确的 `common.Message`（含 MentionedSelf、Original）
4. poke notice → Notices 收到 UnionEvent
5. Call 往返（echo 成功 / retcode≠0 返回错误）
6. 双端 Target 精确路由；多端未指定 Target → `ErrAmbiguousPeer`
7. Stop 后：Inbound 关闭、Call 返回 `ErrNotRunning`

---

## 二、adapter 包：gorm 钩子 + 生命周期管理器

位置：`internal/adapter/`

### 2.1 hooks.go（新文件）— 包级配置变更通知

```go
// 包级 observer, atomic.Value 存 func()
setConfigObserver(fn func())      // runtime 启动时注册
notifyConfigChanged()             // 钩子触发时非阻塞通知; observer 未注册时静默忽略
```

在 `dto.go` 的 `OneBotConfig` 上挂 gorm 钩子（只挂这三个，避免 Create/Save 双触发）：

- `AfterCreate` / `AfterUpdate` / `AfterDelete` → `notifyConfigChanged()`

⚠️ 约束（写入注释）：gorm 的批量 `Update` 若不带主键（`Model(&X{}).Where(...)`），**不会触发 AfterUpdate 钩子**。更新配置必须带主键（便捷 API `UpdateConfig` / `SetEnabled` 已保证）。

### 2.2 runtime.go（重写 AdapterRuntime）

```go
type MessageHandler func(ctx context.Context, msg common.Message)
type NoticeHandler  func(ctx context.Context, ev onebot.UnionEvent)

type AdapterRuntime struct {
    mu      sync.Mutex        // 保护 adapter/cfg, 且串行化 reconcile
    db      *gorm.DB
    adapter *onebot.Adapter   // 当前实例 (可 nil)
    cfg     OneBotConfig      // 当前生效配置快照

    hmu            sync.RWMutex // 保护 handlers
    msgHandlers    []MessageHandler
    noticeHandlers []NoticeHandler

    notify chan struct{}     // gorm 钩子 → reconcile 触发 (缓冲1, 合并抖动)
    done   chan struct{}     // runtime 关闭信号
    ctx    context.Context   // 根 ctx, Stop 时取消 (传给 handler)
    cancel context.CancelFunc
    wg     sync.WaitGroup
    started bool
}
```

方法：

| 方法 | 行为 |
|---|---|
| `NewAdapterRuntime(db *gorm.DB)` | 构造 |
| `Setup(ctx)` | `AutoMigrate(&OneBotConfig{})` + 读配置行，空则插入默认行（Addr=`:8080`、Enable=false） |
| `Start()` | 注册 observer → 启动 reconcileLoop goroutine → 执行首次 reconcile |
| `Stop()` | close(done) → Stop 当前 adapter → cancel ctx → wg.Wait；幂等 |
| reconcile（mu 内） | 读第一条配置（无则 FirstOrCreate 默认行）→ 与快照对比（Addr/Token/Admin/Enable/buffers）→ 未变直接返回；变了则 Stop 旧实例 → `Enable=false` 保持停止 / `true` 则 NewAdapter + Start + 起 pump goroutine |
| `Reload()` | 手动触发一次 reconcile |
| `SetEnabled(ctx, bool)` | First 取行 → 改 `enable` → Save（带主键，钩子必触发）→ reconcile 自动启停（**运行时开关**） |
| `UpdateConfig(ctx, mutate func(*OneBotConfig))` | First → mutate 改字段 → Save → 自动重建 |
| `GetStatus()` | adapter 运行态 + 当前配置 + peer 快照 |
| `OnMessage(h)` / `OnNotice(h)` | 运行时注册 handler（hmu 保护） |

reconcile 稳定性设计：
- notify 缓冲 1 + 非阻塞发送 → 钩子密集触发自动合并
- reconcile 幂等（对比无变化即 no-op）
- Setup 自建默认行触发的 AfterCreate 钩子 → 自发通知 → reconcile 后自消（无害）

### 2.3 handler.go（事件泵与分发，原为空壳）

- `MessageHandler` / `NoticeHandler` 类型定义
- 每个 adapter 实例一个 pump goroutine：
  `select { case msg := <-adapter.Inbound() / case ev := <-adapter.Notices() / case <-退出 }`
- 收到事件后逐个**同步**调用已注册 handler（注释说明：handler 应快速返回或自行开 goroutine）
- handler 调用带 `recover`，防单个 handler panic 打崩事件流
- ctx 传 runtime 根 ctx（Stop 时取消）
- pump 随 adapter 的 inbound/notices 关闭自然退出，无需额外通知

### 2.4 管理 API 汇总（供未来 internal/api 对接）

```
Setup(ctx)  Start()  Stop()  Reload()
SetEnabled(ctx, bool)  UpdateConfig(ctx, mutate)  GetStatus()
OnMessage(h)  OnNotice(h)
```

---

## 三、runtime_test.go（新增）

依赖真实 PostgreSQL（环境变量提供 DSN，未设置则 `t.Skip`）：

1. Setup 建表 + 默认行创建
2. 改 `enable=true` → adapter 自动启动
3. 改 `addr` → adapter 自动重建
4. `enable=false` → 自动停止
5. Delete 配置行 → 自愈为默认配置

---

## 四、文档与验证

- 更新 `AGENT.md` 适配器架构章节：
  - Inbound 为 `common.Message`；新增 Notices 管道说明
  - runtime 生命周期 / reconcile / gorm 钩子机制
  - 约束：批量 Update 不带主键不触发钩子
  - Stop 第 6 步含 close(notices)
- 验证命令：

```bash
go build ./...
go vet ./...
go test ./internal/adapter/... -count=1
go test ./internal/adapter/onebot/ -race
```

---

## 五、不改动的部分

- `onebot.Adapter.Stop()` 六步顺序、done-channel 写入模式（AGENT.md 勿改项）
- 鉴权、路由（route/bindSelfID）、Call pending 机制
- `infrastructure/postgres`、`config/viper`、`cmd`、`internal/api`（空壳不在本次范围）
