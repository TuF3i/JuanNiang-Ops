// Package cache Token 会话存储 (Redis).
//
// 双 Token 机制: AccessToken (短期, 2h) 用于 API 鉴权; RefreshToken (长期, 7d)
// 用于换发新 AccessToken. 两类 token 各自独立 TTL, RefreshToken 换发时旧 AT 立即吊销.
package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Token 类型标识 (Redis key 前缀)
const (
	prefixAccess  = "api:token:access:"
	prefixRefresh = "api:token:refresh:"
)

// 默认有效期
const (
	DefaultAccessTTL  = 2 * time.Hour
	DefaultRefreshTTL = 7 * 24 * time.Hour
)

// TokenCache Redis 会话存储: token -> userName, 原生 TTL 过期 (无需滑动续期逻辑).
type TokenCache struct {
	rdb        *redis.Client
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewTokenCache 依赖注入: redis 客户端由上层装配 (infrastructure/redis)
func NewTokenCache(rdb *redis.Client) *TokenCache {
	return &TokenCache{
		rdb:        rdb,
		accessTTL:  DefaultAccessTTL,
		refreshTTL: DefaultRefreshTTL,
	}
}

// AccessTTL AccessToken 有效期
func (c *TokenCache) AccessTTL() time.Duration { return c.accessTTL }

// RefreshTTL RefreshToken 有效期
func (c *TokenCache) RefreshTTL() time.Duration { return c.refreshTTL }

// SaveAccess 存储 AccessToken -> userName
func (c *TokenCache) SaveAccess(ctx context.Context, token, userName string) error {
	return c.save(ctx, prefixAccess, token, userName, c.accessTTL)
}

// SaveRefresh 存储 RefreshToken -> userName
func (c *TokenCache) SaveRefresh(ctx context.Context, token, userName string) error {
	return c.save(ctx, prefixRefresh, token, userName, c.refreshTTL)
}

// GetAccess 读取 AccessToken 对应的用户名; 不存在或已过期返回 ErrTokenInvalid 语义 (ErrNil)
func (c *TokenCache) GetAccess(ctx context.Context, token string) (string, error) {
	return c.get(ctx, prefixAccess, token)
}

// GetRefresh 读取 RefreshToken 对应的用户名
func (c *TokenCache) GetRefresh(ctx context.Context, token string) (string, error) {
	return c.get(ctx, prefixRefresh, token)
}

// DeleteAccess 吊销 AccessToken
func (c *TokenCache) DeleteAccess(ctx context.Context, token string) error {
	return c.rdb.Del(ctx, prefixAccess+token).Err()
}

// DeleteRefresh 吊销 RefreshToken
func (c *TokenCache) DeleteRefresh(ctx context.Context, token string) error {
	return c.rdb.Del(ctx, prefixRefresh+token).Err()
}

// save 写入 (SET key value EX ttl)
func (c *TokenCache) save(ctx context.Context, prefix, token, userName string, ttl time.Duration) error {
	return c.rdb.Set(ctx, prefix+token, userName, ttl).Err()
}

// get 读取 (GET; redis.Nil 表示不存在/已过期, 原样返回由上层判定)
func (c *TokenCache) get(ctx context.Context, prefix, token string) (string, error) {
	return c.rdb.Get(ctx, prefix+token).Result()
}
