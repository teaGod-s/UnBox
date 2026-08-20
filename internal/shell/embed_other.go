//go:build !linux && !windows

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// NativeWindowID 在非 mpvproc 平台（darwin，走 mpvlib）恒返回 0。
func NativeWindowID(w *application.WebviewWindow) uintptr { return 0 }
