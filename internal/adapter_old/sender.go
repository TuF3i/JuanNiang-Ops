// 统一发送 API: 上层业务通过 AdapterRuntime 发送消息, 无需接触平台实现.
//
// 分层:
//
//	业务层 -> AdapterRuntime (统一抽象, 本文件) -> 平台实现 (onebot.Adapter 等)
//
// 平台实现需实现 MessageSender 接口; AdapterRuntime 将调用委托给当前生效的
// 平台实例并透传统一回执 common.SendReceipt.
package adapter

import (
	"context"
	"errors"
	"fmt"

	"JuanNiang-Ops/internal/adapter_old/common"
	"JuanNiang-Ops/internal/adapter_old/onebot"
)

// MessageSender 平台发送能力接口, 各平台适配器 (onebot/...) 需实现.
// selfID 指定目标机器人 (如 OneBot self_id), 传空时:
//   - 仅一台机器人在线则自动路由
//   - 多台在线返回 ErrAmbiguousPeer
//
// 实现约定: 必须等待平台回执, 发送失败返回 error (可 errors.Is 判定哨兵错误),
// 成功返回统一回执 common.SendReceipt.
type MessageSender interface {
	// SendGroup 发送群消息, chatID 为群会话 ID (OneBot 群号)
	SendGroup(ctx context.Context, chatID string, parts []common.MessagePart, selfID string) (*common.SendReceipt, error)

	// SendPrivate 发送私聊消息, chatID 为对方用户 ID (OneBot QQ 号)
	SendPrivate(ctx context.Context, chatID string, parts []common.MessagePart, selfID string) (*common.SendReceipt, error)
}

// 发送相关哨兵错误 (统一抽象层)
var (
	ErrNoSender        = errors.New("adapter: no active adapter to send")  // 无可用平台实例
	ErrUnsupportedSend = errors.New("adapter: active adapter cannot send") // 平台实现未实现 MessageSender
)

// Adapterer 平台适配器统一视角 (生命周期 + 事件管道), 具体平台实现隐式匹配.
// onebot.Adapter 已满足; 后续平台 (如飞书) 同样实现此接口接入.
type Adapterer interface {
	// 生命周期
	Start() error
	Stop() error
	Done() <-chan struct{}

	// 事件管道
	Inbound() <-chan common.Message
	Notices() <-chan onebot.UnionEvent

	// 状态快照
	GetCurrentState() onebot.CurrentState
}

// senderOf 获取当前生效的 MessageSender (mu 内调用):
// 适配器运行中且实现 MessageSender 时返回, 否则返回对应哨兵错误.
func (rt *AdapterRuntime) senderOf() (MessageSender, error) {
	if rt.adapter == nil {
		return nil, ErrNoSender
	}
	sender, ok := rt.adapter.(MessageSender)
	if !ok {
		// 平台实现无发送能力 (未来可能的只读接入)
		return nil, ErrUnsupportedSend
	}
	return sender, nil
}

// SendGroup 发送群消息 (统一入口): chatID 为群会话 ID, selfID 指定目标机器人 (可空自动路由).
// 消息部件为平台无关的 common.MessagePart, 由平台实现转换为平台格式.
func (rt *AdapterRuntime) SendGroup(ctx context.Context, chatID string, parts []common.MessagePart, selfID string) (*common.SendReceipt, error) {
	rt.mu.Lock()
	sender, err := rt.senderOf()
	rt.mu.Unlock()
	if err != nil {
		return nil, err
	}
	receipt, err := sender.SendGroup(ctx, chatID, parts, selfID)
	if err != nil {
		return nil, fmt.Errorf("adapter: send group %s: %w", chatID, err)
	}
	return receipt, nil
}

// SendPrivate 发送私聊消息 (统一入口): chatID 为对方用户 ID, selfID 指定目标机器人 (可空自动路由).
func (rt *AdapterRuntime) SendPrivate(ctx context.Context, chatID string, parts []common.MessagePart, selfID string) (*common.SendReceipt, error) {
	rt.mu.Lock()
	sender, err := rt.senderOf()
	rt.mu.Unlock()
	if err != nil {
		return nil, err
	}
	receipt, err := sender.SendPrivate(ctx, chatID, parts, selfID)
	if err != nil {
		return nil, fmt.Errorf("adapter: send private %s: %w", chatID, err)
	}
	return receipt, nil
}

// Reply 回复一条收到的消息: 从 Message 提取会话与机器人身份, 按消息类型选择群/私聊发送.
func (rt *AdapterRuntime) Reply(ctx context.Context, origin common.Message, parts []common.MessagePart) (*common.SendReceipt, error) {
	switch origin.MessageType {
	case common.Group:
		return rt.SendGroup(ctx, origin.ChatID, parts, origin.SelfID)
	case common.Private:
		return rt.SendPrivate(ctx, origin.ChatID, parts, origin.SelfID)
	default:
		return nil, fmt.Errorf("adapter: unsupported message type %q", origin.MessageType)
	}
}
