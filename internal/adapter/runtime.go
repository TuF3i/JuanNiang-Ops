package adapter

import (
	"JuanNiang-Ops/internal/adapter/config"
	"JuanNiang-Ops/internal/adapter/driver/onebot"
	"context"
	"fmt"
	"sync"
)

type DriverType string

type configKind interface {
	config.OneBotConfig
}

const (
	OnebotDriver DriverType = "driver.onebot"
	FeishuDriver DriverType = "driver.feishu"
)

type OnebotDriverV11 struct {
	driver *onebot.Driver
	cfg    config.OneBotConfig

	restSignal chan<- struct{}
	ctx        context.Context
	cancel     context.CancelFunc
}

type Runtime struct {
	ConfigManager *config.ConfigManager
	OnebotDriver  *OnebotDriverV11

	mu sync.Mutex

	// eventChan chan <- common.Event
}

// CreateRuntime 装配适配器运行时 (ConfigManager + 各平台 Driver).
// TODO: 待 infrastructure 提供 db 连接后实现装配; 当前为占位, 返回错误而非 nil 以避免调用方空指针.
func CreateRuntime() (*Runtime, error) {
	return nil, fmt.Errorf("adapter -> CreateRuntime not implemented")
}

// ReleaseRuntime 释放运行时持有的 driver 与底层资源.
// TODO: 实现 driver 的 Stop 与连接回收.
func ReleaseRuntime() error {
	return nil
}
