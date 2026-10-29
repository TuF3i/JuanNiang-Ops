// HTTP 接入层: gin 路由与请求处理. 业务逻辑一律下沉 Service;
// 错误统一经 response.GenFinalResponse 输出 (HTTP 状态恒 200, 业务码在 status 字段).
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/middleware"
	"JuanNiang-Ops/internal/api/response"
	"JuanNiang-Ops/internal/api/service"
)

// Handler HTTP 处理器集合
type Handler struct {
	service *service.Service
}

// NewHandler 依赖注入: Service 由上层装配
func NewHandler(svc *service.Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes 注册路由: login/refresh/logout 公开, /user 组挂 Auth 中间件 (middleware 包).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	v1 := rg.Group("/api/v1")

	// 公开: 认证与 token 管理
	v1.POST("/login", h.Login)
	v1.POST("/token/refresh", h.Refresh)
	v1.POST("/token/logout", h.Logout)

	// 鉴权: 用户接口
	user := v1.Group("/user", middleware.Auth(h.service.Tokens()))
	user.GET("/info", h.GetUserInfo)
	user.PUT("/password", h.ChangePassword)
}

// resp 统一响应输出: GenFinalResponse 组装, HTTP 恒 200 (业务码在 status)
func resp(c *gin.Context, rsp response.Response, data any) {
	c.JSON(http.StatusOK, response.GenFinalResponse(rsp, data))
}

// respOK 成功响应 (data 为空)
func respOK(c *gin.Context) {
	resp(c, response.OperationSuccess, nil)
}
