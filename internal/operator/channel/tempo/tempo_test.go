// Tempo 通道单测: httptest 假服务端验证搜索/拉取往返与 TraceQL 拼装.
// 真实 Tempo 连通性按 TEST 环境手测.
package tempo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"JuanNiang-Ops/internal/operator/modules"
)

func TestSearchTraces(t *testing.T) {
	var gotQ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotQ = r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{"traces":[
			{"traceID":"abc123","rootServiceName":"api","rootTraceName":"GET /users","startTimeUnixNano":"1700000000000000000","durationMs":42}
		]}`))
	}))
	defer srv.Close()

	c, err := New(modules.TempoConfig{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	traces, err := c.SearchTraces(context.Background(), modules.TraceQuery{
		ServiceName: "api",
		MinDuration: "1s",
		Limit:       5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotQ != `{ service.name="api" && minDuration="1s" }` {
		t.Fatalf("TraceQL = %q", gotQ)
	}
	if len(traces) != 1 {
		t.Fatalf("traces = %d, want 1", len(traces))
	}
	sum := traces[0]
	if sum.TraceID != "abc123" || sum.RootServiceName != "api" || sum.DurationMS != 42 {
		t.Fatalf("summary mismatch: %+v", sum)
	}
	if sum.StartTime.UnixNano() != 1700000000000000000 {
		t.Fatalf("start time = %d", sum.StartTime.UnixNano())
	}
}

func TestSearchTracesEmptyConditions(t *testing.T) {
	c, _ := New(modules.TempoConfig{Endpoint: "http://localhost:1"})
	if _, err := c.SearchTraces(context.Background(), modules.TraceQuery{}); err == nil || !strings.Contains(err.Error(), "empty query conditions") {
		t.Fatalf("want empty conditions error, got %v", err)
	}
}

func TestGetTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/traces/abc") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"batches":[]}`))
	}))
	defer srv.Close()

	c, _ := New(modules.TempoConfig{Endpoint: srv.URL})
	data, err := c.GetTrace(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "batches") {
		t.Fatalf("trace body = %s", data)
	}
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"stats":{}}`)
	}))
	defer srv.Close()

	c, _ := New(modules.TempoConfig{Endpoint: srv.URL, Timeout: int(time.Second / time.Millisecond)})
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
