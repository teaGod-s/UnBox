package library

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unbox/unbox/internal/library/thumb"
	"golang.org/x/sync/singleflight"
)

var thumbSaveGroup singleflight.Group

func (l *Library) thumbCachePath(path string, mtime int64) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("媒体路径无效: %w", err)
	}
	key := sha1.Sum([]byte(fmt.Sprintf("%s\x00%d", abs, mtime)))
	return filepath.Join(l.postersDir, hex.EncodeToString(key[:])+".jpg"), nil
}

// EnsureThumb 注册视频和缩略图 URL，并报告缩略图是否已缓存。
func (l *Library) EnsureThumb(path string, mtime int64) (videoURL, posterURL string, cached bool, err error) {
	if l == nil || l.server == nil {
		return "", "", false, fmt.Errorf("媒体库未就绪")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", false, fmt.Errorf("媒体路径无效: %w", err)
	}
	cachePath, err := l.thumbCachePath(abs, mtime)
	if err != nil {
		return "", "", false, err
	}
	_, statErr := os.Stat(cachePath)
	return l.server.register(abs), l.server.register(cachePath), statErr == nil, nil
}

// SaveThumb 原子保存前端抓取的 JPEG，并返回缓存 URL。
func (l *Library) SaveThumb(path string, mtime int64, jpeg []byte) (posterURL string, err error) {
	if l == nil || l.server == nil {
		return "", fmt.Errorf("媒体库未就绪")
	}
	cachePath, err := l.thumbCachePath(path, mtime)
	if err != nil {
		return "", err
	}
	key := cachePath
	_, err, _ = thumbSaveGroup.Do(key, func() (any, error) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
			return nil, fmt.Errorf("创建缩略图目录失败: %w", err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(cachePath), ".thumb-*.tmp")
		if err != nil {
			return nil, fmt.Errorf("创建缩略图临时文件失败: %w", err)
		}
		tmpName := tmp.Name()
		defer os.Remove(tmpName)
		if _, err := tmp.Write(jpeg); err != nil {
			_ = tmp.Close()
			return nil, fmt.Errorf("写入缩略图失败: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return nil, fmt.Errorf("关闭缩略图临时文件失败: %w", err)
		}
		if err := os.Rename(tmpName, cachePath); err != nil {
			return nil, fmt.Errorf("保存缩略图失败: %w", err)
		}
		return nil, nil
	})
	if err != nil {
		return "", err
	}
	return l.server.register(cachePath), nil
}

// GenerateThumbMpv 使用注入的生成器抓取视频首帧；生成器为空时返回 ErrNoMpv。
func (l *Library) GenerateThumbMpv(path string, mtime int64, generator ...thumb.Generator) (posterURL string, err error) {
	if l == nil || l.server == nil {
		return "", fmt.Errorf("媒体库未就绪")
	}
	gen := l.thumbGen
	if len(generator) > 0 {
		gen = generator[0]
	}
	if gen == nil {
		return "", thumb.ErrNoMpv
	}
	cachePath, err := l.thumbCachePath(path, mtime)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", fmt.Errorf("创建缩略图目录失败: %w", err)
	}
	if err := gen.Generate(path, cachePath); err != nil {
		return "", err
	}
	return l.server.register(cachePath), nil
}
