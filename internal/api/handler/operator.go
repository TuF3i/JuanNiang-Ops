// operator 资产管理 HTTP 接口: 媒介/数据源实体 CRUD、资产 CRUD、启停与绑定.
// handler 只做参数绑定与错误映射; 业务在 internal/operator service;
// operator 的 standard_go sentinel 错误经 mapOperatorErr 翻译为 response 业务码.
package handler

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"JuanNiang-Ops/internal/api/response"
	"JuanNiang-Ops/internal/operator/modules"
	"JuanNiang-Ops/internal/operator/service"
)

// registerOperatorRoutes 注册 operator 路由 (rg 已挂 Auth 中间件)
func (h *Handler) registerOperatorRoutes(rg gin.IRouter) {
	// 媒介实体
	rg.POST("/media", h.CreateMediaSource)
	rg.GET("/media", h.ListMediaSources)
	rg.GET("/media/:id", h.GetMediaSource)
	rg.PUT("/media/:id", h.UpdateMediaSource)
	rg.DELETE("/media/:id", h.DeleteMediaSource)

	// 数据通道实体
	rg.POST("/channel", h.CreateChannelSource)
	rg.GET("/channel", h.ListChannelSources)
	rg.GET("/channel/:id", h.GetChannelSource)
	rg.PUT("/channel/:id", h.UpdateChannelSource)
	rg.DELETE("/channel/:id", h.DeleteChannelSource)

	// 资产
	rg.POST("/asset", h.CreateAsset)
	rg.GET("/asset", h.ListAssets)
	rg.GET("/asset/:id", h.GetAsset)
	rg.PUT("/asset/:id", h.UpdateAsset)
	rg.DELETE("/asset/:id", h.DeleteAsset)
	rg.POST("/asset/:id/enable", h.EnableAsset)
	rg.POST("/asset/:id/disable", h.DisableAsset)
	rg.GET("/asset/:id/status", h.GetAssetStatus)

	// 资产绑定 (媒介/数据源)
	rg.GET("/asset/:id/media", h.ListAssetMedia)
	rg.POST("/asset/:id/media", h.BindAssetMedia)
	rg.DELETE("/asset/:id/media", h.UnbindAssetMedia)
	rg.GET("/asset/:id/channel", h.ListAssetChannels)
	rg.POST("/asset/:id/channel", h.BindAssetChannel)
	rg.DELETE("/asset/:id/channel", h.UnbindAssetChannel)
}

// requireOperator operator 服务装配守卫 (未装配时所有 operator 路由返回不可用)
func (h *Handler) requireOperator() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.operator == nil {
			resp(c, response.OperatorUnavailable, nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// mapOperatorErr operator 的 standard_go sentinel 错误 → response 业务码
func mapOperatorErr(err error) response.Response {
	switch {
	case errors.Is(err, service.ErrAssetNotFound),
		errors.Is(err, service.ErrMediaSourceNotFound),
		errors.Is(err, service.ErrChannelSourceNotFound):
		return response.OperatorNotFound
	case errors.Is(err, service.ErrMediaSourceInUse),
		errors.Is(err, service.ErrChannelSourceInUse):
		return response.OperatorInUse
	case errors.Is(err, service.ErrAssetNotRunning):
		return response.OperatorNotRunning
	case errors.Is(err, service.ErrAmbiguousMedia),
		errors.Is(err, service.ErrAmbiguousChannel):
		return response.OperatorAmbiguous
	case errors.Is(err, service.ErrInvalidAssetConfig),
		errors.Is(err, service.ErrUnknownMediaKind),
		errors.Is(err, service.ErrUnknownChannelKind):
		return response.RevDataError
	default:
		return response.ServerInternalError(err)
	}
}

// respOperator operator 调用统一出口: 成功 20000, 失败按 sentinel 映射业务码
func respOperator(c *gin.Context, err error, data any) {
	if err != nil {
		resp(c, mapOperatorErr(err), nil)
		return
	}
	resp(c, response.OperationSuccess, data)
}

// pathID 提取路径参数 :id
func pathID(c *gin.Context) string {
	return c.Param("id")
}

// ---------------------------------------------------------------- 媒介实体 --

// createSourceRequest 创建媒介/数据源请求体 (kind + name + config JSON 字符串)
type createSourceRequest struct {
	Name   string `json:"name" binding:"required"`
	Kind   string `json:"kind" binding:"required"`
	Config string `json:"config"`
}

// CreateMediaSource POST /operator/media
func (h *Handler) CreateMediaSource(c *gin.Context) {
	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	src := &modules.MediaSource{Name: req.Name, Kind: modules.MediaKind(req.Kind), Config: req.Config}
	respOperator(c, h.operator.CreateMediaSource(c.Request.Context(), src), src)
}

// ListMediaSources GET /operator/media?kind=
func (h *Handler) ListMediaSources(c *gin.Context) {
	sources, err := h.operator.ListMediaSources(c.Request.Context(), modules.MediaKind(c.Query("kind")))
	respOperator(c, err, sources)
}

// GetMediaSource GET /operator/media/:id
func (h *Handler) GetMediaSource(c *gin.Context) {
	src, err := h.operator.GetMediaSource(c.Request.Context(), pathID(c))
	respOperator(c, err, src)
}

// updateSourceRequest 更新媒介/数据源请求体 (name/kind/config 全量)
type updateSourceRequest = createSourceRequest

// UpdateMediaSource PUT /operator/media/:id
func (h *Handler) UpdateMediaSource(c *gin.Context) {
	var req updateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	dst := &modules.MediaSource{ID: pathID(c), Name: req.Name, Kind: modules.MediaKind(req.Kind), Config: req.Config}
	respOperator(c, h.operator.UpdateMediaSource(c.Request.Context(), dst), dst)
}

// DeleteMediaSource DELETE /operator/media/:id (在用返回 10021)
func (h *Handler) DeleteMediaSource(c *gin.Context) {
	respOperator(c, h.operator.DeleteMediaSource(c.Request.Context(), pathID(c)), nil)
}

// ---------------------------------------------------------------- 通道实体 --

// CreateChannelSource POST /operator/channel
func (h *Handler) CreateChannelSource(c *gin.Context) {
	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	src := &modules.ChannelSource{Name: req.Name, Kind: modules.ChannelKind(req.Kind), Config: req.Config}
	respOperator(c, h.operator.CreateChannelSource(c.Request.Context(), src), src)
}

// ListChannelSources GET /operator/channel?kind=
func (h *Handler) ListChannelSources(c *gin.Context) {
	sources, err := h.operator.ListChannelSources(c.Request.Context(), modules.ChannelKind(c.Query("kind")))
	respOperator(c, err, sources)
}

// GetChannelSource GET /operator/channel/:id
func (h *Handler) GetChannelSource(c *gin.Context) {
	src, err := h.operator.GetChannelSource(c.Request.Context(), pathID(c))
	respOperator(c, err, src)
}

// UpdateChannelSource PUT /operator/channel/:id
func (h *Handler) UpdateChannelSource(c *gin.Context) {
	var req updateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	dst := &modules.ChannelSource{ID: pathID(c), Name: req.Name, Kind: modules.ChannelKind(req.Kind), Config: req.Config}
	respOperator(c, h.operator.UpdateChannelSource(c.Request.Context(), dst), dst)
}

// DeleteChannelSource DELETE /operator/channel/:id (在用返回 10021)
func (h *Handler) DeleteChannelSource(c *gin.Context) {
	respOperator(c, h.operator.DeleteChannelSource(c.Request.Context(), pathID(c)), nil)
}

// ---------------------------------------------------------------- 资产 --

// assetRequest 资产创建/更新请求体 (纯描述信息; 媒介/通道经绑定接口挂载)
type assetRequest struct {
	Name        string            `json:"name" binding:"required"`
	Description string            `json:"description"`
	Docs        string            `json:"docs"`
	Tags        []string          `json:"tags"`
	Metadata    map[string]string `json:"metadata"`
}

// toAsset 请求体 → 资产模型
func toAsset(id string, req assetRequest) *modules.Asset {
	return &modules.Asset{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Docs:        req.Docs,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
	}
}

// CreateAsset POST /operator/asset
func (h *Handler) CreateAsset(c *gin.Context) {
	var req assetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	asset := toAsset("", req)
	respOperator(c, h.operator.CreateAsset(c.Request.Context(), asset), asset)
}

// ListAssets GET /operator/asset?tags=a,b (逗号分隔)
func (h *Handler) ListAssets(c *gin.Context) {
	var tags []string
	if raw := c.Query("tags"); raw != "" {
		for _, t := range splitAndTrim(raw) {
			tags = append(tags, t)
		}
	}
	assets, err := h.operator.ListAssets(c.Request.Context(), tags)
	respOperator(c, err, assets)
}

// GetAsset GET /operator/asset/:id
func (h *Handler) GetAsset(c *gin.Context) {
	asset, err := h.operator.GetAsset(c.Request.Context(), pathID(c))
	respOperator(c, err, asset)
}

// UpdateAsset PUT /operator/asset/:id
func (h *Handler) UpdateAsset(c *gin.Context) {
	var req assetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	respOperator(c, h.operator.UpdateAsset(c.Request.Context(), toAsset(pathID(c), req)), nil)
}

// DeleteAsset DELETE /operator/asset/:id
func (h *Handler) DeleteAsset(c *gin.Context) {
	respOperator(c, h.operator.DeleteAsset(c.Request.Context(), pathID(c)), nil)
}

// EnableAsset POST /operator/asset/:id/enable
func (h *Handler) EnableAsset(c *gin.Context) {
	respOperator(c, h.operator.EnableAsset(c.Request.Context(), pathID(c)), nil)
}

// DisableAsset POST /operator/asset/:id/disable
func (h *Handler) DisableAsset(c *gin.Context) {
	respOperator(c, h.operator.DisableAsset(c.Request.Context(), pathID(c)), nil)
}

// GetAssetStatus GET /operator/asset/:id/status
func (h *Handler) GetAssetStatus(c *gin.Context) {
	status, err := h.operator.GetAssetStatus(c.Request.Context(), pathID(c))
	respOperator(c, err, status)
}

// ---------------------------------------------------------------- 资产绑定 --

// bindRequest 绑定请求体 (媒介/数据源实体 ID)
type bindRequest struct {
	SourceID string `json:"source_id" binding:"required"`
}

// ListAssetMedia GET /operator/asset/:id/media
func (h *Handler) ListAssetMedia(c *gin.Context) {
	sources, err := h.operator.ListAssetMedia(c.Request.Context(), pathID(c))
	respOperator(c, err, sources)
}

// BindAssetMedia POST /operator/asset/:id/media
func (h *Handler) BindAssetMedia(c *gin.Context) {
	var req bindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	respOperator(c, h.operator.BindAssetMedia(c.Request.Context(), pathID(c), req.SourceID), nil)
}

// UnbindAssetMedia DELETE /operator/asset/:id/media (幂等)
func (h *Handler) UnbindAssetMedia(c *gin.Context) {
	var req bindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	respOperator(c, h.operator.UnbindAssetMedia(c.Request.Context(), pathID(c), req.SourceID), nil)
}

// ListAssetChannels GET /operator/asset/:id/channel
func (h *Handler) ListAssetChannels(c *gin.Context) {
	sources, err := h.operator.ListAssetChannels(c.Request.Context(), pathID(c))
	respOperator(c, err, sources)
}

// BindAssetChannel POST /operator/asset/:id/channel
func (h *Handler) BindAssetChannel(c *gin.Context) {
	var req bindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	respOperator(c, h.operator.BindAssetChannel(c.Request.Context(), pathID(c), req.SourceID), nil)
}

// UnbindAssetChannel DELETE /operator/asset/:id/channel (幂等)
func (h *Handler) UnbindAssetChannel(c *gin.Context) {
	var req bindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp(c, response.RevDataError, nil)
		return
	}
	respOperator(c, h.operator.UnbindAssetChannel(c.Request.Context(), pathID(c), req.SourceID), nil)
}

// splitAndTrim 逗号分隔字符串 → 去空白片段列表
func splitAndTrim(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
