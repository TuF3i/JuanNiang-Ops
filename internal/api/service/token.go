// 登录会话: 内存 Token 管理 (随机 token -> 用户名, TTL 滑动过期).
//
// ⚠️ Token 存内存: 服务重启后全部失效 (客户端需重新登录); 如需持久化/多实例,
// 后续可替换为 cache/ 目录的 redis 实现, 接口签名保持不变.
package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// ErrTokenInvalid token 无效或已过期 (handler 映射 401)
var ErrTokenInvalid = errors.New("api -> token is invalid or expired")

// 默认会话配置
const (
	defaultTokenTTL     = 24 * time.Hour // 有效期, 每次校验成功滑动续期
	defaultTokenBufSize = 32             // 随机字节数 (hex 后 64 字符)
)

// session 会话条目
type session struct {
	userName  string
	expiresAt time.Time
}

// TokenManager 会话管理器 (并发安全)
type TokenManager struct {
	mu       sync.RWMutex
	sessions map[string]session // token -> 会话
	ttl      time.Duration
}

// NewTokenManager 创建会话管理器
func NewTokenManager() *TokenManager {
	return &TokenManager{
		sessions: make(map[string]session),
		ttl:      defaultTokenTTL,
	}
}

// IssueToken 签发 token: 32 字节随机数 hex, 绑定用户名
func (r *TokenManager) IssueToken(userName string) (string, error) {
	buf := make([]byte, defaultTokenBufSize)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("api -> generate token failed")
	}
	token := hex.EncodeToString(buf)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[token] = session{
		userName:  userName,
		expiresAt: time.Now().Add(r.ttl),
	}
	return token, nil
}

// ValidateToken 校验 token 并滑动续期; 返回绑定的用户名
func (r *TokenManager) ValidateToken(token string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[token]
	if !ok {
		return "", ErrTokenInvalid
	}
	if time.Now().After(s.expiresAt) {
		delete(r.sessions, token) // 过期清理
		return "", ErrTokenInvalid
	}
	s.expiresAt = time.Now().Add(r.ttl) // 滑动续期
	r.sessions[token] = s
	return s.userName, nil
}

// RevokeToken 吊销 token (登出)
func (r *TokenManager) RevokeToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, token)
}
