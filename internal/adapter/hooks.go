// 配置变更通知: gorm 钩子 -> 包级 observer -> AdapterRuntime.reconcileLoop.
//
// 链路: OneBotConfig 写库 (AfterCreate/AfterUpdate/AfterDelete) -> notifyConfigChanged()
// -> runtime 注册的回调 -> runtime 重新拉取配置并重建适配器.
//
// ⚠️ 约束: gorm 的批量更新 (如 Model(&OneBotConfig{}).Where(...).Update(...)) 不带主键时
// 不会触发 AfterUpdate 钩子. 更新配置必须带主键 (Where("id = ?", id) 或先 First 再 Save),
// adapter 包提供的便捷 API (SetEnabled / UpdateConfig) 已保证这一点.
package adapter

import (
	"sync/atomic"
)

// configObserver 包级配置变更回调, atomic.Value 保证并发读写的安全注册.
// 存 func() 而非直接存 func, 避免 atomic.Value 首次存储后类型不可变的限制.
var configObserver atomic.Value // 实际类型 func()

// setConfigObserver 注册配置变更回调 (runtime 启动时调用); 传入 nil 取消注册.
// 同一时刻仅允许一个观察者, 后注册的覆盖先前的.
func setConfigObserver(fn func()) {
	if fn == nil {
		configObserver.Store(func() {})
		return
	}
	configObserver.Store(fn)
}

// notifyConfigChanged 通知观察者配置已变更 (由 gorm 钩子调用).
// 非阻塞: 回调可能尚未注册 (观察者为空) 或观察者不应被阻塞, 一律吞掉触发.
func notifyConfigChanged() {
	if fn, ok := configObserver.Load().(func()); ok && fn != nil {
		fn()
	}
}
