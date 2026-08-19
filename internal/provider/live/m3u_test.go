package live

import (
	"reflect"
	"testing"
)

func TestParseM3UExtractsAttributes(t *testing.T) {
	raw := []byte("#EXTM3U\n" +
		"#EXTINF:-1 tvg-id=\"cctv1\" tvg-logo=\"http://x/1.png\" group-title=\"央视\",CCTV-1 综合\n" +
		"http://x/live/cctv1.m3u8\n" +
		"#EXTINF:-1 group-title=\"卫视\",湖南卫视\n" +
		"http://x/live/hunan.ts\n")
	got := ParseM3U(raw)
	want := []Entry{
		{Name: "CCTV-1 综合", URL: "http://x/live/cctv1.m3u8", Logo: "http://x/1.png", Group: "央视", ID: "cctv1"},
		{Name: "湖南卫视", URL: "http://x/live/hunan.ts", Group: "卫视"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseM3U = %#v, want %#v", got, want)
	}
}

func TestParseM3UToleratesBOMCRLFAndJunk(t *testing.T) {
	raw := []byte("\xef\xbb\xbf#EXTM3U\r\n" +
		"#EXTINF:-1,频道A\r\n" +
		"http://x/a\r\n" +
		"\r\n" +
		"# 注释行\r\n" +
		"http://x/orphan-without-extinf\r\n") // 无 #EXTINF 前导的 URL 应被跳过
	got := ParseM3U(raw)
	if len(got) != 1 || got[0].Name != "频道A" {
		t.Fatalf("ParseM3U = %#v, want 仅 1 条频道A", got)
	}
}

func TestParseM3UMissingURLDropsEntry(t *testing.T) {
	raw := []byte("#EXTM3U\n#EXTINF:-1,只有名字没有 URL\n#EXTINF:-1,正常\nhttp://x/ok\n")
	got := ParseM3U(raw)
	if len(got) != 1 || got[0].URL != "http://x/ok" {
		t.Fatalf("ParseM3U = %#v, want 1 条", got)
	}
}

func TestParseTXT(t *testing.T) {
	raw := []byte("频道一,http://x/1\n频道二,http://x/2\n")
	got := ParseTXT(raw)
	if len(got) != 2 || got[1].Name != "频道二" || got[1].URL != "http://x/2" {
		t.Fatalf("ParseTXT = %#v", got)
	}
}
