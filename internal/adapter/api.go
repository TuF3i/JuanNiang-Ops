package adapter

import (
	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/config"
	"fmt"
)

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
		// TODO: 飞书
	default:
		return fmt.Errorf("enable driver -> unknown driver type")
	}
}

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
		// TODO: 飞书
	default:
		return fmt.Errorf("disable driver -> unknown driver type")
	}
}

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

		err = r.ConfigManager.UpdateConfig(dst)
		if err != nil {
			return fmt.Errorf("update runtime config -> %w", err)
		}
		return nil
		// TODO: 飞书
	default:
		return nil
	}
}

func (r *Runtime) GetAdapterStatus(driverType DriverType) (*common.RuntimeState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch driverType {
	case OnebotDriver:
		return r.OnebotDriver.driver.GetCurrentState(), nil
	default:
		return nil, fmt.Errorf("get adapter status -> unknown driver type")
	}
}

func (r *Runtime) GetAdapterConfig(driverType DriverType) (*config.OneBotConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get adapter config -> %w", err)
	}

	return cfg, nil
}

func (r *Runtime) SendGroupMessage(target string, groupID int64, msg []common.MessagePart) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	driverType, err := r.route(target)
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
			err := r.OnebotDriver.driver.SendGroupMsg(target, groupID, msg)
			if err != nil {
				return fmt.Errorf("send group message -> %w", err)
			}

			return nil
		}
		// TODO: 飞书
	default:
		return nil
	}
}

func (r *Runtime) SendPrivateMessage(target string, userID int64, msg []common.MessagePart) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	driverType, err := r.route(target)
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
			err := r.OnebotDriver.driver.SendPrivateMsg(target, userID, msg)
			if err != nil {
				return fmt.Errorf("send private message -> %w", err)
			}

			return nil
		}
		// TODO: 飞书
	default:
		return nil
	}
}
