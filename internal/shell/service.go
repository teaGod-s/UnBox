package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/unbox/unbox/internal/config"
	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/provider"
	"github.com/unbox/unbox/internal/provider/live"
	"github.com/unbox/unbox/internal/store"
)

// ImportResult 是导入订阅的摘要。
type ImportResult struct {
	Groups   int
	Channels int
}

// ChannelInfo 是前端展示用的频道信息。
type ChannelInfo struct {
	ID        string
	Name      string
	Group     string
	Logo      string
	Favorited bool
}

// NewShellService 组装壳层服务。p 可为 nil（播放器未就绪）。
func NewShellService(pv provider.Provider, p player.Player, st *store.Store) *ShellService {
	return &ShellService{provider: pv, player: p, store: st}
}

// ImportSubscription 拉取并解析订阅，重建 Provider。支持两类输入：
//   - TVBox 订阅配置（JSON，含 lives/storeHouse/urls，可能多仓）
//   - 独立 M3U/TXT 播放列表（#EXTM3U 或「名称,URL」行）
func (s *ShellService) ImportSubscription(ref string) (ImportResult, error) {
	raw, err := config.NewFetcher().Fetch(context.Background(), ref)
	if err != nil {
		return ImportResult{}, fmt.Errorf("拉取 %s 失败: %w", ref, err)
	}
	if isPlaylist(raw) {
		return s.importPlaylist(raw)
	}
	cfgs, err := resolveConfigs(context.Background(), ref, raw)
	if err != nil {
		return ImportResult{}, err
	}
	channels := collectChannels(context.Background(), cfgs)
	if len(channels) == 0 {
		return ImportResult{}, errors.New("订阅中没有可用直播频道")
	}
	lp := live.New(channels)
	s.mu.Lock()
	s.provider = lp
	s.mu.Unlock()
	secs, _ := lp.Home(context.Background())
	return ImportResult{Groups: len(secs), Channels: len(channels)}, nil
}

// isPlaylist 探测内容是否为独立播放列表而非 JSON 配置。
func isPlaylist(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return false
	}
	if bytes.HasPrefix(t, []byte("#EXTM3U")) {
		return true
	}
	// JSON 配置以 { 或 [ 开头；其余（如「名称,URL」行）按 TXT 播放列表处理。
	return t[0] != '{' && t[0] != '['
}

// importPlaylist 解析独立 M3U/TXT 播放列表并重建 Provider。
func (s *ShellService) importPlaylist(raw []byte) (ImportResult, error) {
	entries := live.ParseM3U(raw)
	if len(entries) == 0 {
		entries = live.ParseTXT(raw) // m3u 解析为空则按 TXT 回退
	}
	channels := live.Assemble(entries)
	if len(channels) == 0 {
		return ImportResult{}, errors.New("播放列表中没有可用频道")
	}
	lp := live.New(channels)
	s.mu.Lock()
	s.provider = lp
	s.mu.Unlock()
	secs, _ := lp.Home(context.Background())
	return ImportResult{Groups: len(secs), Channels: len(channels)}, nil
}

// resolveConfigs 解析 TVBox 配置；索引节点（storeHouse/urls 非空）交给 Resolver 展开。
func resolveConfigs(ctx context.Context, ref string, raw []byte) ([]*config.Config, error) {
	cfg, err := config.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if len(cfg.StoreHouse) > 0 || len(cfg.URLs) > 0 {
		cfgs, rerr := config.NewResolver().Resolve(ctx, ref)
		if rerr != nil && len(cfgs) == 0 {
			return nil, fmt.Errorf("展开订阅失败: %w", rerr)
		}
		return cfgs, nil
	}
	return []*config.Config{cfg}, nil
}

// collectChannels 从多份配置收集直播频道：Channels 内嵌的直接用，URL 指向的拉取解析。
func collectChannels(ctx context.Context, cfgs []*config.Config) []config.Channel {
	var channels []config.Channel
	fetcher := config.NewFetcher()
	for _, cfg := range cfgs {
		for _, lv := range cfg.Lives {
			if len(lv.Channels) > 0 {
				channels = append(channels, lv.Channels...)
				continue
			}
			if lv.URL == "" {
				continue
			}
			chs, ferr := live.FetchLive(ctx, lv, fetcher)
			if ferr == nil {
				channels = append(channels, chs...)
			}
		}
	}
	return channels
}

func (s *ShellService) Groups() ([]string, error) {
	s.mu.RLock()
	pv := s.provider
	s.mu.RUnlock()
	if pv == nil {
		return nil, nil
	}
	secs, err := pv.Home(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]string, len(secs))
	for i, sec := range secs {
		out[i] = sec.Title
	}
	return out, nil
}

func (s *ShellService) Channels(group string, page int) ([]ChannelInfo, error) {
	s.mu.RLock()
	pv := s.provider
	s.mu.RUnlock()
	if pv == nil {
		return nil, nil
	}
	pg, err := pv.Browse(context.Background(), group, page)
	if err != nil {
		return nil, err
	}
	return s.toChannelInfo(pg.Items), nil
}

func (s *ShellService) Search(q string) ([]ChannelInfo, error) {
	s.mu.RLock()
	pv := s.provider
	s.mu.RUnlock()
	if pv == nil {
		return nil, nil
	}
	items, err := pv.Search(context.Background(), q)
	if err != nil {
		return nil, err
	}
	return s.toChannelInfo(items), nil
}

func (s *ShellService) toChannelInfo(items []provider.Item) []ChannelInfo {
	out := make([]ChannelInfo, len(items))
	for i, it := range items {
		var fav bool
		if s.store != nil {
			fav, _ = s.store.IsFavorite(it.ID)
		}
		out[i] = ChannelInfo{ID: it.ID, Name: it.Title, Group: it.Group, Logo: it.Logo, Favorited: fav}
	}
	return out
}

func (s *ShellService) PlayChannel(id string) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	s.mu.RLock()
	pv := s.provider
	s.mu.RUnlock()
	if pv == nil {
		return errors.New("未导入订阅")
	}
	st, err := pv.Resolve(context.Background(), id)
	if err != nil {
		return err
	}
	if err := s.player.Load(context.Background(), st); err != nil {
		return err
	}
	if s.store != nil {
		if m, err := pv.Detail(context.Background(), id); err == nil {
			_ = s.store.AddRecent(id, m.Title, m.Group, st.URL) // 尽力而为，失败不阻断播放
		}
	}
	return s.player.Play()
}

func (s *ShellService) Pause() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Pause()
}

func (s *ShellService) Resume() error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.Play()
}

func (s *ShellService) SetVolume(v int) error {
	if s.player == nil {
		return errors.New("播放器未就绪")
	}
	return s.player.SetVolume(v)
}

func (s *ShellService) AddFavorite(id string) error {
	s.mu.RLock()
	pv := s.provider
	s.mu.RUnlock()
	if pv == nil {
		return errors.New("未导入订阅")
	}
	if s.store == nil {
		return errors.New("数据库未就绪，收藏不可用")
	}
	m, err := pv.Detail(context.Background(), id)
	if err != nil {
		return err
	}
	st, err := pv.Resolve(context.Background(), id)
	if err != nil {
		return err
	}
	return s.store.AddFavorite(id, m.Title, m.Group, st.URL)
}

func (s *ShellService) RemoveFavorite(id string) error {
	if s.store == nil {
		return errors.New("数据库未就绪，收藏不可用")
	}
	return s.store.RemoveFavorite(id)
}

func (s *ShellService) ListFavorites() ([]ChannelInfo, error) {
	if s.store == nil {
		return nil, errors.New("数据库未就绪，收藏不可用")
	}
	favs, err := s.store.ListFavorites()
	if err != nil {
		return nil, err
	}
	out := make([]ChannelInfo, len(favs))
	for i, f := range favs {
		out[i] = ChannelInfo{ID: f.ID, Name: f.Name, Group: f.Group, Favorited: true}
	}
	return out, nil
}

func (s *ShellService) AddGroup(name string) error {
	if s.store == nil {
		return errors.New("数据库未就绪，收藏不可用")
	}
	return s.store.AddGroup(name)
}

func (s *ShellService) ListGroups() ([]string, error) {
	if s.store == nil {
		return nil, errors.New("数据库未就绪，收藏不可用")
	}
	gs, err := s.store.ListGroups()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Name
	}
	return out, nil
}
