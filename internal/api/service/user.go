// 用户业务逻辑: 登录校验 / 信息查询 / 修改密码. 密码只在这层接触明文, 出层即哈希.
// 错误上抛模式: 调 dao 拿 (data, rsp), errors.Is(rsp, OperationSuccess) 判定, 业务错误原样上抛.
package service

import (
	"context"
	"errors"

	"JuanNiang-Ops/internal/api/modules"
	"JuanNiang-Ops/internal/api/response"
)

// Login 登录校验: 用户存在 + 密码匹配; 任何一步失败统一返回 UserNotExiOrWrongPwd (防用户名枚举).
func (r *Service) Login(ctx context.Context, userName, password string) (*modules.User, response.Response) {
	user, rsp := r.dao.GetUserInfo(ctx, userName)
	if !errors.Is(rsp, response.OperationSuccess) {
		// 用户不存在与系统错误在对外表现上统一 (防枚举), 但系统错误需保留细节上抛
		if errors.Is(rsp, response.UserNotExiOrWrongPwd) {
			return nil, response.UserNotExiOrWrongPwd
		}
		return nil, rsp
	}

	if err := VerifyPassword(user.Password, password); err != nil {
		return nil, response.UserNotExiOrWrongPwd
	}
	return user, response.OperationSuccess
}

// GetUserInfo 查询用户信息 (密码哈希不出 Service 层)
func (r *Service) GetUserInfo(ctx context.Context, userName string) (*modules.User, response.Response) {
	user, rsp := r.dao.GetUserInfo(ctx, userName)
	if !errors.Is(rsp, response.OperationSuccess) {
		return nil, rsp
	}
	user.Password = "" // 脱敏
	return user, response.OperationSuccess
}

// ChangePassword 修改密码: 校验旧密码 -> bcrypt 加密新密码 -> 落库.
func (r *Service) ChangePassword(ctx context.Context, userName, oldPassword, newPassword string) response.Response {
	if newPassword == "" {
		return response.EmptyNewPassword
	}

	user, rsp := r.dao.GetUserInfo(ctx, userName)
	if !errors.Is(rsp, response.OperationSuccess) {
		return rsp
	}
	if err := VerifyPassword(user.Password, oldPassword); err != nil {
		return response.WrongPassword
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return response.ServerInternalError(err)
	}
	return r.dao.UpdatePassword(ctx, userName, hash)
}
