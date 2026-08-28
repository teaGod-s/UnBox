# Unbox M4 Web Playback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement M4's built-in Web playback path, authenticated local stream proxy, share-page resolution, automatic mpv fallback, and cross-platform mpv plugin installation UX.

**Architecture:** A Wails-independent `internal/playback` controller resolves indirect URLs, chooses Web or mpv, owns short-lived playback sessions, and lazily starts a loopback proxy. `internal/player/mpvplugin` discovers or installs external mpv; `ShellService` exposes typed playback plans to Vue, where a focused `PlaybackView` selects hls.js, mpegts.js, or native `<video>` and requests mpv fallback after fatal Web errors.

**Tech Stack:** Go 1.26.3, Wails 3.0.0-beta.9, Vue 3, TypeScript 4.9, hls.js 1.7.1, mpegts.js 1.8.2, Vitest 4.1.11, Vue Test Utils 2.5.0, jsdom 30.0.1.

**Spec:** `docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`

## Global Constraints

- Wails code remains limited to `internal/shell/`, `cmd/unbox/`, and `frontend/`.
- `internal/playback` and `internal/player/mpvplugin` must not import Wails.
- Web playback supports HLS, HTTP-FLV, HTTP-TS, and MP4 without external executables.
- RTMP, local files, and positively identified HEVC require mpv; unknown HLS codecs try Web first and may fall back to mpv.
- Proxy listeners bind only to `127.0.0.1`; proxy tokens are cryptographically random and expire from memory.
- Windows mpv is pinned to `mpv-setup-x86_64-0.41.0.exe` with SHA-256 `1b32d5eb7e713ecc5853c18107daffac652e29474dfd517a4ddb792dc45e40fc`.
- Linux/macOS never run privilege-escalation commands; the UI displays a command for the user to execute.
- Public errors and new comments are Chinese.
- Every behavior change begins with a failing test.

---

### Task 1: Playback Contracts and Indirect URL Resolution

**Files:**
- Modify: `internal/player/player.go`
- Modify: `internal/provider/live/live.go`
- Modify: `internal/provider/tvbox/tvbox.go`
- Create: `internal/playback/types.go`
- Create: `internal/playback/resolver.go`
- Create: `internal/playback/resolver_test.go`
- Test: `internal/player/player_test.go`
- Test: `internal/provider/tvbox/tvbox_test.go`

**Interfaces:**
- Consumes: `player.Stream{URL string, Headers map[string]string, Kind player.StreamKind}`.
- Produces: `player.StreamTS`; `playback.Plan`; `Resolver.Resolve(context.Context, player.Stream) (player.Stream, error)`.

- [ ] **Step 1: Write failing kind and resolver tests**

```go
func TestResolveSharePage(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html")
        _, _ = io.WriteString(w, `<script>const url="/media/index.m3u8?sign=a%2Bb";</script>`)
    }))
    defer srv.Close()

    got, err := NewResolver(srv.Client()).Resolve(context.Background(), player.Stream{
        URL: srv.URL + "/share/1", Headers: map[string]string{"Referer": srv.URL},
    })
    if err != nil || got.URL != srv.URL+"/media/index.m3u8?sign=a%2Bb" || got.Kind != player.StreamHLS {
        t.Fatalf("Resolve() = %+v, %v", got, err)
    }
}
```

Cover absolute URLs, escaped JavaScript strings, non-HTML passthrough, response-size limits, non-2xx responses, and `.ts` classification.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/playback ./internal/player ./internal/provider/tvbox -count=1`

Expected: FAIL because `internal/playback`, `StreamTS`, and `Resolver` do not exist.

- [ ] **Step 3: Add contracts and minimal resolver**

```go
type Backend string

const (
    BackendWeb Backend = "web"
    BackendMPV Backend = "mpv"
)

type Plan struct {
    ID          string
    Backend     Backend
    URL         string
    Kind        string
    CanFallback bool
}

type Resolver struct {
    client   *http.Client
    maxBytes int64
}

func (r *Resolver) Resolve(ctx context.Context, stream player.Stream) (player.Stream, error)
```

The resolver sends stream headers, checks `Content-Type`, reads at most 1 MiB only for HTML, extracts `const url = ...`, resolves relative URLs against the final response URL, and recomputes `StreamKind` without dropping headers, subtitles, or backups.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./internal/playback ./internal/player ./internal/provider/tvbox -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/player internal/provider/live internal/provider/tvbox internal/playback
git commit -m "feat(playback): resolve indirect streams"
```

---

### Task 2: Loopback Proxy and HLS Rewriting

**Files:**
- Create: `internal/playback/proxy.go`
- Create: `internal/playback/proxy_test.go`

**Interfaces:**
- Consumes: resolved `player.Stream`.
- Produces: `Proxy.Register(context.Context, player.Stream) (string, error)` and `Proxy.Close() error`.

- [ ] **Step 1: Write failing proxy tests**

```go
func TestProxyInjectsHeadersAndRewritesHLS(t *testing.T) {
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Referer") != "https://origin.example/" || r.Header.Get("User-Agent") != "Unbox-Test" {
            http.Error(w, "missing headers", http.StatusForbidden)
            return
        }
        _, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\nsegment.ts?sig=1\n")
    }))
    defer upstream.Close()

    proxy := NewProxy(upstream.Client(), time.Minute)
    defer proxy.Close()
    proxyURL, err := proxy.Register(context.Background(), player.Stream{
        URL: upstream.URL + "/live/index.m3u8",
        Kind: player.StreamHLS,
        Headers: map[string]string{"Referer": "https://origin.example/", "User-Agent": "Unbox-Test"},
    })
    // GET proxyURL and assert both key.bin and segment.ts are rewritten through the same token.
}
```

Also cover master-playlist child manifests, `EXT-X-MAP`, `EXT-X-MEDIA`, Range forwarding, upstream status/content headers, CORS, expired/unknown tokens, redirect header preservation, and traversal-safe query decoding.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/playback -run 'TestProxy' -count=1`

Expected: FAIL because `NewProxy` does not exist.

- [ ] **Step 3: Implement the proxy**

```go
type Proxy struct {
    client   *http.Client
    ttl      time.Duration
    mu       sync.RWMutex
    streams  map[string]proxyEntry
    listener net.Listener
    server   *http.Server
}

type proxyEntry struct {
    stream    player.Stream
    expiresAt time.Time
}
```

Generate 32 random bytes per token, start `net.Listen("tcp4", "127.0.0.1:0")` lazily, accept only URLs derived from the registered manifest, forward `GET`/`HEAD` and `Range`, strip hop-by-hop response headers, set `Access-Control-Allow-Origin: *`, and rewrite URI lines plus quoted `URI=` attributes with `net/url` rather than string concatenation.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./internal/playback -run 'TestProxy' -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/playback/proxy.go internal/playback/proxy_test.go
git commit -m "feat(playback): add authenticated stream proxy"
```

---

### Task 3: Routing, Sessions, and Automatic mpv Fallback

**Files:**
- Create: `internal/playback/controller.go`
- Create: `internal/playback/controller_test.go`
- Modify: `internal/player/failover/failover.go`
- Modify: `internal/player/failover/failover_test.go`

**Interfaces:**
- Consumes: resolver, proxy, and an optional `player.Player`.
- Produces: `Controller.Prepare`, `Controller.Fallback`, `Controller.SetMPV`, `Controller.MPVReady`, and `Controller.Close`.

- [ ] **Step 1: Write failing routing tests**

```go
func TestControllerRoutesStreams(t *testing.T) {
    cases := []struct {
        name string
        kind player.StreamKind
        manifest string
        haveMPV bool
        want Backend
        wantErr bool
    }{
        {"h264 hls", player.StreamHLS, `#EXT-X-STREAM-INF:CODECS="avc1.640028,mp4a.40.2"`, false, BackendWeb, false},
        {"unknown hls", player.StreamHLS, `#EXTM3U\n#EXTINF:4`, true, BackendWeb, false},
        {"hevc hls", player.StreamHLS, `#EXT-X-STREAM-INF:CODECS="hvc1.1.6.L120"`, true, BackendMPV, false},
        {"hevc without mpv", player.StreamHLS, `#EXT-X-STREAM-INF:CODECS="hev1.1.6.L120"`, false, "", true},
        {"rtmp", player.StreamRTMP, "", false, "", true},
        {"flv", player.StreamFLV, "", false, BackendWeb, false},
    }
    // Build a controller with fake resolver/proxy/player and assert each result.
}
```

Test that fatal Web fallback loads and plays the original resolved stream in mpv, stale session IDs fail, replacing mpv closes the old player, and `Close` releases proxy and player resources.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/playback ./internal/player/failover -count=1`

- [ ] **Step 3: Implement controller and failover-safe replacement**

```go
type Controller struct {
    resolver streamResolver
    proxy    streamProxy
    mu       sync.RWMutex
    mpv      player.Player
    sessions map[string]player.Stream
}

func (c *Controller) Prepare(ctx context.Context, stream player.Stream) (Plan, error)
func (c *Controller) Fallback(ctx context.Context, id string) (Plan, error)
func (c *Controller) SetMPV(next player.Player) error
func (c *Controller) MPVReady() bool
func (c *Controller) Close() error
```

For HLS, inspect only the first manifest up to 1 MiB using the stream headers. Treat `hvc1`, `hev1`, and `hevc` as positive HEVC; treat missing `CODECS` as unknown and route Web first. A Web plan retains the resolved stream under its random session ID so `Fallback` can hand the exact stream to mpv.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./internal/playback ./internal/player/failover -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/playback internal/player/failover
git commit -m "feat(playback): route web streams with mpv fallback"
```

---

### Task 4: External mpv Plugin Discovery and Installation

**Files:**
- Create: `internal/player/mpvplugin/manager.go`
- Create: `internal/player/mpvplugin/manager_test.go`
- Create: `internal/player/mpvplugin/install_unix.go`
- Create: `internal/player/mpvplugin/install_unix_test.go`
- Create: `internal/player/mpvplugin/install_windows.go`
- Create: `internal/player/mpvplugin/install_other.go`
- Modify: `internal/shell/pick.go`
- Modify: `internal/shell/pick_test.go`

**Interfaces:**
- Consumes: OS name, user config directory, executable lookup, HTTP client, and command runner.
- Produces: `Manager.Status()`, `Manager.Install(context.Context)`, `Manager.NewPlayer()`, and typed `Status`/`InstallResult` values.

- [ ] **Step 1: Write failing manager tests**

```go
func TestStatusPrefersPluginExecutable(t *testing.T) {
    dir := t.TempDir()
    pluginExe := filepath.Join(dir, "mpv.exe")
    if err := os.WriteFile(pluginExe, []byte("fake"), 0o755); err != nil { t.Fatal(err) }
    m := newManagerForTest("windows", dir, func(string) (string, error) {
        return `C:\\system\\mpv.exe`, nil
    })
    got := m.Status()
    if !got.Available || got.Path != pluginExe { t.Fatalf("Status() = %+v", got) }
}
```

Cover PATH fallback, apt/dnf/pacman command selection, Homebrew selection, unsupported Unix guidance, download size limit, SHA mismatch cleanup, HTTP failure, silent installer arguments, and rediscovery after install.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/player/mpvplugin ./internal/shell -count=1`

- [ ] **Step 3: Implement discovery and platform actions**

```go
type Status struct {
    Available      bool
    Path           string
    InstallMode    string // "command" | "download" | "manual"
    InstallCommand string
}

type InstallResult struct {
    Installed bool
    Message   string
}
```

Linux/macOS `Install` returns guidance without executing it. Windows downloads the pinned x86_64 installer, validates SHA-256 before execution, and invokes:

```text
/VERYSILENT /CURRENTUSER /SUPPRESSMSGBOXES /NORESTART /DIR=<config>/unbox/plugins/mpv
```

`Manager.NewPlayer` constructs `mpvproc.New(status.Path)`. `shell.PickPlayer` delegates to the manager on every platform and no longer imports `mpvlib`.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `go test ./internal/player/mpvplugin ./internal/shell -count=1`

- [ ] **Step 5: Commit**

```bash
git add internal/player/mpvplugin internal/shell/pick.go internal/shell/pick_test.go
git commit -m "feat(player): manage external mpv plugin"
```

---

### Task 5: Shell API Integration and Legacy mpvlib Removal

**Files:**
- Modify: `internal/shell/app.go`
- Modify: `internal/shell/app_test.go`
- Modify: `internal/shell/service.go`
- Modify: `internal/shell/service_test.go`
- Modify: `cmd/unbox/main.go`
- Delete: `internal/player/mpvlib/mpvlib_darwin.go`
- Delete: `internal/player/mpvlib/mpvlib_stub.go`
- Delete: `internal/shell/embed_linux.go`
- Delete: `internal/shell/embed_other.go`
- Delete: `internal/shell/embed_windows.go`
- Delete: `internal/shell/embed_test.go`

**Interfaces:**
- Consumes: `playback.Controller` and `mpvplugin.Manager`.
- Produces: Wails methods returning `playback.Plan`, `mpvplugin.Status`, and `mpvplugin.InstallResult`.

- [ ] **Step 1: Write failing shell tests**

```go
func TestPlayVodReturnsWebPlanWithoutMPV(t *testing.T) {
    svc := newPlaybackTestService(t, nil)
    svc.vods["s1"] = &stubProvider{key: "s1"}
    plan, err := svc.PlayVod("s1", "1/0/0")
    if err != nil || plan.Backend != playback.BackendWeb || plan.URL == "" {
        t.Fatalf("PlayVod() = %+v, %v", plan, err)
    }
}
```

Cover live and VOD plans, `FallbackToMPV`, plugin status, command-mode install, Windows-install refresh through injected manager, Web-mode pause/volume rejection, and app shutdown cleanup.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/shell ./cmd/unbox -count=1`

- [ ] **Step 3: Wire the controller into ShellService**

```go
func (s *ShellService) PlayChannel(id string) (playback.Plan, error)
func (s *ShellService) PlayVod(site, epID string) (playback.Plan, error)
func (s *ShellService) FallbackToMPV(id string) (playback.Plan, error)
func (s *ShellService) MPVStatus() mpvplugin.Status
func (s *ShellService) InstallMPV() (mpvplugin.InstallResult, error)
func (s *ShellService) RefreshMPV() (mpvplugin.Status, error)
```

Keep `NewShellService(provider.Provider, player.Player, *store.Store)` as a test-compatible convenience constructor and add an internal constructor accepting controller/manager. Main creates the manager, optionally wraps its player in failover, and starts successfully when mpv is absent. Remove native handle polling and all mpvlib/embed files.

- [ ] **Step 4: Generate bindings and verify tests**

Run: `GOCACHE=/tmp/unbox-m4-go-cache mise exec -- wails3 generate bindings -ts ./cmd/unbox`

Run: `go test ./internal/shell ./cmd/unbox -count=1`

- [ ] **Step 5: Commit**

```bash
git add cmd/unbox internal/player internal/shell
git commit -m "feat(shell): expose web playback plans"
```

---

### Task 6: Vue Web Player and mpv Plugin UX

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `frontend/vite.config.ts`
- Create: `frontend/src/playback.ts`
- Create: `frontend/src/components/PlaybackView.vue`
- Create: `frontend/src/components/PlaybackView.test.ts`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/public/style.css`

**Interfaces:**
- Consumes: generated `playback.Plan`, `mpvplugin.Status`, `ShellService.FallbackToMPV`, `InstallMPV`, and `RefreshMPV` bindings.
- Produces: one stable video surface with Web/native controls and a compact plugin action area.

- [ ] **Step 1: Install pinned dependencies and test harness**

Run:

```bash
cd frontend
npm install --save-exact hls.js@1.7.1 mpegts.js@1.8.2
npm install --save-dev --save-exact vitest@4.1.11 @vue/test-utils@2.5.0 jsdom@30.0.1
```

Add `"test": "vitest run"` and configure `test.environment = "jsdom"`.

- [ ] **Step 2: Write failing component tests**

```ts
it('uses hls.js for an HLS web plan and requests fallback on fatal error', async () => {
  const wrapper = mount(PlaybackView, {
    props: { plan: { ID: 'p1', Backend: 'web', URL: 'http://127.0.0.1/x', Kind: 'hls', CanFallback: true } },
  })
  expect(hlsMock.loadSource).toHaveBeenCalledWith('http://127.0.0.1/x')
  fatalHlsError()
  expect(wrapper.emitted('fallback')).toEqual([['p1']])
})
```

Cover MPEG-TS/FLV selection, MP4 native source, cleanup on plan changes/unmount, no repeated fallback, mpv mode messaging, play/pause/volume/progress controls, and installation command/download states.

- [ ] **Step 3: Run tests and verify RED**

Run: `cd frontend && npm test`

- [ ] **Step 4: Implement playback adapters and UI**

```ts
export type PlaybackPlan = {
  ID: string
  Backend: 'web' | 'mpv'
  URL: string
  Kind: 'hls' | 'flv' | 'ts' | 'mp4' | 'rtmp' | 'local'
  CanFallback: boolean
}
```

`PlaybackView` owns exactly one `<video>` element, destroys old library instances before changing sources, uses hls.js for HLS when MSE is available, mpegts.js for FLV/TS, native `src` for MP4/native HLS, and emits a single fallback request for fatal errors. App stores the returned plan, updates it after fallback, and presents command text for Linux/macOS or a progress-disabled install button for Windows.

- [ ] **Step 5: Run tests and build**

Run: `cd frontend && npm test && npm run build`

- [ ] **Step 6: Commit**

```bash
git add frontend
git commit -m "feat(frontend): add embedded web playback"
```

---

### Task 7: Documentation, Cross-Checks, and Final Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/HANDOFF.md`
- Modify: `docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`
- Create: `docs/verification/2026-08-28-m4-web-playback.md`

**Interfaces:**
- Consumes: completed M4 behavior and verification output.
- Produces: current project handoff and reproducible verification record.

- [ ] **Step 1: Update stale architecture documentation**

Remove claims that M4 is cross-platform mpvlib, mark the latest spec implemented, document the Web/mpv routing rule, the plugin directory, the pinned Windows artifact/checksum, and native-host verification still required on Windows/macOS.

- [ ] **Step 2: Run semantic and focused verification**

Run LSP diagnostics for every touched Go/TypeScript/Vue file when the bridge is available. Then run:

```bash
env GOCACHE=/tmp/unbox-m4-go-cache go test ./... -count=1
go vet ./...
test -z "$(gofmt -l .)"
cd frontend && npm test && npm run build
CGO_ENABLED=1 go build ./...
git diff --check
```

Expected: all commands pass. Record any native-only checks that cannot execute on Linux rather than claiming they ran.

- [ ] **Step 3: Browser verification**

Start the Wails/Vite development server, inspect desktop and narrow layouts with Playwright, confirm the video canvas is nonblank using a local fixture stream, and verify controls, fallback messaging, and installation UI do not overlap.

- [ ] **Step 4: Write verification record and commit**

```bash
git add AGENTS.md docs
git commit -m "docs(handoff): record M4 web playback"
```

- [ ] **Step 5: Review branch delta**

Run: `git diff --stat master...HEAD && git log --oneline master..HEAD && git status --short`

Expected: only M4 implementation, tests, dependency lock changes, and documentation are present; worktree is clean.
