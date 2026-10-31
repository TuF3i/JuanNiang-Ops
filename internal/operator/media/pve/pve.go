// Package pve Proxmox VE 操作媒介: REST API 客户端 (API Token 认证).
//
// 认证使用 API Token (PVE 侧需预先创建 user@realm!tokenid 并绑定权限),
// 相比用户名票据免 CSRF 处理. start/stop/reboot 底层为 PVE 异步任务 (返回 UPID),
// v1 仅保证任务被接受, 不等待任务完成; 需要等待结果时经 VMStatus 轮询确认.
package pve

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// Client PVE 媒介客户端 (实现 media.Media 与 media.VMOperator)
type Client struct {
	cfg     modules.PVEConfig
	base    string       // endpoint + /api2/json
	http    *http.Client // 独立实例 (按配置启用 TLS 跳过校验)
	kind    modules.MediaKind
	timeout time.Duration
}

// 编译期断言能力实现
var _ media.VMOperator = (*Client)(nil)

// New 创建客户端 (校验 Endpoint/APIToken 非空)
func New(cfg modules.PVEConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("pve -> empty endpoint")
	}
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("pve -> empty api token")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = int(modules.DefaultTimeout / time.Millisecond)
	}
	transport := &http.Transport{}
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		cfg:     cfg,
		base:    strings.TrimSuffix(cfg.Endpoint, "/") + "/api2/json",
		http:    &http.Client{Transport: transport, Timeout: time.Duration(cfg.Timeout) * time.Millisecond},
		kind:    modules.MediaPVE,
		timeout: time.Duration(cfg.Timeout) * time.Millisecond,
	}, nil
}

// Kind 媒介类型标识
func (c *Client) Kind() modules.MediaKind { return c.kind }

// Ping 连通性检查: 拉取集群节点列表
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.listNodes(ctx)
	return err
}

// Close 释放资源 (HTTP 客户端无持有连接, 幂等 no-op)
func (c *Client) Close() error { return nil }

// ListNodes 列出集群节点名
func (c *Client) ListNodes(ctx context.Context) ([]string, error) {
	nodes, err := c.listNodes(ctx)
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

// ListVMs 列出节点上的 QEMU 虚机 (node 为空则遍历全部节点聚合)
func (c *Client) ListVMs(ctx context.Context, node string) ([]modules.VMInfo, error) {
	if node != "" {
		return c.listVMsOnNode(ctx, node)
	}

	nodes, err := c.listNodes(ctx)
	if err != nil {
		return nil, err
	}
	var all []modules.VMInfo
	for _, n := range nodes {
		vms, err := c.listVMsOnNode(ctx, n)
		if err != nil {
			return nil, err
		}
		all = append(all, vms...)
	}
	return all, nil
}

// VMStatus 查询单台虚机状态
func (c *Client) VMStatus(ctx context.Context, node string, vmid int) (*modules.VMInfo, error) {
	var data qemuStatus
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/current", node, vmid)
	if err := c.getJSON(ctx, path, &data); err != nil {
		return nil, fmt.Errorf("vm status %s/%d -> %w", node, vmid, err)
	}
	return data.toVMInfo(node, vmid), nil
}

// StartVM 开机 (PVE 异步任务, 已被接受即返回)
func (c *Client) StartVM(ctx context.Context, node string, vmid int) error {
	return c.vmAction(ctx, node, vmid, "start")
}

// StopVM 关机 (强制, 对应 stop; 优雅关机请经 SSH 执行 shutdown 或扩展 acpi 接口)
func (c *Client) StopVM(ctx context.Context, node string, vmid int) error {
	return c.vmAction(ctx, node, vmid, "stop")
}

// RebootVM 重启
func (c *Client) RebootVM(ctx context.Context, node string, vmid int) error {
	return c.vmAction(ctx, node, vmid, "reboot")
}

// ---------------------------------------------------------------- 内部实现 --

// listNodes 节点名列表
func (c *Client) listNodes(ctx context.Context) ([]string, error) {
	var resp struct {
		Data []struct {
			Node string `json:"node"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/nodes", &resp); err != nil {
		return nil, fmt.Errorf("list nodes -> %w", err)
	}
	names := make([]string, 0, len(resp.Data))
	for _, n := range resp.Data {
		names = append(names, n.Node)
	}
	return names, nil
}

// listVMsOnNode 单节点 QEMU 虚机列表
func (c *Client) listVMsOnNode(ctx context.Context, node string) ([]modules.VMInfo, error) {
	var resp struct {
		Data []qemuStatus `json:"data"`
	}
	path := fmt.Sprintf("/nodes/%s/qemu", node)
	if err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("list vms on %s -> %w", node, err)
	}
	vms := make([]modules.VMInfo, 0, len(resp.Data))
	for i := range resp.Data {
		vms = append(vms, *resp.Data[i].toVMInfo(node, resp.Data[i].VMID))
	}
	return vms, nil
}

// vmAction 虚机电源操作 (start/stop/reboot)
func (c *Client) vmAction(ctx context.Context, node string, vmid int, action string) error {
	var resp struct {
		Data string `json:"data"` // UPID 任务句柄
	}
	path := fmt.Sprintf("/nodes/%s/qemu/%d/status/%s", node, vmid, action)
	if err := c.postJSON(ctx, path, nil, &resp); err != nil {
		return fmt.Errorf("vm %s %s/%d -> %w", action, node, vmid, err)
	}
	return nil
}

// getJSON GET 请求并解码 {"data": ...} 包裹的响应
func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	return c.do(ctx, http.MethodGet, path, nil, dst)
}

// postJSON POST 请求 (body 为表单编码的 nil/参数)
func (c *Client) postJSON(ctx context.Context, path string, body io.Reader, dst any) error {
	return c.do(ctx, http.MethodPost, path, body, dst)
}

// do 统一请求: PVEAPIToken 认证 + {"data": ...} 包裹解码
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, dst any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.cfg.APIToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, modules.DefaultExecOutputLimit))
	if err != nil {
		return fmt.Errorf("%s %s: read body: %w", method, path, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: API error (%d): %s", method, path, resp.StatusCode, bytes.TrimSpace(raw))
	}

	// PVE 响应统一包裹在 {"data": ...}, 直接对 data 字段二次解码
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("%s %s: decode envelope: %w", method, path, err)
	}
	if dst == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		return fmt.Errorf("%s %s: decode data: %w", method, path, err)
	}
	return nil
}

// qemuStatus PVE QEMU 状态字段 (仅取统一模型所需)
type qemuStatus struct {
	VMID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	CPU     float64 `json:"cpu"`
	MaxMem  int64   `json:"maxmem"`
	MaxDisk int64   `json:"maxdisk"`
}

func (s *qemuStatus) toVMInfo(node string, vmid int) *modules.VMInfo {
	return &modules.VMInfo{
		Node:    node,
		VMID:    vmid,
		Name:    s.Name,
		Status:  s.Status,
		CPU:     s.CPU,
		MaxMem:  s.MaxMem,
		MaxDisk: s.MaxDisk,
	}
}
