// Package thumb 为本地媒体库无海报条目生成首帧缩略图。
package thumb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrNoMpv 表示 mpv 未安装，无法走兜底抓帧。
var ErrNoMpv = errors.New("mpv 未安装")

// Generator 抽象缩略图生成，便于注入 fake 测试。
type Generator interface {
	Generate(videoPath, cachePath string) error
}

// MpvGenerator 用一次性 mpv 子进程抓帧写 JPEG。
type MpvGenerator struct {
	mpvPath string
	timeout time.Duration
}

// NewMpvGenerator 构造抓帧器；mpvPath 为空时 Generate 返回 ErrNoMpv。
func NewMpvGenerator(mpvPath string, timeout time.Duration) *MpvGenerator {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &MpvGenerator{mpvPath: mpvPath, timeout: timeout}
}

// Generate 抓取 videoPath 的 10% 处一帧写入 cachePath。
func (g *MpvGenerator) Generate(videoPath, cachePath string) error {
	if g == nil || g.mpvPath == "" {
		return ErrNoMpv
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("创建缩略图目录失败: %w", err)
	}
	_ = os.Remove(cachePath)
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.mpvPath,
		"--no-config", "--vo=image", "--vo-image-format=jpg",
		"--frames=1", "--start=10%", "--o="+cachePath, videoPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		_ = os.Remove(cachePath)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("mpv 抓帧超时: %w", ctx.Err())
		}
		return fmt.Errorf("mpv 抓帧失败: %w", err)
	}
	info, err := os.Stat(cachePath)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(cachePath)
		if err != nil {
			return fmt.Errorf("mpv 抓帧未产出文件: %w", err)
		}
		return fmt.Errorf("mpv 抓帧未产出文件: %s", cachePath)
	}
	return nil
}
