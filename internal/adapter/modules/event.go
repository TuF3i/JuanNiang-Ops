package modules

// EventType 事件类型标识
type EventType string

// 事件类型列表
const (
	EventMessage EventType = "event.message" // 消息事件 (私聊/群聊)
	EventNotify  EventType = "event.notify"  // 通知事件 (戳一戳/lifecycle/平台自定义)
)

// Event 统一事件: Driver 上报给业务层的唯一载荷.
// Type 决定 Message / Notify 哪个字段有效 (互斥, 非法组合视为损坏事件).
type Event struct {
	Type    EventType    // 事件类型
	SelfID  string       // 机器人自身 ID (OneBot self_id / 飞书 open_id)
	Message *Message     // 消息事件载荷, Type==EventMessage 时非 nil
	Notify  *NotifyEvent // 通知事件载荷, Type==EventNotify 时非 nil
}

// NotifyEvent 通知事件: 平台相关, 原始数据透传, 业务层按平台自行解析.
// Data 的具体结构由平台与 Kind 决定 (如 OneBot PokeEvent / LifeCycleEvent).
type NotifyEvent struct {
	Kind     string   // 通知种类: event.notify.poke / event.meta.lifecycle / 平台自定义
	Platform Platform // 来源平台
	Data     []byte   // 平台原始 JSON
}
