// Package mpvproc 通过 mpv 子进程 + JSON IPC 实现 player.Player。
//
// 当前 --input-ipc-server 仅实现 Unix socket（Linux/macOS）；Windows 需改用
// named pipe，留待后续任务。macOS 播放另有 libmpv 后端，见 ../mpvlib。
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

	// lifecycleMu 守护 cmd/conn/ipcPath 三个字段：只在锁内做快照/赋值，
	// 绝不在持锁时阻塞（不做 IO、不等应答），且不与 sendMu 嵌套。
	lifecycleMu sync.Mutex
	cmd         *exec.Cmd
	conn        net.Conn

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
	// 关闭旧会话（Close 幂等：无会话时直接返回 nil）。
	_ = p.Close()
	// 新会话从空 responses 开始，避免跨会话串味。
	p.responses = make(chan []byte, 16)
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

	p.lifecycleMu.Lock()
	p.cmd = cmd
	p.conn = conn
	p.ipcPath = ipcPath
	p.lifecycleMu.Unlock()
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
	if err := p.send("set_property", "volume", v); err != nil {
		return err
	}
	p.stateMu.Lock()
	p.state.Volume = v
	p.stateMu.Unlock()
	return nil
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
	// 在 lifecycleMu 下快照并置空，锁外再执行 IO，保证幂等且不持锁阻塞。
	p.lifecycleMu.Lock()
	conn := p.conn
	cmd := p.cmd
	ipcPath := p.ipcPath
	p.conn = nil
	p.cmd = nil
	p.ipcPath = ""
	p.lifecycleMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	if ipcPath != "" {
		_ = os.Remove(ipcPath)
	}
	return nil
}

// send 串行发送一条命令并等待应答。readLoop 负责把命令应答路由到
// p.responses，异步事件路由到 p.events——两端互不干扰。
func (p *mpvProc) send(args ...any) error {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()

	// 快照当前 conn 到局部变量；并发 Close 只关掉旧 conn，此处 Write 返回
	// error 而非 nil-deref。不持锁等应答。
	p.lifecycleMu.Lock()
	c := p.conn
	p.lifecycleMu.Unlock()
	if c == nil {
		return errors.New("mpvproc: 尚未 Load")
	}
	if _, err := c.Write([]byte(encodeCommand(args))); err != nil {
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
		// 超时视为连接已坏：关掉并置空，令 readLoop 退出、阻断迟到应答进入。
		p.lifecycleMu.Lock()
		if p.conn != nil {
			_ = p.conn.Close()
			p.conn = nil
		}
		p.lifecycleMu.Unlock()
		return errors.New("mpvproc: 命令应答超时")
	}
}

// readLoop 是连接上唯一的读取者。mpv 连接建立后不会主动发握手行，命令
// 应答与异步事件在同一流上交错，必须由单一 reader 分路，否则多个带预读
// 的 reader（bufio.Reader/Scanner）会互相抢字节。
func (p *mpvProc) readLoop() {
	sc := bufio.NewScanner(p.conn)
	// 单行可能较长（如音轨/字幕列表），给足缓冲：起始 64KB、上限 1MB，
	// 避免 ReadBytes 默认 4096B 缓冲触顶误杀读循环。
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
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
		// 命令应答。Scanner 复用内部缓冲，必须先拷贝再入 channel，否则下一次
		// Scan 覆盖底层数组后，等待应答的 send 会读到脏数据。
		reply := append([]byte(nil), line...)
		select {
		case p.responses <- reply:
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
