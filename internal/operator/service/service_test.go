// operator service 测试:
//   - 路由/能力断言: 纯内存 (stub 媒介/通道直填实例表, 不经数据库)
//   - 生命周期/资产管理: 依赖 TEST_PG_DSN, 未设置自动 Skip (仓库惯例)
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/dao"
	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// ---------------------------------------------------------------- 测试桩 --

// stubMedia 仅实现 Media 接口 (无操作能力)
type stubMedia struct{ kind modules.MediaKind }

func (m *stubMedia) Kind() modules.MediaKind      { return m.kind }
func (m *stubMedia) Ping(ctx context.Context) error { return nil }
func (m *stubMedia) Close() error                   { return nil }

// stubRunner Media + CommandRunner 能力
type stubRunner struct {
	stubMedia
	wantCmd string
}

func (m *stubRunner) Exec(ctx context.Context, cmd string) (*modules.ExecResult, error) {
	if m.wantCmd != "" && cmd != m.wantCmd {
		return nil, fmt.Errorf("unexpected cmd %q", cmd)
	}
	return &modules.ExecResult{ExitCode: 0, Stdout: "ok"}, nil
}

// stubChannel 仅实现 Channel 接口 (无查询能力)
type stubChannel struct{ kind modules.ChannelKind }

func (c *stubChannel) Kind() modules.ChannelKind      { return c.kind }
func (c *stubChannel) Ping(ctx context.Context) error { return nil }
func (c *stubChannel) Close() error                   { return nil }

// stubLogQuerier Channel + LogQuerier 能力
type stubLogQuerier struct {
	stubChannel
	entries []modules.LogEntry
}

func (c *stubLogQuerier) QueryLogs(ctx context.Context, q modules.LogQuery) ([]modules.LogEntry, error) {
	return c.entries, nil
}

// newTestService 构造注入指定实例的 Service (不经 CreateRuntime, 不碰数据库)
func newTestService(instances map[string]*AssetInstance) *Service {
	svc := NewService(dao.NewDao(nil))
	for id, ins := range instances {
		svc.instances[id] = ins
	}
	return svc
}

// newRunnerInstance 单媒介 (CommandRunner) + 单通道 (LogQuerier) 实例
func newRunnerInstance(assetID string) *AssetInstance {
	return &AssetInstance{
		asset: modules.Asset{ID: assetID},
		media: map[modules.MediaKind]media.Media{
			modules.MediaSSH: &stubRunner{stubMedia: stubMedia{modules.MediaSSH}},
		},
		channels: map[modules.ChannelKind]channel.Channel{
			modules.ChannelLoki: &stubLogQuerier{stubChannel: stubChannel{modules.ChannelLoki}, entries: []modules.LogEntry{{Line: "hello"}}},
		},
	}
}

// ---------------------------------------------------------------- 路由测试 --

func TestExecRouting(t *testing.T) {
	svc := newTestService(map[string]*AssetInstance{"a1": newRunnerInstance("a1")})

	// kind 为空: 单一 CommandRunner 自动选择
	result, err := svc.Exec(context.Background(), "a1", "", "uptime")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "ok" {
		t.Fatalf("stdout = %q", result.Stdout)
	}

	// kind 指定: 精确匹配
	if _, err := svc.Exec(context.Background(), "a1", modules.MediaSSH, "uptime"); err != nil {
		t.Fatal(err)
	}

	// 资产未运行
	if _, err := svc.Exec(context.Background(), "missing", "", "x"); !errors.Is(err, ErrAssetNotRunning) {
		t.Fatalf("want ErrAssetNotRunning, got %v", err)
	}

	// kind 不存在
	if _, err := svc.Exec(context.Background(), "a1", modules.MediaPVE, "x"); !errors.Is(err, ErrNoMedia) {
		t.Fatalf("want ErrNoMedia, got %v", err)
	}

	// kind 存在但无 CommandRunner 能力
	svc.instances["a1"].media[modules.MediaPVE] = &stubMedia{modules.MediaPVE}
	if _, err := svc.Exec(context.Background(), "a1", modules.MediaPVE, "x"); !errors.Is(err, ErrNoCapability) {
		t.Fatalf("want ErrNoCapability, got %v", err)
	}
}

func TestExecAmbiguous(t *testing.T) {
	ins := newRunnerInstance("a1")
	ins.media[modules.MediaTelnet] = &stubRunner{stubMedia: stubMedia{modules.MediaTelnet}}
	svc := newTestService(map[string]*AssetInstance{"a1": ins})

	// 两个 CommandRunner 且未指定 kind → 歧义
	if _, err := svc.Exec(context.Background(), "a1", "", "x"); !errors.Is(err, ErrAmbiguousMedia) {
		t.Fatalf("want ErrAmbiguousMedia, got %v", err)
	}
	// 指定 kind 则正常
	if _, err := svc.Exec(context.Background(), "a1", modules.MediaTelnet, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestQueryLogsRouting(t *testing.T) {
	svc := newTestService(map[string]*AssetInstance{"a1": newRunnerInstance("a1")})

	entries, err := svc.QueryLogs(context.Background(), "a1", "", modules.LogQuery{Selector: `{job="x"}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Line != "hello" {
		t.Fatalf("entries = %+v", entries)
	}

	// 无该通道
	if _, err := svc.QueryLogs(context.Background(), "a1", modules.ChannelTempo, modules.LogQuery{Selector: "x"}); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("want ErrNoChannel, got %v", err)
	}

	// 有该类型但无 LogQuerier 能力
	svc.instances["a1"].channels[modules.ChannelTempo] = &stubChannel{modules.ChannelTempo}
	if _, err := svc.QueryLogs(context.Background(), "a1", modules.ChannelTempo, modules.LogQuery{Selector: "x"}); !errors.Is(err, ErrNoCapability) {
		t.Fatalf("want ErrNoCapability, got %v", err)
	}
}

func TestMediaAccessor(t *testing.T) {
	svc := newTestService(map[string]*AssetInstance{"a1": newRunnerInstance("a1")})

	m, err := svc.Media("a1", modules.MediaSSH)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != modules.MediaSSH {
		t.Fatalf("kind = %s", m.Kind())
	}
	if _, err := svc.Media("a1", modules.MediaPVE); !errors.Is(err, ErrNoMedia) {
		t.Fatalf("want ErrNoMedia, got %v", err)
	}
}

// ---------------------------------------------------------------- 生命周期测试 --

// newTestDao 连接测试库并建表 (TEST_PG_DSN 未设置则 Skip)
func newTestDao(t *testing.T) *dao.Dao {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skip db-backed operator test")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("connect test db: %v", err)
	}
	d := dao.NewDao(db)
	if err := d.SetupAssets(context.Background()); err != nil {
		t.Fatalf("setup assets: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return d
}

// testAsset 离线可构建的测试资产 (ssh/loki 的 New 只校验配置不建连)
func testAsset(name string) *modules.Asset {
	return &modules.Asset{
		Name:  name,
		Docs:  "# 测试资产\n仅用于单测",
		Tags:  []string{"test", "operator"},
		Media: map[modules.MediaKind]string{modules.MediaSSH: `{"host":"127.0.0.1","user":"root","password":"x"}`},
		Channels: map[modules.ChannelKind]string{
			modules.ChannelLoki: `{"endpoint":"http://127.0.0.1:3100"}`,
		},
	}
}

func TestAssetLifecycle(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	asset := testAsset(fmt.Sprintf("lifecycle-%d", time.Now().UnixNano()))

	// 创建: UUID 自动填充, 默认停用
	if err := svc.CreateAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	if asset.ID == "" {
		t.Fatal("uuid not filled")
	}
	t.Cleanup(func() { _ = svc.DeleteAsset(ctx, asset.ID) })

	// 启用: 实例创建
	if err := svc.EnableAsset(ctx, asset.ID); err != nil {
		t.Fatal(err)
	}
	status, err := svc.GetAssetStatus(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.Running {
		t.Fatalf("status mismatch: %+v", status)
	}
	if len(status.MediaKinds) != 1 || status.MediaKinds[0] != modules.MediaSSH {
		t.Fatalf("media kinds = %v", status.MediaKinds)
	}

	// Update 无变化: 跳过且实例仍在
	src, err := svc.GetAsset(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateAsset(ctx, src); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, asset.ID); !status.Running {
		t.Fatal("instance lost after no-change update")
	}

	// Update 有变化: 重建
	src.Description = "changed"
	if err := svc.UpdateAsset(ctx, src); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, asset.ID); !status.Running {
		t.Fatal("instance lost after recreate")
	}

	// 停用: 实例移除
	if err := svc.DisableAsset(ctx, asset.ID); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, asset.ID); status.Running || status.Enabled {
		t.Fatalf("status after disable: %+v", status)
	}

	// 删除: 软删后不可见
	if err := svc.DeleteAsset(ctx, asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetAsset(ctx, asset.ID); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("want ErrAssetNotFound after delete, got %v", err)
	}
}

func TestLoadEnabled(t *testing.T) {
	d := newTestDao(t)
	ctx := context.Background()
	asset := testAsset(fmt.Sprintf("load-%d", time.Now().UnixNano()))
	asset.Enabled = true // 直写 DB (绕过 service), 验证 LoadEnabled 兜底加载
	if err := d.CreateAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.DeleteAsset(ctx, asset.ID) })

	svc := NewService(d)
	if err := svc.LoadEnabled(); err != nil {
		t.Fatal(err)
	}
	status, err := svc.GetAssetStatus(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running {
		t.Fatalf("asset should be running after LoadEnabled: %+v", status)
	}
}

func TestListAssetsTagFilter(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	asset := testAsset(fmt.Sprintf("tags-%d", time.Now().UnixNano()))
	if err := svc.CreateAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.DeleteAsset(ctx, asset.ID) })

	got, err := svc.ListAssets(ctx, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range got {
		if a.ID == asset.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("asset with tag 'test' not found")
	}

	got, err = svc.ListAssets(ctx, []string{"nonexistent-tag"})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a.ID == asset.ID {
			t.Fatal("asset should not match nonexistent tag")
		}
	}
}

func TestCreateAssetValidation(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()

	// 空名
	if err := svc.CreateAsset(ctx, testAsset("")); !errors.Is(err, ErrInvalidAssetConfig) {
		t.Fatalf("want ErrInvalidAssetConfig, got %v", err)
	}

	// 媒介配置 JSON 非法
	asset := testAsset(fmt.Sprintf("bad-%d", time.Now().UnixNano()))
	asset.Media[modules.MediaSSH] = `{{{`
	if err := svc.CreateAsset(ctx, asset); !errors.Is(err, ErrInvalidAssetConfig) {
		t.Fatalf("want ErrInvalidAssetConfig, got %v", err)
	}

	// 未知媒介 kind
	asset = testAsset(fmt.Sprintf("unknown-%d", time.Now().UnixNano()))
	asset.Media["media.vmware"] = `{}`
	if err := svc.CreateAsset(ctx, asset); !errors.Is(err, ErrInvalidAssetConfig) {
		t.Fatalf("want ErrInvalidAssetConfig, got %v", err)
	}
}
