package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lxzan/gws"

	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/pkg/uerr"
)

const testTimeout = 5 * time.Second

// testClient 端到端测试用的 gws 客户端封装, 收到数据后投递到通道
type testClient struct {
	conn    *gws.Conn
	raw     chan []byte
	closed  chan struct{}
	closedF bool
}

func (c *testClient) OnOpen(_ *gws.Conn)     {}
func (c *testClient) OnClose(_ *gws.Conn, _ error) {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}
func (c *testClient) OnMessage(_ *gws.Conn, message *gws.Message) {
	defer message.Close()
	data := make([]byte, len(message.Bytes()))
	copy(data, message.Bytes())
	select {
	case c.raw <- data:
	default: // 缓冲满则丢弃, 测试场景不会堆积
	}
}
func (c *testClient) OnPing(_ *gws.Conn, _ []byte)  {}
func (c *testClient) OnPong(_ *gws.Conn, _ []byte)  {}

// newTestClient 连接适配器 (可指定请求头)
func newTestClient(t *testing.T, addr string, header http.Header) *testClient {
	t.Helper()
	c := &testClient{
		raw:    make(chan []byte, 32),
		closed: make(chan struct{}),
	}
	conn, resp, err := gws.NewClient(c, &gws.ClientOption{
		Addr:          "ws://" + addr,
		RequestHeader: header,
	})
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("connect %s: %v (http status %d)", addr, err, status)
	}
	c.conn = conn
	go conn.ReadLoop()
	return c
}

// send 发送 JSON 数据
func (c *testClient) send(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := c.conn.WriteMessage(gws.OpcodeText, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// recvRaw 等待一条下行数据 (带超时)
func (c *testClient) recvRaw(t *testing.T) []byte {
	t.Helper()
	select {
	case data := <-c.raw:
		return data
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for downstream data")
		return nil
	}
}

// waitFor 轮询等待条件成立
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

// waitPeer 等待客户端完成 self_id 绑定
func waitPeer(t *testing.T, a *Adapter, selfID string) {
	t.Helper()
	waitFor(t, testTimeout, func() bool {
		return a.GetCurrentState().PeerList[selfID].SelfID == selfID
	}, "timeout waiting for peer "+selfID)
}

// recvInbound 等待一条统一消息
func recvInbound(t *testing.T, a *Adapter) common.Message {
	t.Helper()
	select {
	case msg := <-a.Inbound():
		return msg
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for inbound message")
		return common.Message{}
	}
}

// recvNotice 等待一条通知事件
func recvNotice(t *testing.T, a *Adapter) UnionEvent {
	t.Helper()
	select {
	case ev := <-a.Notices():
		return ev
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for notice")
		return UnionEvent{}
	}
}

// recvApiCall 收到动作指令并回执 (模拟 OneBot 实现端)
func recvApiCall(t *testing.T, c *testClient, retcode int) ApiResult {
	t.Helper()
	var act Action
	if err := json.Unmarshal(c.recvRaw(t), &act); err != nil {
		t.Fatalf("unmarshal action: %v", err)
	}
	result := ApiResult{
		Status:  "ok",
		RetCode: retcode,
		Echo:    act.Echo,
	}
	if retcode == 0 {
		result.Data = json.RawMessage(`{"message_id":1001}`)
	} else {
		result.Status = "failed"
		result.Message = "simulated api error"
	}
	c.send(t, result)
	return result
}

// freeAddr 获取一个空闲端口
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// newTestAdapter 创建已启动的适配器
func newTestAdapter(t *testing.T, mutate func(*Config)) *Adapter {
	t.Helper()
	cfg := Config{
		Addr:   freeAddr(t),
		Enable: true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	a := NewAdapter(cfg)
	if err := a.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	return a
}

// ---------- 生命周期 ----------

func TestStartStopRestart(t *testing.T) {
	a := newTestAdapter(t, nil)
	addr := a.GetCurrentConfig().Addr

	// 重复 Start 返回 ErrRunning
	if err := a.Start(); !errors.Is(err, uerr.ErrRunning) {
		t.Fatalf("second start: want ErrRunning, got %v", err)
	}

	if err := a.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-a.Done():
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for Done")
	}

	// 重启后可再次服务
	a2 := NewAdapter(a.GetCurrentConfig())
	if err := a2.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer a2.Stop()

	c := newTestClient(t, addr, nil)
	c.send(t, map[string]any{"time": 1, "self_id": 123, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a2, "123")
}

func TestStartDisabled(t *testing.T) {
	a := NewAdapter(Config{Addr: freeAddr(t), Enable: false})
	if err := a.Start(); !errors.Is(err, uerr.ErrDisabled) {
		t.Fatalf("start disabled: want ErrDisabled, got %v", err)
	}
}

func TestAuthRejectWrongToken(t *testing.T) {
	a := newTestAdapter(t, func(c *Config) { c.Token = "secret" })
	defer a.Stop()

	// 无 Token 客户端握手应被拒绝
	_, _, err := gws.NewClient(&testClient{raw: make(chan []byte, 1), closed: make(chan struct{})},
		&gws.ClientOption{Addr: "ws://" + a.GetCurrentConfig().Addr})
	if err == nil {
		t.Fatal("handshake without token: want error, got nil")
	}

	// 正确 Token 可接入
	h := http.Header{}
	h.Set("Authorization", "Bearer secret")
	c := newTestClient(t, a.GetCurrentConfig().Addr, h)
	c.send(t, map[string]any{"time": 1, "self_id": 777, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a, "777")
}

// ---------- 事件接收 ----------

func TestMessageEventToCommonMessage(t *testing.T) {
	a := newTestAdapter(t, nil)
	defer a.Stop()

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c.send(t, MessageEvent{
		BaseEvent:   BaseEvent{Time: 1700000000, SelfID: 10000, PostType: "message"},
		MessageType: "group",
		MessageID:   42,
		GroupID:     888,
		UserID:      20001,
		Message: []Segment{
			Text("hello "),
			At(10000),
			Image("img.jpg"),
		},
		RawMessage: "hello [CQ:at,qq=10000][CQ:image,file=img.jpg]",
		Sender:     Sender{UserID: 20001, Nickname: "nick", Card: "card", Role: "admin"},
	})

	msg := recvInbound(t, a)
	if msg.Platform != common.Onebot {
		t.Errorf("platform = %v", msg.Platform)
	}
	if msg.MessageType != common.Group {
		t.Errorf("message_type = %v", msg.MessageType)
	}
	if msg.ChatID != "888" || msg.SelfID != "10000" {
		t.Errorf("chat/self = %s/%s", msg.ChatID, msg.SelfID)
	}
	if msg.MessageID != "42" {
		t.Errorf("message_id = %s", msg.MessageID)
	}
	if !msg.MentionedSelf {
		t.Error("mentioned_self = false, want true")
	}
	if msg.Sender.UID != "20001" || msg.Sender.Name != "card" || !msg.Sender.IsAdmin {
		t.Errorf("sender = %+v", msg.Sender)
	}
	if msg.Timestamp != 1700000000 {
		t.Errorf("timestamp = %d", msg.Timestamp)
	}
	if len(msg.Content) != 3 {
		t.Fatalf("content parts = %d, want 3", len(msg.Content))
	}
	if p, ok := msg.Content[0].(common.TextPart); !ok || p.Text != "hello " {
		t.Errorf("part0 = %#v", msg.Content[0])
	}
	if _, ok := msg.Content[1].(common.MentionPart); !ok {
		t.Errorf("part1 = %#v", msg.Content[1])
	}
	if _, ok := msg.Content[2].(common.ImagePart); !ok {
		t.Errorf("part2 = %#v", msg.Content[2])
	}

	// Original 保留原始 JSON
	raw, ok := msg.Original.(json.RawMessage)
	if !ok || !strings.Contains(string(raw), `"group_id":888`) {
		t.Errorf("original = %v", msg.Original)
	}
}

func TestPrivateMessageEvent(t *testing.T) {
	a := newTestAdapter(t, nil)
	defer a.Stop()

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c.send(t, MessageEvent{
		BaseEvent:   BaseEvent{Time: 1, SelfID: 10000, PostType: "message"},
		MessageType: "private",
		MessageID:   7,
		UserID:      20002,
		Message:     []Segment{Text("hi")},
		Sender:      Sender{UserID: 20002, Nickname: "p"},
	})

	msg := recvInbound(t, a)
	if msg.MessageType != common.Private || msg.ChatID != "20002" {
		t.Errorf("type/chat = %v/%s", msg.MessageType, msg.ChatID)
	}
	if msg.MentionedSelf {
		t.Error("mentioned_self should be false")
	}
}

func TestNoticeAndLifecycleEvents(t *testing.T) {
	a := newTestAdapter(t, nil)
	defer a.Stop()

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)

	// 先心跳绑定身份
	c.send(t, map[string]any{"time": 1, "self_id": 10000, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a, "10000")

	// 戳一戳 -> 通知管道
	c.send(t, PokeEvent{
		BaseEvent:  BaseEvent{Time: 2, SelfID: 10000, PostType: "notice"},
		NoticeType: "notify",
		SubType:    "poke",
		GroupID:    888,
		UserID:     20001,
		TargetID:   10000,
	})
	ev := recvNotice(t, a)
	if ev.EventType != NotifyEventType {
		t.Errorf("event_type = %s", ev.EventType)
	}
	var poke PokeEvent
	if err := json.Unmarshal(ev.EventData, &poke); err != nil {
		t.Fatalf("unmarshal poke: %v", err)
	}
	if poke.SubType != "poke" || poke.TargetID != 10000 {
		t.Errorf("poke = %+v", poke)
	}

	// lifecycle -> 通知管道
	c.send(t, LifeCycleEvent{
		BaseEvent:     BaseEvent{Time: 3, SelfID: 10000, PostType: "meta_event"},
		MetaEventType: "lifecycle",
		SubType:       "connect",
	})
	ev = recvNotice(t, a)
	if ev.EventType != MetaEventType {
		t.Errorf("lifecycle event_type = %s", ev.EventType)
	}
}

// ---------- API 调用 ----------

func TestCallRoundTrip(t *testing.T) {
	a := newTestAdapter(t, nil)
	defer a.Stop()

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c.send(t, map[string]any{"time": 1, "self_id": 10000, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a, "10000")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// 成功
	go func() {
		_ = recvApiCall(t, c, 0)
	}()
	result, err := a.Call(ctx, "10000", "send_group_msg",
		map[string]any{"group_id": 888, "message": []Segment{Text("ok")}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.RetCode != 0 {
		t.Errorf("retcode = %d", result.RetCode)
	}

	// retcode != 0 返回错误
	go func() {
		_ = recvApiCall(t, c, 1200)
	}()
	_, err = a.Call(ctx, "10000", "send_group_msg", nil)
	if err == nil || !strings.Contains(err.Error(), "retcode=1200") {
		t.Fatalf("call failed api: want retcode=1200 error, got %v", err)
	}
}

func TestCallNotRunning(t *testing.T) {
	a := newTestAdapter(t, nil)
	_ = a.Stop()

	_, err := a.Call(context.Background(), "10000", "get_login_info", nil)
	if !errors.Is(err, uerr.ErrNotRunning) {
		t.Fatalf("call after stop: want ErrNotRunning, got %v", err)
	}
}

func TestSendActionAfterStop(t *testing.T) {
	a := newTestAdapter(t, nil)
	_ = a.Stop()

	err := a.Send("send_group_msg", nil, "")
	if !errors.Is(err, uerr.ErrNotRunning) {
		t.Fatalf("send after stop: want ErrNotRunning, got %v", err)
	}
}

// ---------- 多机器人路由 ----------

func TestTargetRouting(t *testing.T) {
	a := newTestAdapter(t, nil)
	defer a.Stop()

	// 单端: 不指定 Target 自动路由
	c1 := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c1.send(t, map[string]any{"time": 1, "self_id": 1, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a, "1")

	if err := a.SendGroupMsg("", 888, []Segment{Text("x")}); err != nil {
		t.Fatalf("auto route: %v", err)
	}
	if data := c1.recvRaw(t); !strings.Contains(string(data), `"group_id":888`) {
		t.Errorf("auto routed payload = %s", data)
	}

	// 双端: 未指定 Target 返回 ErrAmbiguousPeer
	c2 := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c2.send(t, map[string]any{"time": 2, "self_id": 2, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a, "2")

	// SendAction 只入队不回执 (fire-and-forget 路由失败仅记日志), 用 Call 走 pending 回执验证路由错误
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	_, err := a.Call(ctx, "", "send_group_msg", nil)
	if !errors.Is(err, uerr.ErrAmbiguousPeer) {
		t.Fatalf("ambiguous: want ErrAmbiguousPeer, got %v", err)
	}

	// 指定 Target 精确路由
	if err := a.SendPrivateMsg("2", 20001, []Segment{Text("direct")}); err != nil {
		t.Fatalf("target route: %v", err)
	}
	if data := c2.recvRaw(t); !strings.Contains(string(data), `"user_id":20001`) {
		t.Errorf("target routed payload = %s", data)
	}

	// 未知 Target
	_, err = a.Call(ctx, "999", "send_group_msg", nil)
	if !errors.Is(err, uerr.ErrUnknownPeer) {
		t.Fatalf("unknown peer: want ErrUnknownPeer, got %v", err)
	}
}

// ---------- 状态快照 ----------

func TestGetCurrentStateSnapshot(t *testing.T) {
	a := newTestAdapter(t, nil)
	defer a.Stop()

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c.send(t, map[string]any{"time": 1, "self_id": 555, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, a, "555")

	st := a.GetCurrentState()
	if !st.IsRunning || st.PeerCount != 1 {
		t.Fatalf("state = %+v", st)
	}
	p, ok := st.PeerList["555"]
	if !ok {
		t.Fatalf("peer 555 missing: %+v", st.PeerList)
	}
	if p.SelfID != "555" || p.Session == "" || p.RemoteAddr == "" {
		t.Errorf("peer state = %+v", p)
	}
}

// ---------- Stop 后 Inbound 关闭 ----------

func TestInboundClosedAfterStop(t *testing.T) {
	a := newTestAdapter(t, nil)

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	c.send(t, MessageEvent{
		BaseEvent:   BaseEvent{Time: 1, SelfID: 10000, PostType: "message"},
		MessageType: "private",
		MessageID:   1,
		UserID:      2,
		Message:     []Segment{Text("x")},
		Sender:      Sender{UserID: 2},
	})
	_ = recvInbound(t, a)

	if err := a.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Inbound 与 Notices 均应被关闭
	select {
	case _, ok := <-a.Inbound():
		if ok {
			t.Error("inbound not closed")
		}
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting inbound close")
	}
	select {
	case _, ok := <-a.Notices():
		if ok {
			t.Error("notices not closed")
		}
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting notices close")
	}

	// 重复 Stop 幂等
	if err := a.Stop(); err != nil {
		t.Fatalf("double stop: %v", err)
	}
}

// ---------- 消息事件缓冲背压 ----------

func TestInboundBuffer(t *testing.T) {
	a := newTestAdapter(t, func(c *Config) { c.MessageBuffer = 8 })
	defer a.Stop()

	c := newTestClient(t, a.GetCurrentConfig().Addr, nil)
	for i := 0; i < 8; i++ {
		c.send(t, MessageEvent{
			BaseEvent:   BaseEvent{Time: int64(i), SelfID: 10000, PostType: "message"},
			MessageType: "private",
			MessageID:   int32(i),
			UserID:      2,
			Message:     []Segment{Text(fmt.Sprint(i))},
			Sender:      Sender{UserID: 2},
		})
	}

	seen := make(map[string]bool)
	for i := 0; i < 8; i++ {
		msg := recvInbound(t, a)
		seen[msg.MessageID] = true
	}
	if len(seen) != 8 {
		t.Errorf("received %d distinct messages, want 8", len(seen))
	}
}
