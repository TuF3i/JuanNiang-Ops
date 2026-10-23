package onebot

import "fmt"

// 消息段
type Segment struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// 文本消息
func Text(text string) Segment {
	return Segment{Type: "text", Data: map[string]any{"text": text}}
}

// 图片
func Image(file string) Segment {
	return Segment{Type: "image", Data: map[string]any{"file": file}}
}

// AT
func At(userID int64) Segment {
	return Segment{Type: "at", Data: map[string]any{"qq": fmt.Sprint(userID)}}
}

// At All
func AtAll() Segment {
	return Segment{Type: "at", Data: map[string]any{"qq": "all"}}
}

// Reply
func Reply(id string) Segment {
	return Segment{Type: "reply", Data: map[string]any{"id": id}}
}

// Link
func Link(url string, title string, content string, image string) Segment {
	return Segment{Type: "share", Data: map[string]any{"url": url, "title": title, "content": content, "image": image}}
}

// 推荐好友
func Friends(qqNumber int64) Segment {
	return Segment{Type: "contact", Data: map[string]any{"type": "qq", "id": fmt.Sprint(qqNumber)}}
}

// 合并转发消息
func Set(data []Segment, uid int64, nickname string) Segment {
	return Segment{Type: "node", Data: map[string]any{"user_id": fmt.Sprint(uid), "nickname": nickname, "content": data}}
}
