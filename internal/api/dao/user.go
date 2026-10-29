package dao

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"JuanNiang-Ops/internal/api/modules"
)

// 默认管理员账号 (与前端 Mock 提示一致): admin / admin123
const (
	defaultAdminName     = "admin"
	defaultAdminPassword = "admin123"
)

// ErrUserNotFound 用户不存在 (handler 按 errors.Is 映射 HTTP 状态码)
var ErrUserNotFound = errors.New("api -> user not found")

// SetupUser 建表并幂等写入默认管理员 (admin / admin123, bcrypt 加密存储).
func (r *Dao) SetupUser(ctx context.Context) error {
	if err := r.db.WithContext(ctx).AutoMigrate(&modules.User{}); err != nil {
		return fmt.Errorf("api -> migrate user_table: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("api -> hash default password: %w", err)
	}
	seed := modules.User{
		ID:       1,
		UserName: defaultAdminName,
		Password: string(hash),
	}
	// Clauses 必须挂在 Create 之前, 否则 SQL 已执行、子句不生效 (幂等插入失败)
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return fmt.Errorf("api -> seed default admin: %w", err)
	}
	return nil
}

// GetUserInfo 按用户名查询用户 (含密码哈希, 脱敏由 Service 层负责)
func (r *Dao) GetUserInfo(ctx context.Context, userName string) (*modules.User, error) {
	var user modules.User
	err := r.db.WithContext(ctx).Where("user_name = ?", userName).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("api -> get user %s: %w", userName, err)
	}
	return &user, nil
}

// UpdatePassword 更新用户密码 (须传入 bcrypt 后的哈希)
func (r *Dao) UpdatePassword(ctx context.Context, userName, hashedPassword string) error {
	result := r.db.WithContext(ctx).Model(&modules.User{}).
		Where("user_name = ?", userName).
		Update("password", hashedPassword)
	if result.Error != nil {
		return fmt.Errorf("api -> update password for %s: %w", userName, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}
