//go:build !darwin

// Package mpvlib 是 macOS 上基于 libmpv + CAMetalLayer 的播放实现。
// 非 macOS 平台只保留包占位，真实实现见 mpvlib_darwin.go。
package mpvlib

import (
	"errors"

	"github.com/unbox/unbox/internal/player"
)

// New 在非 macOS 平台返回明确错误；shell 的 pickPlayer 仅在 runtime.GOOS==darwin
// 时才会真正调用它，此处占位只为让 mpvlib.New 在所有平台可编译。
func New() (player.Player, error) {
	return nil, errors.New("mpvlib: 仅 macOS 支持")
}
