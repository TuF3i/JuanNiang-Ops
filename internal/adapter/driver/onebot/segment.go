package onebot

import (
	"encoding/json"
	"fmt"
	"strconv"

	"JuanNiang-Ops/internal/adapter/modules"
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
func convertSegments(segments []Segment, selfID string) ([]modules.MessagePart, bool) {
	var (
		parts         []modules.MessagePart
		mentionedSelf bool
	)
	for _, seg := range segments {
		switch seg.Type {
		case "text":
			var d textData
			if unmarshalSegment(seg.Data, &d) == nil && d.Text != "" {
				parts = append(parts, modules.TextPart{Text: d.Text})
			}
		case "at":
			var d atData
			if unmarshalSegment(seg.Data, &d) == nil {
				if d.QQ == "all" {
					parts = append(parts, modules.MentionAllPart{})
					continue
				}
				parts = append(parts, modules.MentionPart{UserID: d.QQ})
				if d.QQ == selfID {
					mentionedSelf = true
				}
			}
		case "image":
			var d imageData
			if unmarshalSegment(seg.Data, &d) == nil {
				parts = append(parts, modules.ImagePart{FileID: d.File, URL: d.URL})
			}
		case "record", "video", "file":
			var d fileData
			if unmarshalSegment(seg.Data, &d) == nil {
				parts = append(parts, modules.FilePart{FileID: d.File, Name: d.Name, URL: d.URL, Size: d.FileSize})
			}
		case "reply":
			var d replyData
			if unmarshalSegment(seg.Data, &d) == nil {
				parts = append(parts, modules.ReplyPart{MessageID: d.ID})
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

// ---------- 发送侧: 统一消息部件 -> OneBot 消息段 ----------

// PartsToSegments 将统一消息部件转换为 OneBot 消息段 (业务层只需构造 common 部件).
// 支持的部件: TextPart / MentionPart / MentionAllPart / ImagePart / FilePart / ReplyPart;
// 未知部件返回错误 (不静默丢弃, 避免消息内容意外缺失).
func PartsToSegments(parts []modules.MessagePart) ([]Segment, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	segments := make([]Segment, 0, len(parts))
	for i, part := range parts {
		switch p := part.(type) {
		case modules.TextPart:
			segments = append(segments, Text(p.Text))
		case modules.MentionPart:
			uid, err := strconv.ParseInt(p.UserID, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("onebot: part[%d] mention: invalid user_id %q: %w", i, p.UserID, err)
			}
			segments = append(segments, At(uid))
		case modules.MentionAllPart:
			segments = append(segments, AtAll())
		case modules.ImagePart:
			// 优先平台文件标识 (file:// 或 http:// 等实现端可识别的形式), 其次远程 URL
			file := p.FileID
			if file == "" {
				file = p.URL
			}
			if file == "" {
				return nil, fmt.Errorf("onebot: part[%d] image: file_id and url are both empty", i)
			}
			segments = append(segments, Image(file))
		case modules.FilePart:
			data := map[string]any{
				"file": firstNonEmpty(p.FileID, p.URL),
				"name": p.Name,
			}
			if p.URL != "" {
				data["url"] = p.URL
			}
			segments = append(segments, Segment{Type: "file", Data: data})
		case modules.ReplyPart:
			segments = append(segments, Reply(p.MessageID))
		default:
			return nil, fmt.Errorf("onebot: part[%d] unsupported type %q", i, part.PartType())
		}
	}
	return segments, nil
}

// firstNonEmpty 返回第一个非空字符串, 全空返回空串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
