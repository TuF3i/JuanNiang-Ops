package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxzan/gws"

	"JuanNiang-Ops/internal/adapter/common"
)

// ---------- 测试基础设施 ----------

// startTestAdapter 启动一个监听随机端口的适配器, 测试结束自动 Stop
func startTestAdapter(t *testing.T, mutate func(*Config)) *Adapter {
	t.Helper()
	cfg := Config{Addr: "127.0.0.1:0", Enable: true}
	if mutate != nil {
		mutate(&cfg)
	}
	a := NewAdapter(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("start adapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Stop() })
	return a
}

func (a *Adapter) testAddr() string { return a.listener.Addr().String() }

// wsClient 测试用 gws 客户端
type wsClient struct {
	gws.BuiltinEventHandler
	conn *gws.Conn

	// 收到服务端下发的动作时触发 (用于模拟 OneBot 实现端回复 API 回执)
	onMessage func(c *wsClient, data []byte)

	mu       sync.Mutex
	received [][]byte
	closed   bool
}

func (c *wsClient) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	data := append([]byte(nil), message.Bytes()...)
	c.mu.Lock()
	c.received = append(c.received, data)
	c.mu.Unlock()
	if c.onMessage != nil {
		c.onMessage(c, data)
	}
}

func (c *wsClient) OnClose(_ *gws.Conn, _ error) {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
}

func (c *wsClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.received)
}

func (c *wsClient) last() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.received) == 0 {
		return nil
	}
	return c.received[len(c.received)-1]
}

func (c *wsClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// dialTestClient 连接适配器
func dialTestClient(t *testing.T, addr string, header http.Header) *wsClient {
	t.Helper()
	c := &wsClient{}
	option := &gws.ClientOption{Addr: "ws://" + addr}
	if header != nil {
		option.RequestHeader = header
	}
	conn, _, err := gws.NewClient(c, option)
	if err != nil {
		t.Fatalf("dial ws://%s: %v", addr, err)
	}
	c.conn = conn
	go conn.ReadLoop()
	t.Cleanup(func() { _ = conn.NetConn().Close() })
	return c
}

// identify 上报一条群消息事件, 让服务端绑定 self_id
func (c *wsClient) identify(selfID int64) {
	c.send(map[string]any{
		"post_type":    "message",
		"message_type": "group",
		"self_id":      selfID,
		"time":         time.Now().Unix(),
		"message_id":   1,
		"group_id":     888,
		"user_id":      selfID,
		"message":      []map[string]any{{"type": "text", "data": map[string]any{"text": "hello"}}},
		"sender":       map[string]any{"user_id": selfID, "nickname": "bot"},
	})
}

func (c *wsClient) send(payload any) {
	raw, _ := json.Marshal(payload)
	_ = c.conn.WriteMessage(gws.OpcodeText, raw)
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitPeer 等待服务端绑定 self_id
func waitPeer(t *testing.T, a *Adapter, selfID string) {
	t.Helper()
	waitFor(t, 2*time.Second, func() bool {
		a.mu.RLock()
		defer a.mu.RUnlock()
		_, ok := a.peers[selfID]
		return ok
	}, "peer "+selfID+" not registered")
}

// recvInbound 从接收管道读取一条消息 (带超时)
func recvInbound(t *testing.T, a *Adapter) common.Message {
	t.Helper()
	select {
	case msg := <-a.Inbound():
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("等待入站消息超时")
		return common.Message{}
	}
}

// ---------- 生命周期 ----------

func TestLifecycle(t *testing.T) {
	a := NewAdapter(Config{Addr: "127.0.0.1:0"}) // Enable=false

	if err := a.Start(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}

	a.cfg.Enable = true
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := a.Start(); !errors.Is(err, ErrRunning) {
		t.Fatalf("want ErrRunning, got %v", err)
	}

	if err := a.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// 幂等
	if err := a.Stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}

	// Done 已关闭
	select {
	case <-a.Done():
	default:
		t.Fatal("Done should be closed after Stop")
	}

	// Inbound 已关闭
	if _, ok := <-a.Inbound(); ok {
		t.Fatal("Inbound should be closed after Stop")
	}

	// 停止后投递返回 ErrNotRunning
	if err := a.Send("get_login_info", nil); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("want ErrNotRunning, got %v", err)
	}

	// 可重启
	if err := a.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("final stop: %v", err)
	}
}

func TestStopDisconnectsClients(t *testing.T) {
	a := startTestAdapter(t, nil)
	c := dialTestClient(t, a.testAddr(), nil)
	c.identify(10001)
	waitPeer(t, a, "10001")

	done := make(chan struct{})
	go func() {
		_ = a.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop 阻塞未返回")
	}

	waitFor(t, 2*time.Second, c.isClosed, "Stop 后客户端连接应被关闭")
}

// ---------- 握手鉴权 ----------

func TestAuthorize(t *testing.T) {
	// 未配置 Token: 一律放行
	open := NewAdapter(Config{})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	if !open.authorize(req, nil) {
		t.Fatal("未配置 Token 时应放行")
	}

	a := NewAdapter(Config{Token: "secret"})
	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	if a.authorize(req, nil) {
		t.Fatal("无凭证应拒绝")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws?access_token=secret", nil)
	if !a.authorize(req, nil) {
		t.Fatal("query 参数鉴权应通过")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if !a.authorize(req, nil) {
		t.Fatal("Bearer 鉴权应通过")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Authorization", "Token secret")
	if !a.authorize(req, nil) {
		t.Fatal("Token 鉴权应通过")
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if a.authorize(req, nil) {
		t.Fatal("错误凭证应拒绝")
	}
}

func TestWebSocketAuth(t *testing.T) {
	a := startTestAdapter(t, func(c *Config) { c.Token = "secret" })

	// 无凭证被拒绝
	bad := &wsClient{}
	if _, _, err := gws.NewClient(bad, &gws.ClientOption{Addr: "ws://" + a.testAddr()}); err == nil {
		t.Fatal("无凭证握手应失败")
	}

	// 携带 Bearer 凭证成功接入
	ok := dialTestClient(t, a.testAddr(), http.Header{"Authorization": []string{"Bearer secret"}})
	ok.identify(10001)
	waitPeer(t, a, "10001")
}

// ---------- 事件 -> 统一消息 ----------

func TestInboundGroupMessage(t *testing.T) {
	a := startTestAdapter(t, nil)

	a.onPayload(nil, mustJSON(t, map[string]any{
		"post_type":    "message",
		"message_type": "group",
		"self_id":      int64(10001),
		"time":         int64(1700000000),
		"message_id":   int32(42),
		"group_id":     int64(888),
		"user_id":      int64(20002),
		"message": []map[string]any{
			{"type": "at", "data": map[string]any{"qq": "10001"}},
			{"type": "text", "data": map[string]any{"text": "你好"}},
		},
		"sender": map[string]any{"user_id": int64(20002), "nickname": "tester", "card": "TT", "role": "admin"},
	}))

	msg := recvInbound(t, a)
	if msg.Platform != common.Onebot {
		t.Fatalf("platform: want %v, got %v", common.Onebot, msg.Platform)
	}
	if msg.MessageType != common.Group {
		t.Fatalf("message type: want %v, got %v", common.Group, msg.MessageType)
	}
	if msg.MessageID != "42" || msg.ChatID != "888" || msg.SelfID != "10001" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if msg.Sender.UID != "20002" || msg.Sender.Name != "TT" || !msg.Sender.IsAdmin {
		t.Fatalf("unexpected sender: %#v", msg.Sender)
	}
	if !msg.MentionedSelf {
		t.Fatal("应检测到 @ 机器人")
	}
	if msg.Timestamp != 1700000000 {
		t.Fatalf("timestamp: got %d", msg.Timestamp)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("want 2 parts, got %d", len(msg.Content))
	}
	if _, ok := msg.Content[0].(common.MentionPart); !ok {
		t.Fatalf("part 0 应为 MentionPart: %#v", msg.Content[0])
	}
	if tp, ok := msg.Content[1].(common.TextPart); !ok || tp.Text != "你好" {
		t.Fatalf("part 1 应为 TextPart: %#v", msg.Content[1])
	}
}

func TestInboundPrivateMessage(t *testing.T) {
	a := startTestAdapter(t, nil)

	a.onPayload(nil, mustJSON(t, map[string]any{
		"post_type":    "message",
		"message_type": "private",
		"self_id":      int64(10001),
		"time":         int64(1700000001),
		"message_id":   int32(43),
		"user_id":      int64(20002),
		"message":      []map[string]any{{"type": "text", "data": map[string]any{"text": "hello"}}},
		"sender":       map[string]any{"user_id": int64(20002), "nickname": "tester"},
	}))

	msg := recvInbound(t, a)
	if msg.MessageType != common.Private {
		t.Fatalf("message type: want %v, got %v", common.Private, msg.MessageType)
	}
	if msg.ChatID != "20002" {
		t.Fatalf("私聊 ChatID 应为 user_id, got %q", msg.ChatID)
	}
	if msg.Sender.IsAdmin {
		t.Fatal("私聊发送者不应被标记为管理员")
	}
}

func TestInboundInvalidPayload(t *testing.T) {
	a := startTestAdapter(t, nil)
	// 非法 JSON 不应 panic, 也不应入管道
	a.onPayload(nil, []byte("not-json"))
	// 未知 post_type 同样忽略
	a.onPayload(nil, mustJSON(t, map[string]any{"post_type": "whatever", "self_id": int64(1)}))

	select {
	case msg := <-a.Inbound():
		t.Fatalf("不应收到消息: %#v", msg)
	case <-time.After(100 * time.Millisecond):
	}
}

// ---------- 端到端: 真实 WebSocket 客户端 ----------

func TestWebSocketMessageFlow(t *testing.T) {
	a := startTestAdapter(t, nil)
	c := dialTestClient(t, a.testAddr(), nil)

	c.identify(10001)
	waitPeer(t, a, "10001")

	msg := recvInbound(t, a)
	if msg.SelfID != "10001" || msg.ChatID != "888" {
		t.Fatalf("unexpected message: %#v", msg)
	}
}

func TestTargetedAction(t *testing.T) {
	a := startTestAdapter(t, nil)

	c1 := dialTestClient(t, a.testAddr(), nil)
	c1.identify(10001)
	waitPeer(t, a, "10001")
	recvInbound(t, a) // 排空 identify 产生的入站消息

	c2 := dialTestClient(t, a.testAddr(), nil)
	c2.identify(20002)
	waitPeer(t, a, "20002")
	recvInbound(t, a)

	if err := a.SendAction(&Action{
		Action: "send_private_msg",
		Params: map[string]any{"user_id": 20002},
		Target: "20002",
	}); err != nil {
		t.Fatalf("send action: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool { return c2.count() == 1 }, "c2 应收到指令")
	time.Sleep(100 * time.Millisecond)
	if c1.count() != 0 {
		t.Fatal("c1 不应收到定向指令")
	}

	var act Action
	if err := json.Unmarshal(c2.last(), &act); err != nil {
		t.Fatalf("unmarshal action: %v", err)
	}
	if act.Action != "send_private_msg" {
		t.Fatalf("unexpected action: %s", act.Action)
	}
}

func TestCallRoundTrip(t *testing.T) {
	a := startTestAdapter(t, nil)
	c := dialTestClient(t, a.testAddr(), nil)
	c.identify(10001)
	recvInbound(t, a)

	// 模拟 OneBot 实现端: 收到动作后回执
	c.onMessage = func(c *wsClient, data []byte) {
		var req struct {
			Action string `json:"action"`
			Echo   string `json:"echo"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Errorf("unmarshal action: %v", err)
			return
		}
		c.send(map[string]any{
			"status":  "ok",
			"retcode": 0,
			"data":    map[string]any{"message_id": 42},
			"echo":    req.Echo,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := a.Call(ctx, "", "send_group_msg", map[string]any{"group_id": 888})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.RetCode != 0 || result.Status != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
	var data struct {
		MessageID int `json:"message_id"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.MessageID != 42 {
		t.Fatalf("want message_id 42, got %d", data.MessageID)
	}
}

func TestCallApiFailure(t *testing.T) {
	a := startTestAdapter(t, nil)
	c := dialTestClient(t, a.testAddr(), nil)
	c.identify(10001)
	recvInbound(t, a)

	c.onMessage = func(c *wsClient, data []byte) {
		var req struct {
			Echo string `json:"echo"`
		}
		_ = json.Unmarshal(data, &req)
		c.send(map[string]any{"status": "failed", "retcode": 500, "message": "boom", "echo": req.Echo})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := a.Call(ctx, "", "send_group_msg", nil)
	if err == nil {
		t.Fatal("retcode=500 应返回错误")
	}
	if result == nil || result.RetCode != 500 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("错误信息应包含实现端 message: %v", err)
	}
}

func TestCallNoPeer(t *testing.T) {
	a := startTestAdapter(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := a.Call(ctx, "", "get_login_info", nil)
	// 本地路由失败以 retcode=-1 回执返回, 错误信息中包含具体原因
	if err == nil || !strings.Contains(err.Error(), ErrNoPeer.Error()) {
		t.Fatalf("want ErrNoPeer in message, got %v", err)
	}
	if result == nil || result.RetCode == 0 {
		t.Fatalf("应返回失败回执: %#v", result)
	}
}

func TestCallContextCancel(t *testing.T) {
	a := startTestAdapter(t, nil)
	c := dialTestClient(t, a.testAddr(), nil)
	c.identify(10001)
	recvInbound(t, a)

	// 实现端不回复
	c.onMessage = func(c *wsClient, data []byte) {}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := a.Call(ctx, "", "get_version", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

// ---------- 路由 ----------

func TestRoute(t *testing.T) {
	a := startTestAdapter(t, nil)

	if _, err := a.route(""); !errors.Is(err, ErrNoPeer) {
		t.Fatalf("want ErrNoPeer, got %v", err)
	}
	if _, err := a.route("20001"); !errors.Is(err, ErrUnknownPeer) {
		t.Fatalf("want ErrUnknownPeer, got %v", err)
	}

	a.peers["10001"] = &peer{session: "a"}
	a.peers["20001"] = &peer{session: "b"}

	if _, err := a.route(""); !errors.Is(err, ErrAmbiguousPeer) {
		t.Fatalf("want ErrAmbiguousPeer, got %v", err)
	}
	conn, err := a.route("20001")
	if err != nil || conn != a.peers["20001"].conn {
		t.Fatalf("定向路由失败: %v", err)
	}
}

// ---------- 工具 ----------

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
