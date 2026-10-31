// Package media operator 操作媒介: 与资产建立交互连接的通道 (执行命令/操作资源).
//
// Media 为统一视角 (类型标识 + 生命周期), 具体操作能力经能力接口隐式匹配
// (仿 adapter 的 driver.Driver 与 MessageSender): 实现与接口一字不差.
// 能力断言通常由 service 层完成, agent/workflow 经 service 统一 API 操作.
package media

import (
	"context"

	"JuanNiang-Ops/internal/operator/modules"
)

// Media 操作媒介统一视角: 生命周期 + 类型标识
type Media interface {
	// Kind 媒介类型标识
	Kind() modules.MediaKind
	// Ping 连通性检查 (建连即验证, 不留存副作用)
	Ping(ctx context.Context) error
	// Close 释放持有的资源 (幂等)
	Close() error
}

// CommandRunner 命令执行能力 (ssh / telnet 实现)
type CommandRunner interface {
	// Exec 执行单条命令并等待结束 (短连接实现: 每次独立建连)
	Exec(ctx context.Context, cmd string) (*modules.ExecResult, error)
}

// VMOperator 虚机操作能力 (pve 实现)
type VMOperator interface {
	// ListNodes 列出集群节点名
	ListNodes(ctx context.Context) ([]string, error)
	// ListVMs 列出节点上的 QEMU 虚机
	ListVMs(ctx context.Context, node string) ([]modules.VMInfo, error)
	// VMStatus 查询单台虚机状态
	VMStatus(ctx context.Context, node string, vmid int) (*modules.VMInfo, error)
	// StartVM / StopVM / RebootVM 虚机电源操作
	// 底层为 PVE 异步任务 (返回 UPID), 本接口仅保证任务已被接受
	StartVM(ctx context.Context, node string, vmid int) error
	StopVM(ctx context.Context, node string, vmid int) error
	RebootVM(ctx context.Context, node string, vmid int) error
}

// K8sOperator Kubernetes 资源操作能力 (k8s 实现)
type K8sOperator interface {
	// ListPods 列出命名空间下的 Pod (namespace 为空取默认命名空间)
	ListPods(ctx context.Context, namespace string) ([]modules.PodInfo, error)
	// GetDeployment 查询 Deployment
	GetDeployment(ctx context.Context, namespace, name string) (*modules.DeploymentInfo, error)
	// ScaleDeployment 调整 Deployment 副本数
	ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error
	// PodLogs 读取 Pod 日志 (有输出上限, 防大日志撑爆内存)
	PodLogs(ctx context.Context, namespace, name string, opts modules.PodLogsOptions) (string, error)
}
