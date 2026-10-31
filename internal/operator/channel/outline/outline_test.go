// Outline 通道单测: httptest 假服务端验证搜索/读取往返与 Bearer 认证/信封解码.
// 真实 Outline 连通性按 TEST 环境手测.
package outline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"JuanNiang-Ops/internal/operator/modules"
)

func TestSearchDocs(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents.search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Query != "巡检" || body.Limit != 5 {
			t.Errorf("body mismatch: %+v", body)
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":[{"id":"doc-1","title":"巡检手册","context":"定期巡检…"}]}`))
	}))
	defer srv.Close()

	c, err := New(modules.OutlineConfig{Endpoint: srv.URL, APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}

	hits, err := c.SearchDocs(context.Background(), "巡检", 5)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if len(hits) != 1 || hits[0].PageID != "doc-1" || hits[0].Title != "巡检手册" {
		t.Fatalf("hits mismatch: %+v", hits)
	}
}

func TestGetDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/documents.info" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{"id":"doc-1","title":"手册","text":"# 巡检\n步骤…"}}`))
	}))
	defer srv.Close()

	c, _ := New(modules.OutlineConfig{Endpoint: srv.URL, APIKey: "sk-test"})
	page, err := c.GetDoc(context.Background(), "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "手册" || !strings.HasPrefix(page.Markdown, "# 巡检") {
		t.Fatalf("page mismatch: %+v", page)
	}
}

func TestEnvelopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Outline 认证失败返回 401 + 非 ok 信封
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error":"authentication failed"}`))
	}))
	defer srv.Close()

	c, _ := New(modules.OutlineConfig{Endpoint: srv.URL, APIKey: "bad"})
	if err := c.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want API error 401, got %v", err)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(modules.OutlineConfig{APIKey: "k"}); err == nil || !strings.Contains(err.Error(), "empty endpoint") {
		t.Fatalf("want empty endpoint error, got %v", err)
	}
	if _, err := New(modules.OutlineConfig{Endpoint: "http://x"}); err == nil || !strings.Contains(err.Error(), "empty api key") {
		t.Fatalf("want empty api key error, got %v", err)
	}
}
