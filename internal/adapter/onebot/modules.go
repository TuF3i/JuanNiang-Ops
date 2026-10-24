package onebot

// 基础事件
type BaseEvent struct {
	Time     int64  `json:"time"`
	SelfID   int64  `json:"self_id"`
	PostType string `json:"post_type"`
}

// 消息发送者
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

// 私聊消息事件
type PrivateMessageEvent struct {
	BaseEvent
	MessageType string    `json:"message_type"`
	SubType     string    `json:"sub_type"`
	MessageID   int32     `json:"message_id"`
	UserID      int64     `json:"user_id"`
	Message     []Segment `json:"message"`
	RawMessage  string    `json:"raw_message"`
	Font        int32     `json:"font"`
	Sender      Sender    `json:"sender"`

	// 快捷操作
	AutoEscape bool `json:"auto_escape"`
}

// 群聊消息事件
type GroupMessageEvent struct {
	BaseEvent
	MessageType string    `json:"message_type"`
	SubType     string    `json:"sub_type"`
	MessageID   int32     `json:"message_id"`
	GroupID     int64     `json:"group_id"`
	UserID      int64     `json:"user_id"`
	Message     []Segment `json:"message"`
	RawMessage  string    `json:"raw_message"`
	Font        int32     `json:"font"`
	Sender      Sender    `json:"sender"`

	// 快捷操作
	AutoEscape bool `json:"auto_escape"`
	AtSender   bool `json:"at_sender"`
}

// 生命周期事件
type LifeCycleEvent struct {
	BaseEvent
	MetaEventType string `json:"meta_event_type"`
	SubType       string `json:"sub_type"`
}

// 心跳事件
type HeartBeatEvent struct {
	BaseEvent
	MetaEventType string `json:"meta_event_type"`
	Status        Status `json:"status"`
	Interval      int64  `json:"interval"`
}

type Status struct {
	Online bool `json:"online"` // 当前 QQ 在线，null 表示无法查询到在线状态
	Good   bool `json:"good"`   // 状态符合预期，意味着各模块正常运行、功能正常，且 QQ 在线
}

type PokeEvent struct {
	BaseEvent
	NoticeType string `json:"notice_type"` // notify
	SubType    string `json:"sub_type"`    // poke
	GroupID    int64  `json:"group_id"`    // 群号
	UserID     int64  `json:"user_id"`     // 发送者 QQ 号
	TargetID   int64  `json:"target_id"`   // 被戳者 QQ 号
}
