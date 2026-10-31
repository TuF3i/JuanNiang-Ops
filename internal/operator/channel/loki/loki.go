// Package loki Loki 数据通道: 手写 HTTP 客户端查询日志 (LogQL).
// 响应格式: {"status":"success","data":{"resultType":"streams","result":[{"stream":{...},"values":[["<ns>","<line>"]]}]}}
package loki

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

// Client Loki 通道客户端 (实现 channel.Channel 与 channel.LogQuerier)
type Client struct {
	cfg  modules.LokiConfig
	http *http.Client
}

// 编译期断言能力实现
var (
	_ channel.Channel = (*Client)(nil)
	_ channel.LogQuerier = (*Client)(nil)
)

// New 创建客户端
func New(cfg modules.LokiConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("loki -> empty endpoint")
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
func (c *Client) Kind() modules.ChannelKind { return modules.ChannelLoki }

// Ping 连通性检查: 拉取标签名列表
func (c *Client) Ping(ctx context.Context) error {
	return c.getJSON(ctx, "/loki/api/v1/labels", &struct{}{})
}

// Close 释放资源 (HTTP 客户端无持有连接, 幂等 no-op)
func (c *Client) Close() error { return nil }

// QueryLogs 执行 LogQL 区间查询
func (c *Client) QueryLogs(ctx context.Context, q modules.LogQuery) ([]modules.LogEntry, error) {
	if q.Selector == "" {
		return nil, fmt.Errorf("loki -> empty selector")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = modules.DefaultLogLimit
	}
	direction := q.Direction
	if direction != "forward" {
		direction = "backward"
	}

	params := url.Values{}
	params.Set("query", q.Selector)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("direction", direction)
	if !q.Start.IsZero() {
		params.Set("start", strconv.FormatInt(q.Start.UnixNano(), 10))
	}
	if !q.End.IsZero() {
		params.Set("end", strconv.FormatInt(q.End.UnixNano(), 10))
	}

	var resp struct {
		Data struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"` // [纳秒时间戳字符串, 行内容]
			} `json:"result"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, "/loki/api/v1/query_range?"+params.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("loki query %s -> %w", q.Selector, err)
	}

	entries := make([]modules.LogEntry, 0, len(resp.Data.Result)*limit)
	for _, stream := range resp.Data.Result {
		for _, v := range stream.Values {
			ts := time.Time{}
			if ns, err := strconv.ParseInt(v[0], 10, 64); err == nil {
				ts = time.Unix(0, ns)
			}
			entries = append(entries, modules.LogEntry{
				Timestamp: ts,
				Line:      v[1],
				Labels:    stream.Stream,
			})
		}
	}
	return entries, nil
}

// getJSON GET 请求并解码 JSON (状态码 >= 300 报错, 复用 t2i 的 decodeJSON 语义)
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
