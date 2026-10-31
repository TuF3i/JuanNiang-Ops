// Package outline Outline 文档站数据通道: REST API (Bearer 认证).
// 搜索 documents.search, 读取 documents.info (text 字段即 markdown 正文).
package outline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/modules"
)

// Client Outline 通道客户端 (实现 channel.Channel 与 channel.DocSearcher)
type Client struct {
	cfg  modules.OutlineConfig
	http *http.Client
}

// 编译期断言能力实现
var (
	_ channel.Channel   = (*Client)(nil)
	_ channel.DocSearcher = (*Client)(nil)
)

// New 创建客户端
func New(cfg modules.OutlineConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("outline -> empty endpoint")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("outline -> empty api key")
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
func (c *Client) Kind() modules.ChannelKind { return modules.ChannelOutline }

// Ping 连通性检查: 拉取认证用户信息
func (c *Client) Ping(ctx context.Context) error {
	var dst json.RawMessage
	return c.postJSON(ctx, "/auth.info", nil, &dst)
}

// Close 释放资源 (HTTP 客户端无持有连接, 幂等 no-op)
func (c *Client) Close() error { return nil }

// SearchDocs 关键词搜索文档
func (c *Client) SearchDocs(ctx context.Context, keyword string, limit int) ([]modules.DocHit, error) {
	if keyword == "" {
		return nil, fmt.Errorf("outline -> empty keyword")
	}
	if limit <= 0 {
		limit = modules.DefaultDocSearchLimit
	}

	var hits []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Context string `json:"context"`
	}
	body, _ := json.Marshal(map[string]any{"query": keyword, "limit": limit})
	if err := c.postJSON(ctx, "/documents.search", body, &hits); err != nil {
		return nil, fmt.Errorf("outline search %s -> %w", keyword, err)
	}

	result := make([]modules.DocHit, 0, len(hits))
	for _, d := range hits {
		result = append(result, modules.DocHit{PageID: d.ID, Title: d.Title, Context: d.Context})
	}
	return result, nil
}

// GetDoc 读取文档页 (text 字段即 markdown 正文)
func (c *Client) GetDoc(ctx context.Context, pageID string) (*modules.DocPage, error) {
	if pageID == "" {
		return nil, fmt.Errorf("outline -> empty page id")
	}

	var page struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	body, _ := json.Marshal(map[string]any{"id": pageID})
	if err := c.postJSON(ctx, "/documents.info", body, &page); err != nil {
		return nil, fmt.Errorf("outline get doc %s -> %w", pageID, err)
	}
	return &modules.DocPage{PageID: page.ID, Title: page.Title, Markdown: page.Text}, nil
}

// postJSON POST JSON 请求: Bearer 认证 + {"ok":bool,"data":...} 包裹解码
func (c *Client) postJSON(ctx context.Context, path string, body []byte, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(c.cfg.Endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

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

	// Outline 响应统一包裹 {"ok":bool, "data": ...}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if !envelope.OK {
		return fmt.Errorf("API error: %s", raw)
	}
	if dst == nil || len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, dst)
}
