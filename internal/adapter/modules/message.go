package modules

// Platform 平台标识
type Platform string

// 平台列表
const (
	Feishu Platform = "platform.feishu"
	Onebot Platform = "platform.onebot"
)

// MessageType 消息类型
type MessageType string

// 消息类型列表
const (
	Private MessageType = "message.private"
	Group   MessageType = "message.public"
)

// Sender 消息发送者
type Sender struct {
	UID     string
	Name    string
	IsBot   bool
	IsAdmin bool
}

// Message 通用消息类型
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

// MessageMIME 消息部件类型标识
type MessageMIME string

// 部件类型列表
const (
	Text       MessageMIME = "message.mime.text"
	Mention    MessageMIME = "message.mime.mention"
	MentionALL MessageMIME = "message.mime.mention.all"
	Image      MessageMIME = "message.mime.image"
	File       MessageMIME = "message.mime.file"
	Reply      MessageMIME = "message.mime.reply"
)

// MessagePart 消息内容部件
type MessagePart interface {
	PartType() MessageMIME
}

// TextPart 文本部件
type TextPart struct {
	Text string
}

func (TextPart) PartType() MessageMIME { return Text }

// MentionPart Mention 部件
type MentionPart struct {
	UserID   string
	UserName string
}

func (MentionPart) PartType() MessageMIME { return Mention }

// MentionAllPart MentionAll 部件（at所有人）
type MentionAllPart struct{}

func (MentionAllPart) PartType() MessageMIME { return MentionALL }

// ImagePart 图片部件
type ImagePart struct {
	URL      string // 远程地址
	FileID   string // 平台文件标识（OneBot file，飞书 image_key）
	MimeType string
}

func (ImagePart) PartType() MessageMIME { return Image }

// FilePart 文件部件
type FilePart struct {
	Name     string
	URL      string
	FileID   string
	MimeType string
	Size     int64
}

func (FilePart) PartType() MessageMIME { return File }

// ReplyPart 回复部件
type ReplyPart struct {
	MessageID string
}

func (ReplyPart) PartType() MessageMIME { return Reply }
