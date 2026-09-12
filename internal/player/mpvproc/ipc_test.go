package mpvproc

import (
	"encoding/json"
	"testing"

	"github.com/unbox/unbox/internal/player"
)

func TestEncodeCommand(t *testing.T) {
	got := encodeCommand([]any{"loadfile", "/tmp/a.m3u8", "replace"})
	want := `{"command":["loadfile","/tmp/a.m3u8","replace"]}` + "\n"
	if got != want {
		t.Fatalf("encodeCommand = %q, want %q", got, want)
	}
}

func TestEncodeSetProperty(t *testing.T) {
	got := encodeCommand([]any{"set_property", "volume", 80})
	// volume 是数值，必须原样编码，不能被引号包成字符串
	var probe struct {
		Command []json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal([]byte(got), &probe); err != nil {
		t.Fatalf("encodeCommand 产出非法 JSON: %v", err)
	}
	if len(probe.Command) != 3 {
		t.Fatalf("command 长度 = %d, want 3", len(probe.Command))
	}
	if string(probe.Command[2]) != "80" {
		t.Fatalf("volume 被编码为 %s, want 80（数值）", probe.Command[2])
	}
}

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  player.EventKind
	}{
		{name: "cache start", input: `{"event":"property-change","name":"paused-for-cache","data":true}`, kind: player.EventBuffering},
		{name: "cache end", input: `{"event":"property-change","name":"paused-for-cache","data":false}`, kind: player.EventPlaying},
		{name: "position", input: `{"event":"property-change","name":"time-pos","data":12.5}`, kind: player.EventPosition},
		{name: "eof", input: `{"event":"end-file","reason":"eof"}`, kind: player.EventEOF},
		{name: "error", input: `{"event":"end-file","reason":"error"}`, kind: player.EventError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt, ok := parseEvent([]byte(tt.input))
			if !ok || evt.Kind != tt.kind {
				t.Fatalf("parseEvent = (%+v,%v), want kind %v", evt, ok, tt.kind)
			}
			if tt.kind == player.EventError && evt.Err == nil {
				t.Fatal("end-file error should carry an error")
			}
		})
	}

	// 普通 pause 属性不是缓冲信号，避免把用户主动暂停误报成缓冲。
	if _, ok := parseEvent([]byte(`{"event":"property-change","name":"pause","data":true}`)); ok {
		t.Fatal("pause 事件不应被当作播放状态事件")
	}
	// 非目标事件应返回 ok=false 而不是误报。
	if _, ok := parseEvent([]byte(`{"event":"idle"}`)); ok {
		t.Fatal("idle 事件不应被当作位置/播放状态事件")
	}
	// stop/quit/redirect 不是播放失败，报成错误会误触发自动换源。
	for _, reason := range []string{"stop", "quit", "redirect", ""} {
		input := `{"event":"end-file","reason":"` + reason + `"}`
		if _, ok := parseEvent([]byte(input)); ok {
			t.Fatalf("end-file reason=%q 不应被当作播放失败", reason)
		}
	}
}

func TestParsePauseProperty(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		paused bool
		ok     bool
	}{
		{name: "paused", input: `{"event":"property-change","name":"pause","data":true}`, paused: true, ok: true},
		{name: "resumed", input: `{"event":"property-change","name":"pause","data":false}`, paused: false, ok: true},
		{name: "other property", input: `{"event":"property-change","name":"time-pos","data":1}`, ok: false},
		{name: "null data", input: `{"event":"property-change","name":"pause","data":null}`, ok: false},
		{name: "non property event", input: `{"event":"end-file","reason":"eof"}`, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paused, ok := parsePauseProperty([]byte(tt.input))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && paused != tt.paused {
				t.Fatalf("paused = %v, want %v", paused, tt.paused)
			}
		})
	}
}
