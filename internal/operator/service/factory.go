// 媒介与通道工厂: 按类型标识构建具体实现 (各集成实现逐个挂接于此).
// 包含: 集成构建 switch / 默认值修补.
package service

import (
	"fmt"
	"time"

	"JuanNiang-Ops/internal/operator/channel"
	lokichannel "JuanNiang-Ops/internal/operator/channel/loki"
	outlinechannel "JuanNiang-Ops/internal/operator/channel/outline"
	promchannel "JuanNiang-Ops/internal/operator/channel/prometheus"
	tempochannel "JuanNiang-Ops/internal/operator/channel/tempo"
	k8smedia "JuanNiang-Ops/internal/operator/media/k8s"
	pvemedia "JuanNiang-Ops/internal/operator/media/pve"
	sshmedia "JuanNiang-Ops/internal/operator/media/ssh"
	telnetmedia "JuanNiang-Ops/internal/operator/media/telnet"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// newMedia 按类型构建操作媒介 (rawCfg 为对应 Config 结构体的 JSON, 空串取全默认值)
func newMedia(kind modules.MediaKind, rawCfg string) (media.Media, error) {
	parsed, err := modules.ParseMediaConfig(kind, rawCfg)
	if err != nil {
		return nil, fmt.Errorf("new media -> %w", err)
	}
	switch cfg := parsed.(type) {
	case *modules.SSHConfig:
		return sshmedia.New(*cfg)
	case *modules.TelnetConfig:
		return telnetmedia.New(*cfg)
	case *modules.PVEConfig:
		return pvemedia.New(*cfg)
	case *modules.K8sConfig:
		return k8smedia.New(*cfg)
	default:
		return nil, fmt.Errorf("new media -> %w: %s", ErrUnknownMediaKind, kind)
	}
}

// newChannel 按类型构建数据通道 (与 newMedia 同构)
func newChannel(kind modules.ChannelKind, rawCfg string) (channel.Channel, error) {
	parsed, err := modules.ParseChannelConfig(kind, rawCfg)
	if err != nil {
		return nil, fmt.Errorf("new channel -> %w", err)
	}
	switch cfg := parsed.(type) {
	case *modules.PrometheusConfig:
		return promchannel.New(*cfg)
	case *modules.LokiConfig:
		return lokichannel.New(*cfg)
	case *modules.TempoConfig:
		return tempochannel.New(*cfg)
	case *modules.OutlineConfig:
		return outlinechannel.New(*cfg)
	default:
		return nil, fmt.Errorf("new channel -> %w: %s", ErrUnknownChannelKind, kind)
	}
}

// timeoutMs 配置的超时毫秒数 (0/负值取默认)
func timeoutMs(ms int) time.Duration {
	if ms <= 0 {
		return modules.DefaultTimeout
	}
	return time.Duration(ms) * time.Millisecond
}
