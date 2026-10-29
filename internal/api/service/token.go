// 双 Token 会话管理: AccessToken (短期鉴权) + RefreshToken (长期换发).
// 存储走 cache 包 (Redis); 生成/校验/换发逻辑在本层. 错误以 response.Response 上抛.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"JuanNiang-Ops/internal/api/cache"
	"JuanNiang-Ops/internal/api/response"
)

// TokenManager 双会话管理器
type TokenManager struct {
	cache *cache.TokenCache
}

// NewTokenManager 依赖注入: Redis 存储由上层装配
func NewTokenManager(c *cache.TokenCache) *TokenManager {
	return &TokenManager{cache: c}
}

// TokenPair 双 Token 对
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"` // Bearer
	ExpiresIn    int64  `json:"expires_in"` // AccessToken 剩余秒数
}

// IssueToken 签发双 Token 对
func (r *TokenManager) IssueToken(ctx context.Context, userName string) (*TokenPair, response.Response) {
	access, err := randomToken()
	if err != nil {
		return nil, response.ServerInternalError(err)
	}
	refresh, err := randomToken()
	if err != nil {
		return nil, response.ServerInternalError(err)
	}

	if err := r.cache.SaveAccess(ctx, access, userName); err != nil {
		return nil, response.ServerInternalError(errors.New("save access token: "+err.Error()))
	}
	if err := r.cache.SaveRefresh(ctx, refresh, userName); err != nil {
		return nil, response.ServerInternalError(errors.New("save refresh token: "+err.Error()))
	}

	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(r.cache.AccessTTL().Seconds()),
	}, response.OperationSuccess
}

// ValidateAccess 校验 AccessToken, 返回绑定用户名; 无效返回 InvalidToken
func (r *TokenManager) ValidateAccess(ctx context.Context, token string) (string, response.Response) {
	userName, err := r.cache.GetAccess(ctx, token)
	if err != nil {
		// redis.Nil (不存在/过期) 与其他存储错误统一按 Token 失效处理
		return "", response.InvalidToken
	}
	return userName, response.OperationSuccess
}

// Refresh 校验 RefreshToken: 旧 RT 立即吊销, 签发全新 Token 对 (旋转刷新, 防重放).
// RefreshToken 无效返回 InvalidRefresh.
func (r *TokenManager) Refresh(ctx context.Context, refreshToken string) (*TokenPair, response.Response) {
	userName, err := r.cache.GetRefresh(ctx, refreshToken)
	if err != nil {
		return nil, response.InvalidRefresh
	}

	// 旋转: 旧 RefreshToken 立即作废 (旧 AT 等自然过期, 最多 2h)
	_ = r.cache.DeleteRefresh(ctx, refreshToken)

	return r.IssueToken(ctx, userName)
}

// Revoke 登出: 同时吊销 AT 与 RT (两参数均可为空, 为空跳过)
func (r *TokenManager) Revoke(ctx context.Context, accessToken, refreshToken string) {
	if accessToken != "" {
		_ = r.cache.DeleteAccess(ctx, accessToken)
	}
	if refreshToken != "" {
		_ = r.cache.DeleteRefresh(ctx, refreshToken)
	}
}

// randomToken 32 字节随机数 hex (64 字符)
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("generate token: " + err.Error())
	}
	return hex.EncodeToString(buf), nil
}
