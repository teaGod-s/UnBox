package tvbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tvboxUA 是 FongMi多线路 客户端惯例 UA，部分 CMS 按它鉴权。
const tvboxUA = "okhttp/3.12.11"

// cmsVideo 是 CMS 接口返回的影片条目（列表与详情共用）。
type cmsVideo struct {
	VodID       int64  `json:"vod_id"`
	VodName     string `json:"vod_name"`
	VodPic      string `json:"vod_pic"`
	TypeID      int64  `json:"type_id"`
	TypeName    string `json:"type_name"`
	VodRemarks  string `json:"vod_remarks"`
	VodYear     string `json:"vod_year"`
	VodArea     string `json:"vod_area"`
	VodContent  string `json:"vod_content"`
	VodPlayFrom string `json:"vod_play_from"`
	VodPlayURL  string `json:"vod_play_url"`
}

type cmsResp struct {
	Code int        `json:"code"`
	List []cmsVideo `json:"list"`
}

// client 是单个 CMS 站点的 JSON API 客户端。
type client struct {
	base string
	hc   *http.Client
}

// newClient 用站点 api 基地址构造客户端；剥离 api 内可能内嵌的 query。
func newClient(api string) *client {
	base, _, _ := strings.Cut(api, "?")
	return &client{base: base, hc: &http.Client{Timeout: 15 * time.Second}}
}

// videolist 请求列表/分类/搜索（t、wd 为可空过滤条件）。
func (c *client) videolist(ctx context.Context, t, wd string, page int) ([]cmsVideo, error) {
	q := url.Values{"ac": {"videolist"}, "pg": {fmt.Sprintf("%d", page)}}
	if t != "" {
		q.Set("t", t)
	}
	if wd != "" {
		q.Set("wd", wd)
	}
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp cmsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析列表响应失败: %w", err)
	}
	if resp.Code != 1 {
		return nil, fmt.Errorf("站点返回错误码 %d", resp.Code)
	}
	return resp.List, nil
}

// detail 请求单部影片详情。
func (c *client) detail(ctx context.Context, ids string) (*cmsVideo, error) {
	q := url.Values{"ac": {"detail"}, "ids": {ids}}
	raw, err := c.get(ctx, q)
	if err != nil {
		return nil, err
	}
	var resp cmsResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("解析详情响应失败: %w", err)
	}
	if resp.Code != 1 || len(resp.List) == 0 {
		return nil, fmt.Errorf("站点返回错误码 %d 或无数据", resp.Code)
	}
	return &resp.List[0], nil
}

// get 发起带 UA 的 GET，返回响应体。
func (c *client) get(ctx context.Context, q url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", tvboxUA)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("站点返回 HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
