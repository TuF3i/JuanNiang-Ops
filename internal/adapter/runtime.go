package adapter

import (
	"JuanNiang-Ops/internal/adapter/config"
	"JuanNiang-Ops/internal/adapter/driver/onebot"
	"context"
	"sync"
)

type DriverType string

const (
	OnebotDriver = "driver.onebot"
	FeishuDriver = "driver.feishu"
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
}
