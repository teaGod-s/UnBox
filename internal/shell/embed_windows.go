//go:build windows

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// NativeWindowID 返回用于 mpv --wid 嵌入的 HWND。
func NativeWindowID(w *application.WebviewWindow) uintptr {
	if w == nil {
		return 0
	}
	p := w.NativeWindow()
	if p == nil {
		return 0
	}
	return uintptr(p)
}
