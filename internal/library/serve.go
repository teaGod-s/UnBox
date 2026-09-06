package library

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

// server 是只监听 127.0.0.1、按进程内 id 映射文件的本地 HTTP 服务。
type server struct {
	mu     sync.Mutex
	ln     net.Listener
	ids    map[string]string
	seq    int
	token  string
	closed bool
}

func newServer() *server {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("生成本地媒体服务 token 失败: %v", err))
	}
	return &server{ids: make(map[string]string), token: hex.EncodeToString(b)}
}

// register 登记一个文件路径并返回可访问的 URL。
func (s *server) register(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("%d", s.seq)
	s.ids[id] = absPath
	if s.ln == nil && !s.closed {
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err == nil {
			s.ln = ln
			go func() {
				_ = http.Serve(ln, s.handler())
			}()
		}
	}
	host := "127.0.0.1"
	if s.ln != nil {
		host = s.ln.Addr().String()
	}
	return fmt.Sprintf("http://%s/v/%s?t=%s", host, id, s.token)
}

func (s *server) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != s.token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v/")
		s.mu.Lock()
		path, ok := s.ids[id]
		s.mu.Unlock()
		if !ok || id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path)
	})
}

func (s *server) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		s.closed = true
		return nil
	}
	ln := s.ln
	s.ln = nil
	s.closed = true
	err := ln.Close()
	if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		return err
	}
	return nil
}
