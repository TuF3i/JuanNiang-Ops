package common

import "time"

type RuntimeState struct {
	IsRunning bool

	OnebotExtraState
}

type OnebotExtraState struct {
	PeerList  map[string]PeerState // self_id -> 状态快照
	PeerCount int
}

type PeerState struct {
	Session    string    // 接入时分配的会话 ID
	SelfID     string    // 机器人自身 ID, 首个事件上报后绑定
	RemoteAddr string    // 客户端地址
	LastBeat   time.Time // 最近心跳时间
}

// UnionEvent 统一事件结构, 便于路由
type UnionEvent struct {
	EventType string `json:"event_type"`
	EventData []byte `json:"event_data"`
	SelfID    int64  `json:"-"` // 目标机器人 self_id, 为空时自动路由
}
