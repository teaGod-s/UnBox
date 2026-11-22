// Package httpx 提供带显式传输层超时与瞬时错误重试的 HTTP 客户端。
//
// 真实源站里 TLS 握手超时、连接被重置、HTTP/2 PROTOCOL_ERROR 这类错误很常见，
// 它们多为瞬时故障，重试一次通常就能成功；而证书错误是确定性的，不能重试。
package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxAttempts 是瞬时错误下的总尝试次数（首次 + 一次重试）。
const maxAttempts = 2

// ErrBodyTooLarge 表示响应体超过调用方给定的上限。
var ErrBodyTooLarge = errors.New("响应体超过上限")

var (
	insecureMu    sync.RWMutex
	insecureHosts = map[string]bool{}
)

// SetInsecureHosts 设置允许跳过证书校验的主机（用户在设置里显式开启）。
//
// 只影响证书校验失败后的降级重试，不会默认降低任何连接的 TLS 强度。
func SetInsecureHosts(hosts []string) {
	next := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		if h := normalizeHost(host); h != "" {
			next[h] = true
		}
	}
	insecureMu.Lock()
	insecureHosts = next
	insecureMu.Unlock()
}

// InsecureHost 判断某个主机是否允许跳过证书校验。
func InsecureHost(host string) bool {
	h := normalizeHost(host)
	if h == "" {
		return false
	}
	insecureMu.RLock()
	defer insecureMu.RUnlock()
	return insecureHosts[h]
}

// normalizeHost 统一成小写、去掉端口，便于按主机名匹配。
func normalizeHost(host string) string {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" {
		return ""
	}
	if before, after, ok := strings.CutLast(h, ":"); ok && before != "" && !strings.Contains(after, "]") {
		h = before
	}
	return strings.Trim(h, "[]")
}

// NewClient 返回带显式传输层配置的客户端。
//
// 直接写 &http.Client{Timeout: d} 会用 http.DefaultTransport，它的
// TLSHandshakeTimeout 固定为 10 秒，于是「客户端超时 30 秒」实际在 TLS
// 握手阶段 10 秒就失败了。这里让握手超时跟随客户端超时，避免跨境线路
// 上的慢握手被提前掐断。
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   timeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// Do 执行请求；遇到瞬时网络错误时重试一次。
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := range maxAttempts {
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, lastErr
		}
		resp, err := client.Do(attemptReq)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldRetry(req, err) {
			break
		}
	}
	if isCertificateError(lastErr) && InsecureHost(req.URL.Hostname()) {
		if attemptReq, cloneErr := requestForAttempt(req, 1); cloneErr == nil {
			resp, err := insecureClient(client).Do(attemptReq)
			if err != nil {
				return nil, insecureRetryError(err)
			}
			return resp, nil
		}
	}
	return nil, lastErr
}

// DoReadAll 执行请求并读取整个响应体，遇到瞬时错误（含读取阶段的
// HTTP/2 PROTOCOL_ERROR、EOF）时整体重试一次。limit > 0 时超过上限返回
// ErrBodyTooLarge（不重试）。
func DoReadAll(client *http.Client, req *http.Request, limit int64) (*http.Response, []byte, error) {
	var lastErr error
	for attempt := range maxAttempts {
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, nil, lastErr
		}
		resp, err := client.Do(attemptReq)
		if err != nil {
			lastErr = err
			if !shouldRetry(req, err) {
				resp, body, fallbackErr := insecureFallbackReadAll(client, req, err, limit)
				return resp, body, fallbackErr
			}
			continue
		}
		body, readErr := readAllLimit(resp.Body, limit)
		if readErr != nil {
			_ = resp.Body.Close()
			lastErr = readErr
			if !shouldRetry(req, readErr) {
				return resp, nil, readErr
			}
			continue
		}
		return resp, body, nil
	}
	return insecureFallbackReadAll(client, req, lastErr, limit)
}

// insecureFallbackReadAll 是 DoReadAll 的证书降级路径。
func insecureFallbackReadAll(client *http.Client, req *http.Request, err error, limit int64) (*http.Response, []byte, error) {
	if !isCertificateError(err) || !InsecureHost(req.URL.Hostname()) {
		return nil, nil, err
	}
	attemptReq, cloneErr := requestForAttempt(req, 1)
	if cloneErr != nil {
		return nil, nil, err
	}
	resp, fallbackErr := insecureClient(client).Do(attemptReq)
	if fallbackErr != nil {
		return nil, nil, insecureRetryError(fallbackErr)
	}
	body, readErr := readAllLimit(resp.Body, limit)
	if readErr != nil {
		_ = resp.Body.Close()
		return resp, nil, readErr
	}
	return resp, body, nil
}

// insecureRetryError 标注「已按主机豁免跳过证书校验，但仍失败」。
//
// 部分站点证书坏掉的同时 TLS/HTTP 实现也坏了（bad record mac、PROTOCOL_ERROR），
// 跳过校验也连不上；不说明这一点的话，用户会以为豁免没生效。
func insecureRetryError(err error) error {
	return fmt.Errorf("已按主机豁免跳过证书校验，但仍连接失败: %w", err)
}

func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(r)
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

// requestForAttempt 返回本次尝试要用的请求；重试时重建请求体。
func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 {
		return req, nil
	}
	if req.GetBody == nil {
		if req.Body != nil {
			return nil, errors.New("请求体无法重建")
		}
		return req.Clone(req.Context()), nil
	}
	clone := req.Clone(req.Context())
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}

// shouldRetry 判断错误是否值得重试：瞬时错误、调用方未放弃、请求体可重建。
func shouldRetry(req *http.Request, err error) bool {
	if !IsTransient(err) {
		return false
	}
	if req.Context().Err() != nil {
		return false
	}
	return req.Body == nil || req.GetBody != nil
}

// IsTransient 判断错误是否值得重试。
//
// 证书类错误（x509）是确定性的，不重试；DNS 解析失败通常也重试不出结果，
// 但超时可能只是抖动，因此统一按 net.Error.Timeout() 归入可重试。
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"eof",
		"tls: bad record mac",
		"protocol_error",
		"connection reset by peer",
		"broken pipe",
		"server closed idle connection",
		"stream error",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// isCertificateError 判断是否为证书校验失败（过期、主机名不符、未知 CA 等）。
func isCertificateError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return true
	}
	_, ok := errors.AsType[x509.CertificateInvalidError](err)
	return ok
}

// insecureClient 复制客户端并把证书校验关掉，仅用于用户显式许可的主机。
func insecureClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		clone.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // 用户显式按主机开启
		return &clone
	}
	tlsTransport := transport.Clone()
	tlsConfig := &tls.Config{}
	if tlsTransport.TLSClientConfig != nil {
		tlsConfig = tlsTransport.TLSClientConfig.Clone()
	}
	// 这是用户在主机的显式豁免，只影响该主机的失败重试。
	tlsConfig.InsecureSkipVerify = true //nolint:gosec
	tlsTransport.TLSClientConfig = tlsConfig
	clone.Transport = tlsTransport
	return &clone
}
