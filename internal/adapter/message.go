package adapter

type MessageMIME string

const (
	Text       = "message.mime.text"
	Mention    = "message.mime.mention"
	MentionALL = "message.mime.mention.all"
	Image      = "message.mime.image"
	File       = "message.mime.file"
	Reply      = "message.mime.reply"
)

// 消息内容部件
type MessagePart interface {
	PartType() string
}

// Text部件
type TextPart struct {
	Text string
}

func (TextPart) PartType() MessageMIME { return Text }

// Mention 部件
type MentionPart struct {
	UserID   string
	UserName string
}

func (MentionPart) PartType() MessageMIME { return Mention }

// MentionAll 部件（at所有人）
type MentionAllPart struct{}

func (MentionAllPart) PartType() MessageMIME { return MentionALL }

// Image 部件
type ImagePart struct {
	URL      string // 远程地址
	FileID   string // 平台文件标识（OneBot file，飞书 image_key）
	MimeType string
}

func (ImagePart) PartType() MessageMIME { return Image }

// File 部件
type FilePart struct {
	Name     string
	URL      string
	FileID   string
	MimeType string
	Size     int64
}

func (FilePart) PartType() MessageMIME { return File }

// Reply 部件
type ReplyPart struct {
	MessageID string
}

func (ReplyPart) PartType() MessageMIME { return Reply }
