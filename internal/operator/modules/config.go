package modules

import (
	"encoding/json"
	"fmt"
	"time"
)

// 默认配置 (config/viper 落地前为硬编码默认值)
const (
	DefaultSSHPort         = 22
	DefaultTelnetPort      = 23
	DefaultTimeout         = 10 * time.Second
	DefaultTelnetPrompt    = `[#$>]\s*['""]?\s*$` // 命令提示符正则 (通用 shell 提示)
	DefaultDocSearchLimit  = 10
	DefaultLogLimit        = 100
	DefaultExecOutputLimit = 1 << 20 // 命令输出/日志读取上限 1MB, 防止大输出撑爆内存
)

// SSHConfig SSH 操作媒介配置
type SSHConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Timeout int    `json:"timeout_ms"` // 建连与执行超时 (毫秒)

	// 认证: AuthType = "password" (Password) | "private_key" (PrivateKey)
	AuthType   string `json:"auth_type"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"` // PEM 内容 (v1 不支持私钥口令)

	// HostKeyPolicy 主机密钥校验策略: v1 仅支持 "insecure" (跳过校验, 默认)
	HostKeyPolicy string `json:"host_key_policy,omitempty"`
}

// TelnetConfig Telnet 操作媒介配置
type TelnetConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout_ms"` // 建连与执行超时 (毫秒)

	// 可选自动登录 (发送用户名/密码等待登录提示); 为空则跳过登录直接执行
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`

	// Prompt 命令提示符正则 (Exec 读到匹配即认为命令结束)
	Prompt string `json:"prompt,omitempty"`
}

// PVEConfig Proxmox VE 操作媒介配置 (REST API, API Token 认证)
type PVEConfig struct {
	Endpoint string `json:"endpoint"` // 如 https://192.168.1.10:8006
	APIToken string `json:"api_token"`
	// APIToken 格式 "user@realm!tokenid=uuid-secret" (PVE 侧需预先创建)
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
	Timeout            int  `json:"timeout_ms"`
}

// K8sConfig Kubernetes 操作媒介配置
type K8sConfig struct {
	// 接入方式二选一: Kubeconfig (内容或文件路径) 优先; 否则 APIServer+Token 直连
	Kubeconfig string `json:"kubeconfig,omitempty"`

	APIServer string `json:"api_server,omitempty"` // 如 https://192.168.1.10:6443
	Token     string `json:"token,omitempty"`      // ServiceAccount Bearer Token
	CAData    string `json:"ca_data,omitempty"`    // base64 CA 证书

	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
	Timeout            int  `json:"timeout_ms"`
}

// PrometheusConfig Prometheus 数据通道配置
type PrometheusConfig struct {
	Endpoint string `json:"endpoint"`
	Timeout  int    `json:"timeout_ms"`
}

// LokiConfig Loki 数据通道配置
type LokiConfig struct {
	Endpoint string `json:"endpoint"`
	Timeout  int    `json:"timeout_ms"`
}

// TempoConfig Tempo 数据通道配置
type TempoConfig struct {
	Endpoint string `json:"endpoint"`
	Timeout  int    `json:"timeout_ms"`
}

// OutlineConfig Outline 文档站数据通道配置
type OutlineConfig struct {
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"` // Outline 后台生成的 API Key (Bearer)
	Timeout  int    `json:"timeout_ms"`
}

// ParseMediaConfig 将资产媒介配置 JSON 解析为对应类型的 Config (service 校验与工厂构建共用).
// 未知 kind 返回错误.
func ParseMediaConfig(kind MediaKind, raw string) (any, error) {
	switch kind {
	case MediaSSH:
		var cfg SSHConfig
		return &cfg, unmarshal(raw, &cfg)
	case MediaTelnet:
		var cfg TelnetConfig
		return &cfg, unmarshal(raw, &cfg)
	case MediaPVE:
		var cfg PVEConfig
		return &cfg, unmarshal(raw, &cfg)
	case MediaK8s:
		var cfg K8sConfig
		return &cfg, unmarshal(raw, &cfg)
	default:
		return nil, fmt.Errorf("unknown media kind: %s", kind)
	}
}

// ParseChannelConfig 将资产通道配置 JSON 解析为对应类型的 Config (与 ParseMediaConfig 同构)
func ParseChannelConfig(kind ChannelKind, raw string) (any, error) {
	switch kind {
	case ChannelPrometheus:
		var cfg PrometheusConfig
		return &cfg, unmarshal(raw, &cfg)
	case ChannelLoki:
		var cfg LokiConfig
		return &cfg, unmarshal(raw, &cfg)
	case ChannelTempo:
		var cfg TempoConfig
		return &cfg, unmarshal(raw, &cfg)
	case ChannelOutline:
		var cfg OutlineConfig
		return &cfg, unmarshal(raw, &cfg)
	default:
		return nil, fmt.Errorf("unknown channel kind: %s", kind)
	}
}

// unmarshal 空 JSON 视为合法空配置 (各 Config 靠 Default 补默认值)
func unmarshal(raw string, dst any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), dst)
}
