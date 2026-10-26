// Package adapter 定义平台适配器的统一约定.
//
// 每个平台适配器 (如 internal/adapter/onebot) 需要实现:
//   - Start / Stop: 生命周期管理
//   - Inbound:      接收消息管道 (平台事件 -> 统一消息 common.Message)
//   - Outbound:     发送指令管道 (平台指令 -> 平台实现端)
//   - Done:         停止信号, 配合管道一起消费
package adapter

import (
	"JuanNiang-Ops/internal/adapter/onebot"

	"gorm.io/gorm"
)

type AdapterRuntime struct {
	OneBotAdapter *onebot.Adapter
	// TODO LarkAdapter

	db *gorm.DB
}
