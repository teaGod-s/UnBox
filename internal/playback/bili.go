package playback

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/unbox/unbox/internal/httpx"
	"github.com/unbox/unbox/internal/player"
)

// biliPlayURLBase 是 B 站取流接口；包级变量便于测试替换。
var biliPlayURLBase = "https://api.bilibili.com/x/player/playurl"

// bilibili 取流接口按 Referer + UA 校验，缺任一都会返回 -412 风控。
const (
	biliReferer = "https://www.bilibili.com"
	biliUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
)

// parseBiliComposite 解析 TVBox 的 bilibili 复合播放地址：
//
//	<aid>+<cid>+<画质1:画质2:…>+<描述1:描述2:…>
//
// 形如 "117461315749788+42845144529+32:16+5riF…=:5rWB…="。
// bili_open 等脚本的 playerContent 直接吐这种串，客户端要拿 aid/cid 换真实流。
// 不是这种形态（aid/cid 非纯数字、画质列表为空）返回 ok=false。
func parseBiliComposite(raw string) (aid, cid string, qualities []string, ok bool) {
	parts := strings.Split(strings.TrimSpace(raw), "+")
	if len(parts) != 4 {
		return "", "", nil, false
	}
	aid, cid = parts[0], parts[1]
	if !isDigits(aid) || !isDigits(cid) {
		return "", "", nil, false
	}
	for q := range strings.SplitSeq(parts[2], ":") {
		if q != "" {
			qualities = append(qualities, q)
		}
	}
	if len(qualities) == 0 {
		return "", "", nil, false
	}
	return aid, cid, qualities, true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// resolveBili 用 aid/cid/首选画质调 B 站取流接口，换出真实可播 URL。
// 默认取画质列表的第一档（通常是最高），与 TVBox 行为一致。
func (r *Resolver) resolveBili(ctx context.Context, aid, cid string, qualities []string) (player.Stream, error) {
	q := url.Values{"avid": {aid}, "cid": {cid}, "qn": {qualities[0]}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, biliPlayURLBase+"?"+q.Encode(), nil)
	if err != nil {
		return player.Stream{}, fmt.Errorf("创建 bilibili 取流请求失败: %w", err)
	}
	req.Header.Set("User-Agent", biliUA)
	req.Header.Set("Referer", biliReferer)

	resp, err := httpx.Do(r.client, req)
	if err != nil {
		return player.Stream{}, fmt.Errorf("bilibili 取流请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return player.Stream{}, fmt.Errorf("bilibili 取流失败: HTTP %d", resp.StatusCode)
	}

	var out struct {
		Code int `json:"code"`
		Data struct {
			Durl []struct {
				URL string `json:"url"`
			} `json:"durl"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return player.Stream{}, fmt.Errorf("bilibili 取流响应解析失败: %w", err)
	}
	if out.Code != 0 || len(out.Data.Durl) == 0 || out.Data.Durl[0].URL == "" {
		return player.Stream{}, fmt.Errorf("bilibili 返回错误 code=%d", out.Code)
	}

	realURL := out.Data.Durl[0].URL
	return player.Stream{
		URL:  realURL,
		Kind: player.KindForURL(realURL),
		// bilivideo CDN 无 Referer 直接 403：返回带头流，Web 路径经本地代理
		// （proxy.go applyHeaders）转发给上游。
		Headers: map[string]string{"Referer": biliReferer, "User-Agent": biliUA},
	}, nil
}
