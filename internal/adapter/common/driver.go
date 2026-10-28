package common

import "time"

// RuntimeState Driver 运行状态快照
type RuntimeState struct {
	IsRunning bool

	OnebotExtraState
}

// OnebotExtraState OneBot 平台附加状态
type OnebotExtraState struct {
	PeerList  map[string]PeerState // self_id -> 状态快照
	PeerCount int
}

// PeerState 已接入客户端的状态快照
type PeerState struct {
	Session    string    // 接入时分配的会话 ID
	SelfID     string    // 机器人自身 ID, 首个事件上报后绑定
	RemoteAddr string    // 客户端地址
	LastBeat   time.Time // 最近心跳时间
}
