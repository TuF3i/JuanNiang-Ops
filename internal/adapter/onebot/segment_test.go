package onebot

import (
	"encoding/json"
	"testing"

	"JuanNiang-Ops/internal/adapter/common"
)

func TestConvertSegments(t *testing.T) {
	segments := []Segment{
		{Type: "text", Data: map[string]any{"text": "hi "}},
		{Type: "at", Data: map[string]any{"qq": "10001"}},
		{Type: "at", Data: map[string]any{"qq": "all"}},
		{Type: "image", Data: map[string]any{"file": "abc.image", "url": "http://example.com/a.jpg"}},
		{Type: "file", Data: map[string]any{"file": "f.zip", "name": "f.zip", "url": "http://example.com/f.zip", "file_size": 10}},
		{Type: "reply", Data: map[string]any{"id": "42"}},
	}

	parts, mentionedSelf := convertSegments(segments, "10001")
	if !mentionedSelf {
		t.Fatal("应检测到 @ 机器人自身")
	}
	if len(parts) != 6 {
		t.Fatalf("want 6 parts, got %d", len(parts))
	}

	wantTypes := []common.MessageMIME{
		common.Text, common.Mention, common.MentionALL, common.Image, common.File, common.Reply,
	}
	for i, p := range parts {
		if p.PartType() != wantTypes[i] {
			t.Fatalf("part %d: want %v, got %v", i, wantTypes[i], p.PartType())
		}
	}

	// 抽查字段
	if tp, ok := parts[0].(common.TextPart); !ok || tp.Text != "hi " {
		t.Fatalf("unexpected text part: %#v", parts[0])
	}
	if ip, ok := parts[3].(common.ImagePart); !ok || ip.FileID != "abc.image" || ip.URL != "http://example.com/a.jpg" {
		t.Fatalf("unexpected image part: %#v", parts[3])
	}
	if fp, ok := parts[4].(common.FilePart); !ok || fp.FileID != "f.zip" || fp.Size != 10 {
		t.Fatalf("unexpected file part: %#v", parts[4])
	}
	if rp, ok := parts[5].(common.ReplyPart); !ok || rp.MessageID != "42" {
		t.Fatalf("unexpected reply part: %#v", parts[5])
	}
}

func TestConvertSegmentsNotMentioned(t *testing.T) {
	parts, mentionedSelf := convertSegments(
		[]Segment{{Type: "at", Data: map[string]any{"qq": "999"}}},
		"10001",
	)
	if mentionedSelf {
		t.Fatal("不应检测到 @ 机器人自身")
	}
	if len(parts) != 1 {
		t.Fatalf("want 1 part, got %d", len(parts))
	}
	if mp, ok := parts[0].(common.MentionPart); !ok || mp.UserID != "999" {
		t.Fatalf("unexpected mention part: %#v", parts[0])
	}
}

func TestConvertSegmentsIgnoreUnknown(t *testing.T) {
	parts, _ := convertSegments(
		[]Segment{
			{Type: "face", Data: map[string]any{"id": "1"}},
			{Type: "text", Data: map[string]any{"text": ""}}, // 空文本忽略
		},
		"10001",
	)
	if len(parts) != 0 {
		t.Fatalf("未知/空段应被忽略, got %d parts", len(parts))
	}
}

func TestActionSerialization(t *testing.T) {
	raw, err := json.Marshal(Action{
		Action: "send_group_msg",
		Params: map[string]any{"group_id": 888},
		Echo:   "e1",
		Target: "10001",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["action"] != "send_group_msg" || m["echo"] != "e1" {
		t.Fatalf("unexpected action json: %s", raw)
	}
	if _, exists := m["target"]; exists {
		t.Fatal("Target 是路由元数据, 不应被序列化")
	}
}
