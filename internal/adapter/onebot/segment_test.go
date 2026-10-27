package onebot

import (
	"encoding/json"
	"strings"
	"testing"

	"JuanNiang-Ops/internal/adapter/common"
)

// TestConvertSegmentsToParts 接收侧: OneBot 段 -> 统一部件
func TestConvertSegmentsToParts(t *testing.T) {
	segments := []Segment{
		Text("hello "),
		At(10000),
		AtAll(),
		Image("img.jpg"),
		Reply("42"),
	}

	parts, mentionedSelf := convertSegments(segments, "10000")
	if !mentionedSelf {
		t.Error("mentioned_self = false, want true")
	}
	if len(parts) != 5 {
		t.Fatalf("parts = %d, want 5", len(parts))
	}

	if p, ok := parts[0].(common.TextPart); !ok || p.Text != "hello " {
		t.Errorf("part0 = %#v", parts[0])
	}
	if p, ok := parts[1].(common.MentionPart); !ok || p.UserID != "10000" {
		t.Errorf("part1 = %#v", parts[1])
	}
	if _, ok := parts[2].(common.MentionAllPart); !ok {
		t.Errorf("part2 = %#v", parts[2])
	}
	if p, ok := parts[3].(common.ImagePart); !ok || p.FileID != "img.jpg" {
		t.Errorf("part3 = %#v", parts[3])
	}
	if p, ok := parts[4].(common.ReplyPart); !ok || p.MessageID != "42" {
		t.Errorf("part4 = %#v", parts[4])
	}

	// 未 @ 自身
	if _, mentioned := convertSegments([]Segment{At(9999)}, "10000"); mentioned {
		t.Error("mentioned_self should be false")
	}
}

// TestPartsToSegments 发送侧: 统一部件 -> OneBot 段
func TestPartsToSegments(t *testing.T) {
	parts := []common.MessagePart{
		common.ReplyPart{MessageID: "42"},
		common.TextPart{Text: "reply "},
		common.MentionPart{UserID: "10000"},
		common.MentionAllPart{},
		common.ImagePart{FileID: "abc.jpg", URL: "https://example.com/a.jpg"},
		common.FilePart{Name: "doc.pdf", URL: "https://example.com/doc.pdf", Size: 100},
	}

	segments, err := PartsToSegments(parts)
	if err != nil {
		t.Fatalf("PartsToSegments: %v", err)
	}
	if len(segments) != 6 {
		t.Fatalf("segments = %d, want 6", len(segments))
	}

	// 序列化验证实际 JSON 形态
	data, err := json.Marshal(segments)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{
		`"reply"`, `"id":"42"`,
		`"text"`, `"text":"reply "`,
		`"at"`, `"qq":"10000"`, `"qq":"all"`,
		`"image"`, `"file":"abc.jpg"`,
		`"file"`, `"url":"https://example.com/doc.pdf"`, `"name":"doc.pdf"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("payload missing %s: %s", want, s)
		}
	}
}

// TestPartsToSegmentsEdgeCases 边界: 空数组 / Image 双空 / 未知部件 / Mention 非数字
func TestPartsToSegmentsEdgeCases(t *testing.T) {
	// 空输入
	if segs, err := PartsToSegments(nil); err != nil || segs != nil {
		t.Errorf("nil parts: got %v, %v", segs, err)
	}

	// Image 无 file_id 与 url
	_, err := PartsToSegments([]common.MessagePart{common.ImagePart{}})
	if err == nil || !strings.Contains(err.Error(), "part[0] image") {
		t.Errorf("empty image: want error, got %v", err)
	}

	// 未知部件 (自定义实现)
	_, err = PartsToSegments([]common.MessagePart{unknownPart{}})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("unknown part: want error, got %v", err)
	}

	// Mention 非 QQ 号
	_, err = PartsToSegments([]common.MessagePart{common.MentionPart{UserID: "open_id_abc"}})
	if err == nil || !strings.Contains(err.Error(), "invalid user_id") {
		t.Errorf("non-numeric mention: want error, got %v", err)
	}
}

// unknownPart 未支持部件的桩
type unknownPart struct{}

func (unknownPart) PartType() common.MessageMIME { return "message.mime.unknown" }

// TestPartsToSegmentsImageFallback Image 优先 FileID, 其次 URL
func TestPartsToSegmentsImageFallback(t *testing.T) {
	segments, err := PartsToSegments([]common.MessagePart{
		common.ImagePart{URL: "https://example.com/a.jpg"}, // 无 FileID, 回退 URL
	})
	if err != nil {
		t.Fatalf("PartsToSegments: %v", err)
	}
	if len(segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(segments))
	}
	data, _ := json.Marshal(segments[0])
	if !strings.Contains(string(data), `"file":"https://example.com/a.jpg"`) {
		t.Errorf("image fallback payload = %s", data)
	}
}
