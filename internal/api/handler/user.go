package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/middleware"
	"JuanNiang-Ops/internal/api/service"
)

// loginRequest 登录请求体
type loginRequest struct {
	UserName string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login POST /api/v1/login: 校验账号密码, 签发 token
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response{Code: 1, Msg: "参数不完整"})
		return
	}

	user, err := h.service.Login(c.Request.Context(), req.UserName, req.Password)
	if err != nil {
		fail(c, err)
		return
	}

	token, err := h.service.Tokens().IssueToken(user.UserName)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, gin.H{"token": token, "username": user.UserName})
}

// userInfoResponse 用户信息响应 (无密码字段)
type userInfoResponse struct {
	UserName string `json:"username"`
}

// GetUserInfo GET /api/v1/user/info: 当前登录用户信息
func (h *Handler) GetUserInfo(c *gin.Context) {
	userName := c.GetString(middleware.CtxKeyUserName)

	user, err := h.service.GetUserInfo(c.Request.Context(), userName)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, userInfoResponse{UserName: user.UserName})
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
		c.JSON(http.StatusBadRequest, response{Code: 1, Msg: "参数不完整"})
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), userName, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrWrongPassword) {
			fail(c, err)
			return
		}
		fail(c, err)
		return
	}
	ok(c, nil)
}
