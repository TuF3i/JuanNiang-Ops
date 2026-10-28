package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/lxzan/gws"

	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/config"
)

const testTimeout = 5 * time.Second

// ---------- 测试辅助 ----------

// testClient 端到端测试用 gws 客户端, 收到数据投递通道
type testClient struct {
	conn *gws.Conn
	raw  chan []byte
}

func (c *testClient) OnOpen(_ *gws.Conn) {}
func (c *testClient) OnClose(_ *gws.Conn, _ error) {}
func (c *testClient) OnMessage(_ *gws.Conn, message *gws.Message) {
	defer message.Close()
	data := make([]byte, len(message.Bytes()))
	copy(data, message.Bytes())
	select {
	case c.raw <- data:
	default:
	}
}
func (c *testClient) OnPing(_ *gws.Conn, _ []byte) {}
func (c *testClient) OnPong(_ *gws.Conn, _ []byte) {}

// newTestClient 连接驱动
func newTestClient(t *testing.T, addr string) *testClient {
	t.Helper()
	c := &testClient{raw: make(chan []byte, 32)}
	conn, _, err := gws.NewClient(c, &gws.ClientOption{Addr: "ws://" + addr})
	if err != nil {
		t.Fatalf("connect %s: %v", addr, err)
	}
	c.conn = conn
	go conn.ReadLoop()
	return c
}

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

// waitPeer 等待 self_id 绑定完成
func waitPeer(t *testing.T, d *Driver, selfID string) {
	t.Helper()
	waitFor(t, testTimeout, func() bool {
		st := d.GetCurrentState()
		_, ok := st.PeerList[selfID]
		return ok
	}, "timeout waiting for peer "+selfID)
}

// recvEvent 等待一条统一事件
func recvEvent(t *testing.T, d *Driver) common.Event {
	t.Helper()
	select {
	case ev := <-d.Events():
		return ev
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for event")
		return common.Event{}
	}
}

// freeAddr 获取空闲端口
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

// newTestDriver 创建已启动的驱动
func newTestDriver(t *testing.T, mutate func(*config.OneBotConfig)) *Driver {
	t.Helper()
	cfg := config.OneBotConfig{
		ID:     1,
		Addr:   freeAddr(t),
		Enable: true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	d := NewDriver(cfg)
	if err := d.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop() })
	return d
}

// ---------- 用例 ----------

func TestStartStopRestart(t *testing.T) {
	d := newTestDriver(t, nil)

	if err := d.Start(); err == nil || !strings.Contains(err.Error(), "driver is running") {
		t.Fatalf("second start: want running error, got %v", err)
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-d.Done():
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for Done")
	}

	// 重启后可再次服务
	d2 := newTestDriver(t, func(c *config.OneBotConfig) {
		c.Addr = d.GetCurrentConfig().Addr
	})
	c := newTestClient(t, d2.GetCurrentConfig().Addr)
	c.send(t, map[string]any{"time": 1, "self_id": 123, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, d2, "123")
}

func TestMessageEventToCommonEvent(t *testing.T) {
	d := newTestDriver(t, nil)

	c := newTestClient(t, d.GetCurrentConfig().Addr)
	c.send(t, MessageEvent{
		BaseEvent:   BaseEvent{Time: 1700000000, SelfID: 10000, PostType: "message"},
		MessageType: "group",
		MessageID:   42,
		GroupID:     888,
		UserID:      20001,
		Message: []Segment{
			Text("hello "),
			At(10000),
		},
		Sender: Sender{UserID: 20001, Nickname: "nick", Card: "card", Role: "admin"},
	})

	ev := recvEvent(t, d)
	if ev.Type != common.EventMessage {
		t.Fatalf("event type = %q", ev.Type)
	}
	if ev.SelfID != "10000" {
		t.Errorf("self_id = %s", ev.SelfID)
	}
	msg := ev.Message
	if msg == nil {
		t.Fatal("message is nil")
	}
	if msg.Platform != common.Onebot || msg.MessageType != common.Group {
		t.Errorf("platform/type = %v/%v", msg.Platform, msg.MessageType)
	}
	if msg.ChatID != "888" || msg.SelfID != "10000" || msg.MessageID != "42" {
		t.Errorf("chat/self/message_id = %s/%s/%s", msg.ChatID, msg.SelfID, msg.MessageID)
	}
	if !msg.MentionedSelf {
		t.Error("mentioned_self = false, want true")
	}
	if msg.Sender.UID != "20001" || msg.Sender.Name != "card" || !msg.Sender.IsAdmin {
		t.Errorf("sender = %+v", msg.Sender)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("content parts = %d, want 2", len(msg.Content))
	}
	if _, ok := msg.Content[1].(common.MentionPart); !ok {
		t.Errorf("part1 = %#v", msg.Content[1])
	}
}

func TestNoticeEventToCommonEvent(t *testing.T) {
	d := newTestDriver(t, nil)

	c := newTestClient(t, d.GetCurrentConfig().Addr)
	c.send(t, map[string]any{"time": 1, "self_id": 10000, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, d, "10000")

	// 戳一戳 -> EventNotify
	c.send(t, PokeEvent{
		BaseEvent:  BaseEvent{Time: 2, SelfID: 10000, PostType: "notice"},
		NoticeType: "notify",
		SubType:    "poke",
		GroupID:    888,
		UserID:     20001,
		TargetID:   10000,
	})
	ev := recvEvent(t, d)
	if ev.Type != common.EventNotify {
		t.Fatalf("event type = %q", ev.Type)
	}
	if ev.Notify == nil {
		t.Fatal("notify is nil")
	}
	if ev.Notify.Platform != common.Onebot || ev.Notify.Kind != NotifyEventType {
		t.Errorf("notify = %+v", ev.Notify)
	}
	var poke PokeEvent
	if err := json.Unmarshal(ev.Notify.Data, &poke); err != nil {
		t.Fatalf("unmarshal poke: %v", err)
	}
	if poke.SubType != "poke" || poke.TargetID != 10000 {
		t.Errorf("poke = %+v", poke)
	}
}

func TestCallRoundTripAndRouting(t *testing.T) {
	d := newTestDriver(t, nil)

	c := newTestClient(t, d.GetCurrentConfig().Addr)
	c.send(t, map[string]any{"time": 1, "self_id": 10000, "post_type": "meta_event", "meta_event_type": "heartbeat"})
	waitPeer(t, d, "10000")

	// SendPrivateMsg (chatID string) 下行验证
	if err := d.SendPrivateMsg("10000", "20001", []common.MessagePart{common.TextPart{Text: "direct"}}); err != nil {
		t.Fatalf("send private: %v", err)
	}
	if data := c.recvRaw(t); !strings.Contains(string(data), `"user_id":20001`) {
		t.Errorf("payload = %s", data)
	}

	// 非法 chatID 拒绝
	if err := d.SendGroupMsg("10000", "not-a-number", nil); err == nil ||
		!strings.Contains(err.Error(), "invalid group chat_id") {
		t.Fatalf("invalid chat_id: want error, got %v", err)
	}

	// Call 回执往返: 后台收指令并回执 (gws 收发并发安全)
	go func() {
		deadline := time.After(testTimeout)
		for {
			select {
			case <-deadline:
				return
			default:
			}
			select {
			case data := <-c.raw:
				var act Action
				if err := json.Unmarshal(data, &act); err == nil && act.Echo != "" {
					_ = c.conn.WriteMessage(gws.OpcodeText,
						[]byte(fmt.Sprintf(`{"status":"ok","retcode":0,"echo":%q}`, act.Echo)))
					return
				}
			case <-deadline:
				return
			}
		}
	}()
	if _, err := d.SendGroup(context.Background(), "888", []common.MessagePart{common.TextPart{Text: "x"}}, "10000"); err != nil {
		t.Fatalf("send group: %v", err)
	}
}

// TestEventsClosedAfterStop Stop 后事件管道关闭
func TestEventsClosedAfterStop(t *testing.T) {
	d := newTestDriver(t, nil)

	if err := d.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	select {
	case _, ok := <-d.Events():
		if ok {
			t.Error("events not closed")
		}
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting events close")
	}

	// 重复 Stop 幂等
	if err := d.Stop(); err != nil {
		t.Fatalf("double stop: %v", err)
	}

	// fmt/errors 引用守卫
	_ = fmt.Sprint
	_ = errors.New
}
