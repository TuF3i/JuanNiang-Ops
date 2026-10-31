// Package service operator 业务层: 资产实例生命周期管理与统一操作 API.
//
// 资产是 agent/workflow 操作的基本单位: 配置持久化在 dao, 仅启用资产创建实例
// (持有媒介连接与通道客户端). 路由语义仿 adapter 的 route: kind 精确匹配,
// 为空按优先级自动选择; 数据访问下沉 dao, 协议细节在 media/channel 各实现包.
package service

import (
	"errors"
	"sync"

	"JuanNiang-Ops/internal/operator/dao"
)

// sentinel 错误: 调用方用 errors.Is 判定 (standard_go 模式, 见 DEVELOPMENT.yaml)
var (
	ErrAssetNotFound      = errors.New("operator -> asset not found")
	ErrAssetNotRunning    = errors.New("operator -> asset instance not running (disabled or build failed)")
	ErrNoMedia            = errors.New("operator -> asset has no such media")
	ErrNoChannel          = errors.New("operator -> asset has no such channel")
	ErrNoCapability       = errors.New("operator -> media/channel does not implement required capability")
	ErrAmbiguousMedia     = errors.New("operator -> asset has multiple capable media, specify source id")
	ErrAmbiguousChannel   = errors.New("operator -> asset has multiple capable channels, specify source id")
	ErrInvalidAssetConfig = errors.New("operator -> invalid asset config")
	ErrUnknownMediaKind   = errors.New("operator -> unknown media kind")
	ErrUnknownChannelKind = errors.New("operator -> unknown channel kind")

	ErrMediaSourceNotFound   = errors.New("operator -> media source not found")
	ErrChannelSourceNotFound = errors.New("operator -> channel source not found")
	ErrMediaSourceInUse      = errors.New("operator -> media source is bound to assets")
	ErrChannelSourceInUse    = errors.New("operator -> channel source is bound to assets")
)

// Service operator 业务层: 装配后的运行时状态持有者.
// CreateRuntime 创建; 测试可经 NewService 直建 (实例表手工填充).
type Service struct {
	dao       *dao.Dao
	instances map[string]*AssetInstance // assetID -> 实例 (仅启用资产)

	mu sync.Mutex // 保护 instances 与启停/重建串行化
}

// NewService 创建业务层 (实例表为空, 由 LoadEnabled 或 EnableAsset 填充)
func NewService(d *dao.Dao) *Service {
	return &Service{
		dao:       d,
		instances: make(map[string]*AssetInstance),
	}
}
