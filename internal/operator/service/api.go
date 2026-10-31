// 资产管理与统一操作 API: agent/workflow 的入口.
//
// 资产创建流程 (三步):
//  1. 创建操作媒介/数据源实体 (CreateMediaSource/CreateChannelSource)
//  2. 创建资产, 写明其描述信息 (CreateAsset)
//  3. 将媒介/数据源绑定到资产 (BindAssetMedia/BindAssetChannel) —— 支持跨资产共享,
//     如同机 K8s 资产与 PVE 资产共用同一个 SSH (共享语义 = 配置共享, 各资产独立连接)
//
// 管理方法委托 dao 并联动实例生命周期 (绑定/解绑/实体更新触发运行中资产重建);
// 操作方法经 route 选取媒介/通道后转发.
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

// ---------------------------------------------------------- 媒介/数据源管理 --

// CreateMediaSource 新建媒介实体: 校验配置可解析 → 填充 UUID → 落库 (与资产无耦合)
func (r *Service) CreateMediaSource(ctx context.Context, src *modules.MediaSource) error {
	if src.Name == "" {
		return fmt.Errorf("create media source -> %w: empty name", ErrInvalidAssetConfig)
	}
	if _, err := modules.ParseMediaConfig(src.Kind, src.Config); err != nil {
		return fmt.Errorf("create media source -> %w: %v", ErrInvalidAssetConfig, err)
	}
	if src.ID == "" {
		src.ID = uuid.NewString()
	}
	if err := r.dao.CreateMediaSource(ctx, src); err != nil {
		return fmt.Errorf("create media source -> %w", err)
	}
	logSource("media", src.ID, "created")
	return nil
}

// GetMediaSource 查询媒介实体 (未找到返回 ErrMediaSourceNotFound)
func (r *Service) GetMediaSource(ctx context.Context, id string) (*modules.MediaSource, error) {
	src, err := r.dao.GetMediaSource(ctx, id)
	if err != nil {
		return nil, r.translateMediaSourceNotFound(fmt.Errorf("get media source -> %w", err))
	}
	return src, nil
}

// ListMediaSources 媒介实体列表 (kind 非空时按类型过滤)
func (r *Service) ListMediaSources(ctx context.Context, kind modules.MediaKind) ([]modules.MediaSource, error) {
	sources, err := r.dao.ListMediaSources(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("list media sources -> %w", err)
	}
	return sources, nil
}

// UpdateMediaSource 更新媒介实体: sonic+MD5 对比 (无变化跳过) → 落库 → 重建绑定的运行中资产.
func (r *Service) UpdateMediaSource(ctx context.Context, dst *modules.MediaSource) error {
	src, err := r.dao.GetMediaSource(ctx, dst.ID)
	if err != nil {
		return r.translateMediaSourceNotFound(fmt.Errorf("update media source -> %w", err))
	}
	if _, err := modules.ParseMediaConfig(dst.Kind, dst.Config); err != nil {
		return fmt.Errorf("update media source -> %w: %v", ErrInvalidAssetConfig, err)
	}

	same, err := equal(src, dst)
	if err != nil {
		return fmt.Errorf("update media source -> %w", err)
	}
	if same {
		return nil
	}

	// 保留不可变列
	dst.CreatedAt = src.CreatedAt
	if err := r.dao.UpdateMediaSource(ctx, dst); err != nil {
		return fmt.Errorf("update media source -> %w", err)
	}

	// 反查绑定的运行中资产, 逐个重建 (媒介配置已变更)
	assetIDs, err := r.dao.ListAssetIDsByMediaSource(ctx, dst.ID)
	if err != nil {
		return fmt.Errorf("update media source -> %w", err)
	}
	r.recreateBoundAssets(ctx, assetIDs, "media source")
	logSource("media", dst.ID, "updated")
	return nil
}

// DeleteMediaSource 删除媒介实体: 仍被绑定时报 ErrMediaSourceInUse (先解绑再删).
func (r *Service) DeleteMediaSource(ctx context.Context, id string) error {
	assetIDs, err := r.dao.ListAssetIDsByMediaSource(ctx, id)
	if err != nil {
		return fmt.Errorf("delete media source -> %w", err)
	}
	if len(assetIDs) > 0 {
		return fmt.Errorf("delete media source %s -> %w (bound by %d assets)", id, ErrMediaSourceInUse, len(assetIDs))
	}
	if err := r.dao.DeleteMediaSource(ctx, id); err != nil {
		return r.translateMediaSourceNotFound(fmt.Errorf("delete media source -> %w", err))
	}
	logSource("media", id, "deleted")
	return nil
}

// CreateChannelSource 新建数据通道实体 (与 CreateMediaSource 同构)
func (r *Service) CreateChannelSource(ctx context.Context, src *modules.ChannelSource) error {
	if src.Name == "" {
		return fmt.Errorf("create channel source -> %w: empty name", ErrInvalidAssetConfig)
	}
	if _, err := modules.ParseChannelConfig(src.Kind, src.Config); err != nil {
		return fmt.Errorf("create channel source -> %w: %v", ErrInvalidAssetConfig, err)
	}
	if src.ID == "" {
		src.ID = uuid.NewString()
	}
	if err := r.dao.CreateChannelSource(ctx, src); err != nil {
		return fmt.Errorf("create channel source -> %w", err)
	}
	logSource("channel", src.ID, "created")
	return nil
}

// GetChannelSource 查询数据通道实体
func (r *Service) GetChannelSource(ctx context.Context, id string) (*modules.ChannelSource, error) {
	src, err := r.dao.GetChannelSource(ctx, id)
	if err != nil {
		return nil, r.translateChannelSourceNotFound(fmt.Errorf("get channel source -> %w", err))
	}
	return src, nil
}

// ListChannelSources 数据通道实体列表 (kind 非空时按类型过滤)
func (r *Service) ListChannelSources(ctx context.Context, kind modules.ChannelKind) ([]modules.ChannelSource, error) {
	sources, err := r.dao.ListChannelSources(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("list channel sources -> %w", err)
	}
	return sources, nil
}

// UpdateChannelSource 更新数据通道实体 (与 UpdateMediaSource 同构)
func (r *Service) UpdateChannelSource(ctx context.Context, dst *modules.ChannelSource) error {
	src, err := r.dao.GetChannelSource(ctx, dst.ID)
	if err != nil {
		return r.translateChannelSourceNotFound(fmt.Errorf("update channel source -> %w", err))
	}
	if _, err := modules.ParseChannelConfig(dst.Kind, dst.Config); err != nil {
		return fmt.Errorf("update channel source -> %w: %v", ErrInvalidAssetConfig, err)
	}

	same, err := equal(src, dst)
	if err != nil {
		return fmt.Errorf("update channel source -> %w", err)
	}
	if same {
		return nil
	}

	dst.CreatedAt = src.CreatedAt
	if err := r.dao.UpdateChannelSource(ctx, dst); err != nil {
		return fmt.Errorf("update channel source -> %w", err)
	}

	assetIDs, err := r.dao.ListAssetIDsByChannelSource(ctx, dst.ID)
	if err != nil {
		return fmt.Errorf("update channel source -> %w", err)
	}
	r.recreateBoundAssets(ctx, assetIDs, "channel source")
	logSource("channel", dst.ID, "updated")
	return nil
}

// DeleteChannelSource 删除数据通道实体 (在用拒绝)
func (r *Service) DeleteChannelSource(ctx context.Context, id string) error {
	assetIDs, err := r.dao.ListAssetIDsByChannelSource(ctx, id)
	if err != nil {
		return fmt.Errorf("delete channel source -> %w", err)
	}
	if len(assetIDs) > 0 {
		return fmt.Errorf("delete channel source %s -> %w (bound by %d assets)", id, ErrChannelSourceInUse, len(assetIDs))
	}
	if err := r.dao.DeleteChannelSource(ctx, id); err != nil {
		return r.translateChannelSourceNotFound(fmt.Errorf("delete channel source -> %w", err))
	}
	logSource("channel", id, "deleted")
	return nil
}

// ---------------------------------------------------------------- 绑定管理 --

// BindAssetMedia 将媒介实体绑定到资产 (幂等): 运行中资产立即重建以加载新媒介.
func (r *Service) BindAssetMedia(ctx context.Context, assetID, mediaSourceID string) error {
	if _, err := r.dao.GetAsset(ctx, assetID); err != nil {
		return r.translateAssetNotFound(fmt.Errorf("bind asset media -> %w", err))
	}
	if _, err := r.dao.GetMediaSource(ctx, mediaSourceID); err != nil {
		return r.translateMediaSourceNotFound(fmt.Errorf("bind asset media -> %w", err))
	}
	if err := r.dao.BindAssetMedia(ctx, assetID, mediaSourceID); err != nil {
		return fmt.Errorf("bind asset media -> %w", err)
	}

	r.mu.Lock()
	_, running := r.instances[assetID]
	r.mu.Unlock()
	if running {
		if err := r.reCreate(ctx, assetID); err != nil {
			return fmt.Errorf("bind asset media -> %w", err)
		}
	}
	log.Printf("[operator] media bound to asset (media=%s asset=%s)", mediaSourceID, assetID)
	return nil
}

// UnbindAssetMedia 解除媒介绑定 (幂等: 未绑定时 no-op); 运行中资产立即重建.
func (r *Service) UnbindAssetMedia(ctx context.Context, assetID, mediaSourceID string) error {
	if err := r.dao.UnbindAssetMedia(ctx, assetID, mediaSourceID); err != nil {
		return fmt.Errorf("unbind asset media -> %w", err)
	}

	r.mu.Lock()
	_, running := r.instances[assetID]
	r.mu.Unlock()
	if running {
		if err := r.reCreate(ctx, assetID); err != nil {
			return fmt.Errorf("unbind asset media -> %w", err)
		}
	}
	log.Printf("[operator] media unbound from asset (media=%s asset=%s)", mediaSourceID, assetID)
	return nil
}

// ListAssetMedia 资产绑定的媒介实体列表 (配置级查询, 与实例运行状态无关)
func (r *Service) ListAssetMedia(ctx context.Context, assetID string) ([]modules.MediaSource, error) {
	if _, err := r.dao.GetAsset(ctx, assetID); err != nil {
		return nil, r.translateAssetNotFound(fmt.Errorf("list asset media -> %w", err))
	}
	sources, err := r.dao.ListMediaSourcesByAsset(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("list asset media -> %w", err)
	}
	return sources, nil
}

// BindAssetChannel 将通道实体绑定到资产 (幂等, 与 BindAssetMedia 同构)
func (r *Service) BindAssetChannel(ctx context.Context, assetID, channelSourceID string) error {
	if _, err := r.dao.GetAsset(ctx, assetID); err != nil {
		return r.translateAssetNotFound(fmt.Errorf("bind asset channel -> %w", err))
	}
	if _, err := r.dao.GetChannelSource(ctx, channelSourceID); err != nil {
		return r.translateChannelSourceNotFound(fmt.Errorf("bind asset channel -> %w", err))
	}
	if err := r.dao.BindAssetChannel(ctx, assetID, channelSourceID); err != nil {
		return fmt.Errorf("bind asset channel -> %w", err)
	}

	r.mu.Lock()
	_, running := r.instances[assetID]
	r.mu.Unlock()
	if running {
		if err := r.reCreate(ctx, assetID); err != nil {
			return fmt.Errorf("bind asset channel -> %w", err)
		}
	}
	log.Printf("[operator] channel bound to asset (channel=%s asset=%s)", channelSourceID, assetID)
	return nil
}

// UnbindAssetChannel 解除通道绑定 (幂等)
func (r *Service) UnbindAssetChannel(ctx context.Context, assetID, channelSourceID string) error {
	if err := r.dao.UnbindAssetChannel(ctx, assetID, channelSourceID); err != nil {
		return fmt.Errorf("unbind asset channel -> %w", err)
	}

	r.mu.Lock()
	_, running := r.instances[assetID]
	r.mu.Unlock()
	if running {
		if err := r.reCreate(ctx, assetID); err != nil {
			return fmt.Errorf("unbind asset channel -> %w", err)
		}
	}
	log.Printf("[operator] channel unbound from asset (channel=%s asset=%s)", channelSourceID, assetID)
	return nil
}

// ListAssetChannels 资产绑定的通道实体列表
func (r *Service) ListAssetChannels(ctx context.Context, assetID string) ([]modules.ChannelSource, error) {
	if _, err := r.dao.GetAsset(ctx, assetID); err != nil {
		return nil, r.translateAssetNotFound(fmt.Errorf("list asset channels -> %w", err))
	}
	sources, err := r.dao.ListChannelSourcesByAsset(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("list asset channels -> %w", err)
	}
	return sources, nil
}

// ---------------------------------------------------------------- 资产管理 --

// CreateAsset 新建资产 (仅描述信息; 媒介/通道随后经绑定 API 挂载, 见包注释的三步流程)
func (r *Service) CreateAsset(ctx context.Context, asset *modules.Asset) error {
	if asset.Name == "" {
		return fmt.Errorf("create asset -> %w: empty name", ErrInvalidAssetConfig)
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
		return nil, r.translateAssetNotFound(fmt.Errorf("get asset -> %w", err))
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

// UpdateAsset 更新资产描述信息: sonic+MD5 对比 (无变化跳过) → 落库.
// 媒介/通道已实体化, 不再经由资产更新; 绑定变更走 Bind/Unbind API.
func (r *Service) UpdateAsset(ctx context.Context, dst *modules.Asset) error {
	src, err := r.dao.GetAsset(ctx, dst.ID)
	if err != nil {
		return r.translateAssetNotFound(fmt.Errorf("update asset -> %w", err))
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
	logAsset(dst.ID, "updated")
	return nil
}

// DeleteAsset 删除资产: 启用中先停用并关实例, 清理绑定, 再软删.
func (r *Service) DeleteAsset(ctx context.Context, id string) error {
	if _, err := r.dao.GetAsset(ctx, id); err != nil {
		return r.translateAssetNotFound(fmt.Errorf("delete asset -> %w", err))
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
		return nil, r.translateAssetNotFound(fmt.Errorf("get asset status -> %w", err))
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
		status.MediaKinds, status.ChannelKinds = ins.kinds()
	}
	return status, nil
}

// ---------------------------------------------------------------- 统一操作 --

// PingAsset 资产连通性检查: 逐个媒介/通道 Ping, 返回 "实体名" -> 错误 (nil 即可达).
// 资产未运行返回 ErrAssetNotRunning.
func (r *Service) PingAsset(ctx context.Context, assetID string) (map[string]error, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	return ins.Ping(ctx), nil
}

// Exec 在资产的媒介上执行命令.
// sourceID 精确选择 (同 kind 多绑定时必填); 为空则 kind 过滤后按优先级 (ssh → telnet) 自动选择.
func (r *Service) Exec(ctx context.Context, assetID string, mediaKind modules.MediaKind, sourceID, cmd string) (*modules.ExecResult, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	cr, err := ins.commandRunner(mediaKind, sourceID)
	if err != nil {
		return nil, err
	}
	result, err := cr.Exec(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("exec on %s/%s -> %w", assetID, mediaKind, err)
	}
	return result, nil
}

// QueryMetrics 瞬时指标查询 (PromQL)
func (r *Service) QueryMetrics(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID, query string, ts time.Time) ([]modules.MetricSample, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	mq, err := ins.metricsQuerier(channelKind, sourceID)
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
func (r *Service) QueryRange(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID, query string, start, end time.Time, step time.Duration) ([]modules.MetricSeries, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	mq, err := ins.metricsQuerier(channelKind, sourceID)
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
func (r *Service) QueryLogs(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID string, q modules.LogQuery) ([]modules.LogEntry, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	lq, err := ins.logQuerier(channelKind, sourceID)
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
func (r *Service) SearchTraces(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID string, q modules.TraceQuery) ([]modules.TraceSummary, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ts, err := ins.traceSearcher(channelKind, sourceID)
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
func (r *Service) GetTrace(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID, traceID string) ([]byte, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ts, err := ins.traceSearcher(channelKind, sourceID)
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
func (r *Service) SearchDocs(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID, keyword string, limit int) ([]modules.DocHit, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ds, err := ins.docSearcher(channelKind, sourceID)
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
func (r *Service) GetDoc(ctx context.Context, assetID string, channelKind modules.ChannelKind, sourceID, pageID string) (*modules.DocPage, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	ds, err := ins.docSearcher(channelKind, sourceID)
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
// 如 pve 的 VMOperator 虚机启停). sourceID 为空时仅单媒介资产可用. 未运行返回 ErrAssetNotRunning.
func (r *Service) Media(assetID string, kind modules.MediaKind, sourceID string) (media.Media, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	return ins.selectMedia(kind, sourceID)
}

// Channel 取资产运行中实例的通道引用 (调用方自行断言能力接口)
func (r *Service) Channel(assetID string, kind modules.ChannelKind, sourceID string) (channel.Channel, error) {
	ins, err := r.getInstance(assetID)
	if err != nil {
		return nil, err
	}
	return ins.selectChannel(kind, sourceID)
}

// ---------------------------------------------------------------- 内部辅助 --

// equal 对比两个值是否等价 (sonic 序列化 + MD5, 复刻 adapter 的同名实现)
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

// logSource 媒介/通道实体一次性事件日志
func logSource(kind, id, action string) {
	log.Printf("[operator] %s source %s (id=%s)", kind, action, id)
}
