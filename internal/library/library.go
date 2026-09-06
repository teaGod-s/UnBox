package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/store"
)

// LibraryDir 与 store 类型保持兼容，供壳层绑定使用。
type LibraryDir = store.LibraryDir

// LibraryItem 与 store 类型保持兼容，供壳层绑定使用。
type LibraryItem = store.LibraryItem

// ScanResult 是一次扫描的汇总。
type ScanResult struct {
	Dirs        int
	Added       int
	Removed     int
	Unavailable []string
}

// Library 是本地媒体库门面：目录管理、扫描、播放流和进度记录。
type Library struct {
	store  *store.Store
	server *server
}

func New(st *store.Store) *Library {
	return &Library{store: st, server: newServer()}
}

func (l *Library) AddDir(path string) error {
	if l.store == nil {
		return fmt.Errorf("媒体库存储未就绪")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("目录路径无效: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("目录不存在或不可访问: %s", path)
	}
	return l.store.AddLibraryDir(absolute)
}

func (l *Library) RemoveDir(path string) error {
	if l.store == nil {
		return fmt.Errorf("媒体库存储未就绪")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return l.store.RemoveLibraryDir(absolute)
}

func (l *Library) ListDirs() ([]store.LibraryDir, error) {
	if l.store == nil {
		return nil, fmt.Errorf("媒体库存储未就绪")
	}
	return l.store.ListLibraryDirs()
}

// Scan 逐个注册目录扫描并整体替换其条目。
func (l *Library) Scan() (ScanResult, error) {
	if l.store == nil {
		return ScanResult{}, fmt.Errorf("媒体库存储未就绪")
	}
	dirs, err := l.store.ListLibraryDirs()
	if err != nil {
		return ScanResult{}, err
	}
	var result ScanResult
	for _, dir := range dirs {
		result.Dirs++
		allPrevious, err := l.store.ListLibraryItems()
		if err != nil {
			return result, err
		}
		previous := make([]store.LibraryItem, 0, len(allPrevious))
		for _, item := range allPrevious {
			if item.Dir == dir.Path {
				previous = append(previous, item)
			}
		}
		items, scanErr := scanDir(dir.Path)
		if scanErr != nil {
			result.Unavailable = append(result.Unavailable, dir.Path)
			result.Removed += len(previous)
			if err := l.store.ReplaceLibraryItems(dir.Path, nil); err != nil {
				return result, err
			}
			continue
		}
		for i := range items {
			items[i].Dir = dir.Path
			if items[i].Poster != "" {
				items[i].Poster = l.server.register(items[i].Poster)
			}
		}
		oldPaths := make(map[string]struct{}, len(previous))
		for _, item := range previous {
			oldPaths[item.Path] = struct{}{}
		}
		for _, item := range items {
			if _, ok := oldPaths[item.Path]; !ok {
				result.Added++
			}
			delete(oldPaths, item.Path)
		}
		result.Removed += len(oldPaths)
		if err := l.store.ReplaceLibraryItems(dir.Path, items); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (l *Library) List() ([]store.LibraryItem, error) {
	if l.store == nil {
		return nil, fmt.Errorf("媒体库存储未就绪")
	}
	return l.store.ListLibraryItems()
}

// StreamFor 按扩展名分流：mp4/m4v/webm 走本地 HTTP + Web，其余走 file:// + mpv。
func (l *Library) StreamFor(path string) (player.Stream, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return player.Stream{}, fmt.Errorf("媒体路径无效: %w", err)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(absolute), "."))
	if ext == "mp4" || ext == "m4v" || ext == "webm" {
		return player.Stream{URL: l.server.register(absolute), Kind: player.StreamMP4}, nil
	}
	return player.Stream{URL: "file://" + filepath.ToSlash(absolute), Kind: player.StreamLocal}, nil
}

// RecordProgress 以 site="local" 复用 vod_history，供首页续播。
func (l *Library) RecordProgress(path string, progress, duration int) error {
	if l.store == nil {
		return fmt.Errorf("媒体库存储未就绪")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return l.store.UpsertVodHistory(store.VodHistory{
		Site: "local", VodID: absolute, VodTitle: displayName(absolute),
		Source: "local", Progress: progress, Duration: duration,
	})
}
