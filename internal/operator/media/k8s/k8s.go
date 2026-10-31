// Package k8s Kubernetes 操作媒介: client-go 封装.
//
// 接入方式 (二选一): Kubeconfig (YAML 内容或文件路径) 优先, 否则 APIServer+Token 直连.
// v1 基础能力: Pod 列表 / Deployment 查询与副本数调整 / Pod 日志.
package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"JuanNiang-Ops/internal/operator/media"
	"JuanNiang-Ops/internal/operator/modules"
)

// Client K8s 媒介客户端 (实现 media.Media 与 media.K8sOperator)
// cs 声明为 Interface: 测试经 fake clientset 注入
type Client struct {
	cfg modules.K8sConfig
	cs  kubernetes.Interface
}

// 编译期断言能力实现
var _ media.K8sOperator = (*Client)(nil)

// New 创建客户端 (构造 rest.Config 并建 clientset)
func New(cfg modules.K8sConfig) (*Client, error) {
	restCfg, err := buildRestConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s -> %w", err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = int(modules.DefaultTimeout / time.Millisecond)
	}
	restCfg.Timeout = time.Duration(cfg.Timeout) * time.Millisecond

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s -> new clientset: %w", err)
	}
	return &Client{cfg: cfg, cs: cs}, nil
}

// buildRestConfig 按配置构造 rest.Config (kubeconfig 优先, 其次 APIServer+Token 直连)
func buildRestConfig(cfg modules.K8sConfig) (*rest.Config, error) {
	if cfg.Kubeconfig != "" {
		raw := cfg.Kubeconfig
		// 内容与路径二义: 路径特征 (存在文件) 读文件, 否则视为 YAML 内容
		if st, err := os.Stat(cfg.Kubeconfig); err == nil && !st.IsDir() {
			data, err := os.ReadFile(cfg.Kubeconfig)
			if err != nil {
				return nil, fmt.Errorf("read kubeconfig %s: %w", cfg.Kubeconfig, err)
			}
			raw = string(data)
		}
		clientCfg, err := clientcmd.NewClientConfigFromBytes([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("parse kubeconfig: %w", err)
		}
		restCfg, err := clientCfg.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("resolve kubeconfig: %w", err)
		}
		return restCfg, nil
	}

	if cfg.APIServer != "" {
		return &rest.Config{
			Host:        cfg.APIServer,
			BearerToken: cfg.Token,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: cfg.InsecureSkipVerify,
				CAData:   []byte(cfg.CAData),
			},
		}, nil
	}
	return nil, fmt.Errorf("empty access config (need kubeconfig or api_server)")
}

// Kind 媒介类型标识
func (c *Client) Kind() modules.MediaKind { return modules.MediaK8s }

// Ping 连通性检查: 拉取集群版本
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cs.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("k8s ping -> %w", err)
	}
	return nil
}

// Close 释放资源 (clientset 无持有连接, 幂等 no-op)
func (c *Client) Close() error { return nil }

// ListPods 列出命名空间下的 Pod (namespace 为空取 default)
func (c *Client) ListPods(ctx context.Context, namespace string) ([]modules.PodInfo, error) {
	if namespace == "" {
		namespace = "default"
	}
	list, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s list pods %s -> %w", namespace, err)
	}
	pods := make([]modules.PodInfo, 0, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
		pods = append(pods, modules.PodInfo{
			Namespace: p.Namespace,
			Name:      p.Name,
			Status:    string(p.Status.Phase),
			PodIP:     p.Status.PodIP,
			Node:      p.Spec.NodeName,
		})
	}
	return pods, nil
}

// GetDeployment 查询 Deployment
func (c *Client) GetDeployment(ctx context.Context, namespace, name string) (*modules.DeploymentInfo, error) {
	if namespace == "" {
		namespace = "default"
	}
	d, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s get deployment %s/%s -> %w", namespace, name, err)
	}
	images := make([]string, 0, len(d.Spec.Template.Spec.Containers))
	for _, ct := range d.Spec.Template.Spec.Containers {
		images = append(images, ct.Image)
	}
	return &modules.DeploymentInfo{
		Namespace:     d.Namespace,
		Name:          d.Name,
		Replicas:      *d.Spec.Replicas,
		ReadyReplicas: d.Status.ReadyReplicas,
		Images:        images,
	}, nil
}

// ScaleDeployment 调整副本数 (GetScale → UpdateScale 标准模式, 携带 ResourceVersion 防冲突)
func (c *Client) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	if namespace == "" {
		namespace = "default"
	}
	scale, err := c.cs.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("k8s get scale %s/%s -> %w", namespace, name, err)
	}
	scale.Spec.Replicas = replicas
	if _, err := c.cs.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s update scale %s/%s -> %w", namespace, name, err)
	}
	return nil
}

// PodLogs 读取 Pod 日志 (有输出上限, 防大日志撑爆内存)
func (c *Client) PodLogs(ctx context.Context, namespace, name string, opts modules.PodLogsOptions) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	req := c.cs.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		Container: opts.Container,
		TailLines: &opts.TailLines,
		Previous:  opts.Previous,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("k8s pod logs %s/%s -> %w", namespace, name, err)
	}
	defer stream.Close()

	data, err := io.ReadAll(io.LimitReader(stream, modules.DefaultExecOutputLimit))
	if err != nil {
		return "", fmt.Errorf("k8s pod logs %s/%s: read: %w", namespace, name, err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
