package config

import (
	"time"

	"gorm.io/gorm"
)

const (
	defaultAddr          = "0.0.0.0:8090"
	defaultToken         = "JUAN_ONEBOT_TOKEN_1024"
	defaultEnable        = false
	defaultMessageBuffer = 256
	defaultActionBuffer  = 64
)

// defaultOneBotConfig OneBot 默认配置 (幂等种子行)
func defaultOneBotConfig() OneBotConfig {
	return OneBotConfig{
		ID:     1,
		Addr:   defaultAddr,
		Token:  defaultToken,
		Admin:  []string{},
		Enable: defaultEnable,

		MessageBuffer: defaultMessageBuffer,
		ActionBuffer:  defaultActionBuffer,
	}
}

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
