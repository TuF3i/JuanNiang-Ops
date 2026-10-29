package service

import (
	"JuanNiang-Ops/internal/api/cache"
	"JuanNiang-Ops/internal/api/dao"
)

// Service 用户业务层: 组合 Dao (数据访问) 与 TokenManager (双 Token 会话).
type Service struct {
	dao    *dao.Dao
	tokens *TokenManager
}

// NewService 依赖注入: Dao 与 Redis Token 存储由上层装配
func NewService(dao *dao.Dao, tokens *cache.TokenCache) *Service {
	return &Service{
		dao:    dao,
		tokens: NewTokenManager(tokens),
	}
}

// Tokens 暴露会话管理器 (handler 登录/刷新/登出用)
func (r *Service) Tokens() *TokenManager {
	return r.tokens
}
