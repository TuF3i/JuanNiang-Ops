// 资产实例: 启用资产的运行时持有物 (媒介连接与通道客户端的集合).
package service

import (
	"context"
	"log"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// AssetInstance 资产实例: 仅启用资产持有.
// 实例内无 goroutine (SSH/Telnet 为短连接, 其余为 HTTP 客户端), 故无 done/wg.
type AssetInstance struct {
	asset modules.Asset // 创建实例时的配置快照 (以实例为准, DB 为准绳)

	media    map[modules.MediaKind]media.Media
	channels map[modules.ChannelKind]channel.Channel
}

// buildInstance 从资产配置构建实例: 逐个构建媒介与通道.
// 单个构建失败跳过并记日志 (部分能力缺失优于整体不可用), 缺失 kind 查询时报 ErrNoMedia/ErrNoChannel.
func buildInstance(asset modules.Asset) *AssetInstance {
	ins := &AssetInstance{
		asset:    asset,
		media:    make(map[modules.MediaKind]media.Media, len(asset.Media)),
		channels: make(map[modules.ChannelKind]channel.Channel, len(asset.Channels)),
	}

	for kind, raw := range asset.Media {
		m, err := newMedia(kind, raw)
		if err != nil {
			log.Printf("[operator] asset %s build media %s: %v", asset.ID, kind, err)
			continue
		}
		ins.media[kind] = m
	}
	for kind, raw := range asset.Channels {
		c, err := newChannel(kind, raw)
		if err != nil {
			log.Printf("[operator] asset %s build channel %s: %v", asset.ID, kind, err)
			continue
		}
		ins.channels[kind] = c
	}
	return ins
}

// close 关闭实例持有的全部媒介与通道 (单个失败记日志不中断, Close 实现须幂等)
func (ins *AssetInstance) close() {
	for kind, m := range ins.media {
		if err := m.Close(); err != nil {
			log.Printf("[operator] asset %s close media %s: %v", ins.asset.ID, kind, err)
		}
	}
	for kind, c := range ins.channels {
		if err := c.Close(); err != nil {
			log.Printf("[operator] asset %s close channel %s: %v", ins.asset.ID, kind, err)
		}
	}
}

// Ping 连通性检查: 逐个媒介/通道 Ping, 返回各成员可达性 (单个失败不影响其余)
func (ins *AssetInstance) Ping(ctx context.Context) map[string]error {
	result := make(map[string]error, len(ins.media)+len(ins.channels))
	for kind, m := range ins.media {
		result[string(kind)] = m.Ping(ctx)
	}
	for kind, c := range ins.channels {
		result[string(kind)] = c.Ping(ctx)
	}
	return result
}
