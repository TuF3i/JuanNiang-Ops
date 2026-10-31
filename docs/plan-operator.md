# internal/operator 统一操作层 实施计划

> **状态**：已批准实施
> **项目**：JuanNiang-Ops（卷娘巡检姬）
> **范围**：新增 `internal/operator` 模块，聚合 4 种操作媒介（SSH/Telnet/PVE/K8s）与 4 种数据通道
> （Prometheus/Loki/Tempo/Outline）成"资产"（Asset），作为 agent/workflow 操作的基本单位。
> **已确认决策**：① 8 个集成全部实现基础能力；② "Temple"= Grafana Tempo（链路追踪）；
> ③ 资产文档存 DB（markdown），Outline 仅作外部数据通道；④ 本次不做 HTTP CRUD API。
> **不接 cmd/main 入口**，模块经 CreateRuntime 独立装配与测试。

---

## 一、模块骨架与数据模型

位置：`internal/operator/modules/`

- `kind.go`：`MediaKind` / `ChannelKind` 类型化常量（`media.ssh` / `channel.prometheus` 等 8 个）
- `asset.go`：`Asset` gorm 模型（`operator_assets` 表，uuid 主键、四段时间戳、软删；
  `Tags []string` / `Metadata map[string]string` / `Media map[MediaKind]string` /
  `Channels map[ChannelKind]string` 均 `serializer:json`；`Docs` 为 markdown text；
  `Enabled` 默认 false）+ `AssetStatus` 实例状态快照
- `config.go`：8 个 Config 结构体 + `DefaultXxx` 常量块 +
  `ParseMediaConfig/ParseChannelConfig`（kind → 类型化配置，供 service 校验与工厂构建）
- `result.go`：统一结果模型 `ExecResult / MetricSample / MetricSeries / LogQuery / LogEntry /
  TraceQuery / TraceSummary / DocHit / DocPage / VMInfo / PodInfo / DeploymentInfo / PodLogsOptions`

## 二、DAO 层

位置：`internal/operator/dao/`

- `SetupAssets`：仅 AutoMigrate（资产是用户数据，**无幂等种子行**，与 adapter 的配置表不同）
- `CreateAsset / GetAsset / ListAssets / ListEnabledAssets / UpdateAsset / DeleteAsset(软删) / SetEnabled`
- 错误统一 `fmt.Errorf("operator -> xxx: %w", err)` 包装；未找到返回 `gorm.ErrRecordNotFound` 由 service 翻译

## 三、媒介与通道接口

位置：`internal/operator/media/media.go`、`internal/operator/channel/channel.go`

| 接口 | 语义 | 实现者 |
|---|---|---|
| `Media{Kind/Ping/Close}` | 操作媒介统一视角 | 4 个媒介包 |
| `CommandRunner{Exec}` | 命令执行能力 | ssh、telnet |
| `VMOperator{ListNodes/ListVMs/VMStatus/StartVM/StopVM/RebootVM}` | 虚机操作能力 | pve |
| `K8sOperator{ListPods/GetDeployment/ScaleDeployment/PodLogs}` | K8s 资源操作能力 | k8s |
| `Channel{Kind/Close}` | 数据通道统一视角 | 4 个通道包 |
| `MetricsQuerier{Query/QueryRange}` | 指标查询 | prometheus |
| `LogQuerier{QueryLogs}` | 日志查询 | loki |
| `TraceSearcher{SearchTraces/GetTrace}` | 链路查询 | tempo |
| `DocSearcher{SearchDocs/GetDoc}` | 文档搜索 | outline |

能力接口隐式匹配（仿 `driver.Driver` 与 `MessageSender`），签名与接口一字不差。

## 四、Service 层（资产实例生命周期 + 统一 API）

位置：`internal/operator/service/`

- `Service{dao, mu, instances map[string]*AssetInstance}`；实例**仅启用资产持有**
- `AssetInstance{asset 快照, media map, channels map}`；无 goroutine（SSH/Telnet 短连接，
  其余为 HTTP 客户端），无 done/wg
- `buildInstance`：单个媒介/通道构建失败**跳过并记日志**（部分能力缺失优于整体不可用），
  缺失 kind 查询时报 `ErrNoMedia/ErrNoChannel`
- 生命周期：`LoadEnabled`（CreateRuntime 调用，单个失败不中断）/ `EnableAsset`（先落库 enabled=true
  再建实例，**失败不回滚**，保持未运行等重试）/ `DisableAsset` / `reCreate`（配置变更，mu 串行化）/
  `Release`（幂等）
- 统一 API（agent/workflow 入口）：
  - 资产管理：`CreateAsset`（校验配置 JSON + uuid）/ `GetAsset` / `ListAssets`（tags 内存过滤）/
    `UpdateAsset`（sonic+MD5 对比 → 落库 → 实例存在则 reCreate，复刻 `UpdateRuntimeConfig`）/
    `DeleteAsset`（启用中先停用再软删）/ `EnableAsset` / `DisableAsset` / `GetAssetStatus`
  - 高频操作便捷方法：`Exec` / `QueryMetrics` / `QueryRange` / `QueryLogs` / `SearchTraces` /
    `GetTrace` / `SearchDocs` / `GetDoc`
  - 低频特化操作：`Media` / `Channel` 拿实例引用后由调用方断言能力接口（如 PVE 启停虚机）
- 路由语义（仿 adapter `route.go`）：kind 指定则精确匹配，为空则按预定义优先级在具备所需能力的
  媒介/通道中选择，多个可选报 `ErrAmbiguousMedia/ErrAmbiguousChannel`
- sentinel 错误：`ErrAssetNotFound / ErrAssetNotRunning / ErrNoMedia / ErrNoChannel /
  ErrNoCapability / ErrInvalidAssetConfig / ErrUnknownMediaKind / ErrUnknownChannelKind /
  ErrAmbiguousMedia / ErrAmbiguousChannel`
- `factory.go`：`newMedia/newChannel` 按 kind switch 构建实现（各集成逐个挂接）

## 五、集成实现明细

| 集成 | 包 | 依赖 | 基础能力与取舍 |
|---|---|---|---|
| SSH | `media/ssh` | `golang.org/x/crypto/ssh`（已有） | password / private_key 认证；HostKeyPolicy 默认 insecure；**Exec 短连接**（dial→session→run→close，v1 取舍）；ExitCode 取 `ssh.ExitError` |
| Telnet | `media/telnet` | 标准库 net | 最小 IAC 剥离器（IAC IAC/协商拒绝/子协商跳过，表驱动单测）；可选凭据自动登录；Prompt 正则可配（默认 `[#$>]\s*$`）；telnet 无退出码语义，ExitCode 恒 0 |
| PVE | `media/pve` | 标准库 net/http | REST `/api2/json`，`PVEAPIToken=user!id=secret`；TLS skip 可配；start/stop/reboot 返回 UPID 异步任务，**v1 不等待任务完成**（注释说明） |
| K8s | `media/k8s` | `k8s.io/client-go` | kubeconfig 内容/路径或 APIServer+Token+CA 三种接入；ListPods/GetDeployment/ScaleDeployment/PodLogs（GetScale→UpdateScale 标准模式）；PodLogs 限 1MB 防 OOM |
| Prometheus | `channel/prometheus` | `client_golang/api` | Query/QueryRange，Vector/Matrix → 统一模型 |
| Loki | `channel/loki` | net/http | GET `/loki/api/v1/query_range`（纳秒时间戳字符串），streams → LogEntry |
| Tempo | `channel/tempo` | net/http | GET `/api/search`（TraceQL `{ service.name="x" }`）、`/api/traces/{id}` 返回原始字节（**不解析 OTLP**） |
| Outline | `channel/outline` | net/http | Bearer 认证，`documents.search` / `documents.info`（返回 markdown） |

HTTP 手写客户端复用 t2i 的 `do/doJSON/decodeJSON` 范式，各客户端内私有实现，模块自包含。

## 六、Runtime 组装

位置：`internal/operator/runtime.go`

与 `internal/adapter/runtime.go` 同构：`Option(WithDB)` / `CreateRuntime`（db 注入优先否则自建 ownDB
→ Dao → SetupAssets → Service → LoadEnabled）/ `ReleaseRuntime`（幂等，关全部实例 + 自建 db）/ 包级
`defaultRuntime`。日志 `[operator] ` 前缀。

## 七、测试

包内测试（访问非导出成员）；依赖真实 PG 的用例读 `TEST_PG_DSN`，未设置自动 Skip；等待带超时。

- dao：建表 + CRUD + 软删往返
- service：路由/能力断言/生命周期（mock media/channel 直填 instances + 真 DB）；UpdateAsset 无变化跳过
- channel HTTP 客户端：httptest 假服务端往返（loki/tempo/outline）；pve 同款假 REST
- k8s：client-go fake clientset
- ssh/telnet：IAC 剥离、配置默认值等纯函数表驱动测试（端到端无 sshd，标注手测）

## 八、文档与验证

- `AGENT.md`：目录结构补 `internal/operator` 条目 + 新增 operator 模块章节
- `DEVELOPMENT.yaml`：`internal/operator` status → 已完成；status.done/todo 更新
- 验证：`go build ./...` + `go vet ./...` + `go test ./internal/operator/... -count=1`

## 九、不改动

- `internal/adapter` 全部（含 onebot Stop 六步顺序、done-channel 写入模式、equal() 的 sonic+MD5 实现）
- `internal/api`（本次不接资产 CRUD handler）、`infrastructure`、`web`
- `config/viper`（默认值仍由各 Config 的 DefaultXxx 常量承载，落地后接入）


---

## 修订（2026-11-01）：媒介/数据源实体化 + HTTP CRUD

> **状态**：已实施完成
> **范围**：媒介/数据源从"资产内嵌配置"改为**独立实体 + M:N 绑定**（跨资产共享，
> 如同机 K8s 资产与 PVE 资产共用同一个 SSH）；新增 HTTP CRUD（`/api/v1/operator/**`）。

### 已确认决策

① service 重构 + HTTP CRUD 同时做；② 同资产允许绑定多个同类媒介（路由引入实体 ID 精确选择）；
③ 共享语义 v1 = 配置共享、各资产独立连接（连接池留作扩展点）；④ 在用删除拒绝。

### 模型变更

| 变更 | 说明 |
|---|---|
| 新表 `operator_media_sources` / `operator_channel_sources` | 独立实体：uuid 主键 + 全局唯一 Name + Kind + Config(JSON text)，软删 |
| 新表 `operator_asset_media_bindings` / `operator_asset_channel_bindings` | 绑定行：asset_id + source_id 联合唯一；Bind 用 OnConflict DoNothing 幂等；硬删（随实体/资产清理） |
| `Asset` 移除内嵌 `Media`/`Channels` 列 | AutoMigrate 不删旧列，残留无害；资产只承载描述信息 |

### 三步创建流程

1. 创建媒介/数据源实体：`CreateMediaSource/CreateChannelSource`（Parse 校验 + uuid）
2. 创建资产：`CreateAsset`（纯 name/description/docs/tags/metadata）
3. 绑定：`BindAssetMedia/BindAssetChannel`（双存在校验 → 幂等落库 → 运行中资产立即重建）

### 路由与生命周期变更

- `AssetInstance.media/channels` 改为 `[]boundMedia{source, m}` 切片（同 kind 多绑定）
- 选择参数 = `kind`（粗过滤）+ `sourceID`（精确）：sourceID 指定按实体 ID 命中；空则 kind+能力过滤后
  按优先级取首个，多个报 `ErrAmbiguousMedia/ErrAmbiguousChannel`
- `Exec/Query*` 签名增加 `sourceID` 参数；`buildInstance` 变 service 方法（反查 DAO 加载绑定实体）
- 变更触发重建：Bind/Unbind、UpdateMediaSource/UpdateChannelSource（sonic+MD5 对比 → 落库 →
  反查 `ListAssetIDsByXxx` 重建全部绑定运行中资产）
- 实体删除**在用拒绝**（`ErrMediaSourceInUse/ErrChannelSourceInUse`）；删资产清绑定防孤儿

### HTTP CRUD

- internal/api 增 `WithOperator(*operator.Service)` 注入；未注入则内部 `operator.CreateRuntime(WithDB(db))`
  自建（ownDB=false 不重复关连接，Release 时释放）；operator 顶层包暴露 `Service` 类型别名
- handler/operator.go：`/api/v1/operator/**` 全量路由（Auth 组内 + requireOperator 装配守卫）
- response 新业务码：NotFound=10020 / InUse=10021 / NotRunning=10022 / Ambiguous=10023 / Unavailable=10024
- `api/openapi.yaml` v1.3.0：operator tag + 11 端点 + schemas

### 测试

- service：三步流程生命周期 / 共享场景（两资产共用 SSH，各自独立连接）/ 同 kind 多绑定歧义 /
  在用删除拒绝 / UpdateMediaSource 无变化跳过 / LoadEnabled 兜底
- api e2e：登录 → 建媒介 → 建资产 → 绑定（幂等）→ 在用删除 10021 → 启用 → status → 解绑 → 删除 → 404
