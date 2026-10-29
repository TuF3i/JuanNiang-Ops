// Package feishu 飞书平台驱动: WebSocket 长连接 (官方 SDK) 接收事件 + OpenAPI 发送.
//
// 接收: larkws 客户端主动连接飞书开放平台 (无需公网回调地址), 事件推送到本地,
//       转换为统一事件投递 Events().
// 发送: lark.NewClient OpenAPI (tenant_access_token 由 SDK 管理), 纯文本→text /
//       纯图→image / 混合→post, ReplyPart 走 reply 接口.
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"JuanNiang-Ops/internal/adapter/modules"
)

// Driver 飞书平台驱动 (实现 driver.Driver)
type Driver struct {
	cfg modules.FeishuConfig
	// openBaseUrl 飞书 API 基址 (测试可覆盖, 默认 https://open.feishu.cn)
	openBaseUrl string
	// wsClient 长连接客户端 (测试可注入 mock)
	wsClient wsClient
	// httpClient OpenAPI HTTP 客户端 (测试可注入 httptest 拦截)
	httpClient larkcore.HttpClient
	// selfOpenID 机器人自身 open_id (MentionedSelf 判断, 可选注入)
	selfOpenID string

	events chan modules.Event

	// apiClient OpenAPI 客户端 (发送), Start 时创建
	apiClient *lark.Client
	// wsCancel 长连接 ctx 取消函数 (Start 创建, Stop 调用)
	wsCancel context.CancelFunc

	mu      sync.RWMutex
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// wsClient 长连接客户端最小接口 (便于测试注入 mock)
type wsClient interface {
	Start(ctx context.Context) error
}

// NewDriver 创建飞书驱动, 调用 Start 后事件管道才会建立
func NewDriver(cfg modules.FeishuConfig) *Driver {
	if cfg.EventBuffer <= 0 {
		cfg.EventBuffer = defaultEventBuffer
	}
	return &Driver{
		cfg:         cfg,
		openBaseUrl: defaultAPIBase,
	}
}

// SetOpenBaseUrl 覆盖飞书 API 基址 (测试用), Start 前调用
func (r *Driver) SetOpenBaseUrl(base string) { r.openBaseUrl = base }

// SetSelfOpenID 设置机器人自身 open_id (用于 MentionedSelf 判断), Start 前调用.
// 长连接模式下 SDK 不直接暴露机器人身份, 由装配方注入 (可选).
func (r *Driver) SetSelfOpenID(openID string) { r.selfOpenID = openID }

// SetHTTPClient 注入自定义 HTTP 客户端 (测试用, 拦截 OpenAPI 请求), Start 前调用
func (r *Driver) SetHTTPClient(c larkcore.HttpClient) { r.httpClient = c }

// SetWSClient 注入自定义长连接客户端 (测试用), Start 前调用
func (r *Driver) SetWSClient(c wsClient) { r.wsClient = c }

// Start 启动驱动: 建立飞书长连接并接收事件. 重复调用返回错误.
func (r *Driver) Start() error {
	if !r.cfg.Enable {
		return fmt.Errorf("feishu -> driver is disabled")
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("feishu -> driver is running")
	}

	r.events = make(chan modules.Event, r.cfg.EventBuffer)
	r.done = make(chan struct{})
	r.running = true

	// 长连接 ctx: Stop 时取消
	wsCtx, wsCancel := context.WithCancel(context.Background())
	r.wsCancel = wsCancel

	// OpenAPI 客户端 (发送): SDK 管理 token 获取与缓存
	apiOpts := []lark.ClientOptionFunc{lark.WithOpenBaseUrl(r.openBaseUrl)}
	if r.httpClient != nil {
		apiOpts = append(apiOpts, lark.WithHttpClient(r.httpClient))
	}
	r.apiClient = lark.NewClient(r.cfg.AppID, r.cfg.AppSecret, apiOpts...)
	r.mu.Unlock()

	// 事件分发器: 消息事件 → 统一消息; 其余事件 → 通知
	eventDispatcher := dispatcher.NewEventDispatcher(r.cfg.VerificationToken, "").
		OnP2MessageReceiveV1(r.onMessageReceive)

	// 长连接客户端 (接收): 测试注入 mock, 否则用 SDK 真实客户端
	client := r.wsClient
	if client == nil {
		wsOpts := []larkws.ClientOption{
			larkws.WithEventHandler(eventDispatcher),
		}
		client = larkws.NewClient(r.cfg.AppID, r.cfg.AppSecret, wsOpts...)
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// SDK Start 阻塞维持连接, ctx 取消或断连后返回; 返回则标记驱动停止
		defer r.markStopped()
		if err := client.Start(wsCtx); err != nil {
			log.Printf("[feishu] websocket client exited: %v", err)
		}
	}()

	log.Printf("[feishu] websocket driver started (app_id=%s)", r.cfg.AppID)
	return nil
}

// markStopped 长连接断开后收尾 (close done/events, 幂等; Stop 也调用)
func (r *Driver) markStopped() {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.done:
	default:
		close(r.done)
		close(r.events)
	}
}

// Stop 停止驱动: 取消长连接 ctx, 阻塞至完全退出.
// 退出后 Events 会被关闭, 消费方可通过 range 退出.
func (r *Driver) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = false
	cancel := r.wsCancel
	r.mu.Unlock()

	// 取消长连接 ctx → SDK Start 返回; 无论是否及时返回都先收尾
	if cancel != nil {
		cancel()
	}
	r.markStopped() // 幂等收尾: done/events 立即关闭, 不等 SDK 返回
	r.wg.Wait()     // 等 goroutine 退出 (泄漏的 SDK 调用随后自行结束)
	log.Printf("[feishu] driver stopped")
	return nil
}

// Done 返回停止信号通道: Stop 或长连接断开时被关闭
func (r *Driver) Done() <-chan struct{} { return r.done }

// Events 统一事件管道 (消息 EventMessage / 通知 EventNotify 合流), Start 后可用, Stop 时关闭
func (r *Driver) Events() <-chan modules.Event { return r.events }

// GetCurrentState 获取驱动当前状态快照
func (r *Driver) GetCurrentState() *modules.RuntimeState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return &modules.RuntimeState{IsRunning: r.running}
}

// ---------- 事件处理 (SDK 回调) ----------

// onMessageReceive 处理 im.message.receive_v1: 转换为统一消息事件
func (r *Driver) onMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		log.Printf("[feishu] discard nil message event")
		return nil
	}

	msg := event.Event.Message
	sender := event.Event.Sender

	// 应用机器人自己发的消息跳过 (防止自循环)
	if sender != nil && sender.SenderType != nil && *sender.SenderType == "app" {
		return nil
	}

	// 毫秒时间戳字符串 -> 秒
	var ts int64
	if msg.CreateTime != nil {
		if v, err := strconv.ParseInt(*msg.CreateTime, 10, 64); err == nil {
			ts = v / 1000
		}
	}

	// 提取身份字段
	chatID, chatType, messageType, messageID, content := "", "", "", "", ""
	if msg.ChatId != nil {
		chatID = *msg.ChatId
	}
	if msg.ChatType != nil {
		chatType = *msg.ChatType
	}
	if msg.MessageType != nil {
		messageType = *msg.MessageType
	}
	if msg.MessageId != nil {
		messageID = *msg.MessageId
	}
	if msg.Content != nil {
		content = *msg.Content
	}

	// @ 实体
	mentions := make([]Mention, 0, len(msg.Mentions))
	for _, m := range msg.Mentions {
		if m == nil {
			continue
		}
		openID := ""
		if m.Id != nil {
			openID = deref(m.Id.OpenId)
		}
		mentions = append(mentions, Mention{Key: deref(m.Key), ID: openID, Name: deref(m.Name)})
	}

	parts, mentionedSelf, err := convertContent(messageType, content, mentions, r.selfOpenID)
	if err != nil {
		log.Printf("[feishu] discard message %s: %v", messageID, err)
		return nil // 返回 nil 避免触发飞书重推
	}

	// 未支持的 message_type -> 通知事件 (业务层可拿原始数据自行解析)
	if parts == nil {
		r.pushNotify("feishu."+messageType, event.Event)
		return nil
	}

	msgType := modules.Group
	if chatType == "p2p" {
		msgType = modules.Private
	}

	var senderOpenID string
	if sender != nil && sender.SenderId != nil {
		senderOpenID = deref(sender.SenderId.OpenId)
	}

	unified := modules.Message{
		MessageID:   messageID,
		Platform:    modules.Feishu,
		MessageType: msgType,
		ChatID:      chatID,
		SelfID:      r.selfOpenID, // 注入的机器人身份 (未注入则留空)
		Sender: modules.Sender{
			UID:  senderOpenID,
			Name: senderOpenID,
		},
		Timestamp:     ts,
		MentionedSelf: mentionedSelf,
		Content:       parts,
		Original:      event.Event,
	}
	r.push(modules.Event{Type: modules.EventMessage, SelfID: unified.SelfID, Message: &unified})
	return nil
}

// pushNotify 投递通知事件 (未知 event_type / 未支持消息类型)
func (r *Driver) pushNotify(kind string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[feishu] marshal notify payload: %v", err)
		return
	}
	r.push(modules.Event{
		Type: modules.EventNotify,
		Notify: &modules.NotifyEvent{
			Kind:     kind,
			Platform: modules.Feishu,
			Data:     data,
		},
	})
}

// push 投递统一事件; 管道满时阻塞 (背压), 驱动停止后丢弃
func (r *Driver) push(ev modules.Event) {
	select {
	case <-r.done:
	case r.events <- ev:
	}
}

// deref 安全解引用字符串指针
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---------- 发送 (SDK OpenAPI) ----------

// SendGroup 发送群消息: chatID 为飞书 chat_id, selfID 忽略 (长连接单机器人).
// 等待 OpenAPI 回执, 填充统一 SendReceipt.
func (r *Driver) SendGroup(ctx context.Context, chatID string, parts []modules.MessagePart, selfID string) (*modules.SendReceipt, error) {
	return r.sendTo(ctx, chatID, larkim.CreateMessageV1ReceiveIDTypeChatId, parts)
}

// SendPrivate 发送私聊消息: chatID 为对方 open_id.
func (r *Driver) SendPrivate(ctx context.Context, chatID string, parts []modules.MessagePart, selfID string) (*modules.SendReceipt, error) {
	return r.sendTo(ctx, chatID, larkim.CreateMessageV1ReceiveIDTypeOpenId, parts)
}

// sendTo 构造载荷并经 SDK 发送; 带 ReplyPart 时走 reply 接口
func (r *Driver) sendTo(ctx context.Context, chatID, receiveIDType string, parts []modules.MessagePart) (*modules.SendReceipt, error) {
	r.mu.RLock()
	apiClient := r.apiClient
	r.mu.RUnlock()
	if apiClient == nil {
		return nil, fmt.Errorf("feishu -> driver is not running")
	}

	msgType, content, replyID, err := buildOutgoing(parts)
	if err != nil {
		return nil, err
	}

	// 回复走 reply 接口
	if replyID != "" {
		req := larkim.NewReplyMessageReqBuilder().
			MessageId(replyID).
			Body(larkim.NewReplyMessageReqBodyBuilder().
				MsgType(msgType).
				Content(content).
				Uuid(uuidOf(replyID, content)).
				Build()).
			Build()
		resp, err := apiClient.Im.V1.Message.Reply(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("feishu: reply api: %w", err)
		}
	if resp == nil || resp.Data == nil || resp.Data.MessageId == nil {
		return nil, fmt.Errorf("feishu: reply api empty message_id")
	}
	return &modules.SendReceipt{MessageID: *resp.Data.MessageId, Reply: resp}, nil
}

	// 常规发送
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(msgType).
			Content(content).
			Uuid(uuidOf(chatID, content)).
			Build()).
		Build()
	resp, err := apiClient.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("feishu: send api: %w", err)
	}
	if resp == nil || resp.Data == nil || resp.Data.MessageId == nil {
		return nil, fmt.Errorf("feishu: send api empty message_id")
	}
	return &modules.SendReceipt{MessageID: *resp.Data.MessageId, Reply: resp}, nil
}

// uuidOf 生成幂等 UUID (飞书 UUID 参数用于去重重试)
func uuidOf(a, b string) string {
	return fmt.Sprintf("%s-%x", a, time.Now().UnixNano())
}
