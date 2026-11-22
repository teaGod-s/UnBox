// Package mpvproc 通过 mpv 子进程 + JSON IPC 实现 player.Player。
//
// --input-ipc-server 在 Linux/macOS 用 Unix socket、Windows 用命名管道
// （见 ipc_unix.go / ipc_windows.go）。所有平台统一使用该外部进程后端。
package mpvproc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/unbox/unbox/internal/player"
)

// ipcConnectTimeout 是连接 mpv IPC socket 的最大等待时间（mpv 启动后创建
// socket 有微小延迟，需重试）。
const ipcConnectTimeout = 5 * time.Second

// cmdResponseTimeout 是单条命令等待 mpv 应答的超时。
// mpv 打开网络流时主循环可能阻塞（IPC 应答要等它松手），5s 对慢网络太紧；
// 摊回 15s，并靠 buildArgs 里的 --network-timeout=15 保证最坏等待有上限。
var cmdResponseTimeout = 15 * time.Second

// seekReadyTimeout 是首次 seek 等待 mpv 媒体就绪（file-loaded）的上限。
// mpv 在文件加载完成前执行 seek 会回 error running command（WSL/慢源常见）。
// 正常慢源会在该时间内发出 file-loaded；加载失败会经 loadFailed 更早打断，
// 这个兜底只防「既不成功也不报失败」的异常滞留。设到 mpv 的
// --network-timeout（15s）之上，保证能等到慢源的 file-loaded。
// 用 var 而非 const，便于测试缩短等待验证各分支。
var seekReadyTimeout = 20 * time.Second

// response 是 readLoop 路由给 send 的一条命令应答，携带会话代际，
// 供 send 丢弃跨会话串味的迟到应答。
type response struct {
	session int64
	data    []byte
}

type mpvProc struct {
	exePath string
	ipcPath string // --input-ipc-server 暴露的 IPC 路径（Unix socket 或 Windows 命名管道）

	// lifecycleMu 守护 cmd/conn/ipcPath/session：只在锁内做快照/赋值，
	// 绝不在持锁时阻塞（不做 IO、不等应答），且不与 sendMu 嵌套。
	lifecycleMu sync.Mutex
	session     int64 // 会话代际：Load 每次自增，用于丢弃跨会话串味的迟到应答
	cmd         *exec.Cmd
	wait        *waiter // 当前会话的收尸器，Close/超时/失败清理共用，避免 Wait 被调用两次
	conn        io.ReadWriteCloser
	wid         uintptr // 历史嵌入句柄，M4 默认不设置

	sendMu    sync.Mutex    // 串行化命令：保证任一时刻只有一条命令在飞
	responses chan response // readLoop 路由来的命令应答（带会话代际）
	events    chan player.Event

	// ready 是媒体就绪信号：mpv 加载完文件（收到 file-loaded 事件）后关闭，
	// 此后 seek 等依赖媒体的命令才被接受。由 lifecycleMu 守护：
	// Load 每建新会话就重建，旧会话迟到的 file-loaded 关的是旧的 channel。
	ready chan struct{}

	// loadFailed 是媒体加载失败信号：mpv 回 end-file(reason=error) 后关闭。
	// Seek 等就绪时若先收到失败，直接返回明确错误，不再发注定被拒的 seek；
	// 与 ready 同生命周期（每次 Load 重建）。
	loadFailed chan struct{}

	stateMu sync.Mutex
	state   player.State
	// paused 记录用户是否主动暂停：暂停期间屏蔽缓存状态事件，
	// 避免把缓存回填/见底误报成恢复播放或缓冲。
	paused bool
	// stderr 是当前会话 mpv 的 stderr 尾部（含解码/网络错误），命令应答超时
	// 时把它带进报错，否则这种失败只剩一句干巴巴的「命令应答超时」。
	// 由 lifecycleMu 守护：Load 会换会话、换 stderr，而并发的 send 正在读它。
	stderr *stderrTail
}

// New 以指定 mpv 可执行文件启动一个播放器实例。
//
// 本层负责 mpv 进程生命周期与 JSON IPC 对话；默认使用独立窗口。
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
	ipcPath, err := newIPCPath()
	if err != nil {
		return err
	}

	p.lifecycleMu.Lock()
	wid := p.wid
	p.lifecycleMu.Unlock()
	args := buildArgs(s, ipcPath, wid)

	// mpv 起不来时（缺 DLL、被杀软拦截、参数过旧）唯一的原因只出现在它的
	// stderr 上，丢弃它会让这类故障只剩一句「连接 IPC 失败」，无从定位。
	stderr := &stderrTail{max: mpvStderrLimit}

	cmd := exec.CommandContext(ctx, p.exePath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	setupProcAttr(cmd) // Windows：隐藏 mpv 子进程的控制台窗口
	if err := cmd.Start(); err != nil {
		cleanupIPC(ipcPath)
		return mpvLoadError(fmt.Errorf("启动 mpv 失败: %w", err), stderr.String())
	}
	p.lifecycleMu.Lock()
	p.stderr = stderr
	p.lifecycleMu.Unlock()

	w := startWaiter(cmd)

	conn, err := waitForIPC(dialIPC, ipcPath, ipcConnectTimeout, w)
	if err != nil {
		_ = cmd.Process.Kill()
		waitErr := w.wait() // 等收尸完成，此后 stderr 不再有写入
		cleanupIPC(ipcPath)
		if errors.Is(err, errMPVExitedEarly) {
			// 附上退出码：静默秒退时它是唯一能区分缺 DLL / 崩溃 / 架构不匹配的证据。
			err = mpvExitError(waitErr)
		}
		return mpvLoadError(err, stderr.String())
	}

	p.lifecycleMu.Lock()
	p.session++
	sess := p.session
	p.cmd = cmd
	p.wait = w
	p.conn = conn
	p.ipcPath = ipcPath
	// 新会话的媒体就绪信号从零开始：旧会话若已 file-loaded，其 ready 已关闭，
	// 不能让新会话直接跳过等待（新 URL 还没加载完成时 seek 会被 mpv 拒绝）。
	p.ready = make(chan struct{})
	ready := p.ready
	p.loadFailed = make(chan struct{})
	loadFailed := p.loadFailed
	p.lifecycleMu.Unlock()
	p.stateMu.Lock()
	p.state = player.State{Playing: player.StatePlaying, Duration: -1, Volume: 80}
	p.paused = false
	p.stateMu.Unlock()

	go p.readLoop(conn, sess, ready, loadFailed)

	// 观察位置、缓存暂停状态与用户暂停。观察失败不影响播放，故忽略错误。
	// pause 只用来屏蔽暂停期间的缓存事件，不会自己映射成缓冲信号。
	_ = p.send("observe_property", 0, "time-pos")
	_ = p.send("observe_property", 1, "paused-for-cache")
	_ = p.send("observe_property", 2, "pause")
	return nil
}

// SetEmbedWindow 设置 mpv 嵌入的宿主窗口句柄（X11 XID / Windows HWND）。
// 为 0 表示不嵌入，Load 时回退为 --force-window 独立窗口。
func (p *mpvProc) SetEmbedWindow(id uintptr) {
	p.lifecycleMu.Lock()
	p.wid = id
	p.lifecycleMu.Unlock()
}

// envMpvSoft 强制/禁用软件视频输出（1/0），留给现场排查用。
const envMpvSoft = "UNBOX_MPV_SOFT"

// softVideoArgs 是拿不到 GPU 时的兜底视频输出：纯软件 X11，不碰 EGL/GL。
// 这类环境（WSLg 默认不给 /dev/dri/renderD* 权限、容器里没有节点）里
// mpv 的 EGL/Zink 初始化会失败，接着 x11 vo 二次初始化直接 assert 崩掉：
//
//	mpv: ../video/out/x11_common.c:679: vo_x11_init: Assertion `!vo->x11' failed.
var softVideoArgs = []string{"--vo=x11", "--hwdec=no"}

// mpvVideoArgs 返回与视频输出相关的附加参数；测试可替换。
var mpvVideoArgs = videoOutputArgs

// videoOutputArgs 按环境选择视频输出参数：能打开 DRM render 节点就走默认
// GPU 路径，打不开就退软件输出。UNBOX_MPV_SOFT=1/0 可强制覆盖。
func videoOutputArgs() []string {
	switch os.Getenv(envMpvSoft) {
	case "1":
		return softVideoArgs
	case "0":
		return nil
	}
	if runtime.GOOS != "linux" {
		return nil
	}
	if renderNodeUsable() {
		return nil
	}
	return softVideoArgs
}

// renderNodeUsable 判断当前用户能否打开 DRM render 节点。
// 只看节点是否存在不够：WSLg 里 renderD128 是 root:render 0660，
// 普通用户不在 render 组时打开是 EACCES，EGL 一样起不来。
func renderNodeUsable() bool {
	nodes, err := filepath.Glob("/dev/dri/renderD*")
	if err != nil {
		return false
	}
	for _, node := range nodes {
		f, err := os.Open(node)
		if err == nil {
			_ = f.Close()
			return true
		}
	}
	return false
}

// buildArgs 构造 mpv 启动参数。wid != 0 时嵌入宿主窗口并开 OSC；
// wid == 0 时独立开窗，但仍开启 OSC，让 mpv 窗口提供原生播放控件。
func buildArgs(s player.Stream, ipcPath string, wid uintptr) []string {
	args := []string{
		"--idle=yes",
		"--input-ipc-server=" + ipcPath,
		"--keep-open=yes",
		"--volume=80",
		// 网络操作逐个限时：mpv 开流失败时主循环会阻塞在 TCP 连不上/慢读上，
		// IPC 命令得不到应答，表现成「命令应答超时」。给个上限让失败尽快落定。
		"--network-timeout=15",
	}
	// 视频输出参数（无 GPU 时是 --vo=x11 软件输出），见 videoOutputArgs。
	args = append(args, mpvVideoArgs()...)
	if wid != 0 {
		args = append(args, "--wid="+strconv.FormatUint(uint64(wid), 10), "--osc=yes")
	} else {
		args = append(args, "--force-window=yes", "--osc=yes")
	}
	for k, v := range s.Headers {
		args = append(args, "--http-header-fields="+k+": "+v)
	}
	args = append(args, s.URL)
	return args
}

func (p *mpvProc) Play() error  { return p.send("set_property", "pause", false) }
func (p *mpvProc) Pause() error { return p.send("set_property", "pause", true) }

// Seek 跳转到指定秒。首次 seek（断点续播、跳片头片尾）在 mpv 媒体未就绪时
// 会被拒（error running command），这里先等媒体加载结果：
//   - file-loaded（成功）→ 立即发 seek，位置正确恢复；
//   - end-file(reason=error)（媒体加载失败）→ 返回明确错误，
//     不再发一条注定被拒的 seek；
//   - seekReadyTimeout 兜底（异常滞留）→ 照发，交由 mpv 应答如实反馈。
func (p *mpvProc) Seek(sec float64) error {
	if err := p.waitMediaReady(); err != nil {
		return err
	}
	return p.send("seek", sec, "absolute")
}

// waitMediaReady 等本会话 mpv 媒体加载结果。无会话（未 Load）时放行，由
// send 报「尚未 Load」；已就绪立即返回 nil；加载失败返回加载错误。
func (p *mpvProc) waitMediaReady() error {
	p.lifecycleMu.Lock()
	ready := p.ready
	loadFailed := p.loadFailed
	p.lifecycleMu.Unlock()
	if ready == nil {
		return nil
	}

	// 已就绪优先：ready 一旦关闭就永远视为就绪，即使之后播放中途再出现
	// end-file(error)（loadFailed 关闭）也不影响此刻放行 seek。
	select {
	case <-ready:
		return nil
	default:
	}

	select {
	case <-ready:
		return nil
	case <-loadFailed:
		return errors.New("mpv 媒体加载失败，无法定位续播位置")
	case <-time.After(seekReadyTimeout):
		return nil
	}
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
	w := p.wait
	ipcPath := p.ipcPath
	p.conn = nil
	p.cmd = nil
	p.wait = nil
	p.ipcPath = ""
	p.lifecycleMu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	// Kill 之后 Wait 立即返回；w 为 nil（本就无会话）时是空操作。
	_ = w.wait()
	if ipcPath != "" {
		cleanupIPC(ipcPath)
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
			stderr := p.stderr // 会话可能在 Load 里被换掉，锁内取快照
			var conn io.ReadWriteCloser
			var cmd *exec.Cmd
			var w *waiter
			var ipcPath string
			if same {
				conn = p.conn
				cmd = p.cmd
				w = p.wait
				ipcPath = p.ipcPath
				p.conn = nil
				p.cmd = nil
				p.wait = nil
				p.ipcPath = ""
			}
			p.lifecycleMu.Unlock()

			if same {
				if conn != nil {
					_ = conn.Close()
				}
				if cmd != nil && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				_ = w.wait()
				if ipcPath != "" {
					cleanupIPC(ipcPath)
				}
				p.stateMu.Lock()
				p.state.Playing = player.StateStopped
				p.stateMu.Unlock()
			}
			err := errors.New("mpvproc: 命令应答超时")
			if stderr != nil {
				// 附上 mpv 自己吐的 stderr 尾部：网络失败的原因基本都在里面。
				err = mpvLoadError(err, stderr.String())
			}
			return err
		}
	}
}

// readLoop 是连接上唯一的读取者。mpv 连接建立后不会主动发握手行，命令
// 应答与异步事件在同一流上交错，必须由单一 reader 分路，否则多个带预读
// 的 reader（bufio.Reader/Scanner）会互相抢字节。
// conn/sess/ready/loadFailed 由 Load 传入并固定：每条命令应答都标上本会话代际，
// 媒体就绪/失败事件只关本会话的对应 channel（避免误关新会话）。
func (p *mpvProc) readLoop(conn io.ReadWriteCloser, sess int64, ready, loadFailed chan struct{}) {
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
			// 媒体加载完成：seek 等依赖媒体的命令从此可用。mpv 在 seek/切流
			// 后可能重发 file-loaded，故同一 channel 只关一次。
			if probe.Event == "file-loaded" {
				select {
				case <-ready:
				default:
					close(ready)
				}
			}
			// 媒体加载失败（end-file + error 原因）：seek 等就绪的调用方
			// 据此返回明确错误，而不是等超时后发一条注定被拒的 seek。
			if probe.Event == "end-file" {
				var ef struct {
					Reason string `json:"reason"`
				}
				_ = json.Unmarshal(line, &ef)
				if ef.Reason == "error" {
					select {
					case <-loadFailed:
					default:
						close(loadFailed)
					}
				}
			}
			if paused, ok := parsePauseProperty(line); ok {
				// 用户主动暂停：只更新状态，不产生任何播放信号。
				p.stateMu.Lock()
				p.paused = paused
				if paused {
					p.state.Playing = player.StatePaused
				} else if p.state.Playing == player.StatePaused {
					p.state.Playing = player.StatePlaying
				}
				p.stateMu.Unlock()
				continue
			}
			if evt, ok := parseEvent(line); ok {
				p.stateMu.Lock()
				emit := true
				switch evt.Kind {
				case player.EventPosition:
					p.state.Position = evt.Position
				case player.EventBuffering:
					// 暂停期间的缓存状态变化不对外播报，否则会被前端当成
					// 「未起播/持续缓冲」而触发自动换源。
					if p.paused {
						emit = false
					} else {
						p.state.Playing = player.StateBuffering
					}
				case player.EventPlaying:
					if p.paused {
						emit = false
					} else {
						p.state.Playing = player.StatePlaying
					}
				case player.EventEOF, player.EventError:
					p.state.Playing = player.StateStopped
				}
				p.stateMu.Unlock()
				if emit {
					sendEvent(p.events, evt)
				}
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
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("mpvproc: 读取 mpv IPC 中断: %v", err)
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
