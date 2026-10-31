// 资产路由: 实例获取与媒介/通道选择.
// 语义仿 adapter 的 route: kind 指定则精确匹配, 为空则按预定义优先级在具备所需能力者中选择.
package service

import (
	"errors"

	"gorm.io/gorm"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// execPriority Exec 的媒介优先级 (kind 为空时按序取首个具备 CommandRunner 能力者)
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

// selectMedia 按类型取媒介; kind 为空且资产仅有一个媒介时自动选择 (多个报 ErrAmbiguousMedia)
func (ins *AssetInstance) selectMedia(kind modules.MediaKind) (media.Media, error) {
	if kind != "" {
		m, ok := ins.media[kind]
		if !ok {
			return nil, ErrNoMedia
		}
		return m, nil
	}
	switch len(ins.media) {
	case 0:
		return nil, ErrNoMedia
	case 1:
		for _, m := range ins.media {
			return m, nil
		}
	default:
		return nil, ErrAmbiguousMedia
	}
	return nil, ErrNoMedia
}

// selectChannel 按类型取通道 (与 selectMedia 同构)
func (ins *AssetInstance) selectChannel(kind modules.ChannelKind) (channel.Channel, error) {
	if kind != "" {
		c, ok := ins.channels[kind]
		if !ok {
			return nil, ErrNoChannel
		}
		return c, nil
	}
	switch len(ins.channels) {
	case 0:
		return nil, ErrNoChannel
	case 1:
		for _, c := range ins.channels {
			return c, nil
		}
	default:
		return nil, ErrAmbiguousChannel
	}
	return nil, ErrNoChannel
}

// commandRunner 取命令执行媒介: kind 指定精确匹配;
// 为空按 execPriority 顺序取首个具备 CommandRunner 能力者 (多个可选报 ErrAmbiguousMedia).
func (ins *AssetInstance) commandRunner(kind modules.MediaKind) (media.CommandRunner, error) {
	if kind != "" {
		m, ok := ins.media[kind]
		if !ok {
			return nil, ErrNoMedia
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
		m, ok := ins.media[k]
		if !ok {
			continue
		}
		if cr, ok := m.(media.CommandRunner); ok {
			if count == 0 {
				found = cr
			}
			count++
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

// metricsQuerier 取指标查询通道 (channelKind 为空时仅单通道资产可用, 语义同 selectChannel)
func (ins *AssetInstance) metricsQuerier(kind modules.ChannelKind) (channel.MetricsQuerier, error) {
	c, err := ins.selectChannel(kind)
	if err != nil {
		return nil, err
	}
	mq, ok := c.(channel.MetricsQuerier)
	if !ok {
		return nil, ErrNoCapability
	}
	return mq, nil
}

// logQuerier 取日志查询通道 (与 metricsQuerier 同构)
func (ins *AssetInstance) logQuerier(kind modules.ChannelKind) (channel.LogQuerier, error) {
	c, err := ins.selectChannel(kind)
	if err != nil {
		return nil, err
	}
	lq, ok := c.(channel.LogQuerier)
	if !ok {
		return nil, ErrNoCapability
	}
	return lq, nil
}

// traceSearcher 取链路查询通道 (与 metricsQuerier 同构)
func (ins *AssetInstance) traceSearcher(kind modules.ChannelKind) (channel.TraceSearcher, error) {
	c, err := ins.selectChannel(kind)
	if err != nil {
		return nil, err
	}
	ts, ok := c.(channel.TraceSearcher)
	if !ok {
		return nil, ErrNoCapability
	}
	return ts, nil
}

// docSearcher 取文档搜索通道 (与 metricsQuerier 同构)
func (ins *AssetInstance) docSearcher(kind modules.ChannelKind) (channel.DocSearcher, error) {
	c, err := ins.selectChannel(kind)
	if err != nil {
		return nil, err
	}
	ds, ok := c.(channel.DocSearcher)
	if !ok {
		return nil, ErrNoCapability
	}
	return ds, nil
}

// isNotFound 判断错误链上是否为 gorm 未找到/软删
func isNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
