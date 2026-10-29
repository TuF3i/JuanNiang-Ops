package adapter

import (
	"testing"
	"time"

	"JuanNiang-Ops/internal/adapter/dao"
	"JuanNiang-Ops/internal/adapter/driver"
	"JuanNiang-Ops/internal/adapter/modules"
	"JuanNiang-Ops/internal/adapter/service"
)

// fakeDriver 测试用假驱动: 手动投递事件, 手动关闭
type fakeDriver struct {
	events chan modules.Event
	done   chan struct{}
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{
		events: make(chan modules.Event, 4),
		done:   make(chan struct{}),
	}
}

func (f *fakeDriver) Start() error                { return nil }
func (f *fakeDriver) Stop() error                 { close(f.done); close(f.events); return nil }
func (f *fakeDriver) Done() <-chan struct{}       { return f.done }
func (f *fakeDriver) Events() <-chan modules.Event { return f.events }
func (f *fakeDriver) GetCurrentState() *modules.RuntimeState {
	return &modules.RuntimeState{IsRunning: true}
}

// newTestService 构造最小可测 Service (不经 CreateRuntime, 不碰数据库)
func newTestService() *service.Service {
	return service.NewService(dao.NewDao(nil))
}

func TestForwardPumpRelaysEvents(t *testing.T) {
	svc := newTestService()
	fake := newFakeDriver()
	svc.StartForward("fake", driver.Driver(fake))

	// 注入消息与通知事件
	fake.events <- modules.Event{Type: modules.EventMessage, SelfID: "1", Message: &modules.Message{MessageID: "m1"}}
	fake.events <- modules.Event{Type: modules.EventNotify, SelfID: "1", Notify: &modules.NotifyEvent{Kind: "event.notify"}}

	// 均经转发泵到达 Service 管道
	got := make([]modules.Event, 0, 2)
	timeout := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-svc.Events():
			got = append(got, ev)
		case <-timeout:
			t.Fatalf("timeout, got %d events", len(got))
		}
	}
	if got[0].Type != modules.EventMessage || got[0].Message.MessageID != "m1" {
		t.Errorf("event0 = %+v", got[0])
	}
	if got[1].Type != modules.EventNotify || got[1].Notify.Kind != "event.notify" {
		t.Errorf("event1 = %+v", got[1])
	}
}

func TestForwardPumpExitsOnDriverStop(t *testing.T) {
	svc := newTestService()
	fake := newFakeDriver()
	svc.StartForward("fake", driver.Driver(fake))

	// 驱动停止: 管道关闭 -> 泵退出, Service 管道保持可用
	if err := fake.Stop(); err != nil {
		t.Fatalf("stop fake: %v", err)
	}

	// wg 归零后 Release 不应卡死
	svc.Wait()
	if err := svc.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Release 后事件管道关闭
	select {
	case _, ok := <-svc.Events():
		if ok {
			t.Error("events should be closed after release")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting events close")
	}
}

func TestReleaseIdempotent(t *testing.T) {
	svc := newTestService()

	if err := svc.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	// 二次释放幂等
	if err := svc.Release(); err != nil {
		t.Fatalf("double release: %v", err)
	}
}
