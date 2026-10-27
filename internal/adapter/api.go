package adapter

import "fmt"

func (r *Runtime) StartOnebotDriver() error {
	// 获取数据库配置
	dbCfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("start onebot driver -> %v", err.Error())
	}

	// 反转数据库配置
	dbCfg.Enable = true

	// 写回配置
	err = r.ConfigManager.UpdateConfig(dbCfg)
	if err != nil {
		return fmt.Errorf("start onebot driver -> %v", err.Error())
	}

	// 获取锁
	r.mu.Lock()

	// 检查当前的状态
	if r.OnebotDriver.driver.GetCurrentState().IsRunning {
		r.mu.Unlock()
		return nil
	}

}
