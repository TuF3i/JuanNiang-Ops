package adapter

import "fmt"

func (r *Runtime) enableOnebotDriverV11() (bool, error) {
	// 获取数据库内当前配置
	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return false, fmt.Errorf("enable driver -> %v", err.Error())
	}

	// 反转状态
	cfg.Enable = true

	// 写回数据库
	err = r.ConfigManager.UpdateConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("enable driver -> %v", err.Error())
	}

	// 更新Driver的配置
	r.OnebotDriver.cfg = *cfg

	// TODO: 通知重建

	return true, nil
}
