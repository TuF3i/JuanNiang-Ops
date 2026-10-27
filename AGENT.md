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
  adapter/              平台适配器 ★ 最完整的模块
    runtime.go          AdapterRuntime 生命周期管理器（配置联动 / reconcile / 事件泵）
    hooks.go            gorm 钩子 -> 包级配置变更通知
    handler.go          事件泵与 handler 分发
    dto.go              OneBotConfig（gorm 模型，配置存 DB，含变更钩子）
    common/             平台无关的统一消息模型（Message / MessagePart）
    onebot/             OneBot v11 正向 WS 适配器（含单元测试）
  agent/                Agent 编排（待实现）
  api/                  HTTP API（待实现）
  db/                   数据访问（dao / cache，待实现）
  workflow/             工作流（待实现）
inferstructure/         基础设施客户端（postgres / redis / t2i）
pkg/                    可对外复用的库（pkg/uerr 哨兵错误）
sql/                    数据库迁移脚本（空）
web/                    前端（空）
deployment/             部署清单（空）
docs/                   文档（含实施计划）
```

约定：每个模块目录下用 `runtime.go` 承载模块级文档/约定；空目录放 `.gitkeep`。

## 适配器架构（internal/adapter）

每个平台适配器必须提供：

| 成员 | 语义 |
|---|---|
| `Start() / Stop()` | 生命周期；Start 幂等（重复调用返回 `ErrRunning`），Stop 幂等且阻塞至完全退出 |
| `Inbound()` | 接收消息管道 `<-chan common.Message`，平台事件 → 统一消息 → 业务层，Stop 时关闭 |
| `Outbound()` | 发送指令管道 `chan<- *Action`，业务层 → 平台实现端 |
| `Done()` | 停止信号 `<-chan struct{}`，与管道配合消费 |

统一消息模型（`common`）：`Message` 由 `[]MessagePart` 组成（Text/Mention/Image/File/Reply…），
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
- 发消息 API 接受统一消息部件：`SendGroupMsg/SendPrivateMsg`（fire-and-forget）与
  `CallGroupMsg/CallPrivateMsg`（等待回执），业务层无需接触 OneBot 原生段结构。
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

## 适配器运行时（AdapterRuntime）

`AdapterRuntime`（runtime.go）是适配器的生命周期管理器，单条配置（`onebot_configs` 首行）对应单实例：

- **启动链路**：`NewAdapterRuntime(db)` → `Setup()`（AutoMigrate + 空表插默认行 `:8080`/未启用）
  → `Start()`（注册配置观察者 + reconcileLoop + 30s 兜底轮询 + 首次 reconcile）。
- **reconcile**：读配置（无则 FirstOrCreate 默认行）→ 与快照对比（Addr/Token/Admin/Enable/buffers）
  → 无变化 no-op（幂等）；有变化 Stop 旧实例 → `Enable=false` 保持停止 / `true` 则 New+Start+事件泵。
  启动失败（如端口占用）只记日志不中断管理器，等下次配置变更再试。
- **数据库联动（hooks.go）**：`OneBotConfig` 挂 gorm 钩子 `AfterCreate/AfterUpdate/AfterDelete`
  → 包级 observer（atomic.Value）→ runtime 非阻塞通知（缓冲 1，合并抖动）→ reconcile。
  ⚠️ **不带主键的批量 Update 不触发 AfterUpdate 钩子**，更新配置必须带主键
  （`SetEnabled`/`UpdateConfig` 便捷 API 已保证：先 First 再 Save）。
- **事件泵（handler.go）**：每个适配器实例一个 pump goroutine，消费 Inbound/Notices 并同步调用
  handler（带 recover）；handler 应快速返回或自行开 goroutine；ctx 为 runtime 根 ctx（Stop 时取消）。
- **管理 API**：`Setup` / `Start` / `Stop`（幂等，Stop 后可重新 Start）/ `Reload` /
  `SetEnabled(ctx, bool)`（运行时开关）/ `UpdateConfig(ctx, mutate)` / `GetStatus()` /
  `OnMessage(h)` / `OnNotice(h)`。

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
