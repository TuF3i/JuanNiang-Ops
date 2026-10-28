package adapter

import (
	"testing"
	"time"

	"JuanNiang-Ops/internal/adapter/common"
	"JuanNiang-Ops/internal/adapter/driver"
)

// fakeDriver 测试用假驱动: 手动投递事件, 手动关闭
type fakeDriver struct {
	events chan common.Event
	done   chan struct{}
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{
		events: make(chan common.Event, 4),
		done:   make(chan struct{}),
	}
}

func (f *fakeDriver) Start() error                { return nil }
func (f *fakeDriver) Stop() error                 { close(f.done); close(f.events); return nil }
func (f *fakeDriver) Done() <-chan struct{}       { return f.done }
func (f *fakeDriver) Events() <-chan common.Event { return f.events }
func (f *fakeDriver) GetCurrentState() *common.RuntimeState {
	return &common.RuntimeState{IsRunning: true}
}

// newTestRuntime 构造最小可测 Runtime (不经 CreateRuntime, 不碰数据库)
func newTestRuntime() *Runtime {
	return &Runtime{
		events: make(chan common.Event, 16),
		done:   make(chan struct{}),
	}
}

func TestForwardPumpRelaysEvents(t *testing.T) {
	rt := newTestRuntime()
	fake := newFakeDriver()
	rt.startForward("fake", driver.Driver(fake))

	// 注入消息与通知事件
	fake.events <- common.Event{Type: common.EventMessage, SelfID: "1", Message: &common.Message{MessageID: "m1"}}
	fake.events <- common.Event{Type: common.EventNotify, SelfID: "1", Notify: &common.NotifyEvent{Kind: "event.notify"}}

	// 均经转发泵到达 Runtime 管道
	got := make([]common.Event, 0, 2)
	timeout := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-rt.Events():
			got = append(got, ev)
		case <-timeout:
			t.Fatalf("timeout, got %d events", len(got))
		}
	}
	if got[0].Type != common.EventMessage || got[0].Message.MessageID != "m1" {
		t.Errorf("event0 = %+v", got[0])
	}
	if got[1].Type != common.EventNotify || got[1].Notify.Kind != "event.notify" {
		t.Errorf("event1 = %+v", got[1])
	}
}

func TestForwardPumpExitsOnDriverStop(t *testing.T) {
	rt := newTestRuntime()
	fake := newFakeDriver()
	rt.startForward("fake", driver.Driver(fake))

	// 驱动停止: 管道关闭 -> 泵退出, Runtime 管道保持可用
	if err := fake.Stop(); err != nil {
		t.Fatalf("stop fake: %v", err)
	}

	// wg 归零后 ReleaseRuntime 不应卡死
	rt.wg.Wait()
	if err := rt.ReleaseRuntime(); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Release 后事件管道关闭
	select {
	case _, ok := <-rt.Events():
		if ok {
			t.Error("events should be closed after release")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting events close")
	}
}

func TestReleaseRuntimeIdempotent(t *testing.T) {
	rt := newTestRuntime()

	if err := rt.ReleaseRuntime(); err != nil {
		t.Fatalf("release: %v", err)
	}
	// 二次释放幂等
	if err := rt.ReleaseRuntime(); err != nil {
		t.Fatalf("double release: %v", err)
	}
}
