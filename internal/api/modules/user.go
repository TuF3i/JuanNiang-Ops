package modules

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        int64          `gorm:"primaryKey;type:bigint;autoIncrement;comment:表自增主键ID"`
	CreatedAt time.Time      `gorm:"column:created_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"column:updated_at;type:timestamp;not null;default:CURRENT_TIMESTAMP;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;type:timestamp;index;comment:删除时间（软删除）"`

	UserName string `gorm:"column:user_name;type:varchar(64);not null;uniqueIndex:uk_user_name;comment:用户名"`
	Password string `gorm:"column:password;type:varchar(255);not null;comment:密码（加密后存储）"`
}

func (User) TableName() string { return "user_table" }
