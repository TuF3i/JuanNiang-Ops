package uerr

import "errors"

// 适配器状态与路由错误
var (
	ErrDisabled      = errors.New("onebot: adapter is disabled")
	ErrRunning       = errors.New("onebot: adapter is already running")
	ErrNotRunning    = errors.New("onebot: adapter is not running")
	ErrNoPeer        = errors.New("onebot: no client connected")
	ErrUnknownPeer   = errors.New("onebot: target client not connected")
	ErrAmbiguousPeer = errors.New("onebot: multiple clients connected, target required")
)
