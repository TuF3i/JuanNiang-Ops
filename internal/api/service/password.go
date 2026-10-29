// 密码加密: bcrypt 单向哈希 (数据库只存哈希, 不存明文).
package service

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 明文密码 -> bcrypt 哈希 (默认 cost 10, 60 字符)
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("api -> hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword 校验明文密码与哈希是否匹配; 不匹配返回 error (bcrypt.ErrMismatchedHashAndPassword)
func VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
