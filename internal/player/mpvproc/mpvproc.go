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
	"strconv"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// ipcConnectTimeout 是连接 mpv IPC socket 的最大等待时间（mpv 启动后创建
// socket 有微小延迟，需重试）。
const ipcConnectTimeout = 5 * time.Second

// cmdResponseTimeout 是单条命令等待 mpv 应答的超时。
const cmdResponseTimeout = 5 * time.Second

// response 是 readLoop 路由给 send 的一条命令应答，携带会话代际，
// 供 send 丢弃跨会话串味的迟到应答。
type response struct {
	session int64
	data    []byte
}

type mpvProc struct {
	exePath string
	ipcPath string // --input-ipc-server 暴露的 Unix socket 路径

	// lifecycleMu 守护 cmd/conn/ipcPath/session/wid：只在锁内做快照/赋值，
	// 绝不在持锁时阻塞（不做 IO、不等应答），且不与 sendMu 嵌套。
	lifecycleMu sync.Mutex
	session     int64 // 会话代际：Load 每次自增，用于丢弃跨会话串味的迟到应答
	cmd         *exec.Cmd
	conn        net.Conn
	wid         uintptr // 嵌入宿主窗口句柄（0 表示不嵌入，独立开窗）

	sendMu    sync.Mutex    // 串行化命令：保证任一时刻只有一条命令在飞
	responses chan response // readLoop 路由来的命令应答（带会话代际）
	events    chan player.Event

	stateMu sync.Mutex
	state   player.State
}

// New 以指定 mpv 可执行文件启动一个播放器实例。
//
// 本层负责 mpv 进程生命周期与 JSON IPC 对话；把视频嵌入宿主窗口可通过
// SetEmbedWindow 传入 --wid=<窗口句柄>，未设置时用 --force-window 独立开窗。
func New(exePath string) (player.Player, error) {
	if _, err := os.Stat(exePath); err != nil {
		return nil, fmt.Errorf("mpv 可执行文件不可用: %w", err)
	}
	return &mpvProc{
		exePath:   exePath,
		responses: make(chan response, 16),
		events:    make(chan player.Event, 64),
		state:     player.State{Playing: player.StateStopped, Duration: -1},
	}, nil
}

func (p *mpvProc) Load(ctx context.Context, s player.Stream) error {
	// 关闭旧会话（Close 幂等：无会话时直接返回 nil）。
	_ = p.Close()
	sock, err := os.CreateTemp("", "unbox-mpv-*.sock")
	if err != nil {
		return err
	}
	ipcPath := sock.Name()
	_ = sock.Close()
	_ = os.Remove(ipcPath)

	p.lifecycleMu.Lock()
	wid := p.wid
	p.lifecycleMu.Unlock()
	args := buildArgs(s, ipcPath, wid)

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
	p.session++
	sess := p.session
	p.cmd = cmd
	p.conn = conn
	p.ipcPath = ipcPath
	p.lifecycleMu.Unlock()
	p.stateMu.Lock()
	p.state = player.State{Playing: player.StatePlaying, Duration: -1, Volume: 80}
	p.stateMu.Unlock()

	go p.readLoop(conn, sess)

	// 观察 time-pos，让位置事件（EventPosition）可用。观察失败不影响播放，
	// 故忽略错误。
	_ = p.send("observe_property", 0, "time-pos")
	return nil
}

// SetEmbedWindow 设置 mpv 嵌入的宿主窗口句柄（X11 XID / Windows HWND）。
// 为 0 表示不嵌入，Load 时回退为 --force-window 独立窗口。
func (p *mpvProc) SetEmbedWindow(id uintptr) {
	p.lifecycleMu.Lock()
	p.wid = id
	p.lifecycleMu.Unlock()
}

// buildArgs 构造 mpv 启动参数。wid != 0 时嵌入宿主窗口并开 OSC；
// wid == 0 时独立开窗并关 OSC（前端控制）。
func buildArgs(s player.Stream, ipcPath string, wid uintptr) []string {
	args := []string{
		"--idle=yes",
		"--input-ipc-server=" + ipcPath,
		"--keep-open=yes",
		"--volume=80",
	}
	if wid != 0 {
		args = append(args, "--wid="+strconv.FormatUint(uint64(wid), 10), "--osc=yes")
	} else {
		args = append(args, "--force-window=yes", "--osc=no")
	}
	for k, v := range s.Headers {
		args = append(args, "--http-header-fields="+k+": "+v)
	}
	args = append(args, s.URL)
	return args
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

	// 快照当前 conn 与会话代际；并发 Close 只关掉旧 conn，此处 Write 返回
	// error 而非 nil-deref。不持锁等应答。
	p.lifecycleMu.Lock()
	c := p.conn
	sess := p.session
	p.lifecycleMu.Unlock()
	if c == nil {
		return errors.New("mpvproc: 尚未 Load")
	}
	if _, err := c.Write([]byte(encodeCommand(args))); err != nil {
		return err
	}

	timer := time.NewTimer(cmdResponseTimeout)
	defer timer.Stop()
	for {
		select {
		case r := <-p.responses:
			if r.session != sess {
				continue // 跨会话串味的迟到应答，丢弃继续等
			}
			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(r.data, &resp); err != nil {
				return fmt.Errorf("解析 mpv 应答失败: %w", err)
			}
			if resp.Error != "" && resp.Error != "success" {
				return errors.New(resp.Error)
			}
			return nil
		case <-timer.C:
			// 超时视为本会话连接已坏。仅在 p.conn 仍是自己写入的那条（指针同一）
			// 时才拆解，否则说明 Load 已重入建立新会话，不得误杀后继。
			p.lifecycleMu.Lock()
			same := p.conn == c
			var conn net.Conn
			var cmd *exec.Cmd
			var ipcPath string
			if same {
				conn = p.conn
				cmd = p.cmd
				ipcPath = p.ipcPath
				p.conn = nil
				p.cmd = nil
				p.ipcPath = ""
			}
			p.lifecycleMu.Unlock()

			if same {
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
				p.stateMu.Lock()
				p.state.Playing = player.StateStopped
				p.stateMu.Unlock()
			}
			return errors.New("mpvproc: 命令应答超时")
		}
	}
}

// readLoop 是连接上唯一的读取者。mpv 连接建立后不会主动发握手行，命令
// 应答与异步事件在同一流上交错，必须由单一 reader 分路，否则多个带预读
// 的 reader（bufio.Reader/Scanner）会互相抢字节。
// conn/sess 由 Load 传入并固定，保证每条命令应答都标上本会话代际。
func (p *mpvProc) readLoop(conn net.Conn, sess int64) {
	sc := bufio.NewScanner(conn)
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
				sendEvent(p.events, evt)
			}
			continue
		}
		// 命令应答。Scanner 复用内部缓冲，必须先拷贝再入 channel，否则下一次
		// Scan 覆盖底层数组后，等待应答的 send 会读到脏数据。
		reply := append([]byte(nil), line...)
		select {
		case p.responses <- response{session: sess, data: reply}:
		default: // 无命令在等（理论上不应发生）
		}
	}
}

// sendEvent 把事件发往通道：终端事件（EOF/Error）阻塞发送保证送达
// （失败自动切换依赖它们），其余事件通道满则丢弃以避免阻塞读循环。
func sendEvent(ch chan<- player.Event, evt player.Event) {
	switch evt.Kind {
	case player.EventEOF, player.EventError:
		ch <- evt
	default:
		select {
		case ch <- evt:
		default:
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
