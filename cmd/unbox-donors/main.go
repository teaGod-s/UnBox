// Command unbox-donors 从爱发电 OpenAPI 导出离线捐助榜单。
package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	afdianQuerySponsorURL = "https://afdian.com/api/open/query-sponsor"
	sponsorsPerPage       = 100
	maxSponsorPages       = 10_000
	anonymousDonorName    = "热心网友"

	// afdianBrowserUA 让请求带上浏览器指纹：爱发电的风控会把 Go 默认的
	// Go-http-client/1.1 当脚本直接 403（CI 的出口 IP 更容易被拦）。
	afdianBrowserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Donor 是写入 docs/donors.json 的捐助人数据。
type Donor struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Avatar    string  `json:"avatar"`
	Amount    float64 `json:"amount"`
	Anonymous bool    `json:"anonymous"`
}

// Leaderboard 是应用侧直接消费的榜单结构。
type Leaderboard struct {
	UpdatedAt string  `json:"updatedAt"`
	Donors    []Donor `json:"donors"`
}

type rawUser struct {
	UserID      string `json:"user_id"`
	UserIDStr   string `json:"user_id_str"`
	Name        string `json:"name"`
	UserName    string `json:"user_name"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	AvatarURL   string `json:"avatar_url"`
	UserAvatar  string `json:"user_avatar"`
	IsAnonymous bool   `json:"is_anonymous"`

	present bool
	keys    []string
}

type rawSponsor struct {
	Sponsor      rawUser `json:"sponsor"`
	User         rawUser `json:"user"`
	AllSumAmount string  `json:"all_sum_amount"`
	Anonymous    bool    `json:"anonymous"`
	IsAnonymous  bool    `json:"is_anonymous"`

	keys []string
}

// UnmarshalJSON 记录嵌套对象实际出现的键名，用于字段缺失时诊断。
func (u *rawUser) UnmarshalJSON(data []byte) error {
	type userAlias rawUser
	var user userAlias
	if err := json.Unmarshal(data, &user); err != nil {
		return err
	}

	keys, err := objectKeyNames(data)
	if err != nil {
		return err
	}
	*u = rawUser(user)
	u.present = !bytes.Equal(bytes.TrimSpace(data), []byte("null"))
	u.keys = keys
	return nil
}

// UnmarshalJSON 兼容爱发电返回的 sponsor / user 嵌套形态，以及
// anonymous / is_anonymous 字段。缺失匿名字段时按公开条目处理。
func (s *rawSponsor) UnmarshalJSON(data []byte) error {
	type sponsorAlias rawSponsor
	var sponsor sponsorAlias
	if err := json.Unmarshal(data, &sponsor); err != nil {
		return err
	}

	keys, err := objectKeyNames(data)
	if err != nil {
		return err
	}
	*s = rawSponsor(sponsor)
	s.keys = keys
	s.Anonymous = s.Anonymous || s.IsAnonymous || s.Sponsor.IsAnonymous || s.User.IsAnonymous
	return nil
}

func objectKeyNames(data []byte) ([]string, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, errors.New("JSON 对象格式无效")
	}

	keys := make([]string, 0)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("JSON 对象键名格式无效")
		}
		keys = append(keys, key)

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (u rawUser) id() string {
	for _, value := range []string{u.UserID, u.UserIDStr} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (u rawUser) name() string {
	for _, value := range []string{u.Name, u.UserName, u.Nickname} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (u rawUser) avatar() string {
	for _, value := range []string{u.Avatar, u.AvatarURL, u.UserAvatar} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (u rawUser) hasIdentity() bool {
	return u.id() != "" || u.name() != "" || u.avatar() != ""
}

func (s rawSponsor) identity() rawUser {
	if s.Sponsor.hasIdentity() {
		return s.Sponsor
	}
	if s.User.hasIdentity() {
		return s.User
	}
	if s.Sponsor.present {
		return s.Sponsor
	}
	return s.User
}

func (s rawSponsor) actualKeys() string {
	parts := make([]string, 0, 2)
	if s.Sponsor.present {
		parts = append(parts, formatObjectKeys("sponsor", s.Sponsor.keys))
	}
	if s.User.present {
		parts = append(parts, formatObjectKeys("user", s.User.keys))
	}
	if len(parts) == 0 {
		return formatObjectKeys("", s.keys)
	}
	return strings.Join(parts, ",")
}

func formatObjectKeys(name string, keys []string) string {
	return name + "{" + strings.Join(keys, ",") + "}"
}

type querySponsorParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

type querySponsorRequest struct {
	UserID string `json:"user_id"`
	Params string `json:"params"`
	TS     string `json:"ts"`
	Sign   string `json:"sign"`
}

type querySponsorData struct {
	List       []rawSponsor `json:"list"`
	TotalPage  int          `json:"total_page"`
	TotalCount int          `json:"total_count"`
}

type querySponsorResponse struct {
	EC   int              `json:"ec"`
	EM   string           `json:"em"`
	Data querySponsorData `json:"data"`
}

func main() {
	if err := run(context.Background(), os.Stdout, os.Stderr, http.DefaultClient, time.Now); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, stdout, stderr io.Writer, client *http.Client, now func() time.Time) error {
	userID := strings.TrimSpace(os.Getenv("AFDIAN_USER_ID"))
	token := strings.TrimSpace(os.Getenv("AFDIAN_TOKEN"))
	if userID == "" {
		return reportError(stderr, errors.New("缺少环境变量 AFDIAN_USER_ID"))
	}
	if token == "" {
		return reportError(stderr, errors.New("缺少环境变量 AFDIAN_TOKEN"))
	}

	sponsors, err := fetchAllSponsors(ctx, client, afdianQuerySponsorURL, token, userID)
	if err != nil {
		return reportError(stderr, fmt.Errorf("拉取爱发电捐助记录: %w", err))
	}

	leaderboard := Leaderboard{
		UpdatedAt: now().Format(time.RFC3339),
		Donors:    normalizeDonorsWithDiagnostics(sponsors, stderr),
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(leaderboard); err != nil {
		return reportError(stderr, fmt.Errorf("输出捐助榜单: %w", err))
	}
	return nil
}

func reportError(stderr io.Writer, err error) error {
	fmt.Fprintf(stderr, "导出捐助榜单失败: %v\n", err)
	return err
}

func fetchAllSponsors(ctx context.Context, client *http.Client, endpoint, token, userID string) ([]rawSponsor, error) {
	all := make([]rawSponsor, 0)
	for page := 1; page <= maxSponsorPages; page++ {
		sponsors, totalPage, totalCount, err := fetchSponsorPage(ctx, client, endpoint, token, userID, page)
		if err != nil {
			return nil, fmt.Errorf("第 %d 页: %w", page, err)
		}
		all = append(all, sponsors...)

		if len(sponsors) == 0 {
			break
		}
		if totalPage > 0 && page >= totalPage {
			break
		}
		if totalCount > 0 && len(all) >= totalCount {
			break
		}
	}
	if len(all) == 0 && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return all, nil
}

func fetchSponsorPage(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	token string,
	userID string,
	page int,
) ([]rawSponsor, int, int, error) {
	paramsBytes, err := json.Marshal(querySponsorParams{Page: page, PerPage: sponsorsPerPage})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("序列化请求参数: %w", err)
	}
	params := string(paramsBytes)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sign := signParams(token, params, ts, userID)
	bodyBytes, err := json.Marshal(querySponsorRequest{
		UserID: userID,
		Params: params,
		TS:     ts,
		Sign:   sign,
	})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("序列化请求: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("创建请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", afdianBrowserUA)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://afdian.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("读取响应: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, 0, 0, formatSponsorHTTPError(resp.StatusCode, body, token, userID, ts, sign)
	}

	var result querySponsorResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, 0, fmt.Errorf("解析响应 JSON: %w", err)
	}
	if result.EC != http.StatusOK {
		em := sanitizeSponsorHTTPErrorField(result.EM, token, userID, ts, sign)
		if em == "" {
			return nil, 0, 0, fmt.Errorf("接口错误 %d", result.EC)
		}
		return nil, 0, 0, fmt.Errorf("接口错误 %d: %s", result.EC, em)
	}
	return result.Data.List, result.Data.TotalPage, result.Data.TotalCount, nil
}

func normalizeDonors(sponsors []rawSponsor) []Donor {
	return normalizeDonorsWithDiagnostics(sponsors, io.Discard)
}

func normalizeDonorsWithDiagnostics(sponsors []rawSponsor, stderr io.Writer) []Donor {
	donors := make([]Donor, 0, len(sponsors))
	for index, sponsor := range sponsors {
		amount, err := strconv.ParseFloat(strings.TrimSpace(sponsor.AllSumAmount), 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
			amount = 0
		}

		user := sponsor.identity()
		name := user.name()
		if name == "" && stderr != nil {
			fmt.Fprintf(stderr, "警告: 第%d条赞助者缺少姓名字段，实际键: %s\n", index+1, sponsor.actualKeys())
		}

		donor := Donor{
			ID:        user.id(),
			Name:      name,
			Avatar:    user.avatar(),
			Amount:    amount,
			Anonymous: sponsor.Anonymous,
		}
		if donor.Anonymous {
			donor.ID = ""
			donor.Name = anonymousDonorName
			donor.Avatar = ""
		}
		donors = append(donors, donor)
	}

	slices.SortStableFunc(donors, func(a, b Donor) int {
		if a.Amount == b.Amount {
			return cmp.Compare(a.Name, b.Name)
		}
		return cmp.Compare(b.Amount, a.Amount)
	})
	return donors
}

func formatSponsorHTTPError(statusCode int, body []byte, sensitiveValues ...string) error {
	message := fmt.Sprintf("HTTP %d", statusCode)
	// 403 大概率是风控拦了 CI 出口 IP / 非浏览器指纹，提示一条可操作的方向，
	// 避免开发者在日志里只看到一个裸 403。
	if statusCode == http.StatusForbidden {
		message += "（爱发电风控可能拦截了本出口或请求指纹；可在本机运行 go run ./cmd/unbox-donors 刷新）"
		return errors.New(message)
	}

	var response struct {
		EC *int    `json:"ec"`
		EM *string `json:"em"`
	}
	if err := json.Unmarshal(body, &response); err == nil {
		details := make([]string, 0, 2)
		if response.EC != nil {
			details = append(details, fmt.Sprintf("ec=%d", *response.EC))
		}
		if response.EM != nil {
			if em := sanitizeSponsorHTTPErrorField(*response.EM, sensitiveValues...); em != "" {
				details = append(details, "em="+em)
			}
		}
		if len(details) > 0 {
			message += " (" + strings.Join(details, ", ") + ")"
		}
	}
	return errors.New(message)
}

func sanitizeSponsorHTTPErrorField(value string, sensitiveValues ...string) string {
	clean := strings.Join(strings.Fields(value), " ")
	if clean == "" {
		return ""
	}

	lower := strings.ToLower(clean)
	if strings.Contains(lower, "token") || strings.Contains(lower, "user_id") ||
		strings.Contains(lower, "sign") || strings.Contains(lower, "ts") {
		return "响应内容包含敏感字段"
	}
	for _, sensitive := range sensitiveValues {
		if sensitive != "" && strings.Contains(clean, sensitive) {
			return "响应内容包含敏感字段"
		}
	}

	const maxRunes = 200
	runes := []rune(clean)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return clean
}

func signParams(token, params, ts, userID string) string {
	sum := md5.Sum([]byte(token + "params" + params + "ts" + ts + "user_id" + userID))
	return hex.EncodeToString(sum[:])
}
