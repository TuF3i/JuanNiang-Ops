// 驱动生命周期管理: 启用/停用/重建 (配置落库 -> 内存同步 -> Start/Stop) 与 Service 释放.
package service

import (
	"context"
	"fmt"
	"log"
)

// Release 释放业务层: 停止所有驱动与转发泵, 关闭事件管道 (幂等).
// 由 runtime.ReleaseRuntime 调用; 事件管道关闭后业务层 range/recv 退出.
func (r *Service) Release() error {
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return nil
	default:
	}
	close(r.done)

	onebot := r.Onebot
	feishuD := r.Feishu
	r.mu.Unlock()

	// 依次: 取消驱动启停 ctx -> 停驱动 (管道关闭使转发泵退出) -> 等泵退出
	if onebot != nil {
		onebot.cancel()
		if err := onebot.driver.Stop(); err != nil {
			log.Printf("[adapter] stop onebot driver: %v", err)
		}
	}
	if feishuD != nil {
		feishuD.cancel()
		if err := feishuD.driver.Stop(); err != nil {
			log.Printf("[adapter] stop feishu driver: %v", err)
		}
	}
	r.wg.Wait()

	close(r.events)
	log.Printf("[adapter] service released")
	return nil
}

// EnableDriver 启用指定平台驱动 (运行时开关): 落库 enable=true 并启动
func (r *Service) EnableDriver(driverType DriverType) error {
	switch driverType {
	case OnebotDriver:
		select {
		case <-r.Onebot.ctx.Done():
			return fmt.Errorf("enable driver -> driver is shutting down")
		default:
			if err := r.enableOnebotDriverV11(); err != nil {
				return fmt.Errorf("enable driver -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		select {
		case <-r.Feishu.ctx.Done():
			return fmt.Errorf("enable driver -> driver is shutting down")
		default:
			if err := r.enableFeishuDriverV11(); err != nil {
				return fmt.Errorf("enable driver -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("enable driver -> unknown driver type")
	}
}

// DisabledDriver 停用指定平台驱动 (运行时开关): 落库 enable=false 并停止
func (r *Service) DisabledDriver(driverType DriverType) error {
	switch driverType {
	case OnebotDriver:
		select {
		case <-r.Onebot.ctx.Done():
			return fmt.Errorf("disable driver -> driver is shutting down")
		default:
			if err := r.disableOnebotDriverV11(); err != nil {
				return fmt.Errorf("disable driver -> %w", err)
			}
			return nil
		}
	case FeishuDriver:
		select {
		case <-r.Feishu.ctx.Done():
			return fmt.Errorf("disable driver -> driver is shutting down")
		default:
			if err := r.disableFeishuDriverV11(); err != nil {
				return fmt.Errorf("disable driver -> %w", err)
			}
			return nil
		}
	default:
		return fmt.Errorf("disable driver -> unknown driver type")
	}
}

// reCreateOnebotDriverV11 重建 OneBot 驱动 (配置变更后调用):
// Stop 旧实例 -> NewDriver(新配置) -> Start -> 重挂转发泵.
// 事件管道 svc.events 不变, 业务层无感知.
func (r *Service) reCreateOnebotDriverV11() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx := context.Background()
	cfg, err := r.dao.GetOneBotConfig(ctx)
	if err != nil {
		return fmt.Errorf("recreate onebot driver v11 -> %w", err)
	}

	old := r.Onebot
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
	r.Onebot = wrapper

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
func (r *Service) enableOnebotDriverV11() error {
	cfg, err := r.dao.GetOneBotConfig(context.Background())
	if err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %w", err)
	}

	cfg.Enable = true
	if err := r.dao.UpdateOneBotConfig(context.Background(), cfg); err != nil {
		return fmt.Errorf("enable onebot driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.Onebot
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
func (r *Service) disableOnebotDriverV11() error {
	cfg, err := r.dao.GetOneBotConfig(context.Background())
	if err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %w", err)
	}

	cfg.Enable = false
	if err := r.dao.UpdateOneBotConfig(context.Background(), cfg); err != nil {
		return fmt.Errorf("disable onebot driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.Onebot
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

// reCreateFeishuDriverV11 重建飞书驱动 (配置变更后调用)
func (r *Service) reCreateFeishuDriverV11() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cfg, err := r.dao.GetFeishuConfig(context.Background())
	if err != nil {
		return fmt.Errorf("recreate feishu driver v11 -> %w", err)
	}

	old := r.Feishu
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
	r.Feishu = wrapper

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
func (r *Service) enableFeishuDriverV11() error {
	cfg, err := r.dao.GetFeishuConfig(context.Background())
	if err != nil {
		return fmt.Errorf("enable feishu driver v11 -> %w", err)
	}

	cfg.Enable = true
	if err := r.dao.UpdateFeishuConfig(context.Background(), cfg); err != nil {
		return fmt.Errorf("enable feishu driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.Feishu
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
func (r *Service) disableFeishuDriverV11() error {
	cfg, err := r.dao.GetFeishuConfig(context.Background())
	if err != nil {
		return fmt.Errorf("disable feishu driver v11 -> %w", err)
	}

	cfg.Enable = false
	if err := r.dao.UpdateFeishuConfig(context.Background(), cfg); err != nil {
		return fmt.Errorf("disable feishu driver v11 -> %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	wrapper := r.Feishu
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
