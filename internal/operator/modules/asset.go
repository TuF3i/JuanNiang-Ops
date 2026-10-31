package modules

import (
	"time"

	"gorm.io/gorm"
)

// Asset 资产: agent/workflow 操作的基本单位, 聚合多个操作媒介与数据通道.
// 配置持久化到数据库; 仅 Enabled=true 的资产由 service 创建实例 (活动连接/客户端).
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

	// Media/Channels 媒介与通道配置: kind -> 对应 Config 结构体序列化后的 JSON.
	// 存 JSON 字符串由 service 解析 (ParseMediaConfig/ParseChannelConfig), 便于按 kind 扩展.
	Media    map[MediaKind]string    `gorm:"column:media;type:text;serializer:json;comment:操作媒介配置(MediaKind->Config JSON)"`
	Channels map[ChannelKind]string  `gorm:"column:channels;type:text;serializer:json;comment:数据通道配置(ChannelKind->Config JSON)"`
}

// TableName 资产表名
func (Asset) TableName() string { return "operator_assets" }

// AssetStatus 资产实例状态快照 (service.GetAssetStatus 组装)
type AssetStatus struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`  // DB 配置的启用开关
	Running bool   `json:"running"`  // 实例是否已创建 (enabled 且构建成功)

	// 实例实际可用的媒介/通道 (构建失败的 kind 不在其中, 优于整体不可用)
	MediaKinds   []MediaKind   `json:"media_kinds"`
	ChannelKinds []ChannelKind `json:"channel_kinds"`
}
