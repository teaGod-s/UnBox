package config

import (
	"encoding/json"
	"fmt"
)

// Parse 将原始订阅内容解析为 Config。
//
// 管线：Decode（剥离加密/压缩）→ Lenient（转为标准 JSON）→ Unmarshal。
func Parse(raw []byte) (*Config, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("配置内容为空")
	}

	decoded, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("解码: %w", err)
	}

	cleaned := Lenient(decoded)

	var cfg Config
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return nil, fmt.Errorf("JSON 解析: %w", err)
	}
	return &cfg, nil
}
