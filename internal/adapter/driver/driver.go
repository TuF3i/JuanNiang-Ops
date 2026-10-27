package driver

import (
	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter_old/onebot"
)

// Driver 平台适配器统一视角 (生命周期 + 事件管道), 具体平台实现隐式匹配.
type Driver interface {
	// 生命周期
	Start() error
	Stop() error
	Done() <-chan struct{}

	// 事件管道
	Inbound() <-chan common.Message
	Notices() <-chan onebot.UnionEvent

	// 状态快照
	GetCurrentState() onebot.CurrentState
}
