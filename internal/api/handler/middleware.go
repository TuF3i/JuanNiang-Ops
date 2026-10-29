package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/service"
)

// ctxKeyUserName gin context 中用户名的键
const ctxKeyUserName = "api.userName"

// bearerPrefix Authorization 头前缀
const bearerPrefix = "Bearer "

// Auth 会话鉴权中间件: 校验 Authorization: Bearer <token>, 用户名注入 context.
// 失败统一 401 (不再继续路由).
func (h *Handler) Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		token, found := strings.CutPrefix(auth, bearerPrefix)
		if !found || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response{Code: 1, Msg: "未登录"})
			return
		}

		userName, err := h.service.Tokens().ValidateToken(token)
		if err != nil {
			if !errors.Is(err, service.ErrTokenInvalid) {
				// 非 token 类错误按未登录处理并记日志
				c.Error(err) // nolint: errcheck
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, response{Code: 1, Msg: "登录已失效, 请重新登录"})
			return
		}

		c.Set(ctxKeyUserName, userName)
		c.Next()
	}
}
