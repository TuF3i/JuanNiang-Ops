// Package operator 统一操作层的组装层.
//
// 分层 (与 internal/adapter 同构):
//
//	Runtime (本文件, 只负责组装与生命周期) ──▶ service (资产实例生命周期/统一 API) ──▶ dao (gorm) + media/channel (集成实现)
//
// 资产 (Asset) 是 agent/workflow 操作的基本单位: 聚合多个操作媒介 (ssh/telnet/pve/k8s)
// 与多个数据通道 (prometheus/loki/tempo/outline). 配置持久化到 DB, 仅启用资产创建实例.
//
// 使用: CreateRuntime(WithDB(db)) 装配并实例化已启用资产 → 业务层经 rt.Service 操作资产
// → ReleaseRuntime() 释放. 媒介/通道协议细节见 media/ 与 channel/, 业务方法见 service/.
package operator

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"

	"JuanNiang-Ops/infrastructure/postgres"
	"JuanNiang-Ops/internal/operator/dao"
	"JuanNiang-Ops/internal/operator/service"
)

// Runtime operator 模块运行时: 只负责组装与创建/释放, 业务方法全部在 Service 上.
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

// CreateRuntime 装配 operator 模块:
// 建立数据库连接 (可注入) → Dao → 建表 → Service → 实例化已启用资产.
// 装配完成后业务层通过 rt.Service 以资产为单位执行操作.
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
			return nil, fmt.Errorf("operator -> connect postgres: %w", err)
		}
		ownDB = true
	}

	// 依赖注入链: Dao → Service
	d := dao.NewDao(db)
	if err := d.Setup(context.Background()); err != nil {
		return nil, err
	}

	rt := &Runtime{
		Service: service.NewService(d),
		db:      db,
		ownDB:   ownDB,
	}

	// 实例化已启用资产 (单个失败不中断, 等配置变更重试)
	if err := rt.Service.LoadEnabled(); err != nil {
		_ = rt.ReleaseRuntime()
		return nil, err
	}

	defaultRuntime = rt
	log.Printf("[operator] runtime created (own_db=%v)", ownDB)
	return rt, nil
}

// ReleaseRuntime 释放运行时: 关闭全部资产实例, 关闭自建的数据库连接 (幂等).
func (r *Runtime) ReleaseRuntime() error {
	if r == nil {
		return nil
	}
	if err := r.Service.Release(); err != nil {
		log.Printf("[operator] release service: %v", err)
	}

	if r.ownDB && r.db != nil {
		if sqlDB, err := r.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	if defaultRuntime == r {
		defaultRuntime = nil
	}
	log.Printf("[operator] runtime released")
	return nil
}

// ReleaseRuntime 释放包级默认运行时 (CreateRuntime 记录的那个), 幂等
func ReleaseRuntime() error {
	if defaultRuntime == nil {
		return nil
	}
	return defaultRuntime.ReleaseRuntime()
}
