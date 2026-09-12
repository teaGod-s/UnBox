package mpvproc

import (
	"encoding/json"
	"fmt"
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

// parseEvent 解析 mpv 上报的事件行，只返回播放器关心的位置、播放状态和
// 结束事件。其余事件（idle、pause、start-file 等）返回 ok=false。
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
	case raw.Event == "end-file":
		switch raw.Reason {
		case "eof":
			return player.Event{Kind: player.EventEOF}, true
		case "error":
			return player.Event{
				Kind: player.EventError,
				Err:  fmt.Errorf("mpv 播放出错: %s", raw.Reason),
			}, true
		}
		// stop/quit/redirect 等不是播放失败：前者出现在停止播放或换文件时，
		// quit 是进程退出，redirect 是播放列表条目被重定向替换。把它们当成
		// 故障会误触发自动换源，因此直接忽略。
		return player.Event{}, false
	case raw.Event == "property-change" && raw.Name == "time-pos":
		f, err := strconv.ParseFloat(string(raw.Data), 64)
		if err != nil {
			return player.Event{}, false
		}
		return player.Event{Kind: player.EventPosition, Position: f}, true
	case raw.Event == "property-change" && raw.Name == "paused-for-cache":
		var paused *bool
		if err := json.Unmarshal(raw.Data, &paused); err != nil || paused == nil {
			return player.Event{}, false
		}
		if *paused {
			return player.Event{Kind: player.EventBuffering}, true
		}
		return player.Event{Kind: player.EventPlaying}, true
	}
	return player.Event{}, false
}

// parsePauseProperty 解析 mpv 的 pause 属性变化。pause 本身不映射为缓冲或
// 播放信号（用户主动暂停不是缓冲），只用于在暂停期间屏蔽缓存状态事件，
// 避免把「缓存回填」误报成恢复播放、把「缓存见底」误报成缓冲。
func parsePauseProperty(line []byte) (paused bool, ok bool) {
	var raw struct {
		Event string          `json:"event"`
		Name  string          `json:"name"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return false, false
	}
	if raw.Event != "property-change" || raw.Name != "pause" {
		return false, false
	}
	var value *bool
	if err := json.Unmarshal(raw.Data, &value); err != nil || value == nil {
		return false, false
	}
	return *value, true
}
