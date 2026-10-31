// Package modules operator 模块数据模型: 资产 / 媒介与通道类型 / 各集成配置 / 统一结果模型.
//
// 资产 (Asset) 是 agent/workflow 操作的基本单位: 聚合多个操作媒介 (ssh/telnet/pve/k8s)
// 与多个数据通道 (prometheus/loki/tempo/outline). 模块骨架与 internal/adapter 同构.
package modules

// MediaKind 操作媒介类型: 与资产建立交互连接的通道 (执行命令/操作资源)
type MediaKind string

const (
	MediaSSH    MediaKind = "media.ssh"    // SSH 远程执行
	MediaTelnet MediaKind = "media.telnet" // Telnet 远程执行
	MediaPVE    MediaKind = "media.pve"    // Proxmox VE 虚拟化平台
	MediaK8s    MediaKind = "media.k8s"    // Kubernetes 集群
)

// ChannelKind 数据通道类型: 只读数据源 (查询指标/日志/链路/文档)
type ChannelKind string

const (
	ChannelPrometheus ChannelKind = "channel.prometheus" // 指标
	ChannelLoki       ChannelKind = "channel.loki"       // 日志
	ChannelTempo      ChannelKind = "channel.tempo"      // 链路追踪
	ChannelOutline    ChannelKind = "channel.outline"    // Outline 文档站
)
