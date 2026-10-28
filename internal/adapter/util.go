// 通用工具: 配置对比等.
package adapter

import (
	"crypto/md5"
	"fmt"

	"github.com/bytedance/sonic"
)

// equal 对比两个配置对象是否一致 (sonic 序列化 + MD5).
// 任意一方序列化失败返回错误.
func (r *Runtime) equal(src, dst any) (bool, error) {
	srcRaw, err := sonic.Marshal(src)
	if err != nil {
		return false, fmt.Errorf("equal -> %w", err)
	}

	dstRaw, err := sonic.Marshal(dst)
	if err != nil {
		return false, fmt.Errorf("equal -> %w", err)
	}

	if md5.Sum(srcRaw) == md5.Sum(dstRaw) {
		return true, nil
	}

	return false, nil
}
