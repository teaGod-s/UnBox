// Package store 提供收藏、最近观看、自定义分组的 SQLite 持久化。
// 使用 modernc.org/sqlite（纯 Go 无 cgo），满足「安装即用」无外部依赖约束。
package store

import (
	"database/sql"
	"fmt"
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
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
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
