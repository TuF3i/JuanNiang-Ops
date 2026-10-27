package adapter

import (
	"time"

	"gorm.io/gorm"
)

// Onebot11 适配器配置
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

// Onebot11 表名
func (OneBotConfig) TableName() string { return "onebot_configs" }

// ---------- gorm 钩子: 配置变更 -> 通知 runtime 重建适配器 ----------
// 只挂 Create/Update/Delete 三个钩子 (Save 内部为 Create 或 Update, 不会双触发).
// ⚠️ 批量更新不带主键不触发 AfterUpdate, 更新配置必须带主键, 详见 hooks.go 包注释.

// AfterCreate 创建配置后通知
func (OneBotConfig) AfterCreate(*gorm.DB) error {
	notifyConfigChanged()
	return nil
}

// AfterUpdate 更新配置后通知
func (OneBotConfig) AfterUpdate(*gorm.DB) error {
	notifyConfigChanged()
	return nil
}

// AfterDelete 删除配置后通知
func (OneBotConfig) AfterDelete(*gorm.DB) error {
	notifyConfigChanged()
	return nil
}
