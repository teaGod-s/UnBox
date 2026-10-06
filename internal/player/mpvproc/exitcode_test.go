package mpvproc

import (
	"errors"
	"strings"
	"testing"
)

// 缺 DLL 与进程崩溃的表现完全一样：mpv 一个字都没输出就退出了。退出码是
// 唯一能把它们区分开的证据，必须出现在报错里。
func TestDescribeExitNamesKnownWindowsCodes(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{code: 3221225781, want: "找不到依赖的 DLL"}, // 0xC0000135 STATUS_DLL_NOT_FOUND
		{code: 3221225595, want: "架构不匹配"},      // 0xC000007B STATUS_INVALID_IMAGE_FORMAT
		{code: 3221225477, want: "进程崩溃"},       // 0xC0000005 STATUS_ACCESS_VIOLATION
		{code: 3221225794, want: "DLL 初始化失败"},  // 0xC0000142 STATUS_DLL_INIT_FAILED
	}
	for _, tt := range tests {
		got := describeExit(tt.code)
		if !strings.Contains(got, tt.want) {
			t.Fatalf("describeExit(%d) = %q, want 含 %q", tt.code, got, tt.want)
		}
		if !strings.Contains(got, "0x") {
			t.Fatalf("describeExit(%d) = %q, 应带上十六进制便于检索", tt.code, got)
		}
	}
}

func TestDescribeExitFallsBackToDecimalOnly(t *testing.T) {
	got := describeExit(3)
	if !strings.Contains(got, "3") {
		t.Fatalf("describeExit(3) = %q, want 含退出码", got)
	}
	if strings.Contains(got, "0x") {
		t.Fatalf("describeExit(3) = %q, 未知码不该编造十六进制原因", got)
	}
}

// 取不到退出码（被信号终止等）时退回不带码的通用说明，不能假装知道原因。
func TestMPVExitErrorWithoutExitCode(t *testing.T) {
	for _, in := range []error{nil, errors.New("信号终止")} {
		got := mpvExitError(in)
		if !errors.Is(got, errMPVExitedEarly) {
			t.Fatalf("mpvExitError(%v) = %v, want 包装 errMPVExitedEarly", in, got)
		}
		if strings.Contains(got.Error(), "退出码") {
			t.Fatalf("mpvExitError(%v) = %q, 无退出码时不该编造", in, got)
		}
	}
}
