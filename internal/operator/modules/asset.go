package modules

import (
	"time"

	"gorm.io/gorm"
)

// Asset 资产: agent/workflow 操作的基本单位.
// 资产本身只承载描述信息 (文档/标签/元数据), 操作媒介与数据通道为独立实体 (MediaSource/ChannelSource),
// 经绑定关系挂到资产上 (支持跨资产共享, 如同机 K8s 资产与 PVE 资产共用同一个 SSH).
// 仅 Enabled=true 的资产由 service 创建实例 (持有媒介连接与通道客户端).
type Asset struct {
	ID        string         `gorm:"column:id;type:varchar(36);primaryKey;comment:资产UUID"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:timestamp;index;comment:删除时间（软删除）"`

	Name        string `gorm:"column:name;type:varchar(128);not null;uniqueIndex:uk_operator_asset_name;comment:资产名"`
	Description string `gorm:"column:description;type:varchar(512);not null;default:'';comment:资产描述"`
	Docs        string `gorm:"column:docs;type:text;comment:资产文档(markdown)"`
	Enabled     bool   `gorm:"column:enabled;not null;default:false;comment:是否启用(启用才创建实例)"`

	// Tags 标签列表; Metadata 元数据键值对 (JSON 列存储)
	Tags     []string          `gorm:"column:tags;type:text;serializer:json;comment:标签列表(JSON数组)"`
	Metadata map[string]string `gorm:"column:metadata;type:text;serializer:json;comment:元数据(JSON对象)"`
}

// TableName 资产表名
func (Asset) TableName() string { return "operator_assets" }

// AssetStatus 资产实例状态快照 (service.GetAssetStatus 组装)
type AssetStatus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"` // DB 配置的启用开关
	Running bool   `json:"running"` // 实例是否已创建 (enabled 且构建成功)

	// 实例实际可用的媒介/通道类型 (去重; 构建失败的条目不在其中, 部分能力缺失优于整体不可用)
	MediaKinds   []MediaKind   `json:"media_kinds"`
	ChannelKinds []ChannelKind `json:"channel_kinds"`
}

// MediaSource 操作媒介实体: 独立创建/命名, 经绑定挂到资产 (可被多个资产共享).
// 共享语义 v1 = 配置共享: 每个启用的资产各自创建独立连接/客户端, 不共享活动连接
// (避免引用计数; 未来如需连接池在此扩展).
type MediaSource struct {
	ID        string         `gorm:"column:id;type:varchar(36);primaryKey;comment:媒介UUID"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:timestamp;index;comment:删除时间（软删除）"`

	Name   string    `gorm:"column:name;type:varchar(128);not null;uniqueIndex:uk_operator_media_name;comment:媒介名(全局唯一)"`
	Kind   MediaKind `gorm:"column:kind;type:varchar(32);not null;comment:媒介类型(media.ssh/media.telnet/media.pve/media.k8s)"`
	Config string    `gorm:"column:config;type:text;comment:媒介配置(对应 Config 结构体 JSON)"`
}

// TableName 媒介实体表名
func (MediaSource) TableName() string { return "operator_media_sources" }

// ChannelSource 数据通道实体: 独立创建/命名, 经绑定挂到资产 (可被多个资产共享).
type ChannelSource struct {
	ID        string         `gorm:"column:id;type:varchar(36);primaryKey;comment:通道UUID"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:timestamp;index;comment:删除时间（软删除）"`

	Name   string      `gorm:"column:name;type:varchar(128);not null;uniqueIndex:uk_operator_channel_name;comment:通道名(全局唯一)"`
	Kind   ChannelKind `gorm:"column:kind;type:varchar(32);not null;comment:通道类型(channel.prometheus/loki/tempo/outline)"`
	Config string      `gorm:"column:config;type:text;comment:通道配置(对应 Config 结构体 JSON)"`
}

// TableName 通道实体表名
func (ChannelSource) TableName() string { return "operator_channel_sources" }

// AssetMediaBinding 资产-媒介绑定 (asset_id + media_source_id 联合唯一, 绑定幂等)
type AssetMediaBinding struct {
	ID        int64     `gorm:"primaryKey;type:bigint;autoIncrement;comment:表自增主键ID"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`

	AssetID       string `gorm:"column:asset_id;type:varchar(36);not null;uniqueIndex:uk_operator_asset_media,priority:1;comment:资产ID"`
	MediaSourceID string `gorm:"column:media_source_id;type:varchar(36);not null;uniqueIndex:uk_operator_asset_media,priority:2;comment:媒介实体ID"`
}

// TableName 资产-媒介绑定表名 (绑定行随资产/媒介删除硬删, 无软删)
func (AssetMediaBinding) TableName() string { return "operator_asset_media_bindings" }

// AssetChannelBinding 资产-通道绑定 (与 AssetMediaBinding 同构)
type AssetChannelBinding struct {
	ID        int64     `gorm:"primaryKey;type:bigint;autoIncrement;comment:表自增主键ID"`
	CreatedAt time.Time `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`

	AssetID         string `gorm:"column:asset_id;type:varchar(36);not null;uniqueIndex:uk_operator_asset_channel,priority:1;comment:资产ID"`
	ChannelSourceID string `gorm:"column:channel_source_id;type:varchar(36);not null;uniqueIndex:uk_operator_asset_channel,priority:2;comment:通道实体ID"`
}

// TableName 资产-通道绑定表名
func (AssetChannelBinding) TableName() string { return "operator_asset_channel_bindings" }
