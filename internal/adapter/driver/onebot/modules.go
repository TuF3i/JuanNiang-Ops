package onebot

import (
	"encoding/json"
	"time"

	"github.com/lxzan/gws"
)

// defaultNoticeBuffer 通知管道默认缓冲大小
const defaultNoticeBuffer = 64

const (
	MessageEventType = "event.message"
	NotifyEventType  = "event.notify"
	MetaEventType    = "event.meta"
)

// callResult API 调用结果: 远端回执走 result, 本地失败 (路由/序列化/写连接) 走 err 以保留错误链
type callResult struct {
	result *ApiResult
	err    error
}

// peer 一个接入的 OneBot 客户端
type peer struct {
	conn     *gws.Conn
	session  string // 接入时分配的会话 ID, 上报 self_id 前作为占位
	selfID   string // 机器人自身 ID, 首个事件上报后绑定
	lastBeat time.Time
}

// Driver 配置
type Config struct {
	Addr   string   // 监听地址, 格式 host:port
	Token  string   // 接入令牌, 为空则不校验
	Admin  []string // 管理员列表
	Enable bool     // 是否启用

	MessageBuffer int // 接收消息管道缓冲大小, 默认 256
	ActionBuffer  int // 发送指令管道缓冲大小, 默认 64
}

// BaseEvent 基础事件
type BaseEvent struct {
	Time     int64  `json:"time"`
	SelfID   int64  `json:"self_id"`
	PostType string `json:"post_type"`
}

// UnionEvent 统一事件结构, 便于路由
type UnionEvent struct {
	EventType string `json:"event_type"`
	EventData []byte `json:"event_data"`
	SelfID    int64  `json:"-"` // 目标机器人 self_id, 为空时自动路由
}

// Sender 消息发送者
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Sex      string `json:"sex"`
	Age      int32  `json:"age"`

	// 群聊附加字段
	Card  string `json:"card"`
	Area  string `json:"area"`
	Role  string `json:"role"`
	Title string `json:"title"`
}

// MessageEvent 消息事件 (私聊与群聊统一结构)
type MessageEvent struct {
	BaseEvent
	MessageType string    `json:"message_type"` // private / group
	SubType     string    `json:"sub_type"`
	MessageID   int32     `json:"message_id"`
	GroupID     int64     `json:"group_id"` // 私聊时为 0
	UserID      int64     `json:"user_id"`
	Message     []Segment `json:"message"`
	RawMessage  string    `json:"raw_message"`
	Font        int32     `json:"font"`
	Sender      Sender    `json:"sender"`
}

// LifeCycleEvent 生命周期事件
type LifeCycleEvent struct {
	BaseEvent
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type"`
}

// HeartBeatEvent 心跳事件
type HeartBeatEvent struct {
	BaseEvent
	MetaEventType string `json:"meta_event_type"`
	Status        Status `json:"status"`
	Interval      int64  `json:"interval"`
}

// Status OneBot 实现端运行状态
type Status struct {
	Online bool `json:"online"` // 当前 QQ 在线
	Good   bool `json:"good"`   // 状态符合预期
}

// PokeEvent 戳一戳事件
type PokeEvent struct {
	BaseEvent
	NoticeType string `json:"notice_type"` // notify
	SubType    string `json:"sub_type"`    // poke
	GroupID    int64  `json:"group_id"`    // 群号
	UserID     int64  `json:"user_id"`     // 发送者 QQ 号
	TargetID   int64  `json:"target_id"`   // 被戳者 QQ 号
}

// Action 发送给 OneBot 实现端的 API 动作
type Action struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Echo   string         `json:"echo,omitempty"`

	Target string `json:"-"` // 目标机器人 self_id, 为空时自动路由
}

// ApiResult OneBot API 调用回执
type ApiResult struct {
	Status  string          `json:"status"`  // ok / async / failed
	RetCode int             `json:"retcode"` // 0 表示成功
	Data    json.RawMessage `json:"data"`    // 回执数据
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
}

// CurrentState 当前状态
type CurrentState struct {
	IsRunning bool
	PeerList  map[string]PeerState // self_id -> 状态快照
	PeerCount int
}

// PeerState 已接入客户端的状态快照 (导出结构, 供外部读取)
type PeerState struct {
	Session    string    // 接入时分配的会话 ID
	SelfID     string    // 机器人自身 ID, 首个事件上报后绑定
	RemoteAddr string    // 客户端地址
	LastBeat   time.Time // 最近心跳时间
}
