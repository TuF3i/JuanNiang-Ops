// 用户接口: 登录 / 用户信息 / 修改密码.
package handler

import (
	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/middleware"
	"JuanNiang-Ops/internal/api/response"
)

// loginRequest 登录请求体
type loginRequest struct {
	UserName string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login POST /api/v1/login: 校验账号密码, 签发双 Token (AccessToken + RefreshToken)
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}

	user, rsp := h.service.Login(c.Request.Context(), req.UserName, req.Password)
	if !isSuccess(rsp) {
		resp(c, rsp, nil)
		return
	}

	pair, rsp := h.service.Tokens().IssueToken(c.Request.Context(), user.UserName)
	if !isSuccess(rsp) {
		resp(c, rsp, nil)
		return
	}
	resp(c, response.OperationSuccess, gin.H{"username": user.UserName, "tokens": pair})
}

// userInfoResponse 用户信息响应 (无密码字段)
type userInfoResponse struct {
	UserName string `json:"username"`
}

// GetUserInfo GET /api/v1/user/info: 当前登录用户信息
func (h *Handler) GetUserInfo(c *gin.Context) {
	userName := c.GetString(middleware.CtxKeyUserName)

	user, rsp := h.service.GetUserInfo(c.Request.Context(), userName)
	if !isSuccess(rsp) {
		resp(c, rsp, nil)
		return
	}
	resp(c, response.OperationSuccess, userInfoResponse{UserName: user.UserName})
}

// changePasswordRequest 修改密码请求体
type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword PUT /api/v1/user/password: 校验旧密码后更新
func (h *Handler) ChangePassword(c *gin.Context) {
	userName := c.GetString(middleware.CtxKeyUserName)

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}

	if rsp := h.service.ChangePassword(c.Request.Context(), userName, req.OldPassword, req.NewPassword); !isSuccess(rsp) {
		resp(c, rsp, nil)
		return
	}
	respOK(c)
}

// isSuccess 判定业务响应是否成功
func isSuccess(rsp response.Response) bool {
	return rsp.Status == response.OperationSuccess.Status
}
