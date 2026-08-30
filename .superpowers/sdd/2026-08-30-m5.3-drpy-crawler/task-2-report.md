# Task 2 Report: `muban` dynamic object and readback

## RED

Added `TestMubanAutoVivifies` in `internal/crawler/muban_test.go` using the required script:

```text
muban.首图2.二级.desc = '.data:eq(0)&&Text'; var rule={title:"x",host:"https://example.com"}
```

Command:

```text
GOCACHE=/tmp/unbox-m53-task2-red go test ./internal/crawler -run TestMubanAutoVivifies -count=1
```

Result: failed to compile because `e.readMuban` was undefined. This is the expected pre-implementation failure (the brief describes the equivalent runtime failure as `muban is not defined`).

## GREEN

Implemented `muban` installation in `New`, a goja `DynamicObject` handler with nested auto-vivification and value capture, and `Engine.readMuban` flattening exported values into dot-separated keys.

Focused verification:

```text
GOCACHE=/tmp/unbox-m53-task2-green go test ./internal/crawler -run TestMubanAutoVivifies -count=1
```

Result: `ok   github.com/unbox/unbox/internal/crawler`.

Formatting was applied with `gofmt -w internal/crawler/muban.go internal/crawler/muban_test.go`.

## Broader verification

```text
GOCACHE=/tmp/unbox-m53-task2-all go test ./internal/crawler -count=1
```

The suite currently cannot complete in this sandbox: existing `TestReqInjectsAndCalls` panics when `httptest.NewServer` attempts to listen on `[::1]:0` (`operation not permitted`). This is unrelated to the `muban` changes.
