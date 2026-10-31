// IAC 剥离的表驱动单测 + Telnet 配置默认值/正则校验.
// 端到端连接测试需真实 telnetd, 沙箱环境不具备, 标注手测.
package telnet

import (
	"strings"
	"testing"

	"JuanNiang-Ops/internal/operator/modules"
)

func TestStripIAC(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		payload []byte
		reply   []byte
	}{
		{
			name:    "纯数据透传",
			in:      []byte("hello world\n"),
			payload: []byte("hello world\n"),
		},
		{
			name:    "IAC IAC 转义为字面量",
			in:      []byte{'a', iacIAC, iacIAC, 'b'},
			payload: []byte{'a', iacIAC, 'b'},
		},
		{
			name:    "DO 请求回 WONT 拒绝",
			in:      []byte{'x', iacIAC, iacDO, 24, 'y'},
			payload: []byte{'x', 'y'},
			reply:   []byte{iacIAC, iacWONT, 24},
		},
		{
			name:    "WILL 请求回 DONT 拒绝",
			in:      []byte{iacIAC, iacWILL, 1},
			reply:   []byte{iacIAC, iacDONT, 1},
		},
		{
			name:    "DONT/WONT 无需响应",
			in:      []byte{iacIAC, iacDONT, 3, iacIAC, iacWONT, 4, 'z'},
			payload: []byte{'z'},
		},
		{
			name:    "子协商整体跳过",
			in:      []byte{'a', iacIAC, iacSB, 24, 1, 2, 3, iacIAC, iacSE, 'b'},
			payload: []byte{'a', 'b'},
		},
		{
			name:    "子协商内 IAC IAC 转义",
			in:      []byte{iacIAC, iacSB, 0, iacIAC, iacIAC, 9, iacIAC, iacSE, 'c'},
			payload: []byte{'c'},
		},
		{
			name:    "两字节命令 NOP 消费",
			in:      []byte{'a', iacIAC, 241, 'b'}, // 241 = NOP
			payload: []byte{'a', 'b'},
		},
		{
			name:    "尾部截断的 IAC 丢弃",
			in:      []byte{'a', iacIAC, iacDO},
			payload: []byte{'a'},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, reply := stripIAC(tc.in)
			if string(payload) != string(tc.payload) {
				t.Fatalf("payload = %q, want %q", payload, tc.payload)
			}
			if string(reply) != string(tc.reply) {
				t.Fatalf("reply = %v, want %v", reply, tc.reply)
			}
		})
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(modules.TelnetConfig{}); err == nil || !strings.Contains(err.Error(), "empty host") {
		t.Fatalf("want empty host error, got %v", err)
	}
	if _, err := New(modules.TelnetConfig{Host: "h", Prompt: "[invalid"}); err == nil || !strings.Contains(err.Error(), "invalid prompt") {
		t.Fatalf("want invalid prompt error, got %v", err)
	}
}

func TestNewDefaults(t *testing.T) {
	c, err := New(modules.TelnetConfig{Host: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if c.cfg.Port != modules.DefaultTelnetPort {
		t.Fatalf("port default = %d, want %d", c.cfg.Port, modules.DefaultTelnetPort)
	}
	if c.prompt == nil {
		t.Fatal("prompt regexp not compiled")
	}
	if !c.prompt.MatchString("root@host:~# ") {
		t.Fatal("default prompt should match shell prompt")
	}
}
