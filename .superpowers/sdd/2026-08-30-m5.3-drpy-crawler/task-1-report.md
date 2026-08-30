# Task 1 报告

## RED

新增 `TestRuleParsesDrpyFields` 后运行：

```text
env GOCACHE=/tmp/unbox-m53-task1-red-cache go test ./internal/crawler -run TestRuleParsesDrpyFields -count=1
```

测试按预期编译失败，报告 `Rule` 缺少 `URL`、`ClassParse`、`Lazy` 字段。

## GREEN

为 `Rule` 增加 `URL`、`ClassParse`、`Lazy`、`Headers`、`Timeout` 及对应 JSON tags；将 `golang.org/x/text v0.41.0` 从间接依赖转为直接依赖，并加入仅使用 `example.com` 的 `drpy_example.js` fixture。

goja 默认按 Go 字段名导出结构体，现有 JSON tags 不会自动生效，因此在 `New` 中启用 `TagFieldNameMapper("json", true)`，保证 dr_py 小写字段可以解析。

聚焦测试通过：

```text
env GOCACHE=/tmp/unbox-m53-task1-green-cache go test ./internal/crawler -run TestRuleParsesDrpyFields -count=1
ok   github.com/unbox/unbox/internal/crawler  0.005s
```

完整 crawler 包测试尝试运行，但沙箱禁止 `httptest.NewServer` 监听端口，失败于 `TestReqInjectsAndCalls` 的环境权限：`listen tcp6 [::1]:0: socket: operation not permitted`。

## 文件

- `go.mod`
- `internal/crawler/crawler.go`
- `internal/crawler/rule_test.go`
- `internal/crawler/types.go`
- `testdata/crawler/drpy_example.js`
