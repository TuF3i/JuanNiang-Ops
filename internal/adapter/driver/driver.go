package driver

import (
	"JuanNiang-Ops/internal/adapter/modules"
)

// Driver 平台驱动统一视角 (生命周期 + 事件管道), 具体平台实现隐式匹配.
type Driver interface {
	// 生命周期
	Start() error
	Stop() error
	Done() <-chan struct{}

	// 事件管道: 消息 (EventMessage) 与通知 (EventNotify) 合流, Stop 时关闭
	Events() <-chan modules.Event

	// 状态快照
	GetCurrentState() *modules.RuntimeState
}
