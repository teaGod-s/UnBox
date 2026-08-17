package config

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchHTTPSendsOkHTTPUA(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte(`{"sites":[]}`))
	}))
	defer srv.Close()

	f := NewFetcher()
	body, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("拉取失败: %v", err)
	}
	const want = "okhttp/3.12.11"
	if gotUA != want {
		t.Errorf("User-Agent 不符: got %q, want %q", gotUA, want)
	}
	const wantBody = `{"sites":[]}`
	if string(body) != wantBody {
		t.Errorf("内容不符: got %q, want %q", body, wantBody)
	}
}

func TestFetchLocalFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	const content = `{"sites":[]}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	f := NewFetcher()
	body, err := f.Fetch(context.Background(), p)
	if err != nil {
		t.Fatalf("本地文件读取失败: %v", err)
	}
	if string(body) != content {
		t.Errorf("内容不符: got %q, want %q", body, content)
	}
}

func TestFetchFileScheme(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c2.json")
	const content = `{"sites":[{"key":"a"}]}`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	f := NewFetcher()
	// file:// URLs use forward slashes; on Windows filepath.Join gives
	// backslashes, so normalize before building the URL.
	ref := "file://" + filepath.ToSlash(p)
	body, err := f.Fetch(context.Background(), ref)
	if err != nil {
		t.Fatalf("file:// 读取失败: %v", err)
	}
	if string(body) != content {
		t.Errorf("内容不符: got %q, want %q", body, content)
	}
}

func TestFetchClanSchemeRejected(t *testing.T) {
	f := NewFetcher()
	_, err := f.Fetch(context.Background(), "clan://localhost/tvbox/dc.txt")
	if err == nil {
		t.Fatal("clan:// 在无本地仓库上下文时应当返回明确 error")
	}
	if !strings.Contains(err.Error(), "clan") {
		t.Errorf("错误信息应说明 clan 协议: %v", err)
	}
}

func TestFetchFollowsRedirects(t *testing.T) {
	var gotUA string
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte(`{"sites":["final"]}`))
	}))
	defer final.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirector.Close()

	f := NewFetcher()
	body, err := f.Fetch(context.Background(), redirector.URL)
	if err != nil {
		t.Fatalf("跟随重定向失败: %v", err)
	}
	const wantBody = `{"sites":["final"]}`
	if string(body) != wantBody {
		t.Errorf("重定向后内容不符: got %q, want %q", body, wantBody)
	}
	const wantUA = "okhttp/3.12.11"
	if gotUA != wantUA {
		t.Errorf("重定向后 User-Agent 不符: got %q, want %q", gotUA, wantUA)
	}
}

func TestFetchOverCapBodyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", maxDecodedSize+1))
		chunk := make([]byte, 1<<20) // 1 MiB chunks to avoid a huge single allocation
		remaining := int64(maxDecodedSize + 1)
		for remaining > 0 {
			n := int64(len(chunk))
			if remaining < n {
				n = remaining
			}
			w.Write(chunk[:n])
			remaining -= n
		}
	}))
	defer srv.Close()

	f := NewFetcher()
	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("超过大小上限的响应体应当返回 error，而不是静默截断")
	}
}

func TestFetchNonOKStatusRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	f := NewFetcher()
	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("非 2xx 状态码应当返回 error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("错误信息应包含状态码 404: %v", err)
	}
}
