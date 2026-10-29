# AGENT.md

本文件面向在本仓库中工作的 AI Agent（以及新加入的人类开发者），
描述项目结构、编码约定与已知陷阱。改动代码前请先读完本文。

## 项目概述

**JuanNiang-Ops（卷娘巡检姬）**：一个 QQ 机器人运维/巡检平台。
通过 OneBot v11 正向 WebSocket 接入机器人实现端（如 go-cqhttp / Lagrange / NapCat），
将平台事件转换为统一消息模型后交给业务层处理。

- 语言：Go 1.27+（module 名为 `JuanNiang-Ops`，import 路径以此为前缀）
- 核心依赖：`gorm` + PostgreSQL、`go-redis`、`lxzan/gws`（WebSocket）、`google/uuid`
- 注释与文档：统一使用**中文**
- 模块代理：`GOPROXY=https://goproxy.cn,direct`（已配置，无需改动）

## 常用命令

```bash
go build ./...                                  # 编译
go vet ./...                                    # 静态检查
go test ./internal/adapter/... -count=1         # 运行测试
go test ./internal/adapter/onebot/ -race        # 竞争检测
go mod tidy                                     # 依赖整理
```

注意：在低内存机器上编译时，全量重编依赖树可能吃满内存。
可用 `-p 2` 限制编译并行度（如 `go test -p 2 ./...`）；
依赖缓存（`~/.cache/go-build`）预热后编译是秒级的，不要随意删除缓存目录。

## 目录结构

```
cmd/                    程序入口（cobra，待实现）
config/viper/           配置加载（viper，待实现）
internal/
  adapter/              平台适配器 ★ 组装层 + 分包业务
    runtime.go          唯一顶层文件: Runtime 组装/创建/释放 (Option: WithDB)
    modules/            全部数据模型（message / event / driver / config）
    dao/                数据库操作（配置表读写，gorm）
    service/            运行时状态与业务方法（生命周期 / 统一 API / 发送路由 / 驱动包装）
    driver/             Driver 统一接口 + 平台实现
      onebot/           OneBot v11 正向 WS 驱动（含单测）
      feishu/           飞书 WSS 长连接驱动（官方 SDK，含单测）
  agent/                Agent 编排（待实现）
  api/                  HTTP API 模块（gin，依赖注入）
    runtime.go          CreateRuntime/ReleaseRuntime（Option: WithDB/WithAddr，默认 :8082）
    handler/            gin 路由与请求处理（统一响应 {"code","msg","data"}）
    middleware/         gin 中间件（Auth 会话鉴权：Bearer token -> ctx 注入用户名）
    service/            业务层（bcrypt 密码加密 / 内存 Token 会话 / 登录改密）
    dao/                数据访问（user_table：AutoMigrate + 默认 admin 种子）
    modules/            gorm 模型（User）
  db/                   数据访问（dao / cache，待实现）
  workflow/             工作流（待实现）
infrastructure/         基础设施客户端（postgres / redis / t2i）
pkg/                    可对外复用的库
sql/                    数据库迁移脚本（空）
web/                    前端（Vue3 + Vite 脚手架）
deployment/             部署清单（空）
docs/                   文档（含实施计划）
```

约定：每个模块目录下用 `runtime.go` 承载模块级文档/约定；空目录放 `.gitkeep`。

## 适配器架构（internal/adapter）

分层（与 internal/api 同构，顶层 runtime 只负责组装）：

```
业务层 (agent / workflow / api)
  │ 消费: rt.Service.Events() <-chan modules.Event
  │ 发送: rt.Service.SendGroupMessage / SendPrivateMessage / ReplyMessage
  ▼
Runtime (runtime.go)      ← 只负责组装与创建/释放 (db → dao → service → 驱动装配)
  ▼
service.Service           ← 运行时状态持有者: 驱动生命周期 / 配置读写 / 发送路由 / 事件合流管道
  │ dao (gorm)                driver.Driver 接口 (隐式匹配)
  ▼                           ▼
modules (数据模型)        onebot.Driver / feishu.Driver (平台实现层)
```

- **modules/**（全部数据模型，按用途分文件）：`message.go`（Platform/MessageType/Sender/Message/
  MessagePart 各部件/SendReceipt）、`event.go`（Event{Type, SelfID, Message, Notify}）、
  `driver.go`（RuntimeState/PeerState）、`config.go`（OneBotConfig/FeishuConfig + 默认值常量）。
- **dao/**（数据库操作）：`Dao{db}`，`SetupConfig`（AutoMigrate + 幂等种子行）/ 两个平台的
  Get/UpdateConfig；只做 gorm 操作，业务在 service。
- **service/**（运行时状态与方法）：
  - `Service{dao, Onebot, Feishu, events, done, wg, mu}`；`Events()` 合流管道。
  - `lifecycle.go`：`SetupDrivers`（按配置启动+挂转发泵）、`EnableDriver/DisabledDriver`
    （落库+Start/Stop）、`reCreateXxx`（Stop 旧 → New → Start → 重挂泵，事件管道不变）、`Release`（幂等）。
  - `api.go`：`SendGroupMessage/SendPrivateMessage/ReplyMessage`、
    `UpdateRuntimeConfig`（sonic+MD5 对比 → 落库 → 运行中则重建）、`GetAdapterStatus/GetAdapterConfig`。
  - `route.go`：发送路由 —— target 为 self_id，空默认飞书；非空精确匹配 OneBot peer，未接入报错。
  - `wrapper.go`：OnebotDriverV11/FeishuDriverV11 包装（driver 实例+cfg+ctx）+ 转发泵。
- **Runtime 生命周期（runtime.go）**：
  - `CreateRuntime(opts ...Option)`：建库（可 `WithDB` 注入，否则 infrastructure 默认连接并自持）
    → `dao.SetupConfig` → `service.NewService` → `SetupDrivers`。包级 `defaultRuntime` 记录。
  - `rt.ReleaseRuntime()` / 包级 `ReleaseRuntime()`：幂等；停全部驱动 → 转发泵退出
    → close(events) 通知业务层 → 自建 db 关闭。
- **平台实现职责**：onebot —— 正向 WS 服务端（NapCat 等接入）、OneBot 事件 ↔ 统一模型双向转换
  （`convertSegments`/`PartsToSegments`）、echo 请求-响应 Call；feishu —— **WebSocket 长连接**
  （官方 SDK `larksuite/oapi-sdk-go`：`larkws` 收事件 / `lark.NewClient` 发 OpenAPI，
  token 由 SDK 管理，无需公网回调地址）、
  消息转换（纯文本→text / 纯图→image / 混合→post、ReplyPart 走 reply 接口）。
  测试注入点：`SetWSClient`（mock 长连接）/ `SetOpenBaseUrl` + `SetHTTPClient`（拦截 OpenAPI）。

每个平台 Driver 必须提供：

| 成员 | 语义 |
|---|---|
| `Start() / Stop()` | 生命周期；Start 幂等（重复调用报错），Stop 幂等且阻塞至完全退出 |
| `Events()` | 统一事件管道 `<-chan modules.Event`（消息+通知合流），Stop 时关闭 |
| `Done()` | 停止信号 `<-chan struct{}`，与管道配合消费 |

统一消息模型（`modules`）：`Message` 由 `[]MessagePart` 组成（Text/Mention/Image/File/Reply…），
适配器负责平台事件 ↔ 统一模型的双向转换，业务层不接触平台原生结构。

OneBot 适配器要点：

- 三条管道：`Inbound() <-chan common.Message`（message 事件转换后的统一消息）、
  `Notices() <-chan UnionEvent`（notice / lifecycle 事件原始 JSON，业务层自行解析 `PokeEvent`/`LifeCycleEvent`）、
  `Outbound() chan<- *Action`。
- 事件按 `post_type` 分发：`message` → 转换为 `common.Message`（`convertSegments`）进 Inbound；
  `meta_event` 心跳 → 更新 `peer.lastBeat`；`meta_event` lifecycle → Notices；
  `notice`（戳一戳等）→ Notices。
- 消息事件未携带 `self_id` 时用连接已绑定的身份兜底（`connSelfID`）。
- 首个事件携带的 `self_id` 用于绑定连接；同 `self_id` 重复接入会顶掉旧连接。
- 多机器人：`Action.Target` 指定 `self_id` 精确路由；单端自动路由；多端未指定返回 `ErrAmbiguousPeer`。
- 消息转换是双向的：接收侧 `convertSegments`（OneBot 段 → `common.MessagePart`），
  发送侧 `PartsToSegments`（统一部件 → OneBot 段，未知部件/非法 ID 返回错误而非静默丢弃）。
- 实现 `MessageSender`：`SendGroup/SendPrivate(ctx, chatID, parts, selfID)` 等待回执并填充
  `common.SendReceipt.MessageID`（自 `ApiResult.Data.message_id` 提取，`Reply` 字段保留原始 `*ApiResult`）；
  `selfID` 传空时单端自动路由、多端返回 `ErrAmbiguousPeer`。
- `Call(ctx, target, action, params)`：echo 关联的请求-响应模式；本地失败（路由/序列化/写连接）
  经 pending 通道直接返回原始错误（`errors.Is` 可判 `ErrNoPeer/ErrUnknownPeer/ErrAmbiguousPeer/ErrNotRunning`）；
  远端 retcode≠0 返回错误并包装 `uerr.ErrApiFailed`。
- `GetCurrentState()` 返回状态快照，`PeerList` 为导出结构 `map[string]PeerState`（Session/SelfID/RemoteAddr/LastBeat）。
- 握手鉴权：`gws.ServerOption.Authorize`，校验 `Authorization: Bearer/Token` 或 `?access_token=`；Token 为空则放行。

### Stop 的顺序（勿改）

1. 置 `running=false`，`close(done)`
2. 关闭 listener（停止接入）
3. 断开所有客户端连接
4. 等待中的 `Call` 全部回执失败
5. `wg.Wait()` 等内部 goroutine 退出
6. 关闭 Inbound 与 Notices（通知下游），清空连接表（允许重启）

管道写入一律走 `select { case <-done / case ch <- v }` 模式，
保证 Stop 关闭通道时不会出现 send-on-closed panic。

## API 模块（internal/api）

用户管理（单 admin 用户）与业务接口的 HTTP 接入层，依赖注入与 adapter 同风格：

- **装配**：`CreateRuntime(WithDB(db), WithAddr(":8082"))` → Dao → Service → Handler → gin 路由
  → `SetupUser`（AutoMigrate + 默认 admin 种子，`OnConflict DoNothing` 幂等）→ goroutine `ListenAndServe`。
  db 未注入则自建 infrastructure 默认连接（ownDB，Release 时关闭）。
  `ReleaseRuntime()` 幂等优雅关停；`Engine()` 暴露 gin 引擎（测试/cmd 挂载）。
- **分层约束**：handler 只做参数绑定与错误映射；bcrypt 加密、Token 签发/校验在 Service 层；
  Dao 只管 gorm。密码哈希不出 Service（`GetUserInfo` 返回前脱敏）。
- **密码**：bcrypt（`golang.org/x/crypto`，默认 cost），数据库只存哈希；
  默认管理员 `admin/admin123`（与前端 Mock 提示一致），生产部署后应立即改密。
- **会话**：内存 Token（32 字节随机 hex → 用户名，24h 滑动续期），服务重启即失效；
  登录失败统一返回"用户名或密码错误"（防用户名枚举）。后续多实例/持久化时替换为 redis，接口不变。
- **接口**：`POST /api/v1/login`（公开）、`GET /api/v1/user/info`、`PUT /api/v1/user/password`
  （后两者走 `middleware.Auth(tokens)` 中间件：`Authorization: Bearer <token>`，失败 401）。
- **统一响应**：`{"code":0,"msg":"ok","data":{...}}`；可判定错误用包级 sentinel
  （`ErrInvalidCredential`/`ErrWrongPassword`/`ErrUserNotFound`/`ErrTokenInvalid`），handler 按 `errors.Is` 映射 HTTP 状态码。
- **测试**：`runtime_test.go` 用 httptest 走完整 DI 链，依赖 `TEST_PG_DSN`（未设置自动 Skip）。

## 编码规范

- **常量必须带类型**：`Group MessageType = "message.public"`。
  无类型常量会按默认类型推导（曾导致 `msgType := common.Group` 推导成 `string` 的编译错误）。
- **接口签名保持一致**：`MessagePart.PartType() MessageMIME`，实现与接口一字不差。
- **错误处理**：包内定义 sentinel error（`ErrXxx`），调用方用 `errors.Is` 判断；
  包装错误用 `%w`，携带上下文。
- **并发**：互斥锁保护共享 map；管道关闭遵循上文 done 模式；goroutine 退出路径必须有 `wg.Done` 或明确的所有者。
- **日志**：统一 `[onebot] ` 前缀（各模块用自己的前缀），只记录一次性事件，
  严禁在循环中无限制打日志（见下方陷阱）。
- 公共 API 变更需同步更新测试与本文档。

## 已知陷阱（重要）

1. **`gws.RunListener` 不会因 `listener.Close()` 退出** —— 它在 accept 错误上
   无限循环刷日志（v1.10.2 `upgrader.go`），会吃满 CPU 并把内存撑爆。
   因此 `onebot.Adapter` **自行实现 accept 循环**（`runWebSocket` + `handleConn`，
   用 `Upgrader.UpgradeFromConn` 升级连接），判断 `net.ErrClosed` 后正常返回。
   改动网络层时不要回退到 `RunListener`。
2. gws 的 `*gws.Message` 用完必须 `defer message.Close()` 回收缓冲。
3. `UpgradeFromConn` 失败时内部已写回 HTTP 400 并关闭连接，外层不要再关一次。
4. WebSocket 握手请求需要自己 `http.ReadRequest(br)`，并把**同一个** `br` 传给
   `UpgradeFromConn`（内部可能还有缓冲数据）。

## 测试约定

- 测试与被测包同目录（如 `onebot/adapter_test.go`），包内测试以便访问非导出成员。
- 端到端用真实 `gws` 客户端（`dialTestClient`）连接适配器，覆盖鉴权、消息流、Call 往返、定向路由、Stop 断连。
- 所有等待必须带超时：用 `waitFor(t, timeout, cond, msg)` / `recvInbound(t, a)`，
  禁止无超时的阻塞等待；测试整体 `-timeout` 兜底。
- 多客户端场景先 `waitPeer` 等 `self_id` 绑定完成再做断言，避免竞态。
- 提交前跑：`go vet ./...` + `go test ./internal/adapter/... -count=1`，涉并发改动加 `-race`。

## Agent 行为准则

- 改动前先读相关文件与本文档；小步修改，说明"为什么"。
- 运行会产生大量输出的命令时，限制输出（`tail_lines`）或避免 `-v` 全量刷屏。
- 不引入新依赖前先确认必要性；引入后 `go mod tidy` 并确认 go.mod 变化符合预期。
- 不要删除或绕过上文 Stop 顺序与 done-channel 模式。
- 本仓库尚未接入 CI，以上检查全靠自觉执行。
