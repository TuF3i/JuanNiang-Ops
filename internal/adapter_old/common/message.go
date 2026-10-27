package common

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
