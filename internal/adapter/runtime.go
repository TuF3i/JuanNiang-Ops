package adapter

import (
	"JuanNiang-Ops/internal/adapter/config"
	"JuanNiang-Ops/internal/adapter/driver/onebot"
	"context"
	"sync"
)

type OnebotDriverV11 struct {
	driver *onebot.Driver
	cfg    *onebot.Config
}

type Runtime struct {
	ConfigManager *config.ConfigManager
	OnebotDriver  *OnebotDriverV11

	mu         sync.Mutex
	restSignal chan<- struct{}
	ctx        context.Context
	cancel     context.CancelFunc
}
