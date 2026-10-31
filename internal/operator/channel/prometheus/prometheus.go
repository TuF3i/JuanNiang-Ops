// Package prometheus Prometheus 数据通道: 基于 client_golang/api 实现指标查询.
package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"JuanNiang-Ops/internal/operator/channel"
	"JuanNiang-Ops/internal/operator/modules"
)

// Client Prometheus 通道客户端 (实现 channel.Channel 与 channel.MetricsQuerier)
type Client struct {
	api promv1.API
}

// 编译期断言能力实现
var (
	_ channel.Channel       = (*Client)(nil)
	_ channel.MetricsQuerier = (*Client)(nil)
)

// New 创建客户端 (端点不可达在 Ping/Query 时报错; 超时经 http.Client 控制)
func New(cfg modules.PrometheusConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("prometheus -> empty endpoint")
	}
	timeout := modules.DefaultTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Millisecond
	}
	pc, err := promapi.NewClient(promapi.Config{
		Address: cfg.Endpoint,
		Client:  &http.Client{Timeout: timeout},
	})
	if err != nil {
		return nil, fmt.Errorf("prometheus -> new client: %w", err)
	}
	return &Client{api: promv1.NewAPI(pc)}, nil
}

// Kind 通道类型标识
func (c *Client) Kind() modules.ChannelKind { return modules.ChannelPrometheus }

// Ping 连通性检查: 拉取构建信息
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.api.Buildinfo(ctx); err != nil {
		return fmt.Errorf("prometheus ping -> %w", err)
	}
	return nil
}

// Close 释放资源 (HTTP 客户端无持有连接, 幂等 no-op)
func (c *Client) Close() error { return nil }

// Query 瞬时查询 (PromQL); ts 为零值取当前时间
func (c *Client) Query(ctx context.Context, query string, ts time.Time) ([]modules.MetricSample, error) {
	result, _, _, err := c.api.Query(ctx, query, ts)
	if err != nil {
		return nil, fmt.Errorf("prometheus query %s -> %w", query, err)
	}

	switch v := result.(type) {
	case model.Vector:
		samples := make([]modules.MetricSample, 0, len(v))
		for _, s := range v {
			samples = append(samples, modules.MetricSample{
				Labels:    labelMap(s.Metric),
				Value:     float64(s.Value),
				Timestamp: s.Timestamp.Time(),
			})
		}
		return samples, nil
	case *model.Scalar:
		return []modules.MetricSample{{
			Value:     float64(v.Value),
			Timestamp: v.Timestamp.Time(),
		}}, nil
	default:
		return nil, fmt.Errorf("prometheus query %s -> unsupported result type %T", query, result)
	}
}

// QueryRange 区间查询 (PromQL)
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]modules.MetricSeries, error) {
	if step <= 0 {
		step = time.Minute
	}
	result, _, _, err := c.api.QueryRange(ctx, query, promv1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("prometheus query range %s -> %w", query, err)
	}

	matrix, ok := result.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("prometheus query range %s -> unsupported result type %T", query, result)
	}
	series := make([]modules.MetricSeries, 0, len(matrix))
	for _, stream := range matrix {
		samples := make([]modules.MetricSample, 0, len(stream.Values))
		for _, p := range stream.Values {
			samples = append(samples, modules.MetricSample{
				Value:     float64(p.Value),
				Timestamp: p.Timestamp.Time(),
			})
		}
		series = append(series, modules.MetricSeries{Labels: labelMap(stream.Metric), Samples: samples})
	}
	return series, nil
}

// labelMap model.Metric → 普通标签表
func labelMap(m model.Metric) map[string]string {
	labels := make(map[string]string, len(m))
	for k, v := range m {
		labels[string(k)] = string(v)
	}
	return labels
}
