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
	if !strings.Contains(text, "tasklist") || !strings.Contains(strings.ToLower(text), "unbox.exe") {
		t.Fatal("installer must check the unbox.exe process")
	}
	if strings.Contains(text, "FindWindow") {
		t.Fatal("installer must not identify the app by window title")
	}
	if !strings.Contains(text, "UnBox is still running") {
		t.Fatal("installer running-app message must use an ASCII-safe string")
	}
}
