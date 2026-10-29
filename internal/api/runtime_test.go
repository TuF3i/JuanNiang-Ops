package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"JuanNiang-Ops/internal/api/response"
)

// newTestRuntime 连接测试库与测试 Redis 并创建 API Runtime (httptest 语义, 不占端口).
// 需要环境变量 TEST_PG_DSN 与 TEST_REDIS_ADDR, 任一未设置则跳过 (数据库/Redis 不进单测的仓库惯例).
func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("TEST_PG_DSN or TEST_REDIS_ADDR not set, skip db-backed api test")
	}

	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("connect test db: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(t.Context()).Err(); err != nil {
		t.Skipf("connect test redis: %v", err)
	}

	rt, err := CreateRuntime(WithDB(db), WithRedis(rdb), WithAddr(":0"))
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.ReleaseRuntime() })
	return rt
}

// do 向引擎发起 JSON 请求并解析统一响应 (FinalResponse: status/info/data)
func do(t *testing.T, rt *Runtime, method, path, token string, body any) map[string]any {
	t.Helper()
	var req *http.Request
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req = httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	w := httptest.NewRecorder()
	rt.Engine().ServeHTTP(w, req)

	// 业务码模式: HTTP 恒 200, 断言走 status 字段
	if w.Code != http.StatusOK {
		t.Fatalf("http status = %d (want 200), body %s", w.Code, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %s: %v", w.Body.String(), err)
	}
	return result
}

// statusOf 提取响应的业务状态码
func statusOf(t *testing.T, result map[string]any) uint {
	t.Helper()
	f, ok := result["status"].(float64)
	if !ok {
		t.Fatalf("missing status field: %v", result)
	}
	return uint(f)
}

// expectStatus 断言业务状态码
func expectStatus(t *testing.T, result map[string]any, want uint) {
	t.Helper()
	if got := statusOf(t, result); got != want {
		t.Fatalf("status = %d, want %d (body %v)", got, want, result)
	}
}

// login 用指定账号登录并返回双 Token
func login(t *testing.T, rt *Runtime, username, password string) (access, refresh string) {
	t.Helper()
	result := do(t, rt, http.MethodPost, "/api/v1/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	expectStatus(t, result, response.OperationSuccess.Status)

	data, _ := result["data"].(map[string]any)
	tokens, _ := data["tokens"].(map[string]any)
	access, _ = tokens["access_token"].(string)
	refresh, _ = tokens["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("login ok but token pair incomplete: %v", result)
	}
	return access, refresh
}

func TestLoginSuccessAndFailure(t *testing.T) {
	rt := newTestRuntime(t)

	// 正确账号 (默认种子 admin/admin123): 返回双 Token
	access, refresh := login(t, rt, "admin", "admin123")
	if len(access) < 32 || len(refresh) < 32 {
		t.Fatalf("token too short: access=%d refresh=%d", len(access), len(refresh))
	}
	if access == refresh {
		t.Fatal("access and refresh token should differ")
	}

	// 错误密码 -> UserNotExiOrWrongPwd
	result := do(t, rt, http.MethodPost, "/api/v1/login", "", map[string]string{
		"username": "admin", "password": "wrong",
	})
	expectStatus(t, result, response.UserNotExiOrWrongPwd.Status)

	// 不存在的用户 -> 同一错误 (防枚举)
	result = do(t, rt, http.MethodPost, "/api/v1/login", "", map[string]string{
		"username": "nobody", "password": "x",
	})
	expectStatus(t, result, response.UserNotExiOrWrongPwd.Status)

	// 缺参数 -> RevDataError
	result = do(t, rt, http.MethodPost, "/api/v1/login", "", map[string]string{"username": "admin"})
	expectStatus(t, result, response.RevDataError.Status)
}

func TestAuthMiddleware(t *testing.T) {
	rt := newTestRuntime(t)

	// 无 token -> EmptyToken
	result := do(t, rt, http.MethodGet, "/api/v1/user/info", "", nil)
	expectStatus(t, result, response.EmptyToken.Status)

	// 无效 token -> InvalidToken
	result = do(t, rt, http.MethodGet, "/api/v1/user/info", "fake-token", nil)
	expectStatus(t, result, response.InvalidToken.Status)

	// 有效 AccessToken -> 成功且无密码字段泄漏
	access, _ := login(t, rt, "admin", "admin123")
	result = do(t, rt, http.MethodGet, "/api/v1/user/info", access, nil)
	expectStatus(t, result, response.OperationSuccess.Status)
	if raw, _ := json.Marshal(result); bytes.Contains(raw, []byte(`"password"`)) {
		t.Fatalf("password leaked in user info: %s", raw)
	}

	// RefreshToken 不能当 AccessToken 用 (前缀隔离)
	_, refresh := login(t, rt, "admin", "admin123")
	result = do(t, rt, http.MethodGet, "/api/v1/user/info", refresh, nil)
	expectStatus(t, result, response.InvalidToken.Status)
}

func TestRefreshFlow(t *testing.T) {
	rt := newTestRuntime(t)
	access, refresh := login(t, rt, "admin", "admin123")

	// 旧 access 可用
	result := do(t, rt, http.MethodGet, "/api/v1/user/info", access, nil)
	expectStatus(t, result, response.OperationSuccess.Status)

	// 刷新: 换发新对
	result = do(t, rt, http.MethodPost, "/api/v1/token/refresh", "",
		map[string]string{"refresh_token": refresh})
	expectStatus(t, result, response.OperationSuccess.Status)
	data, _ := result["data"].(map[string]any)
	tokens, _ := data["tokens"].(map[string]any)
	newAccess, _ := tokens["access_token"].(string)
	newRefresh, _ := tokens["refresh_token"].(string)
	if newAccess == "" || newRefresh == "" || newRefresh == refresh {
		t.Fatalf("refresh should rotate: %v", tokens)
	}

	// 旧 RefreshToken 已作废 (旋转防重放) -> InvalidRefresh
	result = do(t, rt, http.MethodPost, "/api/v1/token/refresh", "",
		map[string]string{"refresh_token": refresh})
	expectStatus(t, result, response.InvalidRefresh.Status)

	// 新 access 可用
	result = do(t, rt, http.MethodGet, "/api/v1/user/info", newAccess, nil)
	expectStatus(t, result, response.OperationSuccess.Status)
}

func TestLogoutFlow(t *testing.T) {
	rt := newTestRuntime(t)
	access, refresh := login(t, rt, "admin", "admin123")

	// 登出: 吊销双 Token
	result := do(t, rt, http.MethodPost, "/api/v1/token/logout", "",
		map[string]string{"access_token": access, "refresh_token": refresh})
	expectStatus(t, result, response.OperationSuccess.Status)

	// 双 Token 全部失效
	result = do(t, rt, http.MethodGet, "/api/v1/user/info", access, nil)
	expectStatus(t, result, response.InvalidToken.Status)
	result = do(t, rt, http.MethodPost, "/api/v1/token/refresh", "",
		map[string]string{"refresh_token": refresh})
	expectStatus(t, result, response.InvalidRefresh.Status)
}

func TestChangePasswordFlow(t *testing.T) {
	rt := newTestRuntime(t)
	access, _ := login(t, rt, "admin", "admin123")

	// 旧密码错误 -> WrongPassword
	result := do(t, rt, http.MethodPut, "/api/v1/user/password", access,
		map[string]string{"old_password": "bad", "new_password": "newpass456"})
	expectStatus(t, result, response.WrongPassword.Status)

	// 正确修改 -> 成功
	result = do(t, rt, http.MethodPut, "/api/v1/user/password", access,
		map[string]string{"old_password": "admin123", "new_password": "newpass456"})
	expectStatus(t, result, response.OperationSuccess.Status)

	// 旧密码登录失败, 新密码登录成功
	result = do(t, rt, http.MethodPost, "/api/v1/login", "",
		map[string]string{"username": "admin", "password": "admin123"})
	expectStatus(t, result, response.UserNotExiOrWrongPwd.Status)
	newAccess, _ := login(t, rt, "admin", "newpass456")

	// 还原密码 (避免污染其他用例/环境)
	result = do(t, rt, http.MethodPut, "/api/v1/user/password", newAccess,
		map[string]string{"old_password": "newpass456", "new_password": "admin123"})
	expectStatus(t, result, response.OperationSuccess.Status)
}
