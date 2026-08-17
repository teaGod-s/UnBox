package config

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"os"
	"testing"
)

func TestDecodePlainJSON(t *testing.T) {
	in := []byte(`{"sites":[]}`)
	out, err := Decode(in)
	if err != nil {
		t.Fatalf("明文应当原样返回: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("内容丢失: %s", out)
	}
}

func TestDecodeStarStarBase64(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte(`{"sites":[]}`))
	in := []byte("jhSPAyzn**" + payload)
	out, err := Decode(in)
	if err != nil {
		t.Fatalf("**base64 解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}

func TestDecodeBareBase64(t *testing.T) {
	in := []byte(base64.StdEncoding.EncodeToString([]byte(`{"sites":[]}`)))
	out, err := Decode(in)
	if err != nil {
		t.Fatalf("裸 base64 解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}

func TestDecodeGzip(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte(`{"sites":[]}`))
	w.Close()
	out, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("gzip 解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}

// 真实样本回归测试
func TestDecodeRealSample(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/configs/line-1.raw")
	if err != nil {
		t.Skipf("样本缺失: %v", err)
	}
	out, err := Decode(raw)
	if err != nil {
		t.Fatalf("真实样本解码失败: %v", err)
	}
	if !bytes.Contains(out, []byte(`"sites"`)) {
		t.Errorf("解码后未见 sites 字段")
	}
}
