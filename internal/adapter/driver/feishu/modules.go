package feishu

import "encoding/json"

// 飞书事件订阅回调 (明文模式) 相关结构体
// 文档: https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscriptions

const (
	// defaultAPIBase 飞书开放平台 API 基址 (测试可覆盖)
	defaultAPIBase = "https://open.feishu.cn"

	// defaultEventBuffer 事件管道默认缓冲大小
	defaultEventBuffer = 64

	// URLVerificationType 订阅验证请求类型
	URLVerificationType = "url_verification"

	// V2EventType v2 事件回调类型
	V2EventType = "event_callback_v2"
)

// ---------- 接收: 回调请求体 ----------

// CallbackRequest 事件回调请求体 (兼容 url_verification 与 v2 事件两种形态)
type CallbackRequest struct {
	// url_verification 字段
	Type      string `json:"type"`      // url_verification / event_callback_v2
	Challenge string `json:"challenge"` // 校验串, 需原样返回
	Token     string `json:"token"`     // verification_token (v1 兼容字段)

	// v2 事件字段
	Header *EventHeader    `json:"header"`
	Event  json.RawMessage `json:"event"`
}

// EventHeader v2 事件头
type EventHeader struct {
	EventID      string `json:"event_id"`      // 事件唯一 ID (去重防重推)
	EventType    string `json:"event_type"`    // 如 im.message.receive_v1
	Token        string `json:"token"`         // 验证 Token
	CreateTime   string `json:"create_time"`   // 毫秒时间戳
	TenantKey    string `json:"tenant_key"`
	AppID        string `json:"app_id"`
}

// MessageReceivedEvent im.message.receive_v1 事件载荷
type MessageReceivedEvent struct {
	Sender struct {
		SenderID struct {
			UnionID string `json:"union_id"`
			UserID  string `json:"user_id"`
			OpenID  string `json:"open_id"`
		} `json:"sender_id"`
		SenderType string `json:"sender_type"` // user / app
		TenantKey  string `json:"tenant_key"`
	} `json:"sender"`
	Message struct {
		MessageID   string   `json:"message_id"`
		RootID      string   `json:"root_id"`
		ParentID    string   `json:"parent_id"`
		CreateTime  string   `json:"create_time"`  // 毫秒时间戳字符串
		ChatID      string   `json:"chat_id"`
		ChatType    string   `json:"chat_type"`    // p2p / group
		MessageType string   `json:"message_type"` // text / post / image / file ...
		Content     string   `json:"content"`      // JSON 字符串 (双重编码)
		Mentions    []Mention `json:"mentions"`
	} `json:"message"`
}

// Mention 消息中的 @ 实体
type Mention struct {
	Key        string `json:"key"`        // 如 @_user_1, 与 content 中占位符对应
	ID         string `json:"id"`         // open_id
	IDType     string `json:"id_type"`
	Name       string `json:"name"`
	TenantKey  string `json:"tenant_key"`
}

// ---------- 发送: OpenAPI ----------

// TenantTokenResponse tenant_access_token 响应
type TenantTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int64  `json:"expire"` // 有效期秒数 (通常 2h)
}

// SendMessageRequest 发送消息请求体
type SendMessageRequest struct {
	ReceiveID string `json:"receive_id"` // 接收者 ID (chat_id / open_id, 由查询参数决定)
	MsgType   string `json:"msg_type"`   // text / post / image / ...
	Content   string `json:"content"`    // JSON 字符串 (双重编码)
	UUID      string `json:"uuid,omitempty"`
}

// ReplyMessageRequest 回复消息请求体
type ReplyMessageRequest struct {
	Content string `json:"content"`
	MsgType string `json:"msg_type"`
	UUID    string `json:"uuid,omitempty"`
}

// ApiResponse 飞书 OpenAPI 通用响应外壳
type ApiResponse struct {
	Code int             `json:"code"` // 0 成功
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// SendMessageData 发送消息响应 data 字段
type SendMessageData struct {
	MessageID       string `json:"message_id"`
	RootID          string `json:"root_id"`
	ParentID        string `json:"parent_id"`
	ThreadID        string `json:"thread_id"`
	ChatID          string `json:"chat_id"`
	MessageType     string `json:"message_type"`
	CreateTime      int64  `json:"create_time"`
}
