// Package dao operator 数据访问层: 资产的持久化操作 (gorm).
// 与 internal/api/dao 同构: 只做数据库操作, 业务逻辑在 service 层.
package dao

import "gorm.io/gorm"

// Dao operator 数据访问对象
type Dao struct {
	db *gorm.DB
}

// NewDao 依赖注入: 数据库连接由上层装配
func NewDao(db *gorm.DB) *Dao {
	return &Dao{db: db}
}
