// Package adapter 定义平台适配器的统一约定与运行时管理.
//
// 每个平台适配器 (如 internal/adapter/onebot) 需要实现:
//   - Start / Stop: 生命周期管理
//   - Inbound:      接收消息管道 (平台事件 -> 统一消息 common.Message)
//   - Outbound:     发送指令管道 (平台指令 -> 平台实现端)
//   - Done:         停止信号, 配合管道一起消费
//
// AdapterRuntime 是适配器的生命周期管理器:
//   - 从数据库 (onebot_configs) 读取配置, 构建并启动适配器
//   - 通过 gorm 钩子 (见 hooks.go) 感知配置变更, 自动重建适配器 (Stop 旧实例 + New + Start)
//   - 管理事件泵 (见 handler.go), 将适配器事件分发给业务层注册的 handler
package adapter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"JuanNiang-Ops/internal/adapter/onebot"
)

// 默认配置值
const (
	defaultAddr          = ":8080"
	defaultMessageBuffer = 256
	defaultActionBuffer  = 64
)

// reconcileInterval 兜底轮询间隔: gorm 钩子丢失 (如批量更新) 时, 配置变更最迟在此间隔内生效
const reconcileInterval = 30 * time.Second

// AdapterRuntime 适配器生命周期管理器.
// 单条配置 (onebot_configs 首行) -> 单个 OneBot 适配器实例, 配置变更时自动重建.
type AdapterRuntime struct {
	mu      sync.Mutex      // 保护 adapter/cfg, 且串行化 reconcile
	db      *gorm.DB
	adapter *onebot.Adapter // 当前适配器实例, 未启动时为 nil
	cfg     OneBotConfig    // 当前生效配置快照

	handlers handlerSet // 事件 handler (见 handler.go)

	notify chan struct{} // gorm 钩子 -> reconcile 触发 (缓冲 1, 合并抖动)
	done   chan struct{} // runtime 关闭信号, close 时通知所有内部 goroutine
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	started bool
}

// NewAdapterRuntime 创建运行时管理器, db 为已连接的 gorm 实例.
// 使用流程: NewAdapterRuntime -> Setup -> Start -> (业务层 OnMessage/OnNotice 注册)
// -> Stop; Setup 与 Start 均幂等, Stop 后可重新 Start (内部信号通道会重建).
func NewAdapterRuntime(db *gorm.DB) *AdapterRuntime {
	rt := &AdapterRuntime{db: db}
	rt.resetSignals()
	return rt
}

// resetSignals 重建通知/关闭通道与根 ctx (构造时与 Stop->Start 恢复时调用)
func (rt *AdapterRuntime) resetSignals() {
	ctx, cancel := context.WithCancel(context.Background())
	rt.notify = make(chan struct{}, 1)
	rt.done = make(chan struct{})
	rt.ctx = ctx
	rt.cancel = cancel
}

// Setup 初始化: 建表 (AutoMigrate) 并保证存在一条配置 (空表则插入默认行).
func (rt *AdapterRuntime) Setup() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if err := rt.db.AutoMigrate(&OneBotConfig{}); err != nil {
		return fmt.Errorf("adapter: migrate onebot_configs: %w", err)
	}
	if _, err := rt.loadOrCreateConfig(); err != nil {
		return err
	}
	return nil
}

// Start 启动管理器: 注册配置观察者, 执行首次 reconcile, 开始监听配置变更.
func (rt *AdapterRuntime) Start() error {
	rt.mu.Lock()
	if rt.started {
		rt.mu.Unlock()
		return nil
	}
	// 曾 Stop 过则重建信号通道 (done/ctx 已关闭不可复用)
	select {
	case <-rt.done:
		rt.resetSignals()
	default:
	}
	setConfigObserver(rt.onConfigChanged)
	rt.started = true
	rt.mu.Unlock()

	rt.wg.Add(2)
	go rt.reconcileLoop()
	go rt.reconcileTicker()

	// 首次 reconcile 同步执行, Start 返回后适配器状态即确定
	if err := rt.reconcile(); err != nil {
		return err
	}
	log.Printf("[adapter] runtime started")
	return nil
}

// Stop 停止管理器: 关闭适配器与所有内部 goroutine, 幂等.
func (rt *AdapterRuntime) Stop() error {
	rt.mu.Lock()
	if !rt.started {
		rt.mu.Unlock()
		return nil
	}
	rt.started = false
	adapter := rt.adapter
	rt.adapter = nil
	close(rt.done)
	rt.mu.Unlock()

	// 先取消观察者注册, 避免 Stop 后的钩子触发空跑
	setConfigObserver(nil)

	// 依次: 停适配器 (inbound 关闭使 pump 退出) -> 取消根 ctx -> 等 goroutine 退出.
	// Wait 期间 reconcileLoop 可能正持有 mu, 因此这里不能持有 mu.
	if adapter != nil {
		if err := adapter.Stop(); err != nil {
			log.Printf("[adapter] stop onebot adapter: %v", err)
		}
	}
	rt.cancel()
	rt.wg.Wait()

	log.Printf("[adapter] runtime stopped")
	return nil
}

// Reload 手动触发一次 reconcile (一般无需调用, 钩子会自动触发)
func (rt *AdapterRuntime) Reload() {
	rt.onConfigChanged()
}

// ---------- 配置便捷 API (数据库联动入口) ----------

// SetEnabled 运行时开关: 修改数据库 enable 字段, reconcile 自动启停适配器.
func (rt *AdapterRuntime) SetEnabled(ctx context.Context, enable bool) error {
	return rt.UpdateConfig(ctx, func(cfg *OneBotConfig) {
		cfg.Enable = enable
	})
}

// UpdateConfig 更新配置: 先取当前行, 再经 mutate 修改后保存.
// 带主键 Save 保证 gorm AfterUpdate 钩子触发 -> reconcile 自动重建适配器.
func (rt *AdapterRuntime) UpdateConfig(ctx context.Context, mutate func(*OneBotConfig)) error {
	if mutate == nil {
		return errors.New("adapter: mutate is nil")
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	cfg, err := rt.loadOrCreateConfig()
	if err != nil {
		return err
	}
	mutate(&cfg)
	if err := rt.normalizeConfig(&cfg); err != nil {
		return err
	}
	// cfg 携带主键, Save 触发 AfterUpdate 钩子
	if err := rt.db.WithContext(ctx).Save(&cfg).Error; err != nil {
		return fmt.Errorf("adapter: save onebot config: %w", err)
	}
	return nil
}

// GetStatus 获取当前状态: 管理器运行态 / 当前配置 / 适配器状态 (未启动时 adapter 为 nil)
func (rt *AdapterRuntime) GetStatus() RuntimeStatus {
	rt.mu.Lock()
	cfg := rt.cfg
	adapter := rt.adapter
	rt.mu.Unlock()

	st := RuntimeStatus{
		Config: cfg,
	}
	if adapter != nil {
		state := adapter.GetCurrentState()
		st.AdapterRunning = state.IsRunning
		st.PeerCount = state.PeerCount
		st.PeerList = state.PeerList
	}
	return st
}

// RuntimeStatus 管理器状态快照
type RuntimeStatus struct {
	AdapterRunning bool                  // 适配器是否运行中
	Config         OneBotConfig          // 当前生效配置
	PeerCount      int                   // 已接入机器人端数量
	PeerList       map[string]onebot.PeerState // 客户端状态快照
}

// ---------- 事件 handler 注册 ----------

// OnMessage 注册统一消息 handler (可多次调用, 依次分发)
func (rt *AdapterRuntime) OnMessage(fn MessageHandler) {
	rt.handlers.addMsg(fn)
}

// OnNotice 注册通知事件 handler (notice / lifecycle, 可多次调用)
func (rt *AdapterRuntime) OnNotice(fn NoticeHandler) {
	rt.handlers.addNotice(fn)
}

// ---------- 内部: 配置变更触发 ----------

// onConfigChanged 钩子回调: 非阻塞投递通知, 缓冲 1 天然合并密集触发
func (rt *AdapterRuntime) onConfigChanged() {
	select {
	case rt.notify <- struct{}{}:
	default:
	}
}

// reconcileTicker 兜底轮询: 覆盖不带主键的批量更新等不触发钩子的场景
func (rt *AdapterRuntime) reconcileTicker() {
	defer rt.wg.Done()
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rt.done:
			return
		case <-ticker.C:
			rt.onConfigChanged()
		}
	}
}

// reconcileLoop 消费通知并执行 reconcile
func (rt *AdapterRuntime) reconcileLoop() {
	defer rt.wg.Done()
	for {
		select {
		case <-rt.done:
			return
		case <-rt.notify:
		}
		if err := rt.reconcile(); err != nil {
			log.Printf("[adapter] reconcile: %v", err)
		}
	}
}

// ---------- 内部: reconcile 核心逻辑 ----------

// reconcile 对齐数据库配置与适配器实例 (mu 内调用):
// 读配置 -> 与快照对比 -> 无变化 no-op; 有变化则 Stop 旧实例, 按需创建并启动新实例.
func (rt *AdapterRuntime) reconcile() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// runtime 已关闭则不再重建
	select {
	case <-rt.done:
		return nil
	default:
	}

	cfg, err := rt.loadOrCreateConfig()
	if err != nil {
		return err
	}

	// 配置未变化则跳过 (reconcile 幂等)
	if rt.adapter != nil && rt.cfgEqual(cfg) {
		return nil
	}

	// 配置变化: 停止旧实例
	if rt.adapter != nil {
		old := rt.adapter
		rt.adapter = nil
		log.Printf("[adapter] config changed (id=%d), rebuilding onebot adapter", cfg.ID)
		if err := old.Stop(); err != nil {
			log.Printf("[adapter] stop old adapter: %v", err)
		}
	}
	rt.cfg = cfg

	// 未启用则保持停止
	if !cfg.Enable {
		log.Printf("[adapter] onebot adapter disabled (id=%d)", cfg.ID)
		return nil
	}

	// 创建并启动新实例
	adapter := onebot.NewAdapter(onebot.Config{
		Addr:          cfg.Addr,
		Token:         cfg.Token,
		Admin:         cfg.Admin,
		Enable:        cfg.Enable,
		MessageBuffer: cfg.MessageBuffer,
		ActionBuffer:  cfg.ActionBuffer,
	})
	if err := adapter.Start(); err != nil {
		// 端口占用等启动失败不中断管理器, 等下次配置变更再试
		return fmt.Errorf("adapter: start onebot adapter (id=%d): %w", cfg.ID, err)
	}
	rt.adapter = adapter

	// 事件泵随 adapter 管道关闭自然退出; wg.Add 必须发生在 goroutine 启动前 (Stop 可能并发 Wait)
	rt.wg.Add(1)
	go rt.pump(adapter)

	log.Printf("[adapter] onebot adapter started (id=%d, addr=%s)", cfg.ID, cfg.Addr)
	return nil
}

// cfgEqual 对比数据库配置与当前快照 (mu 内调用)
func (rt *AdapterRuntime) cfgEqual(cfg OneBotConfig) bool {
	old := rt.cfg
	return old.Addr == cfg.Addr &&
		old.Token == cfg.Token &&
		old.Enable == cfg.Enable &&
		old.MessageBuffer == cfg.MessageBuffer &&
		old.ActionBuffer == cfg.ActionBuffer &&
		rt.stringsEqual(old.Admin, cfg.Admin)
}

// stringsEqual 比较字符串切片 (nil 与空切片视为相等)
func (rt *AdapterRuntime) stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// loadOrCreateConfig 读取首条配置, 空表则插入默认行 (mu 内调用).
// 删除配置行后 reconcile 会自愈为默认配置.
func (rt *AdapterRuntime) loadOrCreateConfig() (OneBotConfig, error) {
	var cfg OneBotConfig
	err := rt.db.Order("id ASC").First(&cfg).Error
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return OneBotConfig{}, fmt.Errorf("adapter: load onebot config: %w", err)
	}

	// 空表: 创建默认配置 (未启用, 待用户在管理界面开启)
	cfg = OneBotConfig{
		Addr:          defaultAddr,
		Token:         "",
		Admin:         []string{},
		Enable:        false,
		MessageBuffer: defaultMessageBuffer,
		ActionBuffer:  defaultActionBuffer,
	}
	if err := rt.db.Create(&cfg).Error; err != nil {
		return OneBotConfig{}, fmt.Errorf("adapter: create default onebot config: %w", err)
	}
	log.Printf("[adapter] created default onebot config (id=%d, addr=%s, disabled)", cfg.ID, cfg.Addr)
	return cfg, nil
}

// normalizeConfig 校正配置边界值 (保存前调用)
func (rt *AdapterRuntime) normalizeConfig(cfg *OneBotConfig) error {
	if cfg.Addr == "" {
		return errors.New("adapter: addr is required")
	}
	if cfg.MessageBuffer <= 0 {
		cfg.MessageBuffer = defaultMessageBuffer
	}
	if cfg.ActionBuffer <= 0 {
		cfg.ActionBuffer = defaultActionBuffer
	}
	if cfg.Admin == nil {
		cfg.Admin = []string{}
	}
	return nil
}
