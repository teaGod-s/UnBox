# Task 6 实现报告：详情提取

## 状态

已完成。提交：`feat(crawler): 二级详情提取（json:/js:）`

## 实现内容

- 在 `internal/crawler/inline.go` 新增 `(*Engine).extractDetail(html, rule, id)`。
- `二级` 内联规则以 `json:` 开头时：
  - 解析响应 JSON；
  - 复用 `navigateJSON` 与 `evalJSONExpr`，支持嵌套路径、`||` 备选字段和 `+` 拼接；
  - 将常见 drpy 字段映射到 `Detail`/`Vod` 字段，并保留传入的 `id` 作为默认 `vod_id`。
- `二级` 内联规则以 `js:` 开头时：
  - 在现有 goja VM 中执行片段；
  - 注入 `input`、`fetch`、`request`、`VOD` 和 `urljoin2`；
  - 执行后读取 `VOD` 全局并通过既有详情导出逻辑转换。
- 没有内联规则时，读取 `muban` 的 `二级` 选择器，使用现有 HTML 规则和播放解析作为兜底。
- 新增 JSON 与 JS 两个聚焦测试，测试不访问真实网络地址。

## 验证

- `gofmt -w internal/crawler/inline.go internal/crawler/inline_test.go`：通过。
- `GOCACHE=/tmp/unbox-m53-task6-green-cache go test ./internal/crawler -run 'TestExtractDetail(JSON|JS)' -count=1`：通过。
- `lsp_diagnostics`（`internal/crawler/inline.go`）：0 errors / 0 warnings。
- `GOCACHE=/tmp/unbox-m53-task6-all-cache go test ./internal/crawler -count=1`：未完成；沙箱禁止 `httptest.NewServer` 创建监听 socket，失败发生在既有 `TestReqInjectsAndCalls`，与本任务代码无关。

## 关注事项

- `VodDetail` 的实际调用路径由后续任务接入 `extractDetail`；本任务只新增并验证提取器，未改变既有 FongMi action 路径。
- `urljoin2` 支持单参数（使用规则 host）和双参数（显式 base/path）形式。
