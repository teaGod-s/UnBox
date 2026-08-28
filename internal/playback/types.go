// Package playback 编排流解析、Web 代理与 mpv 降级，不依赖 Wails。
package playback

// Backend 是一次播放计划选择的渲染后端。
type Backend string

const (
	BackendWeb Backend = "web"
	BackendMPV Backend = "mpv"
)

// Plan 是壳层返回给前端的播放计划。
type Plan struct {
	ID          string
	Backend     Backend
	URL         string
	Kind        string
	CanFallback bool
}
