// 事件泵: 消费适配器的 Inbound / Notices 管道, 分发给已注册的 handler.
package adapter

import (
	"context"
	"log"
	"sync"

	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/onebot"
)

// MessageHandler 统一消息处理器.
// ⚠️ 事件泵内同步调用, handler 应快速返回或自行开 goroutine, 否则会阻塞后续事件.
type MessageHandler func(ctx context.Context, msg common.Message)

// NoticeHandler 通知事件处理器 (notice / lifecycle, 原始 JSON 由调用方解析).
// ⚠️ 同 MessageHandler, 泵内同步调用.
type NoticeHandler func(ctx context.Context, ev onebot.UnionEvent)

// handlerSet handler 集合, RWMutex 保护 (运行时注册, 泵内并发读)
type handlerSet struct {
	mu      sync.RWMutex
	onMsg   []MessageHandler
	onNote  []NoticeHandler
}

// add 注册消息/通知 handler
func (h *handlerSet) addMsg(fn MessageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onMsg = append(h.onMsg, fn)
}

func (h *handlerSet) addNotice(fn NoticeHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onNote = append(h.onNote, fn)
}

// snapshot 取当前 handler 列表快照
func (h *handlerSet) snapshot() ([]MessageHandler, []NoticeHandler) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.onMsg, h.onNote
}

// dispatchMessage 逐个调用消息 handler, recover 防止单个 handler panic 打崩事件流
func (h *handlerSet) dispatchMessage(ctx context.Context, msg common.Message) {
	handlers, _ := h.snapshot()
	for _, fn := range handlers {
		h.safeCall(func() { fn(ctx, msg) })
	}
}

// dispatchNotice 逐个调用通知 handler
func (h *handlerSet) dispatchNotice(ctx context.Context, ev onebot.UnionEvent) {
	_, handlers := h.snapshot()
	for _, fn := range handlers {
		h.safeCall(func() { fn(ctx, ev) })
	}
}

// safeCall 带 recover 执行
func (h *handlerSet) safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[adapter] handler panic recovered: %v", r)
		}
	}()
	fn()
}

// pump 事件泵: 消费指定适配器的接收管道并分发, 直到管道关闭 (适配器停止).
// 由 runtime 在适配器启动后创建, 无需外部通知退出.
func (rt *AdapterRuntime) pump(adapter *onebot.Adapter) {
	defer rt.wg.Done()
	log.Printf("[adapter] event pump started")
	defer log.Printf("[adapter] event pump stopped")

	for {
		ctx := rt.ctx
		select {
		case msg, ok := <-adapter.Inbound():
			if !ok {
				return
			}
			rt.handlers.dispatchMessage(ctx, msg)
		case ev, ok := <-adapter.Notices():
			if !ok {
				return
			}
			rt.handlers.dispatchNotice(ctx, ev)
		case <-rt.done:
			// runtime 关闭时适配器会先被 Stop, 管道随之关闭; 此分支兜底退出
			return
		}
	}
}
