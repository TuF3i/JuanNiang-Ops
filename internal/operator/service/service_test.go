// operator service 测试:
//   - 路由/能力断言: 纯内存 (stub 媒介/通道直填实例表, 不经数据库)
//   - 生命周期/资产管理/绑定: 依赖 TEST_PG_DSN, 未设置自动 Skip (仓库惯例)
//
// 资产创建流程 (三步): 建媒介/数据源实体 → 建资产 → 绑定 → 启用.
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

func (m *stubMedia) Kind() modules.MediaKind        { return m.kind }
func (m *stubMedia) Ping(ctx context.Context) error { return nil }
func (m *stubMedia) Close() error                   { return nil }

// stubRunner Media + CommandRunner 能力
type stubRunner struct {
	stubMedia
}

func (m *stubRunner) Exec(ctx context.Context, cmd string) (*modules.ExecResult, error) {
	return &modules.ExecResult{ExitCode: 0, Stdout: "ok:" + cmd}, nil
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

// bound 实例条目快捷构造
func bound(id string, kind modules.MediaKind, m media.Media) boundMedia {
	return boundMedia{source: modules.MediaSource{ID: id, Name: "media-" + id, Kind: kind}, m: m}
}

func boundCh(id string, kind modules.ChannelKind, c channel.Channel) boundChannel {
	return boundChannel{source: modules.ChannelSource{ID: id, Name: "channel-" + id, Kind: kind}, c: c}
}

// newRunnerInstance 单媒介 (CommandRunner) + 单通道 (LogQuerier) 实例
func newRunnerInstance(assetID string) *AssetInstance {
	return &AssetInstance{
		asset: modules.Asset{ID: assetID},
		media: []boundMedia{bound("ssh-1", modules.MediaSSH, &stubRunner{stubMedia: stubMedia{modules.MediaSSH}})},
		channels: []boundChannel{boundCh("loki-1", modules.ChannelLoki,
			&stubLogQuerier{stubChannel: stubChannel{modules.ChannelLoki}, entries: []modules.LogEntry{{Line: "hello"}}})},
	}
}

// ---------------------------------------------------------------- 路由测试 --

func TestExecRouting(t *testing.T) {
	svc := newTestService(map[string]*AssetInstance{"a1": newRunnerInstance("a1")})

	// sourceID/kind 均空: 单一 CommandRunner 自动选择
	result, err := svc.Exec(context.Background(), "a1", "", "", "uptime")
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "ok:uptime" {
		t.Fatalf("stdout = %q", result.Stdout)
	}

	// sourceID 精确命中
	if _, err := svc.Exec(context.Background(), "a1", "", "ssh-1", "uptime"); err != nil {
		t.Fatal(err)
	}

	// 资产未运行
	if _, err := svc.Exec(context.Background(), "missing", "", "", "x"); !errors.Is(err, ErrAssetNotRunning) {
		t.Fatalf("want ErrAssetNotRunning, got %v", err)
	}

	// sourceID 未绑定
	if _, err := svc.Exec(context.Background(), "a1", "", "ssh-404", "x"); !errors.Is(err, ErrNoMedia) {
		t.Fatalf("want ErrNoMedia, got %v", err)
	}

	// sourceID 存在但无 CommandRunner 能力
	ins := svc.instances["a1"]
	ins.media = append(ins.media, bound("pve-1", modules.MediaPVE, &stubMedia{modules.MediaPVE}))
	if _, err := svc.Exec(context.Background(), "a1", "", "pve-1", "x"); !errors.Is(err, ErrNoCapability) {
		t.Fatalf("want ErrNoCapability, got %v", err)
	}
}

func TestExecAmbiguous(t *testing.T) {
	ins := newRunnerInstance("a1")
	ins.media = append(ins.media, bound("ssh-2", modules.MediaTelnet, &stubRunner{stubMedia: stubMedia{modules.MediaTelnet}}))
	svc := newTestService(map[string]*AssetInstance{"a1": ins})

	// 两个 CommandRunner 且未指定 → 歧义
	if _, err := svc.Exec(context.Background(), "a1", "", "", "x"); !errors.Is(err, ErrAmbiguousMedia) {
		t.Fatalf("want ErrAmbiguousMedia, got %v", err)
	}
	// sourceID 精确选择则正常
	if _, err := svc.Exec(context.Background(), "a1", "", "ssh-2", "x"); err != nil {
		t.Fatal(err)
	}
}

func TestExecAmbiguousSameKind(t *testing.T) {
	// 同 kind 两个媒介 (两台 SSH): kind 过滤后仍歧义, 须按 sourceID 选择
	ins := newRunnerInstance("a1")
	ins.media = append(ins.media, bound("ssh-2", modules.MediaSSH, &stubRunner{stubMedia: stubMedia{modules.MediaSSH}}))
	svc := newTestService(map[string]*AssetInstance{"a1": ins})

	if _, err := svc.Exec(context.Background(), "a1", modules.MediaSSH, "", "x"); !errors.Is(err, ErrAmbiguousMedia) {
		t.Fatalf("want ErrAmbiguousMedia, got %v", err)
	}
	if _, err := svc.Exec(context.Background(), "a1", modules.MediaSSH, "ssh-2", "x"); err != nil {
		t.Fatal(err)
	}
}

func TestQueryLogsRouting(t *testing.T) {
	svc := newTestService(map[string]*AssetInstance{"a1": newRunnerInstance("a1")})

	entries, err := svc.QueryLogs(context.Background(), "a1", "", "", modules.LogQuery{Selector: `{job="x"}`})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Line != "hello" {
		t.Fatalf("entries = %+v", entries)
	}

	// 无该通道
	if _, err := svc.QueryLogs(context.Background(), "a1", modules.ChannelTempo, "", modules.LogQuery{Selector: "x"}); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("want ErrNoChannel, got %v", err)
	}

	// 有该类型但无 LogQuerier 能力
	svc.instances["a1"].channels = append(svc.instances["a1"].channels,
		boundCh("tempo-1", modules.ChannelTempo, &stubChannel{modules.ChannelTempo}))
	if _, err := svc.QueryLogs(context.Background(), "a1", modules.ChannelTempo, "tempo-1", modules.LogQuery{Selector: "x"}); !errors.Is(err, ErrNoCapability) {
		t.Fatalf("want ErrNoCapability, got %v", err)
	}
}

func TestMediaAccessor(t *testing.T) {
	svc := newTestService(map[string]*AssetInstance{"a1": newRunnerInstance("a1")})

	m, err := svc.Media("a1", modules.MediaSSH, "ssh-1")
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind() != modules.MediaSSH {
		t.Fatalf("kind = %s", m.Kind())
	}
	if _, err := svc.Media("a1", modules.MediaPVE, ""); !errors.Is(err, ErrNoMedia) {
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
	if err := d.Setup(context.Background()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return d
}

// 离线可构建的配置 (ssh/loki 的 New 只校验不建连)
const (
	testSSHConfig  = `{"host":"127.0.0.1","user":"root","password":"x"}`
	testLokiConfig = `{"endpoint":"http://127.0.0.1:3100"}`
)

// createMediaSource 测试辅助: 建一个 SSH 媒介实体并返回 ID
func createMediaSource(t *testing.T, svc *Service, name string) string {
	t.Helper()
	src := &modules.MediaSource{Name: name, Kind: modules.MediaSSH, Config: testSSHConfig}
	if err := svc.CreateMediaSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	return src.ID
}

// createAssetWithMedia 三步流程: 建资产 → 绑定媒介 (可选) → 返回资产 ID
func createAssetWithMedia(t *testing.T, svc *Service, name string, mediaSourceIDs ...string) string {
	t.Helper()
	asset := &modules.Asset{
		Name: name,
		Docs: "# 测试资产\n仅用于单测",
		Tags: []string{"test", "operator"},
	}
	if err := svc.CreateAsset(context.Background(), asset); err != nil {
		t.Fatal(err)
	}
	for _, mid := range mediaSourceIDs {
		if err := svc.BindAssetMedia(context.Background(), asset.ID, mid); err != nil {
			t.Fatal(err)
		}
	}
	return asset.ID
}

func TestAssetLifecycle(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	// 三步流程: 媒介实体 → 资产 → 绑定
	mediaID := createMediaSource(t, svc, "ssh-lifecycle-"+uniq)
	assetID := createAssetWithMedia(t, svc, "lifecycle-"+uniq, mediaID)
	t.Cleanup(func() {
		_ = svc.DeleteMediaSource(ctx, mediaID)
		_ = svc.DeleteAsset(ctx, assetID)
	})

	// 启用: 实例创建 (从绑定实体构建)
	if err := svc.EnableAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	status, err := svc.GetAssetStatus(ctx, assetID)
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
	src, err := svc.GetAsset(ctx, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateAsset(ctx, src); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, assetID); !status.Running {
		t.Fatal("instance lost after no-change update")
	}

	// 解绑 → 实例重建后无媒介
	if err := svc.UnbindAssetMedia(ctx, assetID, mediaID); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, assetID); !status.Running || len(status.MediaKinds) != 0 {
		t.Fatalf("after unbind: %+v", status)
	}

	// 重新绑定 → 媒介恢复
	if err := svc.BindAssetMedia(ctx, assetID, mediaID); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, assetID); len(status.MediaKinds) != 1 {
		t.Fatalf("after rebind: %+v", status)
	}

	// 停用: 实例移除
	if err := svc.DisableAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	if status, _ = svc.GetAssetStatus(ctx, assetID); status.Running || status.Enabled {
		t.Fatalf("status after disable: %+v", status)
	}

	// 删除: 软删后不可见
	if err := svc.DeleteAsset(ctx, assetID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetAsset(ctx, assetID); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("want ErrAssetNotFound after delete, got %v", err)
	}
}

func TestMediaSourceLifecycle(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	// 创建
	mediaID := createMediaSource(t, svc, "ssh-mslc-"+uniq)

	// 在用删除拒绝: 绑定到资产后删除报 ErrMediaSourceInUse
	assetID := createAssetWithMedia(t, svc, "mslc-asset-"+uniq, mediaID)
	t.Cleanup(func() {
		_ = svc.DeleteAsset(ctx, assetID)
		_ = svc.DeleteMediaSource(ctx, mediaID)
	})
	if err := svc.DeleteMediaSource(ctx, mediaID); !errors.Is(err, ErrMediaSourceInUse) {
		t.Fatalf("want ErrMediaSourceInUse, got %v", err)
	}

	// 更新配置: sonic+MD5 无变化跳过
	src, err := svc.GetMediaSource(ctx, mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateMediaSource(ctx, src); err != nil {
		t.Fatal(err)
	}

	// 更新配置: 有变化 → 落库 (绑定资产运行中则重建, 此处未启用不重建)
	src.Config = `{"host":"127.0.0.2","user":"root","password":"x"}`
	if err := svc.UpdateMediaSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetMediaSource(ctx, mediaID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Config != src.Config {
		t.Fatalf("config not updated: %s", got.Config)
	}

	// 解绑后删除成功
	if err := svc.UnbindAssetMedia(ctx, assetID, mediaID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMediaSource(ctx, mediaID); err != nil {
		t.Fatalf("delete after unbind: %v", err)
	}
	if _, err := svc.GetMediaSource(ctx, mediaID); !errors.Is(err, ErrMediaSourceNotFound) {
		t.Fatalf("want ErrMediaSourceNotFound, got %v", err)
	}
}

func TestSharedMediaAcrossAssets(t *testing.T) {
	// 核心场景: 同一 SSH 媒介实体绑定到两个资产 (如同机 K8s 资产与 PVE 资产共用 SSH)
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	mediaID := createMediaSource(t, svc, "ssh-shared-"+uniq)
	assetA := createAssetWithMedia(t, svc, "shared-a-"+uniq, mediaID)
	assetB := createAssetWithMedia(t, svc, "shared-b-"+uniq, mediaID)
	t.Cleanup(func() {
		_ = svc.DeleteAsset(ctx, assetA)
		_ = svc.DeleteAsset(ctx, assetB)
		_ = svc.DeleteMediaSource(ctx, mediaID)
	})

	// 两个资产都启用, 各自实例独立构建 (配置共享, 连接独立)
	if err := svc.EnableAsset(ctx, assetA); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnableAsset(ctx, assetB); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{assetA, assetB} {
		status, err := svc.GetAssetStatus(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !status.Running || len(status.MediaKinds) != 1 {
			t.Fatalf("asset %s status: %+v", id, status)
		}
	}

	// 两个实例均可执行 (共享的是配置, 不是连接)
	if _, err := svc.Exec(ctx, assetA, "", mediaID, "hostname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Exec(ctx, assetB, "", mediaID, "hostname"); err != nil {
		t.Fatal(err)
	}

	// 绑定列表: 两资产均含该媒介
	for _, id := range []string{assetA, assetB} {
		bound, err := svc.ListAssetMedia(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(bound) != 1 || bound[0].ID != mediaID {
			t.Fatalf("asset %s bound media = %+v", id, bound)
		}
	}
}

func TestLoadEnabled(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	mediaID := createMediaSource(t, svc, "ssh-load-"+uniq)
	assetID := createAssetWithMedia(t, svc, "load-"+uniq, mediaID)
	asset, _ := svc.GetAsset(ctx, assetID)
	asset.Enabled = true // 直改 DB (绕过 service), 验证 LoadEnabled 兜底加载
	if err := d.UpdateAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = svc.DeleteAsset(ctx, assetID)
		_ = svc.DeleteMediaSource(ctx, mediaID)
	})

	svc2 := NewService(d)
	if err := svc2.LoadEnabled(); err != nil {
		t.Fatal(err)
	}
	status, err := svc2.GetAssetStatus(ctx, assetID)
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
	assetID := createAssetWithMedia(t, svc, "tags-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	t.Cleanup(func() { _ = svc.DeleteAsset(ctx, assetID) })

	got, err := svc.ListAssets(ctx, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range got {
		if a.ID == assetID {
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
		if a.ID == assetID {
			t.Fatal("asset should not match nonexistent tag")
		}
	}
}

func TestCreateValidation(t *testing.T) {
	d := newTestDao(t)
	svc := NewService(d)
	ctx := context.Background()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())

	// 资产空名
	if err := svc.CreateAsset(ctx, &modules.Asset{}); !errors.Is(err, ErrInvalidAssetConfig) {
		t.Fatalf("want ErrInvalidAssetConfig, got %v", err)
	}

	// 媒介实体: 未知 kind / 非法配置 JSON
	if err := svc.CreateMediaSource(ctx, &modules.MediaSource{Name: "m1-" + uniq, Kind: "media.vmware", Config: `{}`}); !errors.Is(err, ErrInvalidAssetConfig) {
		t.Fatalf("want ErrInvalidAssetConfig (unknown kind), got %v", err)
	}
	if err := svc.CreateMediaSource(ctx, &modules.MediaSource{Name: "m2-" + uniq, Kind: modules.MediaSSH, Config: `{{{`}); !errors.Is(err, ErrInvalidAssetConfig) {
		t.Fatalf("want ErrInvalidAssetConfig (bad json), got %v", err)
	}

	// 媒介实体绑定到不存在的资产
	mediaID := createMediaSource(t, svc, "ssh-val-"+uniq)
	t.Cleanup(func() { _ = svc.DeleteMediaSource(ctx, mediaID) })
	if err := svc.BindAssetMedia(ctx, "asset-404", mediaID); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("want ErrAssetNotFound, got %v", err)
	}
}
