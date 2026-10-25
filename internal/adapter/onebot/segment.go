package onebot

import (
	"encoding/json"
	"fmt"

	"JuanNiang-Ops/internal/adapter/common"
)

// Segment OneBot 消息段
type Segment struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ---------- 发送侧: 构造消息段 ----------

// Text 文本消息
func Text(text string) Segment {
	return Segment{Type: "text", Data: map[string]any{"text": text}}
}

// Image 图片
func Image(file string) Segment {
	return Segment{Type: "image", Data: map[string]any{"file": file}}
}

// At AT
func At(userID int64) Segment {
	return Segment{Type: "at", Data: map[string]any{"qq": fmt.Sprint(userID)}}
}

// AtAll At All
func AtAll() Segment {
	return Segment{Type: "at", Data: map[string]any{"qq": "all"}}
}

// Reply 回复
func Reply(id string) Segment {
	return Segment{Type: "reply", Data: map[string]any{"id": id}}
}

// Link 链接分享
func Link(url string, title string, content string, image string) Segment {
	return Segment{Type: "share", Data: map[string]any{"url": url, "title": title, "content": content, "image": image}}
}

// Friends 推荐好友
func Friends(qqNumber int64) Segment {
	return Segment{Type: "contact", Data: map[string]any{"type": "qq", "id": fmt.Sprint(qqNumber)}}
}

// Set 合并转发消息节点
func Set(data []Segment, uid int64, nickname string) Segment {
	return Segment{Type: "node", Data: map[string]any{"user_id": fmt.Sprint(uid), "nickname": nickname, "content": data}}
}

// ---------- 接收侧: 消息段 -> 统一消息部件 ----------

type textData struct {
	Text string `json:"text"`
}

type atData struct {
	QQ string `json:"qq"`
}

type imageData struct {
	File string `json:"file"`
	URL  string `json:"url"`
}

type replyData struct {
	ID string `json:"id"`
}

type fileData struct {
	File     string `json:"file"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	FileSize int64  `json:"file_size"`
}

// convertSegments 将 OneBot 消息段转换为统一消息部件, 并报告是否 @ 了机器人自身
func convertSegments(segments []Segment, selfID string) ([]common.MessagePart, bool) {
	var (
		parts         []common.MessagePart
		mentionedSelf bool
	)
	for _, seg := range segments {
		switch seg.Type {
		case "text":
			var d textData
			if unmarshalSegment(seg.Data, &d) == nil && d.Text != "" {
				parts = append(parts, common.TextPart{Text: d.Text})
			}
		case "at":
			var d atData
			if unmarshalSegment(seg.Data, &d) == nil {
				if d.QQ == "all" {
					parts = append(parts, common.MentionAllPart{})
					continue
				}
				parts = append(parts, common.MentionPart{UserID: d.QQ})
				if d.QQ == selfID {
					mentionedSelf = true
				}
			}
		case "image":
			var d imageData
			if unmarshalSegment(seg.Data, &d) == nil {
				parts = append(parts, common.ImagePart{FileID: d.File, URL: d.URL})
			}
		case "record", "video", "file":
			var d fileData
			if unmarshalSegment(seg.Data, &d) == nil {
				parts = append(parts, common.FilePart{FileID: d.File, Name: d.Name, URL: d.URL, Size: d.FileSize})
			}
		case "reply":
			var d replyData
			if unmarshalSegment(seg.Data, &d) == nil {
				parts = append(parts, common.ReplyPart{MessageID: d.ID})
			}
		}
	}
	return parts, mentionedSelf
}

// unmarshalSegment 将消息段的 Data 字段落到具体结构上
func unmarshalSegment(data any, target any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}
