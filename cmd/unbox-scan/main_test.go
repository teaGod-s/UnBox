package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/unbox/unbox/internal/config"
)

// Resolver.Resolve 采用"部分成功 + 汇总错误"语义：cfgs 非空时 err 也可能
// 非 nil（部分节点失败，其余成功展开）。handleResult 把这种情况当成
// 警告处理——错误写到 stderr，但报告照常在 stdout 产出，退出码为 0，
// 不能因为个别节点失败就把已经成功的大部分结果扔掉。
func TestHandleResultPartialSuccessStillReportsAndExitsZero(t *testing.T) {
	cfgs := []*config.Config{
		{Sites: []config.Site{{Type: config.SiteTypeCMS, API: "http://x/api.php"}}},
	}
	resolveErr := errors.New("节点 B 拉取失败")
	var stdout, stderr bytes.Buffer

	code := handleResult(cfgs, resolveErr, "ref", false, &stdout, &stderr)

	if code != 0 {
		t.Errorf("退出码 = %d, want 0（部分成功应视为成功）", code)
	}
	if !strings.Contains(stderr.String(), "节点 B 拉取失败") {
		t.Errorf("stderr 应包含警告信息:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "点播站点") {
		t.Errorf("stdout 应照常输出报告:\n%s", stdout.String())
	}
}

// cfgs 为空且 err 非 nil：彻底失败，错误写 stderr，退出码 1，且不应向
// stdout 输出任何报告内容。
func TestHandleResultTotalFailureExitsOne(t *testing.T) {
	resolveErr := errors.New("拉取失败: 连接超时")
	var stdout, stderr bytes.Buffer

	code := handleResult(nil, resolveErr, "ref", false, &stdout, &stderr)

	if code != 1 {
		t.Errorf("退出码 = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "连接超时") {
		t.Errorf("stderr 应包含错误信息:\n%s", stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("彻底失败时不应产出报告:\n%s", stdout.String())
	}
}

// cfgs 为空且 err 为 nil：Resolve 的兜底逻辑让这种情况理论上很难出现，
// 但 handleResult 不能依赖上游的这个不变量——必须自己明确报告"未产生
// 任何可用配置"，退出码 1，且绝不能打印"成功 0 个配置"这种字面上说
// "成功"的话。
func TestHandleResultZeroConfigsNoErrorExitsOneWithExplicitMessage(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := handleResult(nil, nil, "ref", false, &stdout, &stderr)

	if code != 1 {
		t.Errorf("退出码 = %d, want 1", code)
	}
	if strings.Contains(stderr.String()+stdout.String(), "成功 0 个配置") {
		t.Errorf("不应出现字面上的\"成功 0 个配置\"：\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "未") {
		t.Errorf("stderr 应明确说明未产生可用配置:\nstderr:\n%s", stderr.String())
	}
}

// --json 模式下也要遵守同样的成功/失败规则：部分成功时，报告仍然应该
// 以合法 JSON 的形式写到 stdout。
func TestHandleResultJSONModePartialSuccess(t *testing.T) {
	cfgs := []*config.Config{
		{Sites: []config.Site{{Type: config.SiteTypeCMS, API: "http://x/api.php"}}},
	}
	resolveErr := errors.New("节点 B 拉取失败")
	var stdout, stderr bytes.Buffer

	code := handleResult(cfgs, resolveErr, "ref", true, &stdout, &stderr)

	if code != 0 {
		t.Errorf("退出码 = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "\"totalSites\"") {
		t.Errorf("stdout 应输出 JSON 报告:\n%s", stdout.String())
	}
}
