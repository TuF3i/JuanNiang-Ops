// 资产实例: 启用资产的运行时持有物 (媒介连接与通道客户端的集合).
package service

import (
	"context"
	"log"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// boundMedia 已构建的媒介: 来源实体 + 运行时客户端.
// 同类媒介在资产内可多绑定 (map 按 kind 无法承载), 故实例持切片.
type boundMedia struct {
	source modules.MediaSource
	m      media.Media
}

// boundChannel 已构建的通道 (与 boundMedia 同构)
type boundChannel struct {
	source modules.ChannelSource
	c      channel.Channel
}

// AssetInstance 资产实例: 仅启用资产持有.
// 实例内无 goroutine (SSH/Telnet 为短连接, 其余为 HTTP 客户端), 故无 done/wg.
// 共享语义 v1 = 配置共享: 绑定同一媒介实体的多个资产各自持有独立连接/客户端.
type AssetInstance struct {
	asset modules.Asset // 创建实例时的配置快照 (以实例为准, DB 为准绳)

	media    []boundMedia
	channels []boundChannel
}

// buildInstance 从资产绑定的媒介/通道实体构建实例 (service 方法: 需反查 DAO).
// 单个构建失败跳过并记日志 (部分能力缺失优于整体不可用), 缺失条目查询时报 ErrNoMedia/ErrNoChannel.
func (r *Service) buildInstance(ctx context.Context, asset modules.Asset) *AssetInstance {
	ins := &AssetInstance{asset: asset}

	sources, err := r.dao.ListMediaSourcesByAsset(ctx, asset.ID)
	if err != nil {
		log.Printf("[operator] asset %s load media sources: %v", asset.ID, err)
	}
	for i := range sources {
		m, err := newMedia(sources[i].Kind, sources[i].Config)
		if err != nil {
			log.Printf("[operator] asset %s build media %s(%s): %v", asset.ID, sources[i].Name, sources[i].Kind, err)
			continue
		}
		ins.media = append(ins.media, boundMedia{source: sources[i], m: m})
	}

	channelSources, err := r.dao.ListChannelSourcesByAsset(ctx, asset.ID)
	if err != nil {
		log.Printf("[operator] asset %s load channel sources: %v", asset.ID, err)
	}
	for i := range channelSources {
		c, err := newChannel(channelSources[i].Kind, channelSources[i].Config)
		if err != nil {
			log.Printf("[operator] asset %s build channel %s(%s): %v", asset.ID, channelSources[i].Name, channelSources[i].Kind, err)
			continue
		}
		ins.channels = append(ins.channels, boundChannel{source: channelSources[i], c: c})
	}
	return ins
}

// close 关闭实例持有的全部媒介与通道 (单个失败记日志不中断, Close 实现须幂等)
func (ins *AssetInstance) close() {
	for _, b := range ins.media {
		if err := b.m.Close(); err != nil {
			log.Printf("[operator] asset %s close media %s: %v", ins.asset.ID, b.source.Name, err)
		}
	}
	for _, b := range ins.channels {
		if err := b.c.Close(); err != nil {
			log.Printf("[operator] asset %s close channel %s: %v", ins.asset.ID, b.source.Name, err)
		}
	}
}

// Ping 连通性检查: 逐个媒介/通道 Ping, 返回 "媒介名/通道名" -> 错误 (nil 即可达)
func (ins *AssetInstance) Ping(ctx context.Context) map[string]error {
	result := make(map[string]error, len(ins.media)+len(ins.channels))
	for _, b := range ins.media {
		result[b.source.Name] = b.m.Ping(ctx)
	}
	for _, b := range ins.channels {
		result[b.source.Name] = b.c.Ping(ctx)
	}
	return result
}

// kinds 实例实际可用的媒介/通道类型 (去重, 供状态快照)
func (ins *AssetInstance) kinds() (mediaKinds []modules.MediaKind, channelKinds []modules.ChannelKind) {
	seenMedia := make(map[modules.MediaKind]struct{}, len(ins.media))
	for _, b := range ins.media {
		if _, ok := seenMedia[b.source.Kind]; !ok {
			seenMedia[b.source.Kind] = struct{}{}
			mediaKinds = append(mediaKinds, b.source.Kind)
		}
	}
	seenChannel := make(map[modules.ChannelKind]struct{}, len(ins.channels))
	for _, b := range ins.channels {
		if _, ok := seenChannel[b.source.Kind]; !ok {
			seenChannel[b.source.Kind] = struct{}{}
			channelKinds = append(channelKinds, b.source.Kind)
		}
	}
	return mediaKinds, channelKinds
}
