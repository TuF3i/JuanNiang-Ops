package modules

import "time"

// ExecResult 命令执行结果 (ssh/telnet)
type ExecResult struct {
	ExitCode int    // 退出码; ssh 取真实退出码, telnet 无退出码语义恒 0
	Stdout   string // 合并输出 (telnet 无法区分 stdout/stderr, ssh 分开)
	Stderr   string
	Duration time.Duration
}

// MetricSample 单时刻指标样本
type MetricSample struct {
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
}

// MetricSeries 区间指标序列 (QueryRange)
type MetricSeries struct {
	Labels  map[string]string
	Samples []MetricSample
}

// LogQuery 日志查询条件 (Loki LogQL)
type LogQuery struct {
	Selector  string     // LogQL 选择器, 如 {job="nginx"}
	Start     time.Time  // 为零值时省略
	End       time.Time  // 为零值时省略
	Limit     int        // 0 时取 DefaultLogLimit
	Direction string     // "forward" | "backward" (默认 backward)
}

// LogEntry 日志条目
type LogEntry struct {
	Timestamp time.Time
	Line      string
	Labels    map[string]string
}

// TraceQuery 链路搜索条件 (Tempo TraceQL)
type TraceQuery struct {
	ServiceName string    // service.name 条件
	MinDuration string    // 如 "1s" (可选)
	Start       time.Time // 为零值时省略
	End         time.Time
	Limit       int // 0 时用通道默认值
}

// TraceSummary 链路摘要 (搜索结果)
type TraceSummary struct {
	TraceID         string
	RootServiceName string
	RootTraceName   string
	StartTime       time.Time
	DurationMS      int64
}

// DocHit 文档搜索命中
type DocHit struct {
	PageID  string
	Title   string
	Context string // 命中上下文摘要
}

// DocPage 文档页面 (含 markdown 正文)
type DocPage struct {
	PageID   string
	Title    string
	Markdown string
}

// VMInfo PVE 虚机信息
type VMInfo struct {
	Node    string
	VMID    int
	Name    string
	Status  string  // running / stopped / paused
	CPU     float64 // CPU 使用率 (0-1)
	MaxMem  int64   // 字节
	MaxDisk int64   // 字节
}

// PodInfo K8s Pod 信息
type PodInfo struct {
	Namespace string
	Name      string
	Status    string // Pod Phase
	PodIP     string
	Node      string
}

// DeploymentInfo K8s Deployment 信息
type DeploymentInfo struct {
	Namespace     string
	Name          string
	Replicas      int32
	ReadyReplicas int32
	Images        []string
}

// PodLogsOptions Pod 日志读取选项
type PodLogsOptions struct {
	Container string // 为空取主容器
	TailLines int64  // 尾部行数 (0 表示全部)
	Previous  bool   // 上一个崩溃实例的日志
}
