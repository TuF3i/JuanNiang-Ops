package adapter

import (
	"testing"

	"JuanNiang-Ops/internal/adapter/onebot"
)

// TestOnebotAdapterImplementsInterfaces 编译期断言: onebot.Adapter 满足统一抽象层接口.
// 平台实现通过隐式接口接入 AdapterRuntime, 此处锁定契约, 防止实现漂移.
func TestOnebotAdapterImplementsInterfaces(t *testing.T) {
	var _ Adapterer = (*onebot.Adapter)(nil)
	var _ MessageSender = (*onebot.Adapter)(nil)
}
