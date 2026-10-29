// Token 换发与登出接口.
package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/response"
)

// refreshRequest 刷新请求体
type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh POST /api/v1/token/refresh: 用 RefreshToken 换发全新双 Token 对.
// 旋转刷新: 旧 RefreshToken 立即作废 (防重放).
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}

	pair, rsp := h.service.Tokens().Refresh(c.Request.Context(), req.RefreshToken)
	if !isSuccess(rsp) {
		resp(c, rsp, nil)
		return
	}
	resp(c, response.OperationSuccess, gin.H{"tokens": pair})
}

// logoutRequest 登出请求体 (两类 token 均可只传其一)
type logoutRequest struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Logout POST /api/v1/token/logout: 吊销传入的 token 对 (公开接口, 幂等).
func (h *Handler) Logout(c *gin.Context) {
	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil || (req.AccessToken == "" && req.RefreshToken == "") {
		// 无参数也按成功处理 (幂等登出)
		respOK(c)
		return
	}

	h.service.Tokens().Revoke(c.Request.Context(), req.AccessToken, req.RefreshToken)
	respOK(c)
}

// bearerToken 从请求头提取当前 AccessToken (鉴权路由内使用)
func bearerToken(c *gin.Context) string {
	const prefix = "Bearer "
	return strings.TrimPrefix(c.GetHeader("Authorization"), prefix)
}
