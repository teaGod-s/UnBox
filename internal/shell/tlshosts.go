package shell

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/unbox/unbox/internal/httpx"
)

// insecureTLSKey 是 store.kv 里「允许忽略证书校验的主机」列表的键。
const insecureTLSKey = "insecure_tls_hosts"

// ListInsecureTLSHosts 返回允许忽略证书校验的主机列表（已排序）。
func (s *ShellService) ListInsecureTLSHosts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.insecureTLSHosts...)
}

// SetInsecureTLSHost 打开或关闭某个主机的证书校验豁免，返回更新后的列表。
//
// 只影响证书校验失败后的降级重试：默认仍然严格校验证书，用户显式加入的
// 主机才会在握手失败时跳过校验重试一次。
func (s *ShellService) SetInsecureTLSHost(host string, insecure bool) ([]string, error) {
	normalized := normalizeTLSHost(host)
	if normalized == "" {
		return nil, fmt.Errorf("主机名为空")
	}

	s.mu.Lock()
	next := make([]string, 0, len(s.insecureTLSHosts)+1)
	found := false
	for _, existing := range s.insecureTLSHosts {
		if existing == normalized {
			found = true
			if !insecure {
				continue
			}
		}
		next = append(next, existing)
	}
	if insecure && !found {
		next = append(next, normalized)
	}
	sort.Strings(next)
	s.insecureTLSHosts = next
	s.mu.Unlock()

	if s.store != nil {
		raw, err := json.Marshal(next)
		if err != nil {
			return next, err
		}
		if err := s.store.SetKV(insecureTLSKey, string(raw)); err != nil {
			return next, err
		}
	}
	httpx.SetInsecureHosts(next)
	return append([]string(nil), next...), nil
}

// loadInsecureTLSHosts 从 store 恢复证书豁免列表；无 store 或解析失败时保持为空。
func (s *ShellService) loadInsecureTLSHosts() {
	if s.store == nil {
		return
	}
	raw, ok, err := s.store.GetKV(insecureTLSKey)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return
	}
	var hosts []string
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		return
	}
	normalized := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if h := normalizeTLSHost(host); h != "" {
			normalized = append(normalized, h)
		}
	}
	sort.Strings(normalized)
	s.mu.Lock()
	s.insecureTLSHosts = normalized
	s.mu.Unlock()
	httpx.SetInsecureHosts(normalized)
}

// normalizeTLSHost 把用户输入统一成主机名：去掉协议、路径、端口与首尾空白。
func normalizeTLSHost(raw string) string {
	host := strings.TrimSpace(strings.ToLower(raw))
	if host == "" {
		return ""
	}
	if _, after, ok := strings.Cut(host, "://"); ok {
		host = after
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if strings.HasPrefix(host, "[") {
		if before, _, ok := strings.Cut(host, "]"); ok {
			host = before + "]"
		}
	} else if before, _, ok := strings.CutLast(host, ":"); ok && before != "" {
		host = before
	}
	return strings.Trim(host, "[]")
}
