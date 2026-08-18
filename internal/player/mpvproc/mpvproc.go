// Package mpvproc 通过 mpv 子进程 + JSON IPC 实现 player.Player。
//
// 用于 Windows（--wid=<HWND> 嵌入）与 Linux（--wid=<X11 Window> 嵌入）。
// macOS 不用本包，见 ../mpvlib。
package mpvproc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// ipcConnectTimeout 是连接 mpv IPC socket 的最大等待时间（mpv 启动后创建
// socket 有微小延迟，需重试）。
const ipcConnectTimeout = 5 * time.Second

// cmdResponseTimeout 是单条命令等待 mpv 应答的超时。
const cmdResponseTimeout = 5 * time.Second

type mpvProc struct {
	exePath string
	ipcPath string // --input-ipc-server 暴露的 Unix socket 路径

	cmd  *exec.Cmd
	conn net.Conn

	sendMu    sync.Mutex  // 串行化命令：保证任一时刻只有一条命令在飞
	responses chan []byte // readLoop 路由来的命令应答
	events    chan player.Event

	stateMu sync.Mutex
	state   player.State
}

// New 以指定 mpv 可执行文件启动一个播放器实例。
//
// 实际把视频嵌入窗口需要 --wid=<窗口句柄>，但那是 shell 层的事；本层只
// 负责 mpv 进程生命周期与 JSON IPC 对话，窗口句柄由 shell 通过后续扩展
// 参数传入（M1 阶段先用 --force-window 保证无嵌入也能独立开窗冒烟）。
func New(exePath string) (player.Player, error) {
	if _, err := os.Stat(exePath); err != nil {
		return nil, fmt.Errorf("mpv 可执行文件不可用: %w", err)
	}
	return &mpvProc{
		exePath:   exePath,
		responses: make(chan []byte, 16),
		events:    make(chan player.Event, 64),
		state:     player.State{Playing: player.StateStopped, Duration: -1},
	}, nil
}

func (p *mpvProc) Load(ctx context.Context, s player.Stream) error {
	if p.conn != nil {
		_ = p.Close()
	}
	sock, err := os.CreateTemp("", "unbox-mpv-*.sock")
	if err != nil {
		return err
	}
	ipcPath := sock.Name()
	_ = sock.Close()
	_ = os.Remove(ipcPath)

	args := []string{
		"--idle=yes",
		"--input-ipc-server=" + ipcPath,
		"--force-window=yes",
		"--osc=no",
		"--keep-open=yes",
		"--volume=80",
	}
	for k, v := range s.Headers {
		args = append(args, "--http-header-fields="+k+": "+v)
	}
	args = append(args, s.URL)

	cmd := exec.CommandContext(ctx, p.exePath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = os.Remove(ipcPath)
		return fmt.Errorf("启动 mpv 失败: %w", err)
	}

	conn, err := dialIPC(ipcPath)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.Remove(ipcPath)
		return fmt.Errorf("连接 mpv IPC 失败: %w", err)
	}

	p.cmd = cmd
	p.conn = conn
	p.ipcPath = ipcPath
	p.stateMu.Lock()
	p.state = player.State{Playing: player.StatePlaying, Duration: -1, Volume: 80}
	p.stateMu.Unlock()

	go p.readLoop()

	// 观察 time-pos，让位置事件（EventPosition）可用。观察失败不影响播放，
	// 故忽略错误。
	_ = p.send("observe_property", 0, "time-pos")
	return nil
}

func (p *mpvProc) Play() error  { return p.send("set_property", "pause", false) }
func (p *mpvProc) Pause() error { return p.send("set_property", "pause", true) }
func (p *mpvProc) Seek(sec float64) error {
	return p.send("seek", sec, "absolute")
}
func (p *mpvProc) SetVolume(v int) error {
	p.stateMu.Lock()
	p.state.Volume = v
	p.stateMu.Unlock()
	return p.send("set_property", "volume", v)
}
func (p *mpvProc) SelectTrack(kind player.TrackKind, id int) error {
	return errors.New("mpvproc: SelectTrack 未实现")
}
func (p *mpvProc) State() player.State {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.state
}
func (p *mpvProc) Events() <-chan player.Event { return p.events }

func (p *mpvProc) Close() error {
	if p.conn == nil {
		return nil
	}
	_ = p.conn.Close()
	p.conn = nil
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		p.cmd = nil
	}
	if p.ipcPath != "" {
		_ = os.Remove(p.ipcPath)
		p.ipcPath = ""
	}
	return nil
}

// send 串行发送一条命令并等待应答。readLoop 负责把命令应答路由到
// p.responses，异步事件路由到 p.events——两端互不干扰。
func (p *mpvProc) send(args ...any) error {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	if p.conn == nil {
		return errors.New("mpvproc: 尚未 Load")
	}
	if _, err := p.conn.Write([]byte(encodeCommand(args))); err != nil {
		return err
	}
	select {
	case line := <-p.responses:
		var resp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			return fmt.Errorf("解析 mpv 应答失败: %w", err)
		}
		if resp.Error != "" && resp.Error != "success" {
			return errors.New(resp.Error)
		}
		return nil
	case <-time.After(cmdResponseTimeout):
		return errors.New("mpvproc: 等待应答超时")
	}
}

// readLoop 是连接上唯一的读取者。mpv 连接建立后不会主动发握手行，命令
// 应答与异步事件在同一流上交错，必须由单一 reader 分路，否则多个
// bufio.Reader 会因各自预读而互相抢字节。
func (p *mpvProc) readLoop() {
	br := bufio.NewReader(p.conn)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var probe struct {
			Event string `json:"event"`
		}
		_ = json.Unmarshal(line, &probe)
		if probe.Event != "" {
			if evt, ok := parseEvent(line); ok {
				if evt.Kind == player.EventPosition {
					p.stateMu.Lock()
					p.state.Position = evt.Position
					p.stateMu.Unlock()
				}
				select {
				case p.events <- evt:
				default: // 事件通道满则丢弃，避免阻塞读循环
				}
			}
			continue
		}
		// 命令应答
		select {
		case p.responses <- line:
		default: // 无命令在等（理论上不应发生）
		}
	}
}

// dialIPC 以重试方式连接 mpv IPC socket：mpv 启动后创建 socket 有微小延迟。
func dialIPC(path string) (net.Conn, error) {
	deadline := time.Now().Add(ipcConnectTimeout)
	for {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}
