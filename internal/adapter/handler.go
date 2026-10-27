package adapter

import (
	"crypto/md5"
	"fmt"

	"github.com/bytedance/sonic"
)

func (r *Runtime) route(target string) (DriverType, error) {
	_, ok := r.OnebotDriver.driver.GetCurrentState().PeerList[target]
	if ok {
		return OnebotDriver, nil
	}

	// TODO: 飞书

	return "", fmt.Errorf("route -> peer not found or disconnected")
}

func (r *Runtime) equal[T configKind](src *T, dst *T) (bool, error) {
	srcRaw, err := sonic.Marshal(src)
	if err != nil {
		return false, fmt.Errorf("equal -> %v", err.Error())
	}

	dstRaw, err := sonic.Marshal(dst)
	if err != nil {
		return false, fmt.Errorf("equal -> %v", err.Error())
	}

	if md5.Sum(srcRaw) == md5.Sum(dstRaw) {
		return true, nil
	}

	return false, nil
}

func (r *Runtime) enableOnebotDriverV11() error {
	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %v", err.Error())
	}

	cfg.Enable = true

	err = r.ConfigManager.UpdateConfig(cfg)
	if err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %v", err.Error())
	}

	r.OnebotDriver.cfg = *cfg

	// TODO: 通知重建

	return nil
}

func (r *Runtime) disableOnebotDriverV11() error {
	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %v", err.Error())
	}

	cfg.Enable = false

	err = r.ConfigManager.UpdateConfig(cfg)
	if err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %v", err.Error())
	}

	r.OnebotDriver.cfg = *cfg

	// TODO: 通知重建

	return nil
}

func (r *Runtime) reCreateOnebotDriverV11() error {
	// TODO: 重载（先stop然后重建）
	return nil
}
