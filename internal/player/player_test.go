package player

import "testing"

func TestStreamKindString(t *testing.T) {
	cases := map[StreamKind]string{
		StreamHLS:   "hls",
		StreamMP4:   "mp4",
		StreamFLV:   "flv",
		StreamTS:    "ts",
		StreamRTMP:  "rtmp",
		StreamLocal: "local",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("StreamKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestTrackKindString(t *testing.T) {
	cases := map[TrackKind]string{
		TrackAudio:    "audio",
		TrackSubtitle: "subtitle",
		TrackVideo:    "video",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("TrackKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestStateZeroValueIsStopped(t *testing.T) {
	var s State
	if s.Playing != StateStopped {
		t.Errorf("零值 State.Playing = %v, want %v", s.Playing, StateStopped)
	}
}
