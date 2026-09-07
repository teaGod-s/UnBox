package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisplayName(t *testing.T) {
	cases := map[string]string{
		// 元数据后缀截断（中文片名约定）
		"/mnt/电影/流浪地球.2019.BluRay.1080p.mp4":                         "流浪地球",
		"/mnt/f/迅雷下载/维多利亚的秘密2016时装秀.720p.HD中英双字[www.66ys.tv].mp4":    "维多利亚的秘密2016时装秀",
		"/mnt/f/迅雷下载/一条狗的使命.A.Dogs.Purpose.2017.双语字幕.720P-光速字幕组.mp4": "一条狗的使命",
		"/mnt/f/迅雷下载/源代码  英语版_高清.mp4":                                "源代码",
		"/mnt/f/迅雷下载/秒速五厘米.mp4":                                      "秒速五厘米",
		"/mnt/f/迅雷下载/红辣椒.mp4":                                        "红辣椒",
		"/mnt/f/迅雷下载/钢琴之森.mp4":                                       "钢琴之森",
		// 水印标签剥离 + 季集标记保留
		"/mnt/f/迅雷下载/火影忍者(第590集)[高清].mp4":                    "火影忍者 · 第590集",
		"/mnt/剧/庆余年.S01E01.mkv":                              "庆余年 · S01E01",
		"/mnt/剧/漫长的季节 第03集.mp4":                              "漫长的季节 · 第03集",
		"/mnt/剧/bojack.horseman.s01e01.webrip.XviD-pong.avi": "bojack.horseman · S01E01",
		"/mnt/剧/bojack.horseman.s01e06.webrip.XviD-pong.avi": "bojack.horseman · S01E06",
		"/mnt/剧/bojack.horseman.s01e12.webrip.XviD-pong.avi": "bojack.horseman · S01E12",
		"/mnt/剧/老友记 S01E01.mkv":                              "老友记 · S01E01",
		// 英文片名不做点号/空格截断
		"/mnt/f/迅雷下载/Avicii Tribute Concert：In Loving Memory of Tim Bergling.mkv":                                              "Avicii Tribute Concert：In Loving Memory of Tim Bergling",
		"/mnt/f/迅雷下载/UNIX - Making Computers Easier To Use -- AT&T Archives film from 1982, Bell Laboratories-XvDZLjaCJuw.mp4": "UNIX - Making Computers Easier To Use -- AT&T Archives film from 1982, Bell Laboratories-XvDZLjaCJuw",
		// 分辨率里的 e 不被误判成集号（1080p 不是 E1080）
		"/mnt/电影/Some Movie 1080p.BluRay.mkv": "Some Movie 1080p.BluRay",
		// 已知残留：无分隔符的质量词 / 下载器打坏的名字，不做猜测式清洗
		"/mnt/f/迅雷下载/[迅雷下载www.XunBo.Cc]肖申克的救赎BD1280高清中英双字.rmvb": "肖申克的救赎BD1280高清中英双字",
		"/mnt/f/迅雷下载/groud.38664d3a.mp4":                        "groud.38664d3a",
		// 清洗后无可用片名时回落到原 stem
		"/mnt/f/迅雷下载/[字幕组].mp4": "[字幕组]",
	}
	for path, want := range cases {
		if got := displayName(path); got != want {
			t.Errorf("displayName(%q)=%q, want %q", path, got, want)
		}
	}
}

func TestFindPoster(t *testing.T) {
	dir := t.TempDir()
	if got := findPoster(dir, "movie"); got != "" {
		t.Fatalf("空目录应无海报, got %q", got)
	}
	// 同名 poster.jpg
	poster := filepath.Join(dir, "movie-poster.jpg")
	if err := os.WriteFile(poster, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findPoster(dir, "movie"); got != poster {
		t.Fatalf("findPoster=%q, want %q", got, poster)
	}
}

func TestFindPosterReturnsAbsolutePathForRelativeDir(t *testing.T) {
	dir := t.TempDir()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeDir, err := filepath.Rel(workingDir, dir)
	if err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(dir, "movie-poster.jpg")
	if err := os.WriteFile(poster, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findPoster(relativeDir, "movie"); got != poster {
		t.Fatalf("findPoster(%q)=%q, want absolute path %q", relativeDir, got, poster)
	}
}
