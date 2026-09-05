package playback

import (
	"context"
	"errors"
	"testing"

	"github.com/unbox/unbox/internal/player"
)

type fakeResolver struct{ stream player.Stream }

func (f fakeResolver) Resolve(context.Context, player.Stream) (player.Stream, error) {
	if f.stream.URL == "" {
		return player.Stream{}, nil
	}
	return f.stream, nil
}

type fakeProxy struct{ next int }

func (f *fakeProxy) Register(context.Context, player.Stream) (string, error) {
	f.next++
	return "http://127.0.0.1/proxy/test", nil
}
func (f *fakeProxy) Close() error { return nil }

type fakePlayer struct {
	loaded []player.Stream
	played int
	seeked []float64
}

func (f *fakePlayer) Load(_ context.Context, s player.Stream) error {
	f.loaded = append(f.loaded, s)
	return nil
}
func (f *fakePlayer) Play() error                             { f.played++; return nil }
func (f *fakePlayer) Pause() error                            { return nil }
func (f *fakePlayer) Seek(sec float64) error { f.seeked = append(f.seeked, sec); return nil }
func (f *fakePlayer) SetVolume(int) error                     { return nil }
func (f *fakePlayer) SelectTrack(player.TrackKind, int) error { return nil }
func (f *fakePlayer) State() player.State                     { return player.State{} }
func (f *fakePlayer) Events() <-chan player.Event             { return make(chan player.Event) }
func (f *fakePlayer) Close() error                            { return nil }

func TestControllerRouting(t *testing.T) {
	mpv := &fakePlayer{}
	web := &fakeProxy{}
	c := NewController(nil, web, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }

	cases := []struct {
		name   string
		webMSE bool
		stream player.Stream
		want   Backend
	}{
		{"h264 hls web-capable", true, player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS}, BackendWeb},
		{"flv web-capable", true, player.Stream{URL: "https://x/live.flv", Kind: player.StreamFLV}, BackendWeb},
		{"mp4 web-capable", true, player.Stream{URL: "https://x/a.mp4", Kind: player.StreamMP4}, BackendWeb},
		{"rtmp", true, player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP}, BackendMPV},
		{"local", true, player.Stream{URL: "file:///x/a.mkv", Kind: player.StreamLocal}, BackendMPV},
		{"hls no-mse", false, player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS}, BackendMPV},
		{"flv no-mse", false, player.Stream{URL: "https://x/live.flv", Kind: player.StreamFLV}, BackendMPV},
		{"ts no-mse", false, player.Stream{URL: "https://x/live.ts", Kind: player.StreamTS}, BackendMPV},
		{"mp4 no-mse", false, player.Stream{URL: "https://x/a.mp4", Kind: player.StreamMP4}, BackendWeb},
	}
	for _, tc := range cases {
		c.SetWebMSE(tc.webMSE)
		mpv.loaded = nil
		mpv.played = 0
		got, err := c.Prepare(context.Background(), tc.stream)
		if err != nil || got.Backend != tc.want {
			t.Errorf("%s: Prepare = %+v, %v; want backend %s", tc.name, got, err, tc.want)
		}
	}
}

func TestControllerHEVCHLSRoutesToMPV(t *testing.T) {
	mpv := &fakePlayer{}
	c := NewController(nil, &fakeProxy{}, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "hvc1.1.6.L93.B0", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.Backend != BackendMPV {
		t.Fatalf("HEVC 应路由到 mpv: plan=%+v err=%v", plan, err)
	}
	if len(mpv.loaded) != 1 || mpv.played != 1 {
		t.Fatalf("mpv 后端未真正加载: loaded=%d played=%d", len(mpv.loaded), mpv.played)
	}
}

func TestControllerMPVBackendLoadsStream(t *testing.T) {
	mpv := &fakePlayer{}
	c := NewController(nil, &fakeProxy{}, mpv)
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP})
	if err != nil || plan.Backend != BackendMPV {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	if len(mpv.loaded) != 1 || mpv.loaded[0].URL != "rtmp://x/live" || mpv.played != 1 {
		t.Fatalf("mpv 后端未真正加载: loaded=%+v played=%d", mpv.loaded, mpv.played)
	}
}

func TestControllerFallbackLoadsOriginalStream(t *testing.T) {
	proxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, proxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.ID == "" {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	got, err := c.Fallback(context.Background(), plan.ID, 0)
	if err != nil || got.Backend != BackendMPV || len(mpv.loaded) != 1 || mpv.loaded[0].URL != "https://x/live.m3u8" || mpv.played != 1 {
		t.Fatalf("Fallback = %+v, %v, loaded=%+v played=%d", got, err, mpv.loaded, mpv.played)
	}
	if _, err := c.Fallback(context.Background(), plan.ID, 0); err == nil {
		t.Fatal("重复 fallback 应失败")
	}
}

// TestControllerFallbackSeeksToWatchedPosition 断言降级时把网页播放器已看到
// 的位置透传给 mpv——否则 mpv 从视频开头重播，用户丢掉的进度正是本次要修的症状。
func TestControllerFallbackSeeksToWatchedPosition(t *testing.T) {
	proxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, proxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/vod.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.ID == "" {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	got, err := c.Fallback(context.Background(), plan.ID, 123.5)
	if err != nil || got.Backend != BackendMPV {
		t.Fatalf("Fallback = %+v, %v", got, err)
	}
	if len(mpv.seeked) != 1 || mpv.seeked[0] != 123.5 {
		t.Fatalf("降级应定位到已看位置: seeked=%v", mpv.seeked)
	}
}

// TestControllerFallbackZeroStartDoesNotSeek 直播从头接流，start=0 时不该发 seek，
// 避免 mpv 在直播流上执行一次无意义的绝对定位。
func TestControllerFallbackZeroStartDoesNotSeek(t *testing.T) {
	proxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, proxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }
	plan, err := c.Prepare(context.Background(), player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS})
	if err != nil || plan.ID == "" {
		t.Fatalf("Prepare = %+v, %v", plan, err)
	}
	if _, err := c.Fallback(context.Background(), plan.ID, 0); err != nil {
		t.Fatalf("Fallback = %v", err)
	}
	if len(mpv.seeked) != 0 {
		t.Fatalf("从头播放不应 seek: seeked=%v", mpv.seeked)
	}
}

func TestControllerRequiresMPVForRTMP(t *testing.T) {
	c := NewController(nil, &fakeProxy{}, nil)
	_, err := c.Prepare(context.Background(), player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP})
	if err == nil || !errors.Is(err, ErrMPVUnavailable) {
		t.Fatalf("error = %v", err)
	}
}
