package feishu

import (
	"encoding/json"
	"fmt"
	"strings"

	"JuanNiang-Ops/internal/adapter/modules"
)

// ---------- 接收侧: 飞书 content -> 统一消息部件 ----------

// textContent text 消息 content 结构: {"text":"...","mentions"?}
type textContent struct {
	Text string `json:"text"`
}

// postContent 富文本 content 结构: {"title":"...","content":[[{"tag":"text","text":"..."},...],...]}
type postContent struct {
	Title   string       `json:"title"`
	Content [][]postNode `json:"content"`
}

type postNode struct {
	Tag    string          `json:"tag"` // text / a / at / img
	Text   string          `json:"text"`
	Href   string          `json:"href"`
	UserName string        `json:"user_name"` // at 节点
	UserID   string        `json:"user_id"`
	ImageKey string        `json:"image_key"`
}

// imageContent image 消息 content 结构: {"image_key":"..."}
type imageContent struct {
	ImageKey string `json:"image_key"`
}

// fileContent file 消息 content 结构: {"file_key":"...","file_name":"..."}
type fileContent struct {
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name"`
}

// convertContent 将飞书消息 content (JSON 字符串) 转换为统一消息部件.
// 飞书按 message_type 决定 content 结构, 此处逐类型解析.
// 返回 (parts, mentionedSelf, error).
func convertContent(messageType, content string, mentions []Mention, selfOpenID string) ([]modules.MessagePart, bool, error) {
	switch messageType {
	case "text":
		var d textContent
		if err := json.Unmarshal([]byte(content), &d); err != nil {
			return nil, false, fmt.Errorf("feishu: parse text content: %w", err)
		}
		parts := convertTextWithMentions(d.Text, mentions, selfOpenID)
		return parts, mentionedSelfInText(d.Text, mentions, selfOpenID), nil

	case "post":
		var d postContent
		if err := json.Unmarshal([]byte(content), &d); err != nil {
			return nil, false, fmt.Errorf("feishu: parse post content: %w", err)
		}
		parts, mentionedSelf := convertPost(&d, selfOpenID)
		return parts, mentionedSelf, nil

	case "image":
		var d imageContent
		if err := json.Unmarshal([]byte(content), &d); err != nil {
			return nil, false, fmt.Errorf("feishu: parse image content: %w", err)
		}
		return []modules.MessagePart{modules.ImagePart{FileID: d.ImageKey}}, false, nil

	case "file":
		var d fileContent
		if err := json.Unmarshal([]byte(content), &d); err != nil {
			return nil, false, fmt.Errorf("feishu: parse file content: %w", err)
		}
		return []modules.MessagePart{modules.FilePart{FileID: d.FileKey, Name: d.FileName}}, false, nil

	default:
		// 未知类型透传为通知事件由上层处理, 这里返回空部件
		return nil, false, nil
	}
}

// convertTextWithMentions 解析 text 消息: @ 占位符 (@_user_1) 替换为 MentionPart.
// 飞书 text 中 @ 以 "@_user_1" 形式出现在文本里, 对应 mentions 数组中的 Key.
func convertTextWithMentions(text string, mentions []Mention, selfOpenID string) []modules.MessagePart {
	if len(mentions) == 0 {
		return []modules.MessagePart{modules.TextPart{Text: text}}
	}

	var (
		parts     []modules.MessagePart
		remaining = text
	)
	for _, m := range mentions {
		idx := strings.Index(remaining, m.Key)
		if idx < 0 {
			continue
		}
		if idx > 0 {
			parts = append(parts, modules.TextPart{Text: remaining[:idx]})
		}
		parts = append(parts, modules.MentionPart{UserID: m.ID, UserName: m.Name})
		remaining = remaining[idx+len(m.Key):]
	}
	if remaining != "" {
		parts = append(parts, modules.TextPart{Text: remaining})
	}
	return parts
}

// mentionedSelfInText 判断 text 消息是否 @ 了机器人自身
func mentionedSelfInText(text string, mentions []Mention, selfOpenID string) bool {
	for _, m := range mentions {
		if m.ID != "" && m.ID == selfOpenID && strings.Contains(text, m.Key) {
			return true
		}
	}
	return false
}

// convertPost 富文本消息: 按段落展开为部件序列
func convertPost(d *postContent, selfOpenID string) ([]modules.MessagePart, bool) {
	var (
		parts         []modules.MessagePart
		mentionedSelf bool
	)
	for _, paragraph := range d.Content {
		for _, node := range paragraph {
			switch node.Tag {
			case "text":
				if node.Text != "" {
					parts = append(parts, modules.TextPart{Text: node.Text})
				}
			case "a":
				text := node.Text
				if text == "" {
					text = node.Href
				}
				parts = append(parts, modules.TextPart{Text: text + " (" + node.Href + ")"})
			case "at":
				parts = append(parts, modules.MentionPart{UserID: node.UserID, UserName: node.UserName})
				if node.UserID != "" && node.UserID == selfOpenID {
					mentionedSelf = true
				}
			case "img":
				if node.ImageKey != "" {
					parts = append(parts, modules.ImagePart{FileID: node.ImageKey})
				}
			}
		}
	}
	return parts, mentionedSelf
}

// ---------- 发送侧: 统一消息部件 -> 飞书消息体 ----------

// buildOutgoing 将统一部件构造为飞书发送载荷 (msg_type + content JSON 字符串).
// 策略: 单一类型走最小协议 (text/image), 混合内容走 post 富文本.
// 返回 (msgType, content, replyMessageID, error); replyMessageID 非空时走 reply 接口.
func buildOutgoing(parts []modules.MessagePart) (string, string, string, error) {
	var replyID string
	if len(parts) == 0 {
		return "", "", "", fmt.Errorf("feishu: empty message parts")
	}

	// 提取回复引用 (飞书回复是独立接口, 不作为消息内容)
	filtered := make([]modules.MessagePart, 0, len(parts))
	for _, p := range parts {
		if rp, ok := p.(modules.ReplyPart); ok {
			replyID = rp.MessageID
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return "", "", "", fmt.Errorf("feishu: only reply part without content")
	}

	// 纯文本类部件 (Text/Mention) -> text 消息, Mention 转为 <at> 标签
	if isAllText(filtered) {
		var b strings.Builder
		for _, p := range filtered {
			switch v := p.(type) {
			case modules.TextPart:
				b.WriteString(v.Text)
			case modules.MentionPart:
				b.WriteString(`<at user_id="` + v.UserID + `"></at>`)
			case modules.MentionAllPart:
				b.WriteString(`<at user_id="all"></at>`)
			}
		}
		content, err := json.Marshal(map[string]string{"text": b.String()})
		if err != nil {
			return "", "", "", err
		}
		return "text", string(content), replyID, nil
	}

	// 单一 image 部件 -> image 消息
	if len(filtered) == 1 {
		if img, ok := filtered[0].(modules.ImagePart); ok {
			content, err := json.Marshal(map[string]string{"image_key": img.FileID})
			if err != nil {
				return "", "", "", err
			}
			return "image", string(content), replyID, nil
		}
	}

	// 混合内容 -> post 富文本
	post := postContent{Content: [][]postNode{{{}}}}
	line := &post.Content[0]
	for _, p := range filtered {
		switch v := p.(type) {
		case modules.TextPart:
			*line = append(*line, postNode{Tag: "text", Text: v.Text})
		case modules.MentionPart:
			*line = append(*line, postNode{Tag: "at", UserID: v.UserID})
		case modules.MentionAllPart:
			*line = append(*line, postNode{Tag: "at", UserName: "所有人"})
		case modules.ImagePart:
			*line = append(*line, postNode{Tag: "img", ImageKey: v.FileID})
		default:
			return "", "", "", fmt.Errorf("feishu: unsupported part %T in post", p)
		}
	}
	content, err := json.Marshal(post)
	if err != nil {
		return "", "", "", err
	}
	return "post", string(content), replyID, nil
}

// isAllText 判断部件是否全部为纯文本 (Mention 属于文本标签, 走 text 协议的 @ 语法)
func isAllText(parts []modules.MessagePart) bool {
	for _, p := range parts {
		switch p.(type) {
		case modules.TextPart, modules.MentionPart, modules.MentionAllPart:
			continue
		default:
			return false
		}
	}
	return true
}
