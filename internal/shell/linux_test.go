package shell

import (
	"errors"
	"strings"
	"testing"
)

// TestCheckUserNamespacesUnshareMissing 覆盖「unshare 缺失」分支：无法探测应放行，不误拦。
func TestCheckUserNamespacesUnshareMissing(t *testing.T) {
	origLook, origRun := lookPathUnshare, runUnshareProbe
	defer func() { lookPathUnshare, runUnshareProbe = origLook, origRun }()

	lookPathUnshare = func(string) (string, error) {
		return "", errors.New("exec: unshare not found")
	}
	if err := checkUserNamespaces(); err != nil {
		t.Fatalf("unshare 缺失应放行，got err=%v", err)
	}
}

// TestCheckUserNamespacesBlocked 覆盖「userns 被禁」分支：应返回含可操作指引的错误。
func TestCheckUserNamespacesBlocked(t *testing.T) {
	origLook, origRun := lookPathUnshare, runUnshareProbe
	defer func() { lookPathUnshare, runUnshareProbe = origLook, origRun }()

	lookPathUnshare = func(string) (string, error) { return "/usr/bin/unshare", nil }
	runUnshareProbe = func() error {
		return errors.New("unshare failed: Operation not permitted")
	}

	err := checkUserNamespaces()
	if err == nil {
		t.Fatal("userns 被禁应返回错误")
	}
	if !strings.Contains(err.Error(), "apparmor_restrict_unprivileged_userns") {
		t.Fatalf("错误信息应包含可操作指引（apparmor_restrict_unprivileged_userns），got: %v", err)
	}
}

// TestCheckUserNamespacesOK 覆盖「userns 可用」分支：应放行。
func TestCheckUserNamespacesOK(t *testing.T) {
	origLook, origRun := lookPathUnshare, runUnshareProbe
	defer func() { lookPathUnshare, runUnshareProbe = origLook, origRun }()

	lookPathUnshare = func(string) (string, error) { return "/usr/bin/unshare", nil }
	runUnshareProbe = func() error { return nil }

	if err := checkUserNamespaces(); err != nil {
		t.Fatalf("userns 可用应放行，got err=%v", err)
	}
}
