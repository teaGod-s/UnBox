// Package mpvplugin 管理外部 mpv 的探测与安装。
package mpvplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unbox/unbox/internal/player"
	"github.com/unbox/unbox/internal/player/mpvproc"
)

const (
	windowsURL  = "https://github.com/mpv-distributions/mpv-windows-setup/releases/download/0.41.0/mpv-setup-x86_64-0.41.0.exe"
	windowsSHA  = "1b32d5eb7e713ecc5853c18107daffac652e29474dfd517a4ddb792dc45e40fc"
	maxDownload = int64(80 << 20)
)

type Status struct {
	Available      bool
	Path           string
	InstallMode    string
	InstallCommand string
}
type InstallResult struct {
	Installed bool
	Message   string
}

type Manager struct {
	goos     string
	root     string
	lookPath func(string) (string, error)
	client   *http.Client
	run      func(context.Context, string, ...string) error
}

func New(goos, root string) *Manager {
	if goos == "" {
		goos = runtime.GOOS
	}
	if root == "" {
		root, _ = os.UserConfigDir()
	}
	return newManager(goos, root, exec.LookPath)
}

func newManager(goos, root string, lookPath func(string) (string, error)) *Manager {
	return &Manager{goos: goos, root: root, lookPath: lookPath, client: http.DefaultClient, run: func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Run()
	}}
}

func (m *Manager) Status() Status {
	if path := m.pluginPath(); path != "" {
		return Status{Available: true, Path: path}
	}
	if path, err := m.lookPath(exeForOS(m.goos)); err == nil {
		return Status{Available: true, Path: path}
	}
	s := Status{InstallMode: "manual"}
	s.InstallCommand = installCommandForOS(m.goos, m.root)
	if s.InstallCommand != "" {
		s.InstallMode = "command"
	}
	if m.goos == "windows" {
		s.InstallMode = "download"
	}
	return s
}

func (m *Manager) Install(ctx context.Context) (InstallResult, error) {
	switch m.goos {
	case "linux", "darwin":
		command := installCommandForOS(m.goos, m.root)
		if command == "" {
			return InstallResult{}, errors.New("当前系统没有可用的 mpv 安装命令")
		}
		return InstallResult{Message: "请在终端执行: " + command}, nil
	case "windows":
		return m.installWindows(ctx)
	default:
		return InstallResult{}, errors.New("当前系统暂不支持自动安装 mpv")
	}
}

func (m *Manager) NewPlayer() (player.Player, error) {
	s := m.Status()
	if !s.Available {
		return nil, ErrUnavailable()
	}
	return mpvproc.New(s.Path)
}

func ErrUnavailable() error { return errors.New("未找到 mpv 可执行文件") }

func (m *Manager) installWindows(ctx context.Context) (InstallResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, windowsURL, nil)
	if err != nil {
		return InstallResult{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return InstallResult{}, fmt.Errorf("下载 mpv 安装包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return InstallResult{}, fmt.Errorf("下载 mpv 安装包失败: HTTP %d", resp.StatusCode)
	}
	dir := filepath.Join(m.root, "unbox", "plugins", "mpv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("创建 mpv 插件目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "mpv-*.exe")
	if err != nil {
		return InstallResult{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	n, err := io.CopyN(io.MultiWriter(tmp, hash), resp.Body, maxDownload+1)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = tmp.Close()
		return InstallResult{}, fmt.Errorf("保存 mpv 安装包失败: %w", err)
	}
	if n > maxDownload {
		_ = tmp.Close()
		return InstallResult{}, errors.New("mpv 安装包过大")
	}
	if err := tmp.Close(); err != nil {
		return InstallResult{}, err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, windowsSHA) {
		return InstallResult{}, errors.New("mpv 安装包校验失败")
	}
	if err := m.run(ctx, tmpName, "/VERYSILENT", "/CURRENTUSER", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR="+dir); err != nil {
		return InstallResult{}, fmt.Errorf("运行 mpv 安装程序失败: %w", err)
	}
	return InstallResult{Installed: true, Message: "mpv 插件安装完成"}, nil
}

func (m *Manager) pluginPath() string {
	path := filepath.Join(m.root, "unbox", "plugins", "mpv", exeForOS(m.goos))
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func exeForOS(goos string) string {
	if goos == "windows" {
		return "mpv.exe"
	}
	return "mpv"
}
func installCommandForOS(goos, _ string) string {
	switch goos {
	case "darwin":
		return "brew install mpv"
	case "linux":
		for _, pair := range []struct{ name, command string }{{"apt-get", "sudo apt install mpv"}, {"apt", "sudo apt install mpv"}, {"dnf", "sudo dnf install mpv"}, {"pacman", "sudo pacman -S mpv"}} {
			if _, err := exec.LookPath(pair.name); err == nil {
				return pair.command
			}
		}
		return "sudo apt install mpv"
	default:
		return ""
	}
}
func commandFor(name string) string {
	return map[string]string{"apt": "sudo apt install mpv", "dnf": "sudo dnf install mpv", "pacman": "sudo pacman -S mpv"}[name]
}
