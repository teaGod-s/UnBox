//go:build linux

package shell

/*
#cgo pkg-config: gtk4-x11
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>

typedef struct {
	GtkWidget* widget;
	unsigned long xid;
} unbox_xid_result;

static gboolean unbox_get_xid_cb(gpointer data) {
	unbox_xid_result* r = (unbox_xid_result*)data;
	GtkNative* native = gtk_widget_get_native(r->widget);
	if (native != NULL) {
		GdkSurface* surface = gtk_native_get_surface(native);
		if (surface != NULL) {
			r->xid = gdk_x11_surface_get_xid(surface);
		}
	}
	return G_SOURCE_REMOVE;
}

static unsigned long unbox_window_xid(void* widget) {
	unbox_xid_result r = { (GtkWidget*)widget, 0 };
	g_main_context_invoke(NULL, unbox_get_xid_cb, &r);
	return r.xid;
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
