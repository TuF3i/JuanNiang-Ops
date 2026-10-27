package adapter

import (
	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/config"
	"fmt"
)

func (r *Runtime) EnableDriver(driverType DriverType) (bool, error) {
	select {
	case <-r.ctx.Done():
		
	}
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

	if !r.OnebotDriver.driver.GetCurrentState().IsRunning {
		return fmt.Errorf("send group message -> driver is not running")
	}

	select {
	case <-r.ctx.Done():
		return fmt.Errorf("send group message -> driver is shuting down")
	default:
		err := r.OnebotDriver.driver.SendGroupMsg(target, groupID, msg)
		if err != nil {
			return fmt.Errorf("send group message -> %v", err.Error())
		}

		return nil
	}
}

func (r *Runtime) SendPrivateMessage(target string, groupID int64, msg []common.MessagePart) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.OnebotDriver.driver.GetCurrentState().IsRunning {
		return fmt.Errorf("send private message -> driver is not running")
	}

	select {
	case <-r.ctx.Done():
		return fmt.Errorf("send private message -> driver is shuting down")
	default:
		err := r.OnebotDriver.driver.SendPrivateMsg(target, groupID, msg)
		if err != nil {
			return fmt.Errorf("send private message -> %v", err.Error())
		}

		return nil
	}
}
