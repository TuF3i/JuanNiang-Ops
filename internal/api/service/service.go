package service

import "JuanNiang-Ops/internal/api/dao"

// Service 用户业务层: 组合 Dao (数据访问) 与 TokenManager (会话).
type Service struct {
	dao    *dao.Dao
	tokens *TokenManager
}

// NewService 依赖注入: Dao 由上层装配
func NewService(dao *dao.Dao) *Service {
	return &Service{
		dao:    dao,
		tokens: NewTokenManager(),
	}
}

// Tokens 暴露会话管理器 (handler 中间件校验 token 用)
func (r *Service) Tokens() *TokenManager {
	return r.tokens
}
