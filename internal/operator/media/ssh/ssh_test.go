// SSH 配置校验与默认值单测.
// 端到端连接测试需真实 sshd, 沙箱环境不具备, 标注手测.
package ssh

import (
	"strings"
	"testing"

	"JuanNiang-Ops/internal/operator/modules"
)

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     modules.SSHConfig
		wantErr string
	}{
		{"空host", modules.SSHConfig{User: "root", Password: "x"}, "empty host"},
		{"空user", modules.SSHConfig{Host: "h", Password: "x"}, "empty user"},
		{"密码认证缺密码", modules.SSHConfig{Host: "h", User: "root"}, "empty password"},
		{"私钥认证缺私钥", modules.SSHConfig{Host: "h", User: "root", AuthType: "private_key"}, "empty private key"},
		{"未知认证类型", modules.SSHConfig{Host: "h", User: "root", Password: "x", AuthType: "kerberos"}, "unsupported auth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNewDefaults(t *testing.T) {
	c, err := New(modules.SSHConfig{Host: "h", User: "root", Password: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if c.cfg.Port != modules.DefaultSSHPort {
		t.Fatalf("port default = %d, want %d", c.cfg.Port, modules.DefaultSSHPort)
	}
	if c.cfg.Timeout <= 0 {
		t.Fatalf("timeout default not applied: %d", c.cfg.Timeout)
	}
	if c.cfg.AuthType != "password" {
		t.Fatalf("auth type default = %s, want password", c.cfg.AuthType)
	}
}
