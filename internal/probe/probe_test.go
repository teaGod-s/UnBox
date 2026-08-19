package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(make([]byte, 64*1024))
	}))
	defer srv.Close()
	p := NewProber()
	r := p.Probe(context.Background(), srv.URL, nil)
	if !r.Reachable || r.Latency <= 0 || r.Speed <= 0 || r.Err != nil {
		t.Fatalf("Probe = %+v，期望可达且测出延迟/吞吐", r)
	}
}

func TestProbeUnreachable(t *testing.T) {
	p := NewProber()
	r := p.Probe(context.Background(), "http://127.0.0.1:1/nope", nil)
	if r.Reachable || r.Err == nil {
		t.Fatalf("Probe = %+v，期望不可达", r)
	}
}

func TestRankPutsReachableFirst(t *testing.T) {
	var reachable, dead string
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer deadSrv.Close()
	reachable = deadSrv.URL
	dead = "http://127.0.0.1:1/nope"
	got := NewProber().Rank(context.Background(), []string{dead, reachable}, nil)
	if got[0] != reachable {
		t.Fatalf("Rank = %v，期望可达源排最前", got)
	}
}

func TestRankSingleURLUnchanged(t *testing.T) {
	got := NewProber().Rank(context.Background(), []string{"http://x/1"}, nil)
	if len(got) != 1 || got[0] != "http://x/1" {
		t.Fatalf("Rank = %v", got)
	}
}

func TestRankBoundsTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer slow.Close()
	start := time.Now()
	NewProber().Rank(context.Background(), []string{slow.URL}, nil)
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("Rank 未受 1s 探测超时约束，耗时 %v", time.Since(start))
	}
}
