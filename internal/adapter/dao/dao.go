// Package dao 适配器数据访问层: 平台配置的建表 / 读取 / 更新 (gorm).
// 与 internal/api/dao 同构: 只做数据库操作, 业务逻辑在 service 层.
package dao

import "gorm.io/gorm"

// Dao 数据访问对象
type Dao struct {
	db *gorm.DB
}

// NewDao 依赖注入: gorm 连接由上层装配
func NewDao(db *gorm.DB) *Dao {
	return &Dao{db: db}
}
