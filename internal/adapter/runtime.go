package adapter

import (
	"context"
	"fmt"
	"log"
	"sync"

	"gorm.io/gorm"

	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/config"
	"JuanNiang-Ops/internal/adapter/driver"
	"JuanNiang-Ops/internal/adapter/driver/feishu"
	"JuanNiang-Ops/internal/adapter/driver/onebot"
	"JuanNiang-Ops/infrastructure/postgres"
)

type DriverType string

const (
	OnebotDriver DriverType = "driver.onebot"
	FeishuDriver DriverType = "driver.feishu"
)

// defaultEventChanSize Runtime 事件管道缓冲大小
const defaultEventChanSize = 256

// OnebotDriverV11 OneBot 驱动包装: 持有驱动实例与自身启停 ctx
type OnebotDriverV11 struct {
	driver *onebot.Driver
	cfg    config.OneBotConfig

	ctx    context.Context
	cancel context.CancelFunc
}

// FeishuDriverV11 飞书驱动包装: 持有驱动实例与自身启停 ctx
type FeishuDriverV11 struct {
	driver *feishu.Driver
	cfg    config.FeishuConfig

	ctx    context.Context
	cancel context.CancelFunc
}

// Runtime 适配器运行时: 装配配置管理与各平台驱动, 合流事件管道供业务层消费.
type Runtime struct {
	ConfigManager *config.ConfigManager
	OnebotDriver  *OnebotDriverV11
	FeishuDriver  *FeishuDriverV11

	db     *gorm.DB      // 依赖的数据库连接 (自建时 Release 负责关闭)
	ownDB  bool          // db 是否为 CreateRuntime 自建 (决定 Release 是否关闭)
	events chan common.Event

	mu  sync.Mutex // 保护 driver 字段与启停/reconcile 串行化
	done chan struct{}
	wg   sync.WaitGroup
}

// Option CreateRuntime 选项
type Option func(*options)

type options struct {
	db *gorm.DB
}

// WithDB 注入已有数据库连接 (不注入则使用 infrastructure 默认连接并自持)
func WithDB(db *gorm.DB) Option {
	return func(o *options) { o.db = db }
}

// ---------- 生命周期 ----------

// defaultRuntime 包级默认运行时 (CreateRuntime 记录, ReleaseRuntime 消费)
var defaultRuntime *Runtime

// CreateRuntime 装配适配器运行时:
// 建立数据库连接 (可注入) → ConfigManager 幂等初始化 → 启动各平台已启用的驱动 → 合流事件管道.
// 装配完成后业务层通过 Events() 消费事件, 通过 api.go 的发送接口发送消息.
func CreateRuntime(opts ...Option) (*Runtime, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	// 数据库连接: 注入优先, 否则自建 (自建的连接由 ReleaseRuntime 负责)
	db := o.db
	ownDB := false
	if db == nil {
		var err error
		db, err = postgres.NewPostgresClient()
		if err != nil {
			return nil, fmt.Errorf("adapter -> connect postgres: %w", err)
		}
		ownDB = true
	}

	rt := &Runtime{
		events: make(chan common.Event, defaultEventChanSize),
		done:   make(chan struct{}),
		db:     db,
		ownDB:  ownDB,
	}

	// 配置表幂等初始化
	cm := config.NewConfigManager(db)
	if err := cm.SetupConfig(); err != nil {
		return nil, fmt.Errorf("adapter -> setup config: %w", err)
	}
	rt.ConfigManager = cm

	// OneBot 驱动: 配置启用则启动
	if err := rt.setupOnebotDriver(); err != nil {
		return nil, err
	}

	// 飞书驱动: 配置启用则启动
	if err := rt.setupFeishuDriver(); err != nil {
		return nil, err
	}

	defaultRuntime = rt
	log.Printf("[adapter] runtime created (own_db=%v)", ownDB)
	return rt, nil
}

// setupOnebotDriver 按配置装配 OneBot 驱动 (启用则启动+挂转发泵)
func (r *Runtime) setupOnebotDriver() error {
	cfg, err := r.ConfigManager.GetConfig()
	if err != nil {
		return fmt.Errorf("adapter -> load onebot config: %w", err)
	}

	wrapper := &OnebotDriverV11{
		cfg:    *cfg,
		driver: onebot.NewDriver(*cfg),
	}
	wrapper.ctx, wrapper.cancel = context.WithCancel(context.Background())
	r.OnebotDriver = wrapper

	if cfg.Enable {
		if err := wrapper.driver.Start(); err != nil {
			// 启动失败不中断装配 (端口占用等), 保持未运行状态等配置变更重试
			log.Printf("[adapter] start onebot driver: %v", err)
			return nil
		}
		r.startForward("onebot", wrapper.driver)
	}
	return nil
}

// setupFeishuDriver 按配置装配飞书驱动 (启用则启动+挂转发泵)
func (r *Runtime) setupFeishuDriver() error {
	cfg, err := r.ConfigManager.GetFeishuConfig()
	if err != nil {
		return fmt.Errorf("adapter -> load feishu config: %w", err)
	}

	wrapper := &FeishuDriverV11{
		cfg:    *cfg,
		driver: feishu.NewDriver(*cfg),
	}
	wrapper.ctx, wrapper.cancel = context.WithCancel(context.Background())
	r.FeishuDriver = wrapper

	if cfg.Enable {
		if err := wrapper.driver.Start(); err != nil {
			log.Printf("[adapter] start feishu driver: %v", err)
			return nil
		}
		r.startForward("feishu", wrapper.driver)
	}
	return nil
}

// startForward 为运行中的驱动挂转发泵: driver.Events() -> rt.events.
// 泵随驱动管道关闭 (Stop) 或 Runtime 关闭 (done) 退出.
func (r *Runtime) startForward(name string, d driver.Driver) {
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

// ReleaseRuntime 释放运行时: 停止所有驱动与转发泵, 关闭事件管道 (幂等).
// 事件管道关闭后业务层 range/recv 退出, 发送接口返回错误.
func (r *Runtime) ReleaseRuntime() error {
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return nil
	default:
	}
	close(r.done)

	onebot := r.OnebotDriver
	feishuD := r.FeishuDriver
	db := r.db
	ownDB := r.ownDB
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

	// 关闭事件管道通知业务层, 最后回收数据库连接
	close(r.events)
	if ownDB && db != nil {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	if defaultRuntime == r {
		defaultRuntime = nil
	}
	log.Printf("[adapter] runtime released")
	return nil
}

// ReleaseRuntime 释放包级默认运行时 (CreateRuntime 记录的那个), 幂等
func ReleaseRuntime() error {
	if defaultRuntime == nil {
		return nil
	}
	return defaultRuntime.ReleaseRuntime()
}

// Events 业务层事件管道: 各驱动事件合流, ReleaseRuntime 后关闭
func (r *Runtime) Events() <-chan common.Event { return r.events }

// wrapperCtx 创建驱动包装的启停 ctx
func wrapperCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// newOnebotDriver 创建 OneBot 驱动 (供重建链路使用)
func newOnebotDriver(cfg config.OneBotConfig) *onebot.Driver {
	return onebot.NewDriver(cfg)
}

// newContext 发送用基础 ctx (驱动内部自带超时控制, 这里仅作占位)
func newContext() context.Context {
	return context.Background()
}

// newFeishuDriver 创建飞书驱动 (供重建链路使用)
func newFeishuDriver(cfg config.FeishuConfig) *feishu.Driver {
	return feishu.NewDriver(cfg)
}
