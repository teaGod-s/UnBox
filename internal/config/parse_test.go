package config

import (
	"os"
	"path/filepath"
	"testing"
)

type sampleCase struct {
	file string
	kind string // "index" 或 "config"
}

func TestParseAllRealSamples(t *testing.T) {
	cases := []sampleCase{
		{"01-storehouse.json", "index"},
		{"02-urls-aggregate.json", "index"},
		{"line-1.raw", "config"},
		{"line-2.raw", "config"},
		{"line-3.raw", "config"},
		{"line-4.raw", "config"},
		{"line-6.raw", "config"},
		{"line-9.raw", "config"},
		{"line-12.raw", "config"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../testdata/configs", c.file))
			if err != nil {
				t.Skipf("样本缺失: %v", err)
			}
			cfg, err := Parse(raw)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			switch c.kind {
			case "index":
				if len(cfg.StoreHouse) == 0 && len(cfg.URLs) == 0 {
					t.Error("索引类配置应含 storeHouse 或 urls")
				}
			case "config":
				if len(cfg.Sites) == 0 && len(cfg.Lives) == 0 {
					t.Error("终端配置应含 sites 或 lives")
				}
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
