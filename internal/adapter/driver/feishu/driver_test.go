package feishu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/config"
)

// ---------- 测试辅助 ----------

// mockWSClient 长连接客户端 mock: 暴露注入的回调供测试直调
type mockWSClient struct {
	onMessage func(ctx context.Context, event *larkim.P2MessageReceiveV1) error
	block     chan struct{} // Start 阻塞模拟长连接
}

func (m *mockWSClient) Start(ctx context.Context) error {
	<-ctx.Done() // 模拟长连接维持, ctx 取消后退出
	return ctx.Err()
}

// dispatch 直调事件回调 (模拟飞书推送)
func (m *mockWSClient) dispatch(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if m.onMessage == nil {
		return nil
	}
	return m.onMessage(ctx, event)
}

// newAPIStub httptest 模拟飞书 OpenAPI, 记录请求
type apiStub struct {
	path atomic.Value
	body atomic.Value
	auth atomic.Value
	srv  *httptest.Server
}

func newAPIStub(t *testing.T) *apiStub {
	s := &apiStub{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		s.path.Store(req.URL.String())
		s.body.Store(string(body))
		s.auth.Store(req.Header.Get("Authorization"))
		// reply 响应带 message 包装; 发送直接返回 message 字段
		if strings.HasSuffix(req.URL.Path, "/reply") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"message": map[string]any{"message_id": "om_sent"}},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": map[string]any{"message_id": "om_sent"},
			})
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// newTestDriver 创建长连接模式的测试驱动 (mock ws + httptest API)
func newTestDriver(t *testing.T, cfg config.FeishuConfig) (*Driver, *mockWSClient, *apiStub) {
	t.Helper()
	if cfg.EventBuffer <= 0 {
		cfg.EventBuffer = 16
	}
	if cfg.AppID == "" {
		cfg.AppID = "cli_test_app"
	}
	if cfg.AppSecret == "" {
		cfg.AppSecret = "test-secret"
	}

	stub := newAPIStub(t)
	d := NewDriver(cfg)
	d.SetOpenBaseUrl(stub.srv.URL)
	d.SetSelfOpenID("ou_self")

	// 拦截 OpenAPI 请求 (token + 发送均走 stub)
	d.SetHTTPClient(newStubHTTPClient(stub))

	mock := &mockWSClient{block: make(chan struct{})}
	d.SetWSClient(mock)

	if err := d.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop() })
	return d, mock, stub
}

// newStubHTTPClient 用 httptest server 模拟所有 OpenAPI 响应
func newStubHTTPClient(stub *apiStub) larkcore.HttpClient {
	return &stubHTTPClient{stub: stub}
}

type stubHTTPClient struct {
	stub *apiStub
}

func (c *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// 记录非 token 请求
	if !strings.Contains(req.URL.Path, "tenant_access_token") {
		body, _ := io.ReadAll(req.Body)
		c.stub.path.Store(req.URL.String())
		c.stub.body.Store(string(body))
		c.stub.auth.Store(req.Header.Get("Authorization"))
	} else {
		// token 请求: 返回固定 token
		return makeJSONResponse(`{"code":0,"tenant_access_token":"tok","expire":7200}`), nil
	}
	return makeJSONResponse(`{"code":0,"data":{"message_id":"om_sent"}}`), nil
}

func makeJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// recvEvent 等待一条事件
func recvEvent(t *testing.T, d *Driver) common.Event {
	t.Helper()
	select {
	case ev := <-d.Events():
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for event")
		return common.Event{}
	}
}

// ---------- 用例 ----------

// TestMessageEventConvert 消息事件 (text + mention) → Event{Message}
func TestMessageEventConvert(t *testing.T) {
	d, _, _ := newTestDriver(t, config.FeishuConfig{Enable: true})

	content := `{"text":"@_user_1 你好"}`
	event := buildMessageEvent("om_1", "oc_group1", "group", "text", content, "1700000000000",
		[]*larkim.MentionEvent{
			larkim.NewMentionEventBuilder().Key("@_user_1").
				Id(larkim.NewUserIdBuilder().OpenId("ou_self").Build()).Name("Bot").Build(),
		})

	if err := d.onMessageReceive(context.Background(), event); err != nil {
		t.Fatalf("onMessageReceive: %v", err)
	}

	ev := recvEvent(t, d)
	if ev.Type != common.EventMessage {
		t.Fatalf("event type = %q", ev.Type)
	}
	msg := ev.Message
	if msg == nil {
		t.Fatal("message is nil")
	}
	if msg.Platform != common.Feishu || msg.MessageType != common.Group {
		t.Errorf("platform/type = %v/%v", msg.Platform, msg.MessageType)
	}
	if msg.ChatID != "oc_group1" || msg.MessageID != "om_1" {
		t.Errorf("chat/message_id = %s/%s", msg.ChatID, msg.MessageID)
	}
	if msg.Timestamp != 1700000000 {
		t.Errorf("timestamp = %d, want 1700000000", msg.Timestamp)
	}
	if !msg.MentionedSelf {
		t.Error("mentioned_self = false, want true (bot was mentioned)")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("content parts = %d, want 2 (mention + text)", len(msg.Content))
	}
	if p, ok := msg.Content[0].(common.MentionPart); !ok || p.UserID != "ou_self" {
		t.Errorf("part0 = %#v", msg.Content[0])
	}
	if p, ok := msg.Content[1].(common.TextPart); !ok || p.Text != " 你好" {
		t.Errorf("part1 = %#v", msg.Content[1])
	}
}

// TestAppMessageSkipped 机器人自己的消息跳过 (防自循环)
func TestAppMessageSkipped(t *testing.T) {
	d, _, _ := newTestDriver(t, config.FeishuConfig{Enable: true})

	event := buildMessageEvent("om_1", "oc_chat", "p2p", "text", `{"text":"hi"}`, "1700000000000", nil)
	event.Event.Sender.SenderType = ptr("app")

	if err := d.onMessageReceive(context.Background(), event); err != nil {
		t.Fatalf("onMessageReceive: %v", err)
	}
	select {
	case ev := <-d.Events():
		t.Fatalf("app message should be skipped, got %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestUnknownMessageTypeToNotify 未支持消息类型 → EventNotify
func TestUnknownMessageTypeToNotify(t *testing.T) {
	d, _, _ := newTestDriver(t, config.FeishuConfig{Enable: true})

	event := buildMessageEvent("om_2", "oc_chat", "p2p", "audio", `{"file_key":"k"}`, "1700000000000", nil)
	if err := d.onMessageReceive(context.Background(), event); err != nil {
		t.Fatalf("onMessageReceive: %v", err)
	}

	ev := recvEvent(t, d)
	if ev.Type != common.EventNotify {
		t.Fatalf("event type = %q, want notify", ev.Type)
	}
	if ev.Notify == nil || ev.Notify.Platform != common.Feishu {
		t.Fatalf("notify = %+v", ev.Notify)
	}
	if !strings.Contains(ev.Notify.Kind, "audio") {
		t.Errorf("notify kind = %q", ev.Notify.Kind)
	}
}

// TestSendMessage 发送: 无回复 → messages 接口 (chat_id), 带回复 → reply 接口
func TestSendMessage(t *testing.T) {
	d, _, stub := newTestDriver(t, config.FeishuConfig{Enable: true})

	// 带回复引用 -> reply 接口
	receipt, err := d.SendGroup(context.Background(), "oc_group1", []common.MessagePart{
		common.ReplyPart{MessageID: "om_reply_target"},
		common.TextPart{Text: "hi "},
		common.MentionPart{UserID: "ou_x"},
	}, "")
	if err != nil {
		t.Fatalf("send group: %v", err)
	}
	if receipt.MessageID != "om_sent" {
		t.Errorf("receipt.message_id = %q", receipt.MessageID)
	}
	if path := stub.path.Load().(string); !strings.Contains(path, "reply") {
		t.Errorf("reply path = %s", path)
	}
	if body := stub.body.Load().(string); !strings.Contains(body, `"msg_type":"text"`) {
		t.Errorf("reply body = %s", body)
	}

	// 无回复 -> messages 接口, receive_id_type=chat_id
	receipt, err = d.SendGroup(context.Background(), "oc_group1", []common.MessagePart{common.TextPart{Text: "direct"}}, "")
	if err != nil {
		t.Fatalf("send group direct: %v", err)
	}
	if receipt.MessageID != "om_sent" {
		t.Errorf("direct receipt = %q", receipt.MessageID)
	}
	if path := stub.path.Load().(string); !strings.Contains(path, "receive_id_type=chat_id") {
		t.Errorf("send path = %s", path)
	}
	if body := stub.body.Load().(string); !strings.Contains(body, `"receive_id":"oc_group1"`) {
		t.Errorf("send body = %s", body)
	}

	// 私聊: receive_id_type=open_id
	if _, err := d.SendPrivate(context.Background(), "ou_friend", []common.MessagePart{common.TextPart{Text: "p"}}, ""); err != nil {
		t.Fatalf("send private: %v", err)
	}
	if path := stub.path.Load().(string); !strings.Contains(path, "receive_id_type=open_id") {
		t.Errorf("private path = %s", path)
	}
}

// TestNotRunningSendNotStart 未 Start 时发送报错
func TestNotRunningSendNotStart(t *testing.T) {
	d := NewDriver(config.FeishuConfig{Enable: true})
	if _, err := d.SendGroup(context.Background(), "oc", nil, ""); err == nil {
		t.Error("send before start should fail")
	}
}

// ---------- 构造辅助 ----------

func buildMessageEvent(messageID, chatID, chatType, msgType, content, createTime string, mentions []*larkim.MentionEvent) *larkim.P2MessageReceiveV1 {
	return &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderId:   &larkim.UserId{OpenId: ptr("ou_user1")},
				SenderType: ptr("user"),
			},
			Message: &larkim.EventMessage{
				MessageId:   ptr(messageID),
				ChatId:      ptr(chatID),
				ChatType:    ptr(chatType),
				MessageType: ptr(msgType),
				Content:     ptr(content),
				CreateTime:  ptr(createTime),
				Mentions:    mentions,
			},
		},
	}
}

func ptr(s string) *string { return &s }
