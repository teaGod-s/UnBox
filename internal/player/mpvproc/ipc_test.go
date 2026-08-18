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
	// mpv 的位置观察者上报形如 {"event":"property-change","name":"time-pos","data":12.5}
	evt, ok := parseEvent([]byte(`{"event":"property-change","name":"time-pos","data":12.5}`))
	if !ok || evt.Kind != player.EventPosition || evt.Position != 12.5 {
		t.Fatalf("parseEvent = (%+v,%v), want position 12.5", evt, ok)
	}

	// EOF 事件形如 {"event":"end-file","reason":"eof"}
	evt, ok = parseEvent([]byte(`{"event":"end-file","reason":"eof"}`))
	if !ok || evt.Kind != player.EventEOF {
		t.Fatalf("parseEvent = (%+v,%v), want EOF", evt, ok)
	}

	// 非目标事件应返回 ok=false 而不是误报
	if _, ok := parseEvent([]byte(`{"event":"idle"}`)); ok {
		t.Fatal("idle 事件不应被当作位置/EOF 事件")
	}
}
