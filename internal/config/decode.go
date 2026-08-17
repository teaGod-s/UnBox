package config

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
)

// Decode 探测并剥离配置的加密/压缩层，返回明文字节。
//
// TVBox 生态的配置不携带任何加密方式标识，因此只能按特征逐一探测：
//   1. gzip 魔数
//   2. "<随机前缀>**<base64>" 形式（实测见于双龙仓库）
//   3. 裸 base64
//   4. 明文
func Decode(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	if len(raw) == 0 {
		return nil, fmt.Errorf("配置内容为空")
	}

	// 1. gzip
	if len(raw) > 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("gzip 解压失败: %w", err)
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("gzip 读取失败: %w", err)
		}
		return Decode(out) // 解压后可能仍是 base64
	}

	// 2. 明文（已是 JSON）
	if raw[0] == '{' || raw[0] == '[' {
		return raw, nil
	}

	// 3. "<前缀>**<base64>"
	if i := bytes.Index(raw, []byte("**")); i >= 0 && i < 64 {
		if out, ok := tryBase64(raw[i+2:]); ok {
			return Decode(out)
		}
	}

	// 4. 裸 base64
	if out, ok := tryBase64(raw); ok {
		return Decode(out)
	}

	return nil, fmt.Errorf("无法识别的配置编码，前 32 字节: %q", raw[:min(32, len(raw))])
}

func tryBase64(b []byte) ([]byte, bool) {
	s := string(bytes.TrimSpace(b))
	if pad := len(s) % 4; pad != 0 {
		s += string(bytes.Repeat([]byte("="), 4-pad))
	}
	out, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(out) == 0 {
		return nil, false
	}
	return out, true
}
