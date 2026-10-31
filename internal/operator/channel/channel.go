// Package channel operator 数据通道: 只读数据源 (查询指标/日志/链路/文档).
//
// Channel 为统一视角 (类型标识 + 生命周期), 查询能力经能力接口隐式匹配;
// 与 media 包同构, 能力断言通常由 service 层完成.
package channel

import (
	"context"
	"time"

	"JuanNiang-Ops/internal/operator/modules"
)

// Channel 数据通道统一视角: 生命周期 + 类型标识
type Channel interface {
	// Kind 通道类型标识
	Kind() modules.ChannelKind
	// Ping 连通性检查 (对数据源发起轻量请求)
	Ping(ctx context.Context) error
	// Close 释放持有的资源 (幂等)
	Close() error
}

// MetricsQuerier 指标查询能力 (prometheus 实现)
type MetricsQuerier interface {
	// Query 瞬时查询 (PromQL), ts 为零值时取当前时间
	Query(ctx context.Context, query string, ts time.Time) ([]modules.MetricSample, error)
	// QueryRange 区间查询 (PromQL), step 为采样步长
	QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]modules.MetricSeries, error)
}

// LogQuerier 日志查询能力 (loki 实现)
type LogQuerier interface {
	// QueryLogs 执行 LogQL 查询
	QueryLogs(ctx context.Context, q modules.LogQuery) ([]modules.LogEntry, error)
}

// TraceSearcher 链路查询能力 (tempo 实现)
type TraceSearcher interface {
	// SearchTraces 按 TraceQL 条件搜索链路
	SearchTraces(ctx context.Context, q modules.TraceQuery) ([]modules.TraceSummary, error)
	// GetTrace 拉取完整链路 (原始 JSON 字节, OTLP 结构由调用方解析)
	GetTrace(ctx context.Context, traceID string) ([]byte, error)
}

// DocSearcher 文档搜索能力 (outline 实现)
type DocSearcher interface {
	// SearchDocs 关键词搜索 (limit 为 0 时取 DefaultDocSearchLimit)
	SearchDocs(ctx context.Context, keyword string, limit int) ([]modules.DocHit, error)
	// GetDoc 读取文档页 (markdown 正文)
	GetDoc(ctx context.Context, pageID string) (*modules.DocPage, error)
}
