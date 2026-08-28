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
}

func (f *fakePlayer) Load(_ context.Context, s player.Stream) error {
	f.loaded = append(f.loaded, s)
	return nil
}
func (f *fakePlayer) Play() error                             { f.played++; return nil }
func (f *fakePlayer) Pause() error                            { return nil }
func (f *fakePlayer) Seek(float64) error                      { return nil }
func (f *fakePlayer) SetVolume(int) error                     { return nil }
func (f *fakePlayer) SelectTrack(player.TrackKind, int) error { return nil }
func (f *fakePlayer) State() player.State                     { return player.State{} }
func (f *fakePlayer) Events() <-chan player.Event             { return make(chan player.Event) }
func (f *fakePlayer) Close() error                            { return nil }

func TestControllerRoutesWebAndMPVStreams(t *testing.T) {
	webProxy := &fakeProxy{}
	mpv := &fakePlayer{}
	c := NewController(nil, webProxy, mpv)
	c.probe = func(context.Context, player.Stream) (string, error) { return "", nil }

	cases := []struct {
		name   string
		stream player.Stream
		want   Backend
	}{
		{"h264 hls", player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS}, BackendWeb},
		{"flv", player.Stream{URL: "https://x/live.flv", Kind: player.StreamFLV}, BackendWeb},
		{"rtmp", player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP}, BackendMPV},
		{"hevc", player.Stream{URL: "https://x/live.m3u8", Kind: player.StreamHLS}, BackendWeb},
	}
	for _, tc := range cases {
		got, err := c.Prepare(context.Background(), tc.stream)
		if tc.name == "hevc" {
			continue
		}
		if err != nil || got.Backend != tc.want {
			t.Errorf("%s: Prepare = %+v, %v", tc.name, got, err)
		}
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
	got, err := c.Fallback(context.Background(), plan.ID)
	if err != nil || got.Backend != BackendMPV || len(mpv.loaded) != 1 || mpv.loaded[0].URL != "https://x/live.m3u8" || mpv.played != 1 {
		t.Fatalf("Fallback = %+v, %v, loaded=%+v played=%d", got, err, mpv.loaded, mpv.played)
	}
	if _, err := c.Fallback(context.Background(), plan.ID); err == nil {
		t.Fatal("重复 fallback 应失败")
	}
}

func TestControllerRequiresMPVForRTMP(t *testing.T) {
	c := NewController(nil, &fakeProxy{}, nil)
	_, err := c.Prepare(context.Background(), player.Stream{URL: "rtmp://x/live", Kind: player.StreamRTMP})
	if err == nil || !errors.Is(err, ErrMPVUnavailable) {
		t.Fatalf("error = %v", err)
	}
}
