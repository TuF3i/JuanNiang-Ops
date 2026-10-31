// Package api HTTP API 模块: 用户管理 (单 admin 用户) 与后续业务接口的接入层.
//
// 分层与依赖注入 (自上而下单向依赖):
//
//	Runtime (装配, Option 模式)
//	  └─ handler (gin 路由/中间件) ──▶ Service (业务+bcrypt+Token) ──▶ Dao (gorm)
//
// 使用: CreateRuntime(WithDB(db)) 装配并启动 HTTP 服务 → ReleaseRuntime() 释放.
package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"JuanNiang-Ops/infrastructure/postgres"
	infrastructureRedis "JuanNiang-Ops/infrastructure/redis"
	"JuanNiang-Ops/internal/api/cache"
	"JuanNiang-Ops/internal/api/dao"
	"JuanNiang-Ops/internal/api/handler"
	"JuanNiang-Ops/internal/api/response"
	"JuanNiang-Ops/internal/api/service"
	"JuanNiang-Ops/internal/operator"
)

// 默认配置
const (
	defaultAddr           = ":8082"
	defaultShutdownPeriod = 5 * time.Second
)

// Option CreateRuntime 选项
type Option func(*options)

type options struct {
	db       *gorm.DB
	rdb      *redis.Client
	addr     string
	operator *operator.Service
}

// WithDB 注入已有数据库连接 (不注入则使用 infrastructure 默认连接并自持)
func WithDB(db *gorm.DB) Option {
	return func(o *options) { o.db = db }
}

// WithAddr 覆盖 HTTP 监听地址 (默认 :8082)
func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

// WithRedis 注入已有 Redis 客户端 (不注入则使用 infrastructure 默认连接并自持)
func WithRedis(rdb *redis.Client) Option {
	return func(o *options) { o.rdb = rdb }
}

// WithOperator 注入 operator 服务 (cmd 装配两个模块后传入; 不注入则内部经
// operator.CreateRuntime(WithDB(db)) 自建, 经对方 Runtime API 交互不 import 内部包)
func WithOperator(svc *operator.Service) Option {
	return func(o *options) { o.operator = svc }
}

// newTokenCache 创建 Redis Token 存储: 注入优先, 否则 infrastructure 默认连接
func newTokenCache(rdb *redis.Client) (*cache.TokenCache, error) {
	if rdb == nil {
		var err error
		rdb, err = infrastructureRedis.NewRedisSentinelClient()
		if err != nil {
			return nil, fmt.Errorf("api -> connect redis: %w", err)
		}
	}
	return cache.NewTokenCache(rdb), nil
}

// Runtime API 模块运行时: 持有装配链与 HTTP Server.
type Runtime struct {
	engine    *gin.Engine
	dao       *dao.Dao
	service   *service.Service
	handler   *handler.Handler
	operator  *operator.Service // operator 服务 (路由转发)
	opRuntime *operator.Runtime // 自建的 operator Runtime (WithOperator 注入时为 nil, Release 时释放)

	db    *gorm.DB // 依赖的数据库连接 (自建时 Release 负责关闭)
	ownDB bool
	srv   *http.Server
}

// CreateRuntime 装配 API 模块:
// 数据库连接 (可注入) → Dao → Service → Handler → gin 路由 → 建表/种子 → 启动 HTTP 服务.
func CreateRuntime(opts ...Option) (*Runtime, error) {
	o := &options{addr: defaultAddr}
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
			return nil, fmt.Errorf("api -> connect postgres: %w", err)
		}
		ownDB = true
	}

	// 依赖注入链: Dao + TokenCache(Redis) → Service → Handler
	d := dao.NewDao(db)
	tokenCache, err := newTokenCache(o.rdb)
	if err != nil {
		return nil, err
	}
	svc := service.NewService(d, tokenCache)

	// operator 服务: 注入优先, 否则内部自建 operator Runtime (共享同一 db, ownDB=false 不重复关闭)
	opSvc := o.operator
	var opRuntime *operator.Runtime
	if opSvc == nil {
		opRuntime, err = operator.CreateRuntime(operator.WithDB(db))
		if err != nil {
			return nil, fmt.Errorf("api -> create operator runtime: %w", err)
		}
		opSvc = opRuntime.Service
	}
	h := handler.NewHandler(svc, opSvc)

	// gin 引擎与路由 (gin 自身不刷调试日志)
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	h.RegisterRoutes(engine)

	// 建表 + 默认管理员种子 (幂等)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if rsp := d.SetupUser(ctx); rsp.Status != response.OperationSuccess.Status {
		return nil, errors.New(rsp.Info)
	}

	rt := &Runtime{
		engine:    engine,
		dao:       d,
		service:   svc,
		handler:   h,
		operator:  opSvc,
		opRuntime: opRuntime,
		db:        db,
		ownDB:     ownDB,
		srv:       &http.Server{Addr: o.addr, Handler: engine},
	}

	// 启动 HTTP 服务
	go func() {
		if err := rt.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[api] http server error: %v", err)
		}
	}()
	log.Printf("[api] runtime created, listening on %s (own_db=%v)", o.addr, ownDB)
	return rt, nil
}

// ReleaseRuntime 释放 API 模块: 优雅关闭 HTTP 服务, 关闭自建的数据库连接 (幂等).
func (r *Runtime) ReleaseRuntime() error {
	if r == nil || r.srv == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownPeriod)
	defer cancel()
	if err := r.srv.Shutdown(ctx); err != nil {
		log.Printf("[api] http shutdown: %v", err)
	}
	r.srv = nil

	// 释放自建的 operator Runtime (注入的不归 api 管)
	if r.opRuntime != nil {
		if err := r.opRuntime.ReleaseRuntime(); err != nil {
			log.Printf("[api] release operator runtime: %v", err)
		}
		r.opRuntime = nil
	}

	if r.ownDB && r.db != nil {
		if sqlDB, err := r.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	log.Printf("[api] runtime released")
	return nil
}

// Engine 暴露 gin 引擎 (测试用 httptest / cmd 挂载子路由)
func (r *Runtime) Engine() *gin.Engine { return r.engine }
