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

// 测试深层嵌套 gzip 超过限制会返回错误而不是栈溢出
func TestDecodeNestedGzipDepthLimit(t *testing.T) {
	payload := []byte(`{"sites":[]}`)
	// 嵌套 10 层 gzip（超过 maxDecodeDepth=8 的限制）
	for i := 0; i < 10; i++ {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		w.Write(payload)
		w.Close()
		payload = buf.Bytes()
	}
	out, err := Decode(payload)
	if err == nil {
		t.Errorf("深层嵌套应当返回错误，但成功解码: %s", out)
	}
	// 验证错误信息中包含深度限制
	if !bytes.Contains([]byte(err.Error()), []byte("深度")) {
		t.Errorf("错误信息应当提及深度限制: %v", err)
	}
}

// 测试 gzip 炸弹（压缩炸弹）会返回错误而不是耗尽内存
func TestDecodeGzipBomb(t *testing.T) {
	// 创建压缩炸弹：少量压缩数据解压到超过 maxDecodedSize 的大小
	// gzip 对重复字节压缩效率极高，所以用零字节循环写入即可
	// 避免一次性分配 100 MB，改为分块写入（32 KB 块 × 3200 次 = 100 MB）
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	// 32 KB 的零字节块
	chunk := bytes.Repeat([]byte{0}, 32*1024)
	// 写入足以解压超过 64 MB 的数据（3200 × 32 KB = 100 MB）
	// gzip 压缩效率约 1000:1，所以 100 MB 会压缩到 ~100 KB
	for i := 0; i < 3200; i++ {
		w.Write(chunk)
	}
	w.Close()

	out, err := Decode(buf.Bytes())
	if err == nil {
		t.Errorf("gzip 炸弹应当返回错误，但成功解码 %d 字节", len(out))
	}
	// 验证错误信息中包含大小限制
	if !bytes.Contains([]byte(err.Error()), []byte("限制")) {
		t.Errorf("错误信息应当提及大小限制: %v", err)
	}
}

// 测试合法的嵌套编码（gzip 包装 base64 包装 JSON）仍能正常工作
func TestDecodeLegitimateNesting(t *testing.T) {
	payload := []byte(`{"sites":[]}`)
	// 1. base64 编码
	b64 := base64.StdEncoding.EncodeToString(payload)
	// 2. gzip 压缩 base64 字符串
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte(b64))
	w.Close()
	// 3. 解码应该能处理：gzip -> base64 -> JSON
	out, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("合法嵌套编码应当解码成功: %v", err)
	}
	if !bytes.Contains(out, []byte("sites")) {
		t.Errorf("解码结果不含预期内容: %s", out)
	}
}
