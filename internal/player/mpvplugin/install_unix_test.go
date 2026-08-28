package mpvplugin

import "testing"

func TestInstallCommandPackageManagerSelection(t *testing.T) {
	if got := commandFor("apt"); got != "sudo apt install mpv" {
		t.Fatalf("apt = %q", got)
	}
	if got := commandFor("dnf"); got != "sudo dnf install mpv" {
		t.Fatalf("dnf = %q", got)
	}
	if got := commandFor("pacman"); got != "sudo pacman -S mpv" {
		t.Fatalf("pacman = %q", got)
	}
}
