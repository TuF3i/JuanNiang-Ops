// 资产管理与统一操作 API: agent/workflow 的入口.
// 管理方法委托 dao 并联动实例生命周期; 操作方法经 route 选取媒介/通道后转发.
package service

import (
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// ---------------------------------------------------------------- 资产管理 --

// CreateAsset 新建资产: 校验配置可解析 → 填充 UUID → 落库 (不自动启用).
func (r *Service) CreateAsset(ctx context.Context, asset *modules.Asset) error {
	if asset.Name == "" {
		return fmt.Errorf("create asset -> %w: empty name", ErrInvalidAssetConfig)
	}
	if err := validateAssetConfig(asset); err != nil {
		return fmt.Errorf("create asset -> %w", err)
	}
	if asset.ID == "" {
		asset.ID = uuid.NewString()
	}
	if err := r.dao.CreateAsset(ctx, asset); err != nil {
		return fmt.Errorf("create asset -> %w", err)
	}
	logAsset(asset.ID, "created")
	return nil
}

// GetAsset 查询资产 (未找到返回 ErrAssetNotFound)
func (r *Service) GetAsset(ctx context.Context, id string) (*modules.Asset, error) {
	asset, err := r.dao.GetAsset(ctx, id)
	if err != nil {
		return nil, r.translateNotFound(fmt.Errorf("get asset -> %w", err))
	}
	return asset, nil
}

// ListAssets 资产列表 (tags 非空时按交集内存过滤; 资产数量级小无需 SQL 过滤)
func (r *Service) ListAssets(ctx context.Context, tags []string) ([]modules.Asset, error) {
	assets, err := r.dao.ListAssets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list assets -> %w", err)
	}
	if len(tags) == 0 {
		return assets, nil
	}

	filtered := make([]modules.Asset, 0, len(assets))
	for _, a := range assets {
		if hasAllTags(a.Tags, tags) {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

// UpdateAsset 更新资产: sonic+MD5 对比 (无变化跳过) → 落库 → 实例存在则重建.
// dst 须携带 ID.
func (r *Service) UpdateAsset(ctx context.Context, dst *modules.Asset) error {
	src, err := r.dao.GetAsset(ctx, dst.ID)
	if err != nil {
		return r.translateNotFound(fmt.Errorf("update asset -> %w", err))
	}
	if err := validateAssetConfig(dst); err != nil {
		return fmt.Errorf("update asset -> %w", err)
	}

	same, err := equal(src, dst)
	if err != nil {
		return fmt.Errorf("update asset -> %w", err)
	}
	if same {
		return nil
	}

	// 保留不可变列 (ID 主键一致, 创建时间以 DB 为准)
	dst.CreatedAt = src.CreatedAt
	if err := r.dao.UpdateAsset(ctx, dst); err != nil {
		return fmt.Errorf("update asset -> %w", err)
	}

	r.mu.Lock()
	_, running := r.instances[dst.ID]
	r.mu.Unlock()
	if running {
		if err := r.reCreate(ctx, dst.ID); err != nil {
			return fmt.Errorf("update asset -> %w", err)
		}
	}
	logAsset(dst.ID, "updated")
	return nil
}

// DeleteAsset 删除资产: 启用中先落库停用并关实例, 再软删.
func (r *Service) DeleteAsset(ctx context.Context, id string) error {
	if _, err := r.dao.GetAsset(ctx, id); err != nil {
		return r.translateNotFound(fmt.Errorf("delete asset -> %w", err))
	}

	// 停用 (落库 + 关实例), 失败不阻断删除 (实例可能本就未运行)
	_ = r.DisableAsset(ctx, id)
	if err := r.dao.DeleteAsset(ctx, id); err != nil {
		return fmt.Errorf("delete asset -> %w", err)
	}
	logAsset(id, "deleted")
	return nil
}

// EnableAsset 启用资产 (落库 + 创建实例), 实现在 lifecycle.go

// GetAssetStatus 资产状态快照 (Enabled 为 DB 开关, Running 为实例是否在内存)
func (r *Service) GetAssetStatus(ctx context.Context, id string) (*modules.AssetStatus, error) {
	asset, err := r.dao.GetAsset(ctx, id)
	if err != nil {
		return nil, r.translateNotFound(fmt.Errorf("get asset status -> %w", err))
	}

	r.mu.Lock()
	ins, running := r.instances[id]
	r.mu.Unlock()

	status := &modules.AssetStatus{
		ID:      asset.ID,
		Name:    asset.Name,
		Enabled: asset.Enabled,
		Running: running,
	}
	if ins != nil {
		for kind := range ins.media {
			status.MediaKinds = append(status.MediaKinds, kind)
		}
		for kind := range ins.channels {
			status.ChannelKinds = append(status.ChannelKinds, kind)
		}
	}
	return status, nil
}

// ---------------------------------------------------------------- 统一操作 --

// PingAsset 资产连通性检查: 逐个媒介/通道 Ping, 返回 kind -> 错误 (nil 即可达).
// 资产未运行返回 ErrAssetNotRunning.
func (r *Service) PingAsset(ctx context.Context, assetID string) (map[string]error, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	return ins.Ping(ctx), nil
}

// Exec 在资产的媒介上执行命令: mediaKind 指定则精确匹配, 为空按优先级 (ssh → telnet) 自动选择.
func (r *Service) Exec(ctx context.Context, assetID string, mediaKind modules.MediaKind, cmd string) (*modules.ExecResult, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	cr, err := ins.commandRunner(mediaKind)
	if err != nil {
		return nil, err
	}
	result, err := cr.Exec(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("exec on %s/%s -> %w", assetID, mediaKind, err)
	}
	return result, nil
}

// QueryMetrics 瞬时指标查询: channelKind 为空时仅单通道资产可用.
func (r *Service) QueryMetrics(ctx context.Context, assetID string, channelKind modules.ChannelKind, query string, ts time.Time) ([]modules.MetricSample, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	mq, err := ins.metricsQuerier(channelKind)
	if err != nil {
		return nil, err
	}
	samples, err := mq.Query(ctx, query, ts)
	if err != nil {
		return nil, fmt.Errorf("query metrics on %s/%s -> %w", assetID, channelKind, err)
	}
	return samples, nil
}

// QueryRange 区间指标查询 (PromQL, step 采样步长)
func (r *Service) QueryRange(ctx context.Context, assetID string, channelKind modules.ChannelKind, query string, start, end time.Time, step time.Duration) ([]modules.MetricSeries, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	mq, err := ins.metricsQuerier(channelKind)
	if err != nil {
		return nil, err
	}
	series, err := mq.QueryRange(ctx, query, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("query range on %s/%s -> %w", assetID, channelKind, err)
	}
	return series, nil
}

// QueryLogs 日志查询 (LogQL)
func (r *Service) QueryLogs(ctx context.Context, assetID string, channelKind modules.ChannelKind, q modules.LogQuery) ([]modules.LogEntry, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	lq, err := ins.logQuerier(channelKind)
	if err != nil {
		return nil, err
	}
	entries, err := lq.QueryLogs(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query logs on %s/%s -> %w", assetID, channelKind, err)
	}
	return entries, nil
}

// SearchTraces 链路搜索 (TraceQL)
func (r *Service) SearchTraces(ctx context.Context, assetID string, channelKind modules.ChannelKind, q modules.TraceQuery) ([]modules.TraceSummary, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ts, err := ins.traceSearcher(channelKind)
	if err != nil {
		return nil, err
	}
	traces, err := ts.SearchTraces(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("search traces on %s/%s -> %w", assetID, channelKind, err)
	}
	return traces, nil
}

// GetTrace 拉取完整链路 (原始 JSON)
func (r *Service) GetTrace(ctx context.Context, assetID string, channelKind modules.ChannelKind, traceID string) ([]byte, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ts, err := ins.traceSearcher(channelKind)
	if err != nil {
		return nil, err
	}
	data, err := ts.GetTrace(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("get trace on %s/%s -> %w", assetID, channelKind, err)
	}
	return data, nil
}

// SearchDocs 文档搜索
func (r *Service) SearchDocs(ctx context.Context, assetID string, channelKind modules.ChannelKind, keyword string, limit int) ([]modules.DocHit, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ds, err := ins.docSearcher(channelKind)
	if err != nil {
		return nil, err
	}
	hits, err := ds.SearchDocs(ctx, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("search docs on %s/%s -> %w", assetID, channelKind, err)
	}
	return hits, nil
}

// GetDoc 读取文档页 (markdown)
func (r *Service) GetDoc(ctx context.Context, assetID string, channelKind modules.ChannelKind, pageID string) (*modules.DocPage, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ds, err := ins.docSearcher(channelKind)
	if err != nil {
		return nil, err
	}
	page, err := ds.GetDoc(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("get doc on %s/%s -> %w", assetID, channelKind, err)
	}
	return page, nil
}

// Media 取资产运行中实例的媒介引用 (低频特化操作: 调用方自行断言能力接口,
// 如 pve 的 VMOperator 虚机启停). 未运行返回 ErrAssetNotRunning.
func (r *Service) Media(assetID string, kind modules.MediaKind) (media.Media, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	return ins.selectMedia(kind)
}

// Channel 取资产运行中实例的通道引用 (调用方自行断言能力接口)
func (r *Service) Channel(assetID string, kind modules.ChannelKind) (channel.Channel, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	return ins.selectChannel(kind)
}

// ---------------------------------------------------------------- 内部辅助 --

// equal 对比两个资产是否等价 (sonic 序列化 + MD5, 复刻 adapter 的同名实现)
func equal(src, dst any) (bool, error) {
	srcRaw, err := sonic.Marshal(src)
	if err != nil {
		return false, err
	}
	dstRaw, err := sonic.Marshal(dst)
	if err != nil {
		return false, err
	}
	return md5.Sum(srcRaw) == md5.Sum(dstRaw), nil
}

// validateAssetConfig 校验资产携带的媒介/通道配置 JSON 均可解析为对应 Config
func validateAssetConfig(asset *modules.Asset) error {
	for kind, raw := range asset.Media {
		if _, err := modules.ParseMediaConfig(kind, raw); err != nil {
			return fmt.Errorf("%w: media %s: %v", ErrInvalidAssetConfig, kind, err)
		}
	}
	for kind, raw := range asset.Channels {
		if _, err := modules.ParseChannelConfig(kind, raw); err != nil {
			return fmt.Errorf("%w: channel %s: %v", ErrInvalidAssetConfig, kind, err)
		}
	}
	return nil
}

// hasAllTags 判断 assetTags 是否包含全部 wanted
func hasAllTags(assetTags, wanted []string) bool {
	set := make(map[string]struct{}, len(assetTags))
	for _, t := range assetTags {
		set[t] = struct{}{}
	}
	for _, t := range wanted {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

// logAsset 一次性资产事件日志
func logAsset(id, action string) {
	log.Printf("[operator] asset %s (id=%s)", action, id)
}
