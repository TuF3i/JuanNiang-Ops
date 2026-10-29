// 统一对外 API: 驱动开关 / 配置读写 / 状态查询 / 消息发送. 业务层 (agent/api 模块) 唯一入口.
package service

import (
	"context"
	"fmt"

	"JuanNiang-Ops/internal/adapter/modules"
)

// UpdateRuntimeConfig 更新运行时配置: MD5 对比无变化直接返回; 有变化落库并重建对应驱动.
func (r *Service) UpdateRuntimeConfig(cfg any) error {
	switch dst := (cfg).(type) {
	case *modules.OneBotConfig:
		src, err := r.dao.GetOneBotConfig(context.Background())
		if err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}

		ok, err := r.equal(src, dst)
		if err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		if ok {
			return nil
		}

		if err := r.dao.UpdateOneBotConfig(context.Background(), dst); err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		// 配置已变更: 驱动运行中则重建使其生效
		if r.Onebot != nil && r.Onebot.driver.GetCurrentState().IsRunning {
			if err := r.reCreateOnebotDriverV11(); err != nil {
				return fmt.Errorf("update runtime config -> %w", err)
			}
		}
		return nil
	case *modules.FeishuConfig:
		src, err := r.dao.GetFeishuConfig(context.Background())
		if err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}

		ok, err := r.equal(src, dst)
		if err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		if ok {
			return nil
		}

		if err := r.dao.UpdateFeishuConfig(context.Background(), dst); err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		if r.Feishu != nil && r.Feishu.driver.GetCurrentState().IsRunning {
			if err := r.reCreateFeishuDriverV11(); err != nil {
				return fmt.Errorf("update runtime config -> %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("update runtime config -> unsupported config type %T", cfg)
	}
}

// GetAdapterStatus 获取指定平台驱动运行状态
func (r *Service) GetAdapterStatus(driverType DriverType) (*modules.RuntimeState, error) {
	switch driverType {
	case OnebotDriver:
		return r.Onebot.driver.GetCurrentState(), nil
	case FeishuDriver:
		return r.Feishu.driver.GetCurrentState(), nil
	default:
		return nil, fmt.Errorf("get adapter status -> unknown driver type")
	}
}

// GetAdapterConfig 获取指定平台配置
func (r *Service) GetAdapterConfig(driverType DriverType) (any, error) {
	switch driverType {
	case OnebotDriver:
		return r.dao.GetOneBotConfig(context.Background())
	case FeishuDriver:
		return r.dao.GetFeishuConfig(context.Background())
	default:
		return nil, fmt.Errorf("get adapter config -> unknown driver type")
	}
}

// SendGroupMessage 发送群消息: target 为机器人自身标识 self_id ——
// 空则走飞书驱动; 非空则精确匹配 OneBot 已接入的机器人, 未接入报错.
// chatID 为群会话 ID (OneBot 群号 / 飞书 chat_id), 部件为平台无关统一消息.
func (r *Service) SendGroupMessage(target string, chatID string, msg []modules.MessagePart) error {
	r.mu.Lock()
	driverType, err := r.route(target)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send group message -> %w", err)
	}

	switch driverType {
	case OnebotDriver:
		if !r.Onebot.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send group message -> driver is not running")
		}
		select {
		case <-r.Onebot.ctx.Done():
			return fmt.Errorf("send group message -> driver is shutting down")
		default:
			if err := r.Onebot.driver.SendGroupMsg(target, chatID, msg); err != nil {
				return fmt.Errorf("send group message -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		if !r.Feishu.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send group message -> driver is not running")
		}
		select {
		case <-r.Feishu.ctx.Done():
			return fmt.Errorf("send group message -> driver is shutting down")
		default:
			if _, err := r.Feishu.driver.SendGroup(newContext(), chatID, msg, target); err != nil {
				return fmt.Errorf("send group message -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("send group message -> unknown driver type %q", driverType)
	}
}

// SendPrivateMessage 发送私聊消息: target 为机器人自身标识 self_id ——
// 空则走飞书驱动; 非空则精确匹配 OneBot 已接入的机器人, 未接入报错.
// chatID 为对方用户 ID (OneBot QQ 号 / 飞书 open_id).
func (r *Service) SendPrivateMessage(target string, chatID string, msg []modules.MessagePart) error {
	r.mu.Lock()
	driverType, err := r.route(target)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send private message -> %w", err)
	}

	switch driverType {
	case OnebotDriver:
		if !r.Onebot.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send private message -> driver is not running")
		}
		select {
		case <-r.Onebot.ctx.Done():
			return fmt.Errorf("send private message -> driver is shutting down")
		default:
			if err := r.Onebot.driver.SendPrivateMsg(target, chatID, msg); err != nil {
				return fmt.Errorf("send private message -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		if !r.Feishu.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send private message -> driver is not running")
		}
		select {
		case <-r.Feishu.ctx.Done():
			return fmt.Errorf("send private message -> driver is shutting down")
		default:
			if _, err := r.Feishu.driver.SendPrivate(newContext(), chatID, msg, target); err != nil {
				return fmt.Errorf("send private message -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("send private message -> unknown driver type %q", driverType)
	}
}

// ReplyMessage 回复一条收到的消息: 从 Message 提取会话与机器人身份, 按消息类型选择群/私聊发送.
func (r *Service) ReplyMessage(target string, origin modules.Message, msg []modules.MessagePart) error {
	switch origin.MessageType {
	case modules.Group:
		return r.SendGroupMessage(target, origin.ChatID, msg)
	case modules.Private:
		return r.SendPrivateMessage(target, origin.ChatID, msg)
	default:
		return fmt.Errorf("reply message -> unsupported message type %q", origin.MessageType)
	}
}
