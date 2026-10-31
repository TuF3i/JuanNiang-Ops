// Package tempo Grafana Tempo 数据通道: 手写 HTTP 客户端查询链路.
//
// 搜索走 /api/search (TraceQL 条件); 完整链路 /api/traces/{id} 返回原始 JSON
// (Tempo 默认 OTLP 格式), v1 不解析 OTLP 结构, 由调用方按需处理.
package tempo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/modules"
)

// Client Tempo 通道客户端 (实现 channel.Channel 与 channel.TraceSearcher)
type Client struct {
	cfg  modules.TempoConfig
	http *http.Client
}

// 编译期断言能力实现
var (
	_ channel.Channel       = (*Client)(nil)
	_ channel.TraceSearcher = (*Client)(nil)
)

// New 创建客户端
func New(cfg modules.TempoConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("tempo -> empty endpoint")
	}
	timeout := modules.DefaultTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Millisecond
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
	}, nil
}

// Kind 通道类型标识
func (c *Client) Kind() modules.ChannelKind { return modules.ChannelTempo }

// Ping 连通性检查: 拉取统计信息 (轻量接口)
func (c *Client) Ping(ctx context.Context) error {
	var dst json.RawMessage
	return c.getJSON(ctx, "/api/status/stats", &dst)
}

// Close 释放资源 (HTTP 客户端无持有连接, 幂等 no-op)
func (c *Client) Close() error { return nil }

// SearchTraces 按 TraceQL 条件搜索链路.
// 条件拼装: ServiceName → { service.name="x" }, MinDuration → 追加 minDuration="1s".
func (c *Client) SearchTraces(ctx context.Context, q modules.TraceQuery) ([]modules.TraceSummary, error) {
	conditions := make([]string, 0, 2)
	if q.ServiceName != "" {
		conditions = append(conditions, fmt.Sprintf("service.name=%q", q.ServiceName))
	}
	if q.MinDuration != "" {
		conditions = append(conditions, fmt.Sprintf("minDuration=%q", q.MinDuration))
	}
	if len(conditions) == 0 {
		return nil, fmt.Errorf("tempo -> empty query conditions")
	}

	params := url.Values{}
	params.Set("q", fmt.Sprintf("{ %s }", strings.Join(conditions, " && ")))
	limit := q.Limit
	if limit <= 0 {
		limit = modules.DefaultLogLimit
	}
	params.Set("limit", strconv.Itoa(limit))
	if !q.Start.IsZero() {
		params.Set("start", strconv.FormatInt(q.Start.Unix(), 10))
	}
	if !q.End.IsZero() {
		params.Set("end", strconv.FormatInt(q.End.Unix(), 10))
	}

	var resp struct {
		Traces []struct {
			TraceID         string `json:"traceID"`
			RootServiceName string `json:"rootServiceName"`
			RootTraceName   string `json:"rootTraceName"`
			StartTimeUnixNS string `json:"startTimeUnixNano"`
			DurationMS      int64  `json:"durationMs"`
		} `json:"traces"`
	}
	if err := c.getJSON(ctx, "/api/search?"+params.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("tempo search -> %w", err)
	}

	traces := make([]modules.TraceSummary, 0, len(resp.Traces))
	for _, t := range resp.Traces {
		summary := modules.TraceSummary{
			TraceID:         t.TraceID,
			RootServiceName: t.RootServiceName,
			RootTraceName:   t.RootTraceName,
			DurationMS:      t.DurationMS,
		}
		if ns, err := strconv.ParseInt(t.StartTimeUnixNS, 10, 64); err == nil {
			summary.StartTime = time.Unix(0, ns)
		}
		traces = append(traces, summary)
	}
	return traces, nil
}

// GetTrace 拉取完整链路 (原始 JSON 字节, OTLP 结构由调用方解析)
func (c *Client) GetTrace(ctx context.Context, traceID string) ([]byte, error) {
	if traceID == "" {
		return nil, fmt.Errorf("tempo -> empty trace id")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(c.cfg.Endpoint, "/")+"/api/traces/"+url.PathEscape(traceID), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get trace %s: %w", traceID, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, modules.DefaultExecOutputLimit))
	if err != nil {
		return nil, fmt.Errorf("get trace %s: read body: %w", traceID, err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get trace %s: API error (%d): %s", traceID, resp.StatusCode, raw)
	}
	return raw, nil
}

// getJSON GET 请求并解码 JSON (与 loki 通道同构, t2i 的 decodeJSON 语义)
func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.cfg.Endpoint, "/")+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, modules.DefaultExecOutputLimit))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, raw)
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}
