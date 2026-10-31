// Loki 通道单测: httptest 假服务端验证 query_range 往返与参数拼装.
// 真实 Loki 连通性按 TEST 环境手测.
package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"JuanNiang-Ops/internal/operator/modules"
)

func TestQueryLogs(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/loki/api/v1/query_range") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[
			{"stream":{"job":"nginx"},"values":[["1700000000000000000","GET / 200"]]}
		]}}`))
	}))
	defer srv.Close()

	c, err := New(modules.LokiConfig{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	entries, err := c.QueryLogs(context.Background(), modules.LogQuery{
		Selector: `{job="nginx"}`,
		Limit:    10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != `{job="nginx"}` {
		t.Fatalf("query param = %q", gotQuery)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Line != "GET / 200" || e.Labels["job"] != "nginx" {
		t.Fatalf("entry mismatch: %+v", e)
	}
	if e.Timestamp.UnixNano() != 1700000000000000000 {
		t.Fatalf("timestamp = %d", e.Timestamp.UnixNano())
	}
}

func TestQueryLogsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`parse error at char 5`))
	}))
	defer srv.Close()

	c, err := New(modules.LokiConfig{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.QueryLogs(context.Background(), modules.LogQuery{Selector: `{bad`})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("want API error 400, got %v", err)
	}
}

func TestQueryLogsEmptySelector(t *testing.T) {
	c, _ := New(modules.LokiConfig{Endpoint: "http://localhost:1"})
	if _, err := c.QueryLogs(context.Background(), modules.LogQuery{}); err == nil || !strings.Contains(err.Error(), "empty selector") {
		t.Fatalf("want empty selector error, got %v", err)
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/labels" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":["job"]}`))
	}))
	defer srv.Close()

	c, _ := New(modules.LokiConfig{Endpoint: srv.URL, Timeout: int(time.Second / time.Millisecond)})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
