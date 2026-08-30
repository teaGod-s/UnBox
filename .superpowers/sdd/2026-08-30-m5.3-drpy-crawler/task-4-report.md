# Task 4：`class_parse` 分类解析

## RED

新增 `internal/crawler/classparse_test.go`，执行：

```text
GOCACHE=/tmp/unbox-m53-task4-red go test ./internal/crawler -run TestParseClasses -count=1
```

结果：失败，`undefined: parseClasses`。

## GREEN

新增 `internal/crawler/classparse.go`，实现四段 `class_parse` 解析：选择器、名称规则、ID 规则和 ID 正则；规则通过 `evalRule` 执行，并兼容无括号的 `Text`/`Html` 终端写法。执行：

```text
gofmt -w internal/crawler/classparse.go internal/crawler/classparse_test.go
GOCACHE=/tmp/unbox-m53-task4-green go test ./internal/crawler -run TestParseClasses -count=1
```

结果：通过。

完整包测试尝试：

```text
GOCACHE=/tmp/unbox-m53-task4-all go test ./internal/crawler -count=1
```

结果：受沙箱网络权限限制，既有 `TestReqInjectsAndCalls` 在 `httptest.NewServer` 监听 IPv6 回环地址时 panic（`listen tcp6 [::1]:0: operation not permitted`），与本任务代码无关。

## Review 修复

补充回归测试覆盖无关元素和非法段数。实现改为先按完整选择器链取得条目，再对每个条目分别执行名称和 ID 规则；同时严格拒绝非四段声明。

修复后的 focused 测试需在同包新增的 `extractVods` 实现完成后运行；此前工作区的 `inline_test.go` 因依赖尚未实现而阻止 Go 编译。
