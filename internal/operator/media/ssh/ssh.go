// Package ssh SSH 操作媒介: 基于 golang.org/x/crypto/ssh 实现远程命令执行.
//
// v1 取舍: Exec 采用短连接 (每次独立建连→执行→断开), 无持有资源最稳,
// 运维命令频率下连接开销可接受; 长连接会话复用待后续按需引入.
package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"JuanNiang-Ops/internal/operator/modules"
)

// Client SSH 媒介客户端 (实现 media.Media 与 media.CommandRunner)
type Client struct {
	cfg modules.SSHConfig
}

// New 创建客户端 (修补默认值; 校验认证配置)
func New(cfg modules.SSHConfig) (*Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("ssh -> empty host")
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("ssh -> empty user")
	}
	if cfg.Port <= 0 {
		cfg.Port = modules.DefaultSSHPort
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = int(modules.DefaultTimeout / time.Millisecond)
	}
	if cfg.AuthType == "" {
		if cfg.PrivateKey != "" {
			cfg.AuthType = "private_key"
		} else {
			cfg.AuthType = "password"
		}
	}
	switch cfg.AuthType {
	case "password":
		if cfg.Password == "" {
			return nil, fmt.Errorf("ssh -> empty password for password auth")
		}
	case "private_key":
		if cfg.PrivateKey == "" {
			return nil, fmt.Errorf("ssh -> empty private key for private_key auth")
		}
	default:
		return nil, fmt.Errorf("ssh -> unsupported auth type: %s", cfg.AuthType)
	}
	return &Client{cfg: cfg}, nil
}

// Kind 媒介类型标识
func (c *Client) Kind() modules.MediaKind { return modules.MediaSSH }

// Ping 连通性检查: 建连 + 认证成功即认为可达
func (c *Client) Ping(ctx context.Context) error {
	cli, err := c.dial(ctx)
	if err != nil {
		return err
	}
	_ = cli.Close()
	return nil
}

// Close 释放资源 (短连接模式无持有资源, 幂等 no-op)
func (c *Client) Close() error { return nil }

// Exec 执行单条命令: 短连接 dial → session → run → close.
// 命令非零退出属业务结果 (ExitCode 携带, 不作为错误); 建连/认证/网络失败才是错误.
func (c *Client) Exec(ctx context.Context, cmd string) (*modules.ExecResult, error) {
	start := time.Now()
	cli, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("ssh exec -> dial: %w", err)
	}
	defer cli.Close()

	session, err := cli.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh exec -> new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	result := &modules.ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}
	// 非零退出码是命令执行结果而非通道故障
	var exitErr *gossh.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return nil, fmt.Errorf("ssh exec -> run: %w", err)
	}
	if exitErr != nil {
		result.ExitCode = exitErr.ExitStatus()
	}
	return result, nil
}

// dial 建立并认证 SSH 连接 (ctx 超时/取消经 net.Dialer 生效)
func (c *Client) dial(ctx context.Context) (*gossh.Client, error) {
	auths, err := c.authMethods()
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(c.cfg.Timeout) * time.Millisecond
	config := &gossh.ClientConfig{
		User:            c.cfg.User,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback(c.cfg.HostKeyPolicy),
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", c.cfg.Port))
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	cli, chans, reqs, err := gossh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}
	return gossh.NewClient(cli, chans, reqs), nil
}

// authMethods 按认证类型构造认证方法
func (c *Client) authMethods() ([]gossh.AuthMethod, error) {
	switch c.cfg.AuthType {
	case "password":
		return []gossh.AuthMethod{gossh.Password(c.cfg.Password)}, nil
	case "private_key":
		signer, err := gossh.ParsePrivateKey([]byte(c.cfg.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", c.cfg.AuthType)
	}
}

// hostKeyCallback 主机密钥校验策略 (v1 仅支持 insecure: 跳过校验)
func hostKeyCallback(policy string) gossh.HostKeyCallback {
	return gossh.InsecureIgnoreHostKey()
}
