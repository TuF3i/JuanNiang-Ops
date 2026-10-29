// 驱动包装: 持有平台 Driver 实例与自身启停 ctx, 以及装配/转发泵逻辑.
package service

import (
	"context"
	"fmt"
	"log"

	"JuanNiang-Ops/internal/adapter/driver"
	"JuanNiang-Ops/internal/adapter/driver/feishu"
	"JuanNiang-Ops/internal/adapter/driver/onebot"
	"JuanNiang-Ops/internal/adapter/modules"
)

// OnebotDriverV11 OneBot 驱动包装: 持有驱动实例与自身启停 ctx
type OnebotDriverV11 struct {
	driver *onebot.Driver
	cfg    modules.OneBotConfig

	ctx    context.Context
	cancel context.CancelFunc
}

// FeishuDriverV11 飞书驱动包装: 持有驱动实例与自身启停 ctx
type FeishuDriverV11 struct {
	driver *feishu.Driver
	cfg    modules.FeishuConfig

	ctx    context.Context
	cancel context.CancelFunc
}

// wrapperCtx 创建驱动包装的启停 ctx
func wrapperCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// newOnebotDriver 创建 OneBot 驱动 (供重建链路使用)
func newOnebotDriver(cfg modules.OneBotConfig) *onebot.Driver {
	return onebot.NewDriver(cfg)
}

// newFeishuDriver 创建飞书驱动 (供重建链路使用)
func newFeishuDriver(cfg modules.FeishuConfig) *feishu.Driver {
	return feishu.NewDriver(cfg)
}

// newContext 发送用基础 ctx (驱动内部自带超时控制, 这里仅作占位)
func newContext() context.Context {
	return context.Background()
}

// setupOnebotDriver 按配置装配 OneBot 驱动 (启用则启动+挂转发泵)
func (r *Service) setupOnebotDriver() error {
	cfg, err := r.dao.GetOneBotConfig(context.Background())
	if err != nil {
		return fmt.Errorf("adapter -> load onebot config: %w", err)
	}

	wrapper := &OnebotDriverV11{
		cfg:    *cfg,
		driver: onebot.NewDriver(*cfg),
	}
	wrapper.ctx, wrapper.cancel = wrapperCtx()
	r.Onebot = wrapper

	if cfg.Enable {
		if err := wrapper.driver.Start(); err != nil {
			log.Printf("[adapter] start onebot driver: %v", err)
			return nil
		}
		r.startForward("onebot", wrapper.driver)
	}
	return nil
}

// setupFeishuDriver 按配置装配飞书驱动 (启用则启动+挂转发泵)
func (r *Service) setupFeishuDriver() error {
	cfg, err := r.dao.GetFeishuConfig(context.Background())
	if err != nil {
		return fmt.Errorf("adapter -> load feishu config: %w", err)
	}

	wrapper := &FeishuDriverV11{
		cfg:    *cfg,
		driver: feishu.NewDriver(*cfg),
	}
	wrapper.ctx, wrapper.cancel = wrapperCtx()
	r.Feishu = wrapper

	if cfg.Enable {
		if err := wrapper.driver.Start(); err != nil {
			log.Printf("[adapter] start feishu driver: %v", err)
			return nil
		}
		r.startForward("feishu", wrapper.driver)
	}
	return nil
}

// StartForward 为运行中的驱动挂转发泵: driver.Events() -> svc.events.
// 泵随驱动管道关闭 (Stop) 或 Service 关闭 (done) 退出.
func (r *Service) StartForward(name string, d driver.Driver) {
	r.startForward(name, d)
}

// Wait 等待所有转发泵退出 (测试辅助; 正常释放走 Release)
func (r *Service) Wait() { r.wg.Wait() }

// startForward 为运行中的驱动挂转发泵: driver.Events() -> svc.events.
// 泵随驱动管道关闭 (Stop) 或 Service 关闭 (done) 退出.
func (r *Service) startForward(name string, d driver.Driver) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		log.Printf("[adapter] forward pump started: %s", name)
		defer log.Printf("[adapter] forward pump stopped: %s", name)
		for {
			select {
			case <-r.done:
				return
			case ev, ok := <-d.Events():
				if !ok {
					return
				}
				select {
				case <-r.done:
					return
				case r.events <- ev:
				}
			}
		}
	}()
}
