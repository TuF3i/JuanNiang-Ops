// Package service 适配器业务层: 持有运行时状态 (驱动实例/事件管道), 提供驱动生命周期管理、
// 配置读写与消息发送等统一 API. 数据访问下沉 dao, 平台协议细节在 driver.
package service

import (
	"sync"

	"JuanNiang-Ops/internal/adapter/dao"
	"JuanNiang-Ops/internal/adapter/modules"
)

// defaultEventChanSize Service 事件管道缓冲大小
const defaultEventChanSize = 256

// Service 适配器业务层: 装配后的运行时状态持有者.
// CreateRuntime 创建; 测试可经 NewService 直建 (不经数据库).
type Service struct {
	dao    *dao.Dao
	Onebot *OnebotDriverV11
	Feishu *FeishuDriverV11

	events chan modules.Event

	mu   sync.Mutex     // 保护 driver 字段与启停/重建串行化
	done chan struct{}  // 关闭信号: Release 时 close, 转发泵随之退出
	wg   sync.WaitGroup // 转发泵 goroutine
}

// NewService 创建业务层 (事件管道初始化; 驱动装配由 SetupDrivers 完成)
func NewService(d *dao.Dao) *Service {
	return &Service{
		dao:    d,
		events: make(chan modules.Event, defaultEventChanSize),
		done:   make(chan struct{}),
	}
}

// Events 业务层事件管道: 各驱动事件合流, Release 后关闭
func (r *Service) Events() <-chan modules.Event { return r.events }

// SetupDrivers 按数据库配置装配并启动各平台驱动 (CreateRuntime 调用).
// 单个驱动启动失败只记日志不中断装配, 保持未运行状态等配置变更重试.
func (r *Service) SetupDrivers() error {
	if err := r.setupOnebotDriver(); err != nil {
		return err
	}
	return r.setupFeishuDriver()
}
