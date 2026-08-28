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
