package mpvproc

import (
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestBuildArgsForceWindowWhenNoWid(t *testing.T) {
	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 0)
	if !contains(args, "--force-window=yes") {
		t.Fatalf("无 wid 时应有 --force-window=yes: %v", args)
	}
	if contains(args, "--wid=") {
		t.Fatalf("无 wid 时不应有 --wid: %v", args)
	}
	if !contains(args, "--osc=no") {
		t.Fatalf("无 wid 时应关 OSC: %v", args)
	}
}

func TestBuildArgsWidWhenSet(t *testing.T) {
	args := buildArgs(player.Stream{URL: "http://x/a.m3u8"}, "/tmp/x.sock", 42)
	if !contains(args, "--wid=42") {
		t.Fatalf("有 wid 时应有 --wid=42: %v", args)
	}
	if contains(args, "--force-window") {
		t.Fatalf("有 wid 时不应有 --force-window: %v", args)
	}
	if !contains(args, "--osc=yes") {
		t.Fatalf("有 wid 时应开 OSC: %v", args)
	}
}

func TestBuildArgsCarriesHeaders(t *testing.T) {
	s := player.Stream{URL: "http://x/a.m3u8", Headers: map[string]string{"Referer": "http://x/"}}
	args := buildArgs(s, "/tmp/x.sock", 0)
	if !contains(args, "--http-header-fields=Referer: http://x/") {
		t.Fatalf("应携带 http header: %v", args)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
