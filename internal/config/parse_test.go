package config

import (
	"os"
	"path/filepath"
	"testing"
)

type sampleCase struct {
	file           string
	kind           string // "index" 或 "config"
	wantSites      int
	wantLives      int
	wantStoreHouse int
	wantURLs       int
}

func TestParseAllRealSamples(t *testing.T) {
	cases := []sampleCase{
		{file: "01-storehouse.json", kind: "index", wantSites: 0, wantLives: 0, wantStoreHouse: 4, wantURLs: 0},
		{file: "02-urls-aggregate.json", kind: "index", wantSites: 0, wantLives: 0, wantStoreHouse: 0, wantURLs: 24},
		{file: "line-1.raw", kind: "config", wantSites: 55, wantLives: 7, wantStoreHouse: 0, wantURLs: 0},
		{file: "line-2.raw", kind: "config", wantSites: 28, wantLives: 2, wantStoreHouse: 0, wantURLs: 0},
		{file: "line-3.raw", kind: "config", wantSites: 96, wantLives: 10, wantStoreHouse: 0, wantURLs: 0},
		{file: "line-4.raw", kind: "config", wantSites: 44, wantLives: 2, wantStoreHouse: 0, wantURLs: 0},
		{file: "line-6.raw", kind: "config", wantSites: 98, wantLives: 12, wantStoreHouse: 0, wantURLs: 0},
		{file: "line-9.raw", kind: "config", wantSites: 71, wantLives: 1, wantStoreHouse: 0, wantURLs: 0},
		{file: "line-12.raw", kind: "config", wantSites: 173, wantLives: 9, wantStoreHouse: 0, wantURLs: 0},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../testdata/configs", c.file))
			if err != nil {
				t.Fatalf("样本缺失: %v", err)
			}
			cfg, err := Parse(raw)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if got := len(cfg.Sites); got != c.wantSites {
				t.Errorf("%s: sites 数量 = %d, 期望 %d", c.file, got, c.wantSites)
			}
			if got := len(cfg.Lives); got != c.wantLives {
				t.Errorf("%s: lives 数量 = %d, 期望 %d", c.file, got, c.wantLives)
			}
			if got := len(cfg.StoreHouse); got != c.wantStoreHouse {
				t.Errorf("%s: storeHouse 数量 = %d, 期望 %d", c.file, got, c.wantStoreHouse)
			}
			if got := len(cfg.URLs); got != c.wantURLs {
				t.Errorf("%s: urls 数量 = %d, 期望 %d", c.file, got, c.wantURLs)
			}
		})
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("这不是配置")); err == nil {
		t.Error("畸形输入应当返回 error 而非 nil")
	}
}

func TestParseNeverPanics(t *testing.T) {
	inputs := [][]byte{nil, {}, []byte("{"), []byte("**"), []byte("\x00\x01\x02")}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("输入 %q 触发 panic: %v", in, r)
				}
			}()
			_, _ = Parse(in)
		}()
	}
}
