// 平台 Driver 配置模型 (gorm 持久化) 与默认值.
// dao 层负责读写, Service/Runtime 只消费模型.
package modules

import (
	"time"

	"gorm.io/gorm"
)

// ---------- OneBot ----------

// OneBot 配置默认值
const (
	DefaultAddr          = "0.0.0.0:8090"
	DefaultToken         = "JUAN_ONEBOT_TOKEN_1024"
	DefaultEnable        = false
	DefaultMessageBuffer = 256
	DefaultActionBuffer  = 64
)

// OneBotConfig OneBot Driver 配置 (正向 WebSocket 服务端, 等实现端接入)
type OneBotConfig struct {
	ID        int64          `gorm:"primaryKey;type:bigint;autoIncrement;comment:表自增主键ID"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:timestamp;index;comment:删除时间（软删除）"`

	Addr   string   `gorm:"column:addr;type:varchar(255);not null;uniqueIndex;comment:监听地址 host:port"`
	Token  string   `gorm:"column:token;type:varchar(512);not null;default:'';comment:接入令牌"`
	Admin  []string `gorm:"column:admin;type:text;serializer:json;comment:管理员列表(JSON)"`
	Enable bool     `gorm:"column:enable;not null;default:false;comment:是否启用"`

	MessageBuffer int `gorm:"column:message_buffer;not null;default:256;comment:接收消息管道缓冲大小"`
	ActionBuffer  int `gorm:"column:action_buffer;not null;default:64;comment:发送指令管道缓冲大小"`
}

// TableName 表名
func (OneBotConfig) TableName() string { return "onebot_configs" }

// DefaultOneBotConfig OneBot 默认配置 (幂等种子行)
func DefaultOneBotConfig() OneBotConfig {
	return OneBotConfig{
		ID:     1,
		Addr:   DefaultAddr,
		Token:  DefaultToken,
		Admin:  []string{},
		Enable: DefaultEnable,

		MessageBuffer: DefaultMessageBuffer,
		ActionBuffer:  DefaultActionBuffer,
	}
}

// ---------- 飞书 ----------

// 飞书 Driver 配置默认值
const (
	DefaultFeishuEventBuffer = 64
	DefaultFeishuEnable      = false
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

// DefaultFeishuConfig 飞书默认配置 (幂等种子行)
func DefaultFeishuConfig() FeishuConfig {
	return FeishuConfig{
		ID:          1,
		Enable:      DefaultFeishuEnable,
		EventBuffer: DefaultFeishuEventBuffer,
	}
}
