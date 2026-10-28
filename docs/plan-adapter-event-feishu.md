# Adapter 完善：统一 Event + 生命周期 + 飞书 Driver 实施计划

> 状态：已实施完成（2026-10-28）
> 变更记录：飞书 Driver 由 HTTP Webhook 回调模式（初版）改为 **WebSocket 长连接模式**（用户决定），
> 引入官方 SDK `github.com/larksuite/oapi-sdk-go/v3 v3.12.2`（larkws 接收 + lark.NewClient 发送）。
> 基于现状：重构后的 `Runtime` + `ConfigManager` + `driver/` 包（保留 sonic、配置 MD5 对比等现有写法）

---

## 一、common 包：统一 Event 结构（新增 event.go）

```go
type EventType string
const (
    EventMessage EventType = "event.message" // 消息事件
    EventNotify  EventType = "event.notify"  // 通知事件 (戳一戳/lifecycle/平台自定义)
)

type Event struct {
    Type    EventType
    SelfID  string       // 机器人自身 ID
    Message *Message     // Type==EventMessage 时非 nil
    Notify  *NotifyEvent // Type==EventNotify 时非 nil
}

type NotifyEvent struct {
    Kind     string   // 通知种类: event.notify.poke / event.meta.lifecycle / 平台自定义
    Platform Platform
    Data     []byte   // 平台原始 JSON, 业务层按平台解析
}
```
- `common/driver.go`：删除 `UnionEvent`（被 NotifyEvent 取代）；`RuntimeState` 保持
- `common/core.go`：新增 `SendReceipt`（onebot 层已在用）

## 二、Driver 接口与 onebot driver：管道合流

**driver/driver.go**：
```go
type Driver interface {
    Start() error; Stop() error; Done() <-chan struct{}
    Events() <-chan common.Event          // 统一事件管道
    GetCurrentState() *common.RuntimeState
}
```

**driver/onebot/**：
- `events chan common.Event` 替代 inbound+notices；写侧保持 done-select 背压；Stop 六步不变（第 6 步只 close(events)）
- 删除内部冗余 `Config` 结构，直接持有 `config.OneBotConfig`
- `SendGroupMsg/SendPrivateMsg` 的 groupID/userID 改为 `chatID string`（内部 ParseInt）

## 三、Runtime 完善（runtime.go + handler.go + api.go）

**事件管道**：
```go
type Runtime struct {
    ConfigManager *config.ConfigManager
    OnebotDriver  *OnebotDriverV11
    FeishuDriver  *FeishuDriverV11
    events chan common.Event         // 业务层消费 (缓冲 256)
    done chan struct{}; wg sync.WaitGroup
}
func (r *Runtime) Events() <-chan common.Event
```
- 每个 driver 启动时起 forward goroutine：`driver.Events() → rt.events`
- 删除 `OnebotDriverV11.restSignal` 死字段与 `configKind` 泛型

**生命周期**：
```go
var defaultRuntime *Runtime
type Option func(*options); func WithDB(db *gorm.DB) Option

func CreateRuntime(opts ...Option) (*Runtime, error)
    // db 未注入则默认 infrastructure/postgres 连接
    // → ConfigManager.SetupConfig()（onebot+feishu 表幂等初始化）
    // → Enable=true 的 driver: NewDriver+Start+forward

func (r *Runtime) ReleaseRuntime() error   // 幂等: close(done) → 停 driver → wg.Wait → close(events)
func ReleaseRuntime() error                // 包级
```

**配置变更→重建**：
- `reCreateOnebotDriverV11`：Stop 旧 → NewDriver(新配置) → Start → 重挂 forward
- enable/disable 落库 + Start/Stop；`UpdateRuntimeConfig` 变化 → 落库 → 运行中则 reCreate

**发送 API**：
- `SendGroupMessage/SendPrivateMessage(target, chatID string, parts)`；default 分支报错；route() 支持 target 空自动路由

## 四、config 包：飞书配置

```go
type FeishuConfig struct {
    ID/CreatedAt/UpdatedAt/DeletedAt
    Addr string  // 默认 0.0.0.0:8091
    AppID string; AppSecret string; VerificationToken string
    Enable bool; EventBuffer int // 默认 64
} // TableName: feishu_configs
```
- ConfigManager 提炼泛型 helper（setupConfig[T]/getConfig[T]），新增 Feishu 三个方法

## 五、飞书 Driver（driver/feishu/，WebSocket 长连接，官方 SDK）

```
driver/feishu/driver.go   # Driver 实现 (larkws 接收 + lark.NewClient 发送)
driver/feishu/modules.go  # 通用结构体 (Mention 等, 事件/API 结构由 SDK 提供)
driver/feishu/convert.go  # 飞书 content ↔ common.MessagePart
```
- **接收**：`larkws.NewClient(AppID, AppSecret)` 长连接, `dispatcher.OnP2MessageReceiveV1`
  回调 → 转换 `common.Message`（text/post/image/file、@ 占位符、MentionedSelf 需注入
  `SetSelfOpenID`）进 events；未支持类型 → Notify；机器人 app 消息跳过防自循环。
  无需公网地址, `FeishuConfig` 已去除 Addr。
- **发送**：`lark.NewClient` OpenAPI（token 由 SDK 管理缓存）→ `Im.V1.Message.Create`
  （群=chat_id, 私聊=open_id）/ `Reply`（ReplyPart）；部件映射：纯文本类→text（Mention→`<at>`）/
  纯图→image / 混合→post；回执 `Data.MessageId` → `common.SendReceipt`。
- 测试注入点：`SetWSClient`（mock 长连接）/ `SetOpenBaseUrl` + `SetHTTPClient`（拦截 OpenAPI）。
- 生命周期细节：`Start` 建 events + apiClient + ws goroutine；`Stop` 取消 ws ctx →
  `markStopped` 幂等 close(done/events) → `wg.Wait`；长连接意外断开同样触发 markStopped。

## 六、测试（只测新代码，不碰数据库）

- feishu：challenge / token 拒绝 / message 事件转换 / event_id 去重 / token 缓存+发送构造
- onebot：gws 端到端（message→Event{Message}、poke→Event{Notify}、Stop 后 events 关闭）
- runtime 转发：db=nil 手动构造验证 forward 与 Events()

## 七、文档

- AGENT.md：分层图（Events()）、Event 结构、CreateRuntime/ReleaseRuntime、飞书接入、chatID string 变更
- 本计划落盘 docs/plan-adapter-event-feishu.md

## 不改动

- onebot Stop 六步顺序与 done-select 模式
- equal() 的 sonic+MD5 实现
- config/viper、cmd、internal/api、web/
