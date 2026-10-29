// Package adapter 平台适配器模块的组装层.
//
// 分层 (与 internal/api 同构):
//
//	Runtime (本文件, 只负责组装与生命周期) ──▶ service (运行时状态/业务方法) ──▶ dao (gorm) + driver (平台实现)
//
// 使用: CreateRuntime(WithDB(db)) 装配并启动已启用的驱动 → 业务层经 rt.Service 消费事件与发送消息
// → ReleaseRuntime() 释放. 驱动协议细节见 driver/, 业务方法见 service/.
package adapter

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"

	"JuanNiang-Ops/infrastructure/postgres"
	"JuanNiang-Ops/internal/adapter/dao"
	"JuanNiang-Ops/internal/adapter/service"
)

// Runtime 适配器模块运行时: 只负责组装与创建/释放, 业务方法全部在 Service 上.
type Runtime struct {
	Service *service.Service

	db    *gorm.DB // 依赖的数据库连接 (自建时 Release 负责关闭)
	ownDB bool     // db 是否为 CreateRuntime 自建 (决定 Release 是否关闭)
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

// defaultRuntime 包级默认运行时 (CreateRuntime 记录, ReleaseRuntime 消费)
var defaultRuntime *Runtime

// CreateRuntime 装配适配器模块:
// 建立数据库连接 (可注入) → Dao → 建表/种子 → Service → 启动已启用的驱动并挂转发泵.
// 装配完成后业务层通过 rt.Service.Events() 消费事件, 经 rt.Service 发送消息.
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

	// 依赖注入链: Dao → Service
	d := dao.NewDao(db)
	if err := d.SetupConfig(context.Background()); err != nil {
		return nil, err
	}

	rt := &Runtime{
		Service: service.NewService(d),
		db:      db,
		ownDB:   ownDB,
	}

	// 按配置装配并启动各平台驱动 (单个失败不中断, 等配置变更重试)
	if err := rt.Service.SetupDrivers(); err != nil {
		_ = rt.ReleaseRuntime()
		return nil, err
	}

	defaultRuntime = rt
	log.Printf("[adapter] runtime created (own_db=%v)", ownDB)
	return rt, nil
}

// ReleaseRuntime 释放运行时: 停止所有驱动与转发泵, 关闭事件管道, 关闭自建的数据库连接 (幂等).
func (r *Runtime) ReleaseRuntime() error {
	if r == nil {
		return nil
	}
	if err := r.Service.Release(); err != nil {
		log.Printf("[adapter] release service: %v", err)
	}

	if r.ownDB && r.db != nil {
		if sqlDB, err := r.db.DB(); err == nil {
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
