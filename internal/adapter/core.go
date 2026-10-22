package adapter

// 平台标识
type Platform string

// 平台列表
const (
	Feishu = "platform.feishu"
	Onebot = "platform.onebot"
)

// 消息类型
type MessageType string

// 消息类型列表
const (
	Private = "message.private"
	Group   = "message.public"
)

// 消息发送者
type Sender struct {
	UID     string
	Name    string
	IsBot   bool
	IsAdmin bool
}

// 通用消息类型
type Message struct {
	MessageID     string // 平台消息 ID
	Platform      Platform
	MessageType   Message
	ChatID        string // 会话 ID（OneBot group_id/user_id，飞书 chat_id）
	SelfID        string // 机器人自身 ID
	Sender        Sender
	Timestamp     int64
	MentionedSelf bool // 是否明确 @ 了机器人
	Content       []
	Original  any // 原始事件
}
