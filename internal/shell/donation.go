package shell

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

const (
	donationCacheKey    = "donations.cache"
	donationCacheTTL    = 6 * time.Hour
	donationHTTPTimeout = 10 * time.Second
	donationURL         = "https://raw.githubusercontent.com/teaGod-s/UnBox/master/docs/donors.json"
	donationMaxBytes    = 1 << 20
	anonymousDonorName  = "热心网友"
)

// donors_snapshot.json 是随 docs/donors.json 同步的内置离线快照。
//
//go:embed donors_snapshot.json
var donorsSnapshot []byte

// DonorInfo 是捐助榜单中可展示给前端的字段。金额只参与后端排序，不向前端暴露。
type DonorInfo struct {
	ID        string
	Name      string
	Avatar    string
	Anonymous bool
}

// DonationLeaderboard 是捐助榜单快照。
type DonationLeaderboard struct {
	UpdatedAt string
	Donors    []DonorInfo
}

type donationDonor struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Avatar    string  `json:"avatar"`
	Amount    float64 `json:"amount"`
	Anonymous bool    `json:"anonymous"`
}

type donationCacheEntry struct {
	UpdatedAt string          `json:"updatedAt"`
	Donors    []donationDonor `json:"donors"`
	CachedAt  int64           `json:"cachedAt,omitempty"`
}

// GetDonationLeaderboard 按「未过期缓存 → 远端 → 缓存 → 内置快照」返回榜单。
// 网络或解析失败均静默降级，调用方无需处理错误。
func (s *ShellService) GetDonationLeaderboard() DonationLeaderboard {
	now := time.Now
	if s != nil && s.now != nil {
		now = s.now
	}
	nowTime := now()

	cached, hasCache := s.readDonationCache()
	if hasCache && donationCacheFresh(cached, nowTime) {
		return donationLeaderboardFromCache(cached)
	}

	ctx, cancel := context.WithTimeout(context.Background(), donationHTTPTimeout)
	defer cancel()
	remote, err := s.fetchLeaderboard(ctx, donationURL)
	if err == nil {
		s.writeDonationCache(remote, nowTime.Unix())
		return donationLeaderboardFromCache(remote)
	}
	if hasCache {
		return donationLeaderboardFromCache(cached)
	}
	return builtInDonationLeaderboard()
}

// fetchLeaderboard 拉取并解析远端榜单；错误只供内部决定回退层级。
func (s *ShellService) fetchLeaderboard(ctx context.Context, url string) (donationCacheEntry, error) {
	client := (*http.Client)(nil)
	if s != nil {
		client = s.httpClient
	}
	if client == nil {
		client = &http.Client{Timeout: donationHTTPTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return donationCacheEntry{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "UnBox")

	resp, err := client.Do(req)
	if err != nil {
		return donationCacheEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return donationCacheEntry{}, fmt.Errorf("捐助榜单 HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, donationMaxBytes+1))
	if err != nil {
		return donationCacheEntry{}, err
	}
	if len(body) > donationMaxBytes {
		return donationCacheEntry{}, errors.New("捐助榜单响应过大")
	}

	return decodeDonationEntry(body)
}

func (s *ShellService) readDonationCache() (donationCacheEntry, bool) {
	if s == nil || s.store == nil {
		return donationCacheEntry{}, false
	}
	raw, ok, err := s.store.GetKV(donationCacheKey)
	if err != nil || !ok {
		return donationCacheEntry{}, false
	}
	entry, err := decodeDonationEntry([]byte(raw))
	if err != nil {
		return donationCacheEntry{}, false
	}
	return entry, true
}

func (s *ShellService) writeDonationCache(entry donationCacheEntry, cachedAt int64) {
	if s == nil || s.store == nil {
		return
	}
	entry.CachedAt = cachedAt
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = s.store.SetKV(donationCacheKey, string(raw))
}

func donationCacheFresh(entry donationCacheEntry, now time.Time) bool {
	if entry.CachedAt <= 0 {
		return false
	}
	expiresAt := time.Unix(entry.CachedAt, 0).Add(donationCacheTTL)
	return now.Before(expiresAt)
}

func decodeDonationEntry(data []byte) (donationCacheEntry, error) {
	var raw struct {
		UpdatedAt *string            `json:"updatedAt"`
		Donors    *[]json.RawMessage `json:"donors"`
		CachedAt  int64              `json:"cachedAt"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return donationCacheEntry{}, err
	}
	if raw.UpdatedAt == nil || raw.Donors == nil {
		return donationCacheEntry{}, errors.New("捐助榜单字段缺失")
	}

	donors := make([]donationDonor, 0, len(*raw.Donors))
	for _, rawDonor := range *raw.Donors {
		var fields struct {
			ID        *string  `json:"id"`
			Name      *string  `json:"name"`
			Avatar    *string  `json:"avatar"`
			Amount    *float64 `json:"amount"`
			Anonymous *bool    `json:"anonymous"`
		}
		if err := json.Unmarshal(rawDonor, &fields); err != nil {
			return donationCacheEntry{}, err
		}
		if fields.ID == nil || fields.Name == nil || fields.Avatar == nil || fields.Amount == nil || fields.Anonymous == nil {
			return donationCacheEntry{}, errors.New("捐助人字段缺失")
		}
		donors = append(donors, donationDonor{
			ID:        *fields.ID,
			Name:      *fields.Name,
			Avatar:    *fields.Avatar,
			Amount:    *fields.Amount,
			Anonymous: *fields.Anonymous,
		})
	}

	return donationCacheEntry{
		UpdatedAt: *raw.UpdatedAt,
		Donors:    donors,
		CachedAt:  raw.CachedAt,
	}, nil
}

func donationLeaderboardFromCache(entry donationCacheEntry) DonationLeaderboard {
	donors := append([]donationDonor(nil), entry.Donors...)
	slices.SortStableFunc(donors, func(a, b donationDonor) int {
		if a.Amount != b.Amount {
			return cmp.Compare(b.Amount, a.Amount)
		}
		return cmp.Compare(a.Name, b.Name)
	})

	out := make([]DonorInfo, len(donors))
	for i, donor := range donors {
		if donor.Anonymous {
			out[i] = DonorInfo{Name: anonymousDonorName, Anonymous: true}
			continue
		}
		out[i] = DonorInfo{
			ID:        donor.ID,
			Name:      donor.Name,
			Avatar:    donor.Avatar,
			Anonymous: false,
		}
	}
	return DonationLeaderboard{UpdatedAt: entry.UpdatedAt, Donors: out}
}

func builtInDonationLeaderboard() DonationLeaderboard {
	entry, err := decodeDonationEntry(donorsSnapshot)
	if err != nil {
		return DonationLeaderboard{}
	}
	return donationLeaderboardFromCache(entry)
}
