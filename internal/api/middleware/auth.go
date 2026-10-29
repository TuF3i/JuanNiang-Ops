// Package middleware gin 中间件: 会话鉴权等横切关注点.
// 中间件只做校验与上下文注入, 业务校验依赖 service.TokenManager.
// 错误输出走 response 封装 (业务码在 status, HTTP 恒 200).
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/response"
	"JuanNiang-Ops/internal/api/service"
)

// CtxKeyUserName gin context 中用户名的键 (handler 经 c.GetString 取值)
const CtxKeyUserName = "api.userName"

// bearerPrefix Authorization 头前缀
const bearerPrefix = "Bearer "

// Auth 会话鉴权中间件: 校验 Authorization: Bearer <AccessToken>, 用户名注入 context.
// 失败按 response 业务码输出 (EmptyToken / InvalidToken), 不再继续路由.
func Auth(tokens *service.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, found := strings.CutPrefix(c.GetHeader("Authorization"), bearerPrefix)
		if !found || token == "" {
			c.AbortWithStatusJSON(httpStatusOK, response.GenFinalResponse(response.EmptyToken, nil))
			return
		}

		userName, rsp := tokens.ValidateAccess(c.Request.Context(), token)
		if !isSuccess(rsp) {
			c.AbortWithStatusJSON(httpStatusOK, response.GenFinalResponse(rsp, nil))
			return
		}

		c.Set(CtxKeyUserName, userName)
		c.Next()
	}
}

// isSuccess 判定业务响应是否成功 (与 handler 同判定, 避免循环依赖)
func isSuccess(rsp response.Response) bool {
	return rsp.Status == response.OperationSuccess.Status
}

// httpStatusOK 业务码模式: HTTP 状态恒 200
const httpStatusOK = 200
