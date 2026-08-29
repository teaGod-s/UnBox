// unbox-scan 体检 FongMi多线路 订阅源，报告其中有多少站点可被 Unbox 使用。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/unbox/unbox/internal/config"
)

func main() {
	asJSON := flag.Bool("json", false, "以 JSON 格式输出")
	timeout := flag.Duration("timeout", 3*time.Minute, "整体超时")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: unbox-scan [--json] <订阅链接或本地文件>")
		os.Exit(2)
	}
	ref := flag.Arg(0)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfgs, err := config.NewResolver().Resolve(ctx, ref)
	os.Exit(handleResult(cfgs, err, ref, *asJSON, os.Stdout, os.Stderr))
}

// handleResult 落实 Resolve 的"部分成功 + 汇总错误"契约：cfgs 与 err 可能
// 同时非空/非 nil（部分节点失败，其余节点已成功展开）。三种情形：
//
//   - cfgs 非空、err 非 nil：部分成功。已经展开出来的部分是真实可用的
//     体检结果，不能因为个别节点失败就整体判死——错误作为警告写到
//     stderr，报告照常在 stdout 产出，退出码 0。
//   - cfgs 为空、err 非 nil：彻底失败，错误写 stderr，退出码 1，不产出
//     报告。
//   - cfgs 为空、err 为 nil：Resolve 内部有兜底逻辑让这种情况理论上很
//     难触发，但这里不依赖那个不变量——必须自己识别并报告"未产生任何
//     可用配置"，退出码 1，且绝不能打印"成功 0 个配置"这种字面上宣称
//     成功的话。
func handleResult(cfgs []*config.Config, resolveErr error, ref string, asJSON bool, stdout, stderr io.Writer) int {
	if len(cfgs) == 0 {
		if resolveErr != nil {
			fmt.Fprintf(stderr, "体检失败: %v\n", resolveErr)
		} else {
			fmt.Fprintln(stderr, "订阅源展开后未产生任何可用配置")
		}
		return 1
	}

	if resolveErr != nil {
		fmt.Fprintf(stderr, "警告：部分节点展开失败，以下报告仅覆盖成功展开的部分: %v\n", resolveErr)
	}

	rep := BuildReport(cfgs)
	if asJSON {
		b, err := rep.JSON()
		if err != nil {
			fmt.Fprintf(stderr, "序列化失败: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(b))
		return 0
	}
	fmt.Fprintf(stdout, "订阅源: %s\n\n%s", ref, rep.Text())
	return 0
}
