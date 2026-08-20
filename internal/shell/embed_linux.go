//go:build linux

package shell

/*
#cgo pkg-config: gtk4-x11
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>

// unbox_window_xid 必须在 GTK 主线程调用（GTK 非线程安全）。
static unsigned long unbox_window_xid(void* widget) {
	GtkNative* native = gtk_widget_get_native((GtkWidget*)widget);
	if (native == NULL) {
		return 0;
	}
	GdkSurface* surface = gtk_native_get_surface(native);
	if (surface == NULL) {
		return 0;
	}
	return gdk_x11_surface_get_xid(surface);
}
*/
import "C"
import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// NativeWindowID 返回用于 mpv --wid 嵌入的 X11 窗口 ID（XID）。
// 窗口未 realize 时返回 0（调用方应回退为独立窗口）。
//
// GTK 非线程安全：XID 提取必须跑在 GTK 主线程。这里经 Wails 的
// application.InvokeSync（内部走 g_idle_add 调度到主循环）执行，
// 而非在调用方 goroutine 上直接跨线程调 GTK——后者会在主循环启动前
// 就地执行回调、破坏 GTK 内部状态，导致 g_application_run 段错误。
func NativeWindowID(w *application.WebviewWindow) uintptr {
	if w == nil {
		return 0
	}
	p := w.NativeWindow()
	if p == nil {
		return 0
	}
	var id uintptr
	application.InvokeSync(func() {
		id = uintptr(C.unbox_window_xid(p))
	})
	return id
}
