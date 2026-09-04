package nsis

import (
	"os"
	"strings"
	"testing"
)

func TestInstallerChecksUnboxProcessWithoutMatchingItsOwnWindow(t *testing.T) {
	script, err := os.ReadFile("project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	if !strings.Contains(text, "tasklist") || !strings.Contains(text, "${PRODUCT_EXECUTABLE}") {
		t.Fatal("installer must check the unbox.exe process via tasklist filtered by ${PRODUCT_EXECUTABLE}")
	}
	if strings.Contains(text, "FindWindow") {
		t.Fatal("installer must not identify the app by window title")
	}
	if !strings.Contains(text, "UnBox is still running") {
		t.Fatal("installer running-app message must use an ASCII-safe string")
	}
}

func TestEnsureAppClosedFindsRunningProcess(t *testing.T) {
	script, err := os.ReadFile("project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)

	// findstr /X prints only lines that match EXACTLY (the whole line). A
	// tasklist output line is "unbox.exe  12345 Console  1  50,123 K" — it
	// always carries PID/session/memory columns, so it never equals just the
	// image name. With /X, findstr never matches and the "still running"
	// dialog never appears even when UnBox IS running. The check must use a
	// substring (literal) match instead.
	if strings.Contains(text, "findstr /I /X") {
		t.Fatal(`ensureAppClosed must not use "findstr /I /X": /X requires an exact ` +
			`whole-line match, but tasklist output includes PID/session/memory, ` +
			`so the running-app check never matches and the retry dialog never appears`)
	}
	if !strings.Contains(text, `findstr /I /C:`) {
		t.Fatal(`ensureAppClosed must use "findstr /I /C:..." for a substring match on the image name`)
	}
}

func TestInstallerRemembersPreviousInstallDir(t *testing.T) {
	script, err := os.ReadFile("project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)

	// wails.writeUninstaller writes InstallLocation under SetRegView 64 (the
	// native 64-bit registry view). NSIS's InstallDirRegKey is NOT affected by
	// SetRegView, so it reads the 32-bit (WOW6432Node) view by default and
	// never finds the stored path — the directory page resets to the default
	// on every update. The installer must read InstallLocation itself, in the
	// 64-bit view, during .onInit and assign it to $INSTDIR.
	if !strings.Contains(text, "SetRegView 64") {
		t.Fatal(".onInit must call SetRegView 64 before reading the stored install dir")
	}
	if !strings.Contains(text, "ReadRegStr") {
		t.Fatal(".onInit must ReadRegStr InstallLocation (InstallDirRegKey is unaffected by SetRegView)")
	}
}
