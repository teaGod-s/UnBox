package shell

import (
	"os"
	"runtime"
	"testing"
)

func TestForceLinuxX11Backend(t *testing.T) {
	t.Setenv("GDK_BACKEND", "")
	forceLinuxX11Backend()
	if runtime.GOOS == "linux" {
		if got := os.Getenv("GDK_BACKEND"); got != "x11" {
			t.Fatalf("Linux 下 GDK_BACKEND = %q, 期望 x11", got)
		}
		return
	}
	if got := os.Getenv("GDK_BACKEND"); got != "" {
		t.Fatalf("非 Linux 下 GDK_BACKEND = %q, 期望保持为空", got)
	}
}

func TestConfigureLinuxRenderingFallsBackWithoutDRI(t *testing.T) {
	t.Setenv("LIBGL_ALWAYS_SOFTWARE", "")
	t.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "")
	configureLinuxRendering(func(string) bool { return false })
	if got := os.Getenv("LIBGL_ALWAYS_SOFTWARE"); got != "1" {
		t.Fatalf("LIBGL_ALWAYS_SOFTWARE = %q, 期望 1", got)
	}
	if got := os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER"); got != "1" {
		t.Fatalf("WEBKIT_DISABLE_DMABUF_RENDERER = %q, 期望 1", got)
	}
}
