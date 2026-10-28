package common

// 平台标识
type Platform string

// 平台列表
const (
	Feishu Platform = "platform.feishu"
	Onebot Platform = "platform.onebot"
)

// 消息类型
type MessageType string

// 消息类型列表
const (
	Private MessageType = "message.private"
	Group   MessageType = "message.public"
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
	MessageType   MessageType
	ChatID        string // 会话 ID（OneBot group_id/user_id，飞书 chat_id）
	SelfID        string // 机器人自身 ID
	Sender        Sender
	Timestamp     int64
	MentionedSelf bool // 是否明确 @ 了机器人
	Content       []MessagePart
	Original      any // 原始事件
}

// SendReceipt 发送回执 (统一抽象, 各平台 Driver 负责从平台回执填充)
type SendReceipt struct {
	MessageID string // 平台消息 ID (发送成功后由平台分配)
	Reply     any    // 平台原始回执 (OneBot ApiResult / 飞书回执...), 需要时由业务层断言
}
