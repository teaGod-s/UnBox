//go:build linux

package shell

/*
#cgo pkg-config: gtk+-3.0 gdk-x11-3.0
#include <gtk/gtk.h>
#include <gdk/gdkx.h>

static unsigned long unbox_window_xid(void* widget) {
	GdkWindow* gdk = gtk_widget_get_window((GtkWidget*)widget);
	if (gdk == NULL) {
		return 0;
	}
	return gdk_x11_window_get_xid(gdk);
}
*/
import "C"
import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// NativeWindowID 返回用于 mpv --wid 嵌入的 X11 窗口 ID（XID）。
// 窗口未 realize 时返回 0（调用方应回退为独立窗口）。
func NativeWindowID(w *application.WebviewWindow) uintptr {
	if w == nil {
		return 0
	}
	p := w.NativeWindow()
	if p == nil {
		return 0
	}
	return uintptr(C.unbox_window_xid(p))
}
