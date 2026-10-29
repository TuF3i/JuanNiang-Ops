// 用户业务逻辑: 登录校验 / 信息查询 / 修改密码. 密码只在这层接触明文, 出层即哈希.
package service

import (
	"context"
	"errors"
	"fmt"

	"JuanNiang-Ops/internal/api/dao"
	"JuanNiang-Ops/internal/api/modules"
)

// 可判定业务错误 (handler 按 errors.Is 映射 HTTP 状态码与提示)
var (
	ErrInvalidCredential = errors.New("api -> invalid username or password")
	ErrWrongPassword     = errors.New("api -> old password is incorrect")
)

// Login 登录校验: 用户存在 + 密码匹配; 任何一步失败统一返回 ErrInvalidCredential (防用户名枚举).
func (r *Service) Login(ctx context.Context, userName, password string) (*modules.User, error) {
	user, err := r.dao.GetUserInfo(ctx, userName)
	if err != nil {
		if errors.Is(err, dao.ErrUserNotFound) {
			return nil, ErrInvalidCredential
		}
		return nil, err
	}
	if err := VerifyPassword(user.Password, password); err != nil {
		return nil, ErrInvalidCredential
	}
	return user, nil
}

// GetUserInfo 查询用户信息 (密码哈希不出 Service 层)
func (r *Service) GetUserInfo(ctx context.Context, userName string) (*modules.User, error) {
	user, err := r.dao.GetUserInfo(ctx, userName)
	if err != nil {
		return nil, err
	}
	user.Password = "" // 脱敏
	return user, nil
}

// ChangePassword 修改密码: 校验旧密码 -> bcrypt 加密新密码 -> 落库.
func (r *Service) ChangePassword(ctx context.Context, userName, oldPassword, newPassword string) error {
	if newPassword == "" {
		return fmt.Errorf("api -> new password is empty")
	}

	user, err := r.dao.GetUserInfo(ctx, userName)
	if err != nil {
		return err
	}
	if err := VerifyPassword(user.Password, oldPassword); err != nil {
		return ErrWrongPassword
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := r.dao.UpdatePassword(ctx, userName, hash); err != nil {
		return err
	}
	return nil
}
