// HTTP 接入层: gin 路由与请求/响应处理. 业务逻辑一律下沉 Service, 此层只做参数绑定与错误映射.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/dao"
	"JuanNiang-Ops/internal/api/middleware"
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

// RegisterRoutes 注册路由: login 公开, /user 组挂 Auth 中间件 (middleware 包).
func (h *Handler) RegisterRoutes(rg gin.IRouter) {
	v1 := rg.Group("/api/v1")
	v1.POST("/login", h.Login)

	user := v1.Group("/user", middleware.Auth(h.service.Tokens()))
	user.GET("/info", h.GetUserInfo)
	user.PUT("/password", h.ChangePassword)
}

// ---------- 统一响应 ----------

// response 统一响应结构: {"code":0, "msg":"ok", "data":{...}}
type response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{Code: 0, Msg: "ok", Data: data})
}

// fail 按错误映射 HTTP 状态码 (可判定错误 -> 具体码, 其余 500)
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredential):
		c.JSON(http.StatusUnauthorized, response{Code: 1, Msg: "用户名或密码错误"})
	case errors.Is(err, service.ErrTokenInvalid):
		c.JSON(http.StatusUnauthorized, response{Code: 1, Msg: "登录已失效, 请重新登录"})
	case errors.Is(err, service.ErrWrongPassword):
		c.JSON(http.StatusBadRequest, response{Code: 1, Msg: "旧密码错误"})
	case errors.Is(err, dao.ErrUserNotFound):
		c.JSON(http.StatusNotFound, response{Code: 1, Msg: "用户不存在"})
	default:
		c.JSON(http.StatusInternalServerError, response{Code: 1, Msg: "内部错误"})
	}
}
