package adapter

import (
	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/config"
	"fmt"
)

// EnableDriver 启用指定平台驱动 (运行时开关): 落库 enable=true 并启动
func (r *Runtime) EnableDriver(driverType DriverType) error { // status, error
	switch driverType {
	case OnebotDriver:
		select {
		case <-r.OnebotDriver.ctx.Done():
			return fmt.Errorf("enable driver -> driver is shutting down")
		default:
			err := r.enableOnebotDriverV11()
			if err != nil {
				return fmt.Errorf("enable driver -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		select {
		case <-r.FeishuDriver.ctx.Done():
			return fmt.Errorf("enable driver -> driver is shutting down")
		default:
			err := r.enableFeishuDriverV11()
			if err != nil {
				return fmt.Errorf("enable driver -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("enable driver -> unknown driver type")
	}
}

// DisabledDriver 停用指定平台驱动 (运行时开关): 落库 enable=false 并停止
func (r *Runtime) DisabledDriver(driverType DriverType) error {
	switch driverType {
	case OnebotDriver:
		select {
		case <-r.OnebotDriver.ctx.Done():
			return fmt.Errorf("disable driver -> driver is shutting down")
		default:
			err := r.disableOnebotDriverV11()
			if err != nil {
				return fmt.Errorf("disable driver -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		select {
		case <-r.FeishuDriver.ctx.Done():
			return fmt.Errorf("disable driver -> driver is shutting down")
		default:
			err := r.disableFeishuDriverV11()
			if err != nil {
				return fmt.Errorf("disable driver -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("disable driver -> unknown driver type")
	}
}

// UpdateRuntimeConfig 更新运行时配置: MD5 对比无变化直接返回; 有变化落库并重建对应驱动.
func (r *Runtime) UpdateRuntimeConfig(cfg any) error {
	switch dst := (cfg).(type) {
	case *config.OneBotConfig:
		src, err := r.ConfigManager.GetConfig()
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

		if err := r.ConfigManager.UpdateConfig(dst); err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		// 配置已变更: 驱动运行中则重建使其生效
		if r.OnebotDriver != nil && r.OnebotDriver.driver.GetCurrentState().IsRunning {
			if err := r.reCreateOnebotDriverV11(); err != nil {
				return fmt.Errorf("update runtime config -> %w", err)
			}
		}
		return nil
	case *config.FeishuConfig:
		src, err := r.ConfigManager.GetFeishuConfig()
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

		if err := r.ConfigManager.UpdateFeishuConfig(dst); err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		if r.FeishuDriver != nil && r.FeishuDriver.driver.GetCurrentState().IsRunning {
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
func (r *Runtime) GetAdapterStatus(driverType DriverType) (*common.RuntimeState, error) {
	switch driverType {
	case OnebotDriver:
		return r.OnebotDriver.driver.GetCurrentState(), nil
	case FeishuDriver:
		return r.FeishuDriver.driver.GetCurrentState(), nil
	default:
		return nil, fmt.Errorf("get adapter status -> unknown driver type")
	}
}

// GetAdapterConfig 获取指定平台配置
func (r *Runtime) GetAdapterConfig(driverType DriverType) (any, error) {
	switch driverType {
	case OnebotDriver:
		return r.ConfigManager.GetConfig()
	case FeishuDriver:
		return r.ConfigManager.GetFeishuConfig()
	default:
		return nil, fmt.Errorf("get adapter config -> unknown driver type")
	}
}

// SendGroupMessage 发送群消息: target 为机器人自身标识 self_id ——
// 空则走飞书驱动; 非空则精确匹配 OneBot 已接入的机器人, 未接入报错.
// chatID 为群会话 ID (OneBot 群号 / 飞书 chat_id), 部件为平台无关统一消息.
func (r *Runtime) SendGroupMessage(target string, chatID string, msg []common.MessagePart) error {
	r.mu.Lock()
	driverType, err := r.route(target)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send group message -> %w", err)
	}

	switch driverType {
	case OnebotDriver:
		if !r.OnebotDriver.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send group message -> driver is not running")
		}
		select {
		case <-r.OnebotDriver.ctx.Done():
			return fmt.Errorf("send group message -> driver is shutting down")
		default:
			if err := r.OnebotDriver.driver.SendGroupMsg(target, chatID, msg); err != nil {
				return fmt.Errorf("send group message -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		if !r.FeishuDriver.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send group message -> driver is not running")
		}
		select {
		case <-r.FeishuDriver.ctx.Done():
			return fmt.Errorf("send group message -> driver is shutting down")
		default:
			if _, err := r.FeishuDriver.driver.SendGroup(newContext(), chatID, msg, target); err != nil {
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
func (r *Runtime) SendPrivateMessage(target string, chatID string, msg []common.MessagePart) error {
	r.mu.Lock()
	driverType, err := r.route(target)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send private message -> %w", err)
	}

	switch driverType {
	case OnebotDriver:
		if !r.OnebotDriver.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send private message -> driver is not running")
		}
		select {
		case <-r.OnebotDriver.ctx.Done():
			return fmt.Errorf("send private message -> driver is shutting down")
		default:
			if err := r.OnebotDriver.driver.SendPrivateMsg(target, chatID, msg); err != nil {
				return fmt.Errorf("send private message -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		if !r.FeishuDriver.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send private message -> driver is not running")
		}
		select {
		case <-r.FeishuDriver.ctx.Done():
			return fmt.Errorf("send private message -> driver is shutting down")
		default:
			if _, err := r.FeishuDriver.driver.SendPrivate(newContext(), chatID, msg, target); err != nil {
				return fmt.Errorf("send private message -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("send private message -> unknown driver type %q", driverType)
	}
}

// ReplyMessage 回复一条收到的消息: 从 Event 提取会话与机器人身份, 按消息类型选择群/私聊发送.
func (r *Runtime) ReplyMessage(target string, origin common.Message, msg []common.MessagePart) error {
	switch origin.MessageType {
	case common.Group:
		return r.SendGroupMessage(target, origin.ChatID, msg)
	case common.Private:
		return r.SendPrivateMessage(target, origin.ChatID, msg)
	default:
		return fmt.Errorf("reply message -> unsupported message type %q", origin.MessageType)
	}
}
