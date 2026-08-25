//go:build windows

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// NativeWindowID 当前恒返回 0：Windows 上 mpv --wid 嵌入延后到 M4（mpvlib），
// 现在让 mpv 独立开窗（--force-window），与 Linux 的现状一致。
// 真正嵌入需前端视频区 + 子窗口，否则 WebView2 会覆盖 mpv 画面导致黑屏。
func NativeWindowID(w *application.WebviewWindow) uintptr {
	return 0
}
