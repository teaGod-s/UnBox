package shell

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestNativeWindowIDNilWindow(t *testing.T) {
	// 一个尚未创建/显示的窗口，其 NativeWindow() 应为 nil，NativeWindowID 返回 0。
	var w *application.WebviewWindow
	if NativeWindowID(w) != 0 {
		t.Fatalf("NativeWindowID(nil) = 非 0")
	}
}
