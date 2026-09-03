// Package store 提供收藏、最近观看、自定义分组的 SQLite 持久化。
// 使用 modernc.org/sqlite（纯 Go 无 cgo），满足「安装即用」无外部依赖约束。
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Favorite 是一条收藏。
type Favorite struct {
	ID      string // 频道稳定标识（group/name）
	Name    string
	Group   string
	URL     string
	Logo    string
	AddedAt int64
}

// Recent 是一条最近观看记录。
type Recent struct {
	ID        string
	Name      string
	Group     string
	URL       string
	WatchedAt int64
}

// Group 是一个自定义分组。
type Group struct {
	ID    int64
	Name  string
	Order int64
}

// Source 是一条导入源的历史记录。
type Source struct {
	Kind string // "vod" | "live"
	Ref  string
	At   int64
}

// VodHistory 是一条点播观看记录（首页「续播」用）。
type VodHistory struct {
	Site      string
	VodID     string
	VodTitle  string
	VodLogo   string
	EpID      string
	EpName    string
	Source    string
	Progress  int // 秒
	Duration  int // 秒
	UpdatedAt int64
}

// VodFavorite 是一条点播收藏。
type VodFavorite struct {
	Site    string
	VodID   string
	Title   string
	Logo    string
	Group   string
	AddedAt int64
}

// Store 封装 SQLite 连接。
type Store struct{ db *sql.DB }

// Open 打开（或创建）数据库并跑迁移。
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// SetKV 以 key 存储一条字符串值（覆盖写）。用于订阅快照等非结构化持久化。
func (s *Store) SetKV(k, v string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO kv(k,v) VALUES(?,?)`, k, v)
	return err
}

// GetKV 读取 key 对应的字符串值。不存在时 ok=false 且返回空串。
func (s *Store) GetKV(k string) (v string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// migrate 用 CREATE TABLE IF NOT EXISTS 建表（M1 无需版本化迁移）。
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS favorites (id TEXT PRIMARY KEY, name TEXT NOT NULL, grp TEXT, url TEXT, logo TEXT, added_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS recent (id TEXT PRIMARY KEY, name TEXT NOT NULL, grp TEXT, url TEXT, watched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS grp (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE NOT NULL, ord INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS kv (k TEXT PRIMARY KEY, v TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sources (ref TEXT NOT NULL, kind TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY (ref, kind))`,
		`CREATE TABLE IF NOT EXISTS vod_history (site TEXT NOT NULL, vod_id TEXT NOT NULL, vod_title TEXT NOT NULL, vod_logo TEXT, ep_id TEXT, ep_name TEXT, source TEXT, progress INTEGER NOT NULL DEFAULT 0, duration INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL, PRIMARY KEY (site, vod_id))`,
		`CREATE TABLE IF NOT EXISTS vod_search_history (query TEXT PRIMARY KEY, searched_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS vod_favorites (site TEXT NOT NULL, vod_id TEXT NOT NULL, title TEXT NOT NULL, logo TEXT, grp TEXT, added_at INTEGER NOT NULL, PRIMARY KEY (site, vod_id))`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return s.migrateSourcesSchema()
}

// migrateSourcesSchema 把早期 sources 表的 ref 单主键迁移为 (ref, kind) 复合主键。
// 旧表上 ON CONFLICT(ref, kind) 会因缺少约束而报错，导致源无法登记（回显失败）。
func (s *Store) migrateSourcesSchema() error {
	var pkCols int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sources') WHERE pk > 0`).Scan(&pkCols); err != nil {
		return err
	}
	if pkCols >= 2 {
		return nil // 已是复合主键
	}
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS sources`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE TABLE sources (ref TEXT NOT NULL, kind TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY (ref, kind))`)
	return err
}

func (s *Store) AddFavorite(id, name, group, url string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO favorites(id,name,grp,url,added_at) VALUES(?,?,?,?,?)`,
		id, name, group, url, time.Now().Unix())
	return err
}

func (s *Store) RemoveFavorite(id string) error {
	_, err := s.db.Exec(`DELETE FROM favorites WHERE id=?`, id)
	return err
}

func (s *Store) IsFavorite(id string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM favorites WHERE id=?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ListFavorites() ([]Favorite, error) {
	rows, err := s.db.Query(`SELECT id,name,grp,url,added_at FROM favorites ORDER BY added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Favorite
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.Name, &f.Group, &f.URL, &f.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) AddRecent(id, name, group, url string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO recent(id,name,grp,url,watched_at) VALUES(?,?,?,?,?)`,
		id, name, group, url, time.Now().Unix())
	return err
}

func (s *Store) ListRecent(limit int) ([]Recent, error) {
	rows, err := s.db.Query(`SELECT id,name,grp,url,watched_at FROM recent ORDER BY watched_at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recent
	for rows.Next() {
		var r Recent
		if err := rows.Scan(&r.ID, &r.Name, &r.Group, &r.URL, &r.WatchedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AddGroup(name string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO grp(name,ord) VALUES(?,0)`, name)
	return err
}

func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT id,name,ord FROM grp ORDER BY ord,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Order); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) RemoveGroup(name string) error {
	_, err := s.db.Exec(`DELETE FROM grp WHERE name=?`, name)
	return err
}

// AddSource 记录/更新一条导入源（按 ref+kind 去重，更新时间戳）。
func (s *Store) AddSource(kind, ref string) error {
	_, err := s.db.Exec(`INSERT INTO sources(ref, kind, at) VALUES(?,?,?)
		ON CONFLICT(ref, kind) DO UPDATE SET at=excluded.at`, ref, kind, time.Now().Unix())
	return err
}

// ListSources 返回导入源历史（最近导入在前）。
func (s *Store) ListSources() ([]Source, error) {
	rows, err := s.db.Query(`SELECT kind, ref, at FROM sources ORDER BY at DESC, ref`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var sc Source
		if err := rows.Scan(&sc.Kind, &sc.Ref, &sc.At); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// DeleteSource 删除一条导入源历史。
func (s *Store) DeleteSource(kind, ref string) error {
	_, err := s.db.Exec(`DELETE FROM sources WHERE ref=? AND kind=?`, ref, kind)
	return err
}

// UpsertVodHistory 记录/更新一条点播观看记录（按 site+vod_id 去重）。
func (s *Store) UpsertVodHistory(h VodHistory) error {
	_, err := s.db.Exec(`INSERT INTO vod_history(site, vod_id, vod_title, vod_logo, ep_id, ep_name, source, progress, duration, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(site, vod_id) DO UPDATE SET
			vod_title=excluded.vod_title, vod_logo=excluded.vod_logo,
			ep_id=excluded.ep_id, ep_name=excluded.ep_name, source=excluded.source,
			progress=excluded.progress, duration=excluded.duration, updated_at=excluded.updated_at`,
		h.Site, h.VodID, h.VodTitle, h.VodLogo, h.EpID, h.EpName, h.Source, h.Progress, h.Duration, time.Now().Unix())
	return err
}

// UpdateVodProgress 仅更新点播观看进度（秒）。
func (s *Store) UpdateVodProgress(site, vodID string, progress, duration int) error {
	_, err := s.db.Exec(`UPDATE vod_history SET progress=?, duration=?, updated_at=? WHERE site=? AND vod_id=?`,
		progress, duration, time.Now().Unix(), site, vodID)
	return err
}

// ListVodHistory 返回点播观看历史（最近观看在前）。
func (s *Store) ListVodHistory(limit int) ([]VodHistory, error) {
	rows, err := s.db.Query(`SELECT site, vod_id, vod_title, vod_logo, ep_id, ep_name, source, progress, duration, updated_at FROM vod_history ORDER BY updated_at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VodHistory
	for rows.Next() {
		var h VodHistory
		if err := rows.Scan(&h.Site, &h.VodID, &h.VodTitle, &h.VodLogo, &h.EpID, &h.EpName, &h.Source, &h.Progress, &h.Duration, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// DeleteVodHistory 删除指定点播的观看记录。
func (s *Store) DeleteVodHistory(site, vodID string) error {
	_, err := s.db.Exec(`DELETE FROM vod_history WHERE site=? AND vod_id=?`, site, vodID)
	return err
}

// RecordVodSearch 记录一条点播搜索词，重复搜索会更新时间而不新增记录。
func (s *Store) RecordVodSearch(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO vod_search_history(query, searched_at) VALUES(?,?)
		ON CONFLICT(query) DO UPDATE SET searched_at=excluded.searched_at`, query, time.Now().UnixNano())
	return err
}

// ListVodSearchHistory 返回最近搜索词（最近在前）。
func (s *Store) ListVodSearchHistory(limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT query FROM vod_search_history ORDER BY searched_at DESC, rowid DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var query string
		if err := rows.Scan(&query); err != nil {
			return nil, err
		}
		out = append(out, query)
	}
	return out, rows.Err()
}

// DeleteVodSearch 删除一条点播搜索词。
func (s *Store) DeleteVodSearch(query string) error {
	_, err := s.db.Exec(`DELETE FROM vod_search_history WHERE query=?`, strings.TrimSpace(query))
	return err
}

// AddVodFavorite 新增或更新一条点播收藏。
func (s *Store) AddVodFavorite(site, vodID, title, logo, group string) error {
	_, err := s.db.Exec(`INSERT INTO vod_favorites(site, vod_id, title, logo, grp, added_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(site, vod_id) DO UPDATE SET title=excluded.title, logo=excluded.logo, grp=excluded.grp, added_at=excluded.added_at`,
		site, vodID, title, logo, group, time.Now().UnixNano())
	return err
}

// RemoveVodFavorite 删除一条点播收藏。
func (s *Store) RemoveVodFavorite(site, vodID string) error {
	_, err := s.db.Exec(`DELETE FROM vod_favorites WHERE site=? AND vod_id=?`, site, vodID)
	return err
}

// IsVodFavorite 判断指定点播是否已收藏。
func (s *Store) IsVodFavorite(site, vodID string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM vod_favorites WHERE site=? AND vod_id=?`, site, vodID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// ListVodFavorites 返回点播收藏（最近添加在前）。
func (s *Store) ListVodFavorites() ([]VodFavorite, error) {
	rows, err := s.db.Query(`SELECT site, vod_id, title, logo, grp, added_at FROM vod_favorites ORDER BY added_at DESC, rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VodFavorite
	for rows.Next() {
		var f VodFavorite
		if err := rows.Scan(&f.Site, &f.VodID, &f.Title, &f.Logo, &f.Group, &f.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
