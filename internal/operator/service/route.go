// 资产路由: 实例获取与媒介/通道选择.
// 选择参数 = kind (类型粗过滤, 可选) + sourceID (媒介/通道实体 ID 精确匹配, 可选):
//   - sourceID 指定 → 在实例中按实体 ID 命中 (kind 同时给出时仅作附加校验);
//     未绑定该实体报 ErrNoMedia/ErrNoChannel
//   - sourceID 为空 → 按 kind (可选) 与能力过滤, 剩多个按预定义优先级取首个, 仍多个报歧义
//
// 多绑定支持: 同类媒介在资产内可绑定多个 (如两台 SSH), 故精确选择以实体 ID 为准.
package service

import (
	"errors"

	"gorm.io/gorm"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// execPriority Exec 的媒介优先级 (sourceID 为空时按序取首个具备 CommandRunner 能力者)
var execPriority = []modules.MediaKind{modules.MediaSSH, modules.MediaTelnet}

// getInstance 取运行中实例.
// 无实例返回 ErrAssetNotRunning (涵盖未启用与构建失败两种情况; 是否存在可用 GetAsset 区分).
func (r *Service) getInstance(id string) (*AssetInstance, error) {
	r.mu.Lock()
	ins, ok := r.instances[id]
	r.mu.Unlock()
	if !ok {
		return nil, ErrAssetNotRunning
	}
	return ins, nil
}

// selectMedia 取媒介实例: sourceID 精确 → kind 过滤 → 单个自动 → 歧义报错
func (ins *AssetInstance) selectMedia(kind modules.MediaKind, sourceID string) (media.Media, error) {
	if sourceID != "" {
		for _, b := range ins.media {
			if b.source.ID == sourceID {
				return b.m, nil
			}
		}
		return nil, ErrNoMedia
	}

	candidates := ins.filterMedia(kind)
	switch len(candidates) {
	case 0:
		return nil, ErrNoMedia
	case 1:
		return candidates[0].m, nil
	default:
		return nil, ErrAmbiguousMedia
	}
}

// selectChannel 取通道实例 (与 selectMedia 同构)
func (ins *AssetInstance) selectChannel(kind modules.ChannelKind, sourceID string) (channel.Channel, error) {
	if sourceID != "" {
		for _, b := range ins.channels {
			if b.source.ID == sourceID {
				return b.c, nil
			}
		}
		return nil, ErrNoChannel
	}

	candidates := ins.filterChannels(kind)
	switch len(candidates) {
	case 0:
		return nil, ErrNoChannel
	case 1:
		return candidates[0].c, nil
	default:
		return nil, ErrAmbiguousChannel
	}
}

// filterMedia 按类型过滤媒介绑定 (kind 为空返回全部)
func (ins *AssetInstance) filterMedia(kind modules.MediaKind) []boundMedia {
	if kind == "" {
		return ins.media
	}
	out := make([]boundMedia, 0, len(ins.media))
	for _, b := range ins.media {
		if b.source.Kind == kind {
			out = append(out, b)
		}
	}
	return out
}

// filterChannels 按类型过滤通道绑定 (kind 为空返回全部)
func (ins *AssetInstance) filterChannels(kind modules.ChannelKind) []boundChannel {
	if kind == "" {
		return ins.channels
	}
	out := make([]boundChannel, 0, len(ins.channels))
	for _, b := range ins.channels {
		if b.source.Kind == kind {
			out = append(out, b)
		}
	}
	return out
}

// commandRunner 取命令执行媒介: sourceID 精确 / kind 过滤 / 优先级自动, 能力断言收口于此.
func (ins *AssetInstance) commandRunner(kind modules.MediaKind, sourceID string) (media.CommandRunner, error) {
	if sourceID != "" {
		m, err := ins.selectMedia(kind, sourceID)
		if err != nil {
			return nil, err
		}
		cr, ok := m.(media.CommandRunner)
		if !ok {
			return nil, ErrNoCapability
		}
		return cr, nil
	}

	var found media.CommandRunner
	count := 0
	for _, k := range execPriority {
		for _, b := range ins.filterMedia(k) {
			if cr, ok := b.m.(media.CommandRunner); ok {
				if count == 0 {
					found = cr
				}
				count++
			}
		}
	}
	switch {
	case count == 0:
		return nil, ErrNoCapability
	case count > 1:
		return nil, ErrAmbiguousMedia
	}
	return found, nil
}

// capabilityQuerier 通道能力选择的通用收口: 选通道 → 断言能力
func selectChannelCapability[T any](ins *AssetInstance, kind modules.ChannelKind, sourceID string, assert func(channel.Channel) (T, bool)) (T, error) {
	var zero T
	c, err := ins.selectChannel(kind, sourceID)
	if err != nil {
		return zero, err
	}
	capable, ok := assert(c)
	if !ok {
		return zero, ErrNoCapability
	}
	return capable, nil
}

// metricsQuerier 取指标查询通道
func (ins *AssetInstance) metricsQuerier(kind modules.ChannelKind, sourceID string) (channel.MetricsQuerier, error) {
	return selectChannelCapability(ins, kind, sourceID, func(c channel.Channel) (channel.MetricsQuerier, bool) {
		mq, ok := c.(channel.MetricsQuerier)
		return mq, ok
	})
}

// logQuerier 取日志查询通道
func (ins *AssetInstance) logQuerier(kind modules.ChannelKind, sourceID string) (channel.LogQuerier, error) {
	return selectChannelCapability(ins, kind, sourceID, func(c channel.Channel) (channel.LogQuerier, bool) {
		lq, ok := c.(channel.LogQuerier)
		return lq, ok
	})
}

// traceSearcher 取链路查询通道
func (ins *AssetInstance) traceSearcher(kind modules.ChannelKind, sourceID string) (channel.TraceSearcher, error) {
	return selectChannelCapability(ins, kind, sourceID, func(c channel.Channel) (channel.TraceSearcher, bool) {
		ts, ok := c.(channel.TraceSearcher)
		return ts, ok
	})
}

// docSearcher 取文档搜索通道
func (ins *AssetInstance) docSearcher(kind modules.ChannelKind, sourceID string) (channel.DocSearcher, error) {
	return selectChannelCapability(ins, kind, sourceID, func(c channel.Channel) (channel.DocSearcher, bool) {
		ds, ok := c.(channel.DocSearcher)
		return ds, ok
	})
}

// isNotFound 判断错误链上是否为 gorm 未找到/软删
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
