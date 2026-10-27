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
			cfg, err := r.ConfigManager.GetConfig()
			if err != nil {
				return fmt.Errorf("enable driver -> %v", err.Error())
			}

			cfg.Enable = true

			err = r.ConfigManager.UpdateConfig(cfg)
			if err != nil {
				return fmt.Errorf("enable driver -> %v", err.Error())
			}

			r.OnebotDriver.cfg = *cfg

			// TODO: 通知重建

			return nil
		}
		// TODO: 飞书
	default:
		return fmt.Errorf("enable driver -> unknow driver type")
	}
}

func (r *Runtime) DisabledDriver(driverType DriverType) error {
	return nil
}

func (r *Runtime) GetAdapterStatus(driverType DriverType) (*common.RuntimeState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch driverType {
	case OnebotDriver:
		return r.OnebotDriver.driver.GetCurrentState(), nil
	default:
		return nil, fmt.Errorf("get adapter status -> unknow driver type")
	}
}

func (r *Runtime) GetAdapterConfig(driverType DriverType) (*config.OneBotConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get adapter config -> %v", err.Error())
	}

	return cfg, nil
}

func (r *Runtime) SendGroupMessage(target string, groupID int64, msg []common.MessagePart) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	driverType, err := r.route(target)
	if err != nil {
		return fmt.Errorf("send group message -> %v", err.Error())
	}

	switch driverType {
	case OnebotDriver:
		if !r.OnebotDriver.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send group message -> driver is not running")
		}

		select {
		case <-r.OnebotDriver.ctx.Done():
			return fmt.Errorf("send group message -> driver is shuting down")
		default:
			err := r.OnebotDriver.driver.SendGroupMsg(target, groupID, msg)
			if err != nil {
				return fmt.Errorf("send group message -> %v", err.Error())
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
		return fmt.Errorf("send group message -> %v", err.Error())
	}

	switch driverType {
	case OnebotDriver:
		if !r.OnebotDriver.driver.GetCurrentState().IsRunning {
			return fmt.Errorf("send group message -> driver is not running")
		}

		select {
		case <-r.OnebotDriver.ctx.Done():
			return fmt.Errorf("send group message -> driver is shuting down")
		default:
			err := r.OnebotDriver.driver.SendPrivateMsg(target, userID, msg)
			if err != nil {
				return fmt.Errorf("send group message -> %v", err.Error())
			}

			return nil
		}
		// TODO: 飞书
	default:
		return nil
	}
}
