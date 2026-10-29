// Package middleware gin 中间件: 会话鉴权等横切关注点.
// 中间件只做校验与上下文注入, 业务校验依赖 service.TokenManager.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/service"
)

// CtxKeyUserName gin context 中用户名的键 (handler 经 c.GetString 取值)
const CtxKeyUserName = "api.userName"

// bearerPrefix Authorization 头前缀
const bearerPrefix = "Bearer "

// Auth 会话鉴权中间件: 校验 Authorization: Bearer <token>, 用户名注入 context.
// 失败统一 401 (不再继续路由).
func Auth(tokens *service.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		token, found := strings.CutPrefix(auth, bearerPrefix)
		if !found || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 1, "msg": "未登录"})
			return
		}

		userName, err := tokens.ValidateToken(token)
		if err != nil {
			if !errors.Is(err, service.ErrTokenInvalid) {
				// 非 token 类错误按未登录处理并记日志
				_ = c.Error(err)
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 1, "msg": "登录已失效, 请重新登录"})
			return
		}

		c.Set(CtxKeyUserName, userName)
		c.Next()
	}
}
