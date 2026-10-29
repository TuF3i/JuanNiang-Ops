package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newTestRuntime 连接测试库并创建 API Runtime (httptest 语义, 不占端口).
// 需要环境变量 TEST_PG_DSN, 未设置则跳过 (数据库不进单测的仓库惯例).
func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skip db-backed api test")
	}

	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("connect test db: %v", err)
	}

	rt, err := CreateRuntime(WithDB(db), WithAddr(":0"))
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.ReleaseRuntime() })
	return rt
}

// post 向引擎发起 JSON 请求
func post(t *testing.T, rt *Runtime, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %s: %v", w.Body.String(), err)
	}
	return w.Result(), result
}

// login 用指定账号登录并返回 token
func login(t *testing.T, rt *Runtime, username, password string) string {
	t.Helper()
	resp, result := post(t, rt, "/api/v1/login", map[string]string{
		"username": username,
		"password": password,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: http %d body %v", username, resp.StatusCode, result)
	}
	data, _ := result["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("login ok but token empty: %v", result)
	}
	return token
}

func TestLoginSuccessAndFailure(t *testing.T) {
	rt := newTestRuntime(t)

	// 正确账号 (默认种子 admin/admin123)
	token := login(t, rt, "admin", "admin123")
	if len(token) < 32 {
		t.Fatalf("token too short: %q", token)
	}

	// 错误密码 -> 401
	resp, result := post(t, rt, "/api/v1/login", map[string]string{
		"username": "admin", "password": "wrong",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: http %d body %v", resp.StatusCode, result)
	}

	// 不存在的用户 -> 401 (统一错误, 防枚举)
	resp, _ = post(t, rt, "/api/v1/login", map[string]string{
		"username": "nobody", "password": "x",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown user: http %d", resp.StatusCode)
	}

	// 缺参数 -> 400
	resp, _ = post(t, rt, "/api/v1/login", map[string]string{"username": "admin"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing param: http %d", resp.StatusCode)
	}
}

func TestAuthMiddleware(t *testing.T) {
	rt := newTestRuntime(t)

	// 无 token -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/info", nil)
	w := httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: http %d", w.Code)
	}

	// 无效 token -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/user/info", nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	w = httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: http %d", w.Code)
	}

	// 有效 token -> 200 且无密码字段泄漏
	token := login(t, rt, "admin", "admin123")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/user/info", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("valid token: http %d body %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); bytes.Contains([]byte(body), []byte(`"password"`)) {
		t.Fatalf("password leaked in user info: %s", body)
	}
}

func TestChangePasswordFlow(t *testing.T) {
	rt := newTestRuntime(t)
	token := login(t, rt, "admin", "admin123")

	// 旧密码错误 -> 400
	req := httptest.NewRequest(http.MethodPut, "/api/v1/user/password",
		bytes.NewBufferString(`{"old_password":"bad","new_password":"newpass456"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong old password: http %d body %s", w.Code, w.Body.String())
	}

	// 正确修改 -> 200
	req = httptest.NewRequest(http.MethodPut, "/api/v1/user/password",
		bytes.NewBufferString(`{"old_password":"admin123","new_password":"newpass456"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("change password: http %d body %s", w.Code, w.Body.String())
	}

	// 旧密码登录失败, 新密码登录成功
	resp, _ := post(t, rt, "/api/v1/login", map[string]string{"username": "admin", "password": "admin123"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password should fail after change: http %d", resp.StatusCode)
	}
	_ = login(t, rt, "admin", "newpass456")

	// 还原密码 (避免污染其他用例/环境)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/user/password",
		bytes.NewBufferString(`{"old_password":"newpass456","new_password":"admin123"}`))
	req.Header.Set("Authorization", "Bearer "+login(t, rt, "admin", "newpass456"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("restore password: http %d body %s", w.Code, w.Body.String())
	}
}
