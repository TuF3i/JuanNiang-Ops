// Package telnet Telnet 操作媒介: 标准库 net 实现, 含最小 IAC 协商处理.
//
// v1 取舍: Exec 采用短连接; 可选凭据自动登录 (等待 login/password 提示);
// 命令结束以 Prompt 正则匹配为准. telnet 无退出码语义, ExecResult.ExitCode 恒 0.
// 设备提示符差异大, Prompt 正则按资产配置 (默认通用 shell 提示).
package telnet

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"JuanNiang-Ops/internal/operator/modules"
)

// Client Telnet 媒介客户端 (实现 media.Media 与 media.CommandRunner)
type Client struct {
	cfg    modules.TelnetConfig
	prompt *regexp.Regexp
}

// New 创建客户端 (修补默认值; 预编译 Prompt 正则)
func New(cfg modules.TelnetConfig) (*Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("telnet -> empty host")
	}
	if cfg.Port <= 0 {
		cfg.Port = modules.DefaultTelnetPort
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = int(modules.DefaultTimeout / time.Millisecond)
	}
	promptPattern := cfg.Prompt
	if promptPattern == "" {
		promptPattern = modules.DefaultTelnetPrompt
	}
	prompt, err := regexp.Compile(promptPattern)
	if err != nil {
		return nil, fmt.Errorf("telnet -> invalid prompt regexp: %w", err)
	}
	return &Client{cfg: cfg, prompt: prompt}, nil
}

// Kind 媒介类型标识
func (c *Client) Kind() modules.MediaKind { return modules.MediaTelnet }

// Ping 连通性检查: 建连成功即认为可达
func (c *Client) Ping(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// Close 释放资源 (短连接模式无持有资源, 幂等 no-op)
func (c *Client) Close() error { return nil }

// Exec 执行单条命令: 建连 → (可选登录) → 发送命令 → 读至 Prompt 匹配或超时.
// 输出经 IAC 剥离; telnet 无法区分 stdout/stderr, 全部记入 Stdout.
func (c *Client) Exec(ctx context.Context, cmd string) (*modules.ExecResult, error) {
	start := time.Now()
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("telnet exec -> dial: %w", err)
	}
	defer conn.Close()

	if err := c.login(ctx, conn); err != nil {
		return nil, fmt.Errorf("telnet exec -> login: %w", err)
	}

	if _, err := conn.Write([]byte(cmd + "\n")); err != nil {
		return nil, fmt.Errorf("telnet exec -> write: %w", err)
	}

	var buf bytes.Buffer
	if err := c.readUntilPrompt(ctx, conn, &buf); err != nil {
		return nil, fmt.Errorf("telnet exec -> read: %w", err)
	}

	output := buf.String()
	// 去掉回显的命令行, 仅留命令输出
	if idx := strings.Index(output, "\n"); idx >= 0 {
		output = output[idx+1:]
	}

	return &modules.ExecResult{
		ExitCode: 0, // telnet 无退出码语义
		Stdout:   strings.TrimSpace(output),
		Duration: time.Since(start),
	}, nil
}

// dial 建立 TCP 连接 (ctx 控制超时/取消)
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	timeout := time.Duration(c.cfg.Timeout) * time.Millisecond
	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", c.cfg.Port))
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	return conn, nil
}

// login 可选自动登录: 等待 login/password 提示并回应 (未配置凭据则跳过).
// 提示只回应一次, 防止重复 Contains 命中导致重发.
func (c *Client) login(ctx context.Context, conn net.Conn) error {
	if c.cfg.Username == "" && c.cfg.Password == "" {
		return nil
	}

	var buf bytes.Buffer
	sentUser, sentPassword := false, c.cfg.Password == ""

	for {
		if err := c.readChunk(ctx, conn, &buf); err != nil {
			return err
		}
		tail := buf.String()

		switch {
		case !sentUser && (strings.Contains(tail, "ogin:") || strings.Contains(tail, "sername:")):
			sentUser = true
			buf.Reset()
			if _, err := conn.Write([]byte(c.cfg.Username + "\n")); err != nil {
				return fmt.Errorf("write username: %w", err)
			}
		case !sentPassword && strings.Contains(tail, "assword:"):
			sentPassword = true
			buf.Reset()
			if _, err := conn.Write([]byte(c.cfg.Password + "\n")); err != nil {
				return fmt.Errorf("write password: %w", err)
			}
		case sentUser && sentPassword && c.prompt.MatchString(tail):
			return nil // 已进入 shell 提示符
		}
	}
}

// readUntilPrompt 读输出直至 Prompt 正则命中
func (c *Client) readUntilPrompt(ctx context.Context, conn net.Conn, buf *bytes.Buffer) error {
	for {
		if err := c.readChunk(ctx, conn, buf); err != nil {
			return err
		}
		tail := buf.String()
		if c.prompt.MatchString(tail) {
			return nil
		}
		if buf.Len() > modules.DefaultExecOutputLimit {
			return fmt.Errorf("output exceeds limit (%d bytes)", modules.DefaultExecOutputLimit)
		}
	}
}

// readChunk 读取一段数据, 经 IAC 剥离后追加到 buf.
// 服务端的协商请求 (DO/WILL) 以拒绝响应回写 (WONT/DONT), 维持最小能力集.
func (c *Client) readChunk(ctx context.Context, conn net.Conn, buf *bytes.Buffer) error {
	raw := make([]byte, 4096)
	n, err := conn.Read(raw)
	if err != nil {
		return err
	}
	payload, reply := stripIAC(raw[:n])
	if len(reply) > 0 {
		if _, err := conn.Write(reply); err != nil {
			return fmt.Errorf("write iac reply: %w", err)
		}
	}
	buf.Write(payload)
	return nil
}
