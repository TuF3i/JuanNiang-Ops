// 配置变更与驱动启停/重建链路: 落库 -> 内存同步 -> 驱动 Start/Stop/重建.
package adapter

import (
	"fmt"
	"log"
)

// route 消息发送路由: target 为机器人自身标识 self_id.
//   - target 为空: 默认走飞书驱动 (单机器人长连接, 无路由歧义)
//   - target 非空: 精确匹配 OneBot 已接入的机器人 (peer), 未接入或断开则报错
func (r *Runtime) route(target string) (DriverType, error) {
	if target == "" {
		if r.FeishuDriver != nil && r.FeishuDriver.driver.GetCurrentState().IsRunning {
			return FeishuDriver, nil
		}
		return "", fmt.Errorf("route -> feishu driver is not running")
	}

	if r.OnebotDriver == nil {
		return "", fmt.Errorf("route -> onebot driver is not initialized")
	}
	st := r.OnebotDriver.driver.GetCurrentState()
	if _, ok := st.PeerList[target]; ok {
		return OnebotDriver, nil
	}
	return "", fmt.Errorf("route -> peer %s not found or disconnected", target)
}

// reCreateOnebotDriverV11 重建 OneBot 驱动 (配置变更后调用):
// Stop 旧实例 -> NewDriver(新配置) -> Start -> 重挂转发泵.
// 事件管道 rt.events 不变, 业务层无感知.
func (r *Runtime) reCreateOnebotDriverV11() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("recreate onebot driver v11 -> %w", err)
	}

	old := r.OnebotDriver
	if old != nil {
		old.cancel()
		if err := old.driver.Stop(); err != nil {
			log.Printf("[adapter] stop old onebot driver: %v", err)
		}
	}

	wrapper := &OnebotDriverV11{
		cfg:    *cfg,
		driver: newOnebotDriver(*cfg),
	}
	wrapper.ctx, wrapper.cancel = wrapperCtx()
	r.OnebotDriver = wrapper

	if !cfg.Enable {
		log.Printf("[adapter] onebot driver disabled (addr=%s)", cfg.Addr)
		return nil
	}

	if err := wrapper.driver.Start(); err != nil {
		// 启动失败 (端口占用等) 不回滚: 配置已生效, 保持未运行等下次变更重试
		return fmt.Errorf("recreate onebot driver v11 -> %w", err)
	}
	r.startForward("onebot", wrapper.driver)
	log.Printf("[adapter] onebot driver recreated (addr=%s)", cfg.Addr)
	return nil
}

// enableOnebotDriverV11 启用 OneBot: 落库 enable=true, 未运行则启动
func (r *Runtime) enableOnebotDriverV11() error {
	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %w", err)
	}

	cfg.Enable = true
	if err := r.ConfigManager.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.OnebotDriver
	if wrapper == nil {
		return fmt.Errorf("enable onebot driver v11 -> driver not initialized")
	}
	wrapper.cfg = *cfg
	if wrapper.driver.GetCurrentState().IsRunning {
		return nil
	}
	if err := wrapper.driver.Start(); err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %w", err)
	}
	r.startForward("onebot", wrapper.driver)
	log.Printf("[adapter] onebot driver enabled (addr=%s)", cfg.Addr)
	return nil
}

// disableOnebotDriverV11 停用 OneBot: 落库 enable=false, 运行中则停止
func (r *Runtime) disableOnebotDriverV11() error {
	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %w", err)
	}

	cfg.Enable = false
	if err := r.ConfigManager.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.OnebotDriver
	if wrapper == nil {
		return fmt.Errorf("disable onebot driver v11 -> driver not initialized")
	}
	wrapper.cfg = *cfg
	if err := wrapper.driver.Stop(); err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %w", err)
	}
	log.Printf("[adapter] onebot driver disabled")
	return nil
}

// ---------- 飞书 (与 OneBot 同构) ----------

// reCreateFeishuDriverV11 重建飞书驱动 (配置变更后调用)
func (r *Runtime) reCreateFeishuDriverV11() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, err := r.ConfigManager.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("recreate feishu driver v11 -> %w", err)
	}

	old := r.FeishuDriver
	if old != nil {
		old.cancel()
		if err := old.driver.Stop(); err != nil {
			log.Printf("[adapter] stop old feishu driver: %v", err)
		}
	}

	wrapper := &FeishuDriverV11{
		cfg:    *cfg,
		driver: newFeishuDriver(*cfg),
	}
	wrapper.ctx, wrapper.cancel = wrapperCtx()
	r.FeishuDriver = wrapper

	if !cfg.Enable {
		log.Printf("[adapter] feishu driver disabled (app_id=%s)", cfg.AppID)
		return nil
	}

	if err := wrapper.driver.Start(); err != nil {
		return fmt.Errorf("recreate feishu driver v11 -> %w", err)
	}
	r.startForward("feishu", wrapper.driver)
	log.Printf("[adapter] feishu driver recreated (app_id=%s)", cfg.AppID)
	return nil
}

// enableFeishuDriverV11 启用飞书: 落库 enable=true, 未运行则启动
func (r *Runtime) enableFeishuDriverV11() error {
	cfg, err := r.ConfigManager.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("enable feishu driver v11 -> %w", err)
	}

	cfg.Enable = true
	if err := r.ConfigManager.UpdateFeishuConfig(cfg); err != nil {
		return fmt.Errorf("enable feishu driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.FeishuDriver
	if wrapper == nil {
		return fmt.Errorf("enable feishu driver v11 -> driver not initialized")
	}
	wrapper.cfg = *cfg
	if wrapper.driver.GetCurrentState().IsRunning {
		return nil
	}
	if err := wrapper.driver.Start(); err != nil {
		return fmt.Errorf("enable feishu driver v11 -> %w", err)
	}
	r.startForward("feishu", wrapper.driver)
	log.Printf("[adapter] feishu driver enabled (app_id=%s)", cfg.AppID)
	return nil
}

// disableFeishuDriverV11 停用飞书: 落库 enable=false, 运行中则停止
func (r *Runtime) disableFeishuDriverV11() error {
	cfg, err := r.ConfigManager.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("disable feishu driver v11 -> %w", err)
	}

	cfg.Enable = false
	if err := r.ConfigManager.UpdateFeishuConfig(cfg); err != nil {
		return fmt.Errorf("disable feishu driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.FeishuDriver
	if wrapper == nil {
		return fmt.Errorf("disable feishu driver v11 -> driver not initialized")
	}
	wrapper.cfg = *cfg
	if err := wrapper.driver.Stop(); err != nil {
		return fmt.Errorf("disable feishu driver v11 -> %w", err)
	}
	log.Printf("[adapter] feishu driver disabled")
	return nil
}
