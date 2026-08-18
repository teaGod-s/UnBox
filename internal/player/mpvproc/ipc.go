package mpvproc

import (
	"encoding/json"
	"strconv"

	"github.com/unbox/unbox/internal/player"
)

// encodeCommand 把一条 mpv JSON IPC 命令编码为以换行结尾的请求行。
func encodeCommand(args []any) string {
	b, err := json.Marshal(map[string]any{"command": args})
	if err != nil {
		// args 均为字符串与数值，Marshal 不会失败；此处防御性兜底
		return "{}" + "\n"
	}
	return string(b) + "\n"
}

// parseEvent 解析 mpv 上报的事件行，只返回播放器关心的位置/缓冲/EOF 事件。
// 其余事件（idle、pause、start-file 等）返回 ok=false。
func parseEvent(line []byte) (player.Event, bool) {
	var raw struct {
		Event  string          `json:"event"`
		Name   string          `json:"name"`
		Data   json.RawMessage `json:"data"`
		Reason string          `json:"reason"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return player.Event{}, false
	}
	switch {
	case raw.Event == "end-file" && raw.Reason == "eof":
		return player.Event{Kind: player.EventEOF}, true
	case raw.Event == "property-change" && raw.Name == "time-pos":
		f, err := strconv.ParseFloat(string(raw.Data), 64)
		if err != nil {
			return player.Event{}, false
		}
		return player.Event{Kind: player.EventPosition, Position: f}, true
	}
	return player.Event{}, false
}
