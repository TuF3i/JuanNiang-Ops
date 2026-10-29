// 消息发送路由与配置对比工具.
package service

import (
	"crypto/md5"
	"fmt"

	"github.com/bytedance/sonic"
)

// DriverType 驱动类型标识
type DriverType string

// 驱动类型列表
const (
	OnebotDriver DriverType = "driver.onebot"
	FeishuDriver DriverType = "driver.feishu"
)

// route 消息发送路由: target 为机器人自身标识 self_id.
//   - target 为空: 默认走飞书驱动 (单机器人长连接, 无路由歧义)
//   - target 非空: 精确匹配 OneBot 已接入的机器人 (peer), 未接入或断开则报错
func (r *Service) route(target string) (DriverType, error) {
	if target == "" {
		if r.Feishu != nil && r.Feishu.driver.GetCurrentState().IsRunning {
			return FeishuDriver, nil
		}
		return "", fmt.Errorf("route -> feishu driver is not running")
	}

	if r.Onebot == nil {
		return "", fmt.Errorf("route -> onebot driver is not initialized")
	}
	st := r.Onebot.driver.GetCurrentState()
	if _, ok := st.PeerList[target]; ok {
		return OnebotDriver, nil
	}
	return "", fmt.Errorf("route -> peer %s not found or disconnected", target)
}

// equal 对比两个配置对象是否一致 (sonic 序列化 + MD5).
// 任意一方序列化失败返回错误.
func (r *Service) equal(src, dst any) (bool, error) {
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
