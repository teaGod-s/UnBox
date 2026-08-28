package mpvplugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerStatusPrefersPluginExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "unbox", "plugins", "mpv", "mpv.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newManager("windows", dir, func(string) (string, error) { return `C:\\system\\mpv.exe`, nil })
	got := m.Status()
	if !got.Available || got.Path != exe {
		t.Fatalf("Status = %+v", got)
	}
}

func TestManagerUnixReturnsInstallCommandWithoutRunningIt(t *testing.T) {
	for _, tc := range []struct{ goos, want string }{{"linux", "sudo apt install mpv"}, {"darwin", "brew install mpv"}} {
		m := newManager(tc.goos, t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
		got := m.Status()
		if got.Available || got.InstallMode != "command" || got.InstallCommand == "" {
			t.Fatalf("%s status = %+v", tc.goos, got)
		}
		if got.InstallCommand != tc.want {
			t.Fatalf("%s command = %q", tc.goos, got.InstallCommand)
		}
	}
}

func TestManagerInstallUnsupportedOSIsManual(t *testing.T) {
	m := newManager("plan9", t.TempDir(), func(string) (string, error) { return "", errors.New("missing") })
	if got := m.Status(); got.InstallMode != "manual" {
		t.Fatalf("status = %+v", got)
	}
	if _, err := m.Install(context.Background()); err == nil {
		t.Fatal("unsupported install should fail")
	}
}
