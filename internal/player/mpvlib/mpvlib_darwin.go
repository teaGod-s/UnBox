//go:build darwin

// Package mpvlib 是 macOS 上基于 libmpv + CAMetalLayer 的播放实现（spec §3.4）。
//
// M1 阶段本文件只给出结构与 build tag，确保包能在 macOS 上编译通过；
// 真正的 libmpv cgo 绑定与 CAMetalLayer 分层渲染在拿到 macOS 构建机后补齐。
package mpvlib

import (
	"context"
	"errors"

	"github.com/unbox/unbox/internal/player"
)

type libmpvPlayer struct{}

// New 返回 macOS 的 libmpv 播放器。
func New() (player.Player, error) {
	return &libmpvPlayer{}, nil
}

func (p *libmpvPlayer) Load(ctx context.Context, s player.Stream) error {
	return errors.New("mpvlib: 尚未实现（M1 macOS 构建机就绪后补齐）")
}
func (p *libmpvPlayer) Play() error  { return errors.New("mpvlib: 未实现") }
func (p *libmpvPlayer) Pause() error { return errors.New("mpvlib: 未实现") }
func (p *libmpvPlayer) Seek(sec float64) error {
	return errors.New("mpvlib: 未实现")
}
func (p *libmpvPlayer) SetVolume(v int) error { return errors.New("mpvlib: 未实现") }
func (p *libmpvPlayer) SelectTrack(kind player.TrackKind, id int) error {
	return errors.New("mpvlib: 未实现")
}
func (p *libmpvPlayer) State() player.State {
	return player.State{Playing: player.StateStopped, Duration: -1}
}
func (p *libmpvPlayer) Events() <-chan player.Event { return nil }
func (p *libmpvPlayer) Close() error                { return nil }
