package config

import (
	"time"

	"gorm.io/gorm"
)

// 飞书 Driver 配置默认值
const (
	defaultFeishuEventBuffer = 64
	defaultFeishuEnable      = false
)

// FeishuConfig 飞书 Driver 配置 (WebSocket 长连接模式, 无需公网回调地址).
// 长连接由官方 SDK (larksuite/oapi-sdk-go) 建立, 事件主动推送到本地.
type FeishuConfig struct {
	ID        int64          `gorm:"primaryKey;type:bigint;autoIncrement;comment:表自增主键ID"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:timestamp;index;comment:删除时间（软删除）"`

	AppID             string `gorm:"column:app_id;type:varchar(255);not null;uniqueIndex;comment:飞书应用 App ID"`
	AppSecret         string `gorm:"column:app_secret;type:varchar(512);not null;default:'';comment:飞书应用 App Secret"`
	VerificationToken string `gorm:"column:verification_token;type:varchar(512);not null;default:'';comment:事件订阅验证 Token (长连接模式仅透传给 SDK, 可留空)"`
	Enable            bool   `gorm:"column:enable;not null;default:false;comment:是否启用"`

	EventBuffer int `gorm:"column:event_buffer;not null;default:64;comment:事件管道缓冲大小"`
}

// TableName 表名
func (FeishuConfig) TableName() string { return "feishu_configs" }

// defaultFeishuConfig 飞书默认配置 (幂等种子行)
func defaultFeishuConfig() FeishuConfig {
	return FeishuConfig{
		ID:          1,
		Enable:      defaultFeishuEnable,
		EventBuffer: defaultFeishuEventBuffer,
	}
}
