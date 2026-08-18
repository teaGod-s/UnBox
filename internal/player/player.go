// Package player 定义 Unbox 的播放层抽象与媒体类型。
//
// 本包不依赖 Wails，也不依赖任何具体播放后端；UI 层与 Provider 层只面对
// Player 接口，更换播放实现（mpvproc ↔ mpvlib）不触动它们。
package player

import "context"

// StreamKind 是播放流的容器/传输形态。
type StreamKind int

const (
	StreamHLS StreamKind = iota
	StreamMP4
	StreamFLV
	StreamRTMP
	StreamLocal // 本地文件或本地代理地址
)

func (k StreamKind) String() string {
	switch k {
	case StreamHLS:
		return "hls"
	case StreamMP4:
		return "mp4"
	case StreamFLV:
		return "flv"
	case StreamRTMP:
		return "rtmp"
	case StreamLocal:
		return "local"
	default:
		return "unknown"
	}
}

// TrackKind 是可被 SelectTrack 选中的轨道类型。
type TrackKind int

const (
	TrackAudio TrackKind = iota
	TrackSubtitle
	TrackVideo
)

func (k TrackKind) String() string {
	switch k {
	case TrackAudio:
		return "audio"
	case TrackSubtitle:
		return "subtitle"
	case TrackVideo:
		return "video"
	default:
		return "unknown"
	}
}

// SubtitleTrack 是一条外挂字幕轨。
type SubtitleTrack struct {
	URL     string
	Lang    string
	Default bool
}

// Stream 是播放一条媒体所需的一切信息。
type Stream struct {
	URL      string
	Headers  map[string]string // Referer / UA / Cookie 等
	Kind     StreamKind
	Subtitle []SubtitleTrack
	Backups  []string // 同频道备用流，供测速切换使用
}

// PlayState 是播放状态机的基本态。
type PlayState int

const (
	StateStopped PlayState = iota
	StatePlaying
	StatePaused
	StateBuffering
)

// State 是播放器当前的完整可观测状态。
type State struct {
	Playing  PlayState
	Position float64 // 秒
	Duration float64 // 秒；未知时为 -1
	Volume   int     // 0–100
}

// EventKind 是播放器通过 Events() 上报的事件类型。
type EventKind int

const (
	EventPosition  EventKind = iota // 周期性位置更新，携带 Position
	EventBuffering                  // 缓冲中
	EventError                      // 播放出错，Err 非空
	EventEOF                        // 播放自然结束
)

// Event 是播放器上报的异步事件。
type Event struct {
	Kind     EventKind
	Position float64
	Err      error
}

// Player 是所有播放实现的统一接口。
type Player interface {
	Load(ctx context.Context, s Stream) error
	Play() error
	Pause() error
	Seek(sec float64) error
	SetVolume(v int) error
	SelectTrack(kind TrackKind, id int) error
	State() State
	Events() <-chan Event
	Close() error
}
