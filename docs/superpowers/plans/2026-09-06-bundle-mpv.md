# 随应用分发 mpv 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 mpv 随应用分发，装好即用——`MPVStatus()` 直接返回 `Available=true`，用户无需手动安装 mpv；现有按需安装保留为兜底。

**Architecture:** 三平台各自按最贴惯例的方式补齐 mpv：Windows 走 NSIS 内嵌便携 mpv（双架构）；Linux `.deb` 声明 `Depends: mpv`（发行版仓库装）；macOS 与 Linux AppImage 维持现有「显示安装命令」兜底（无可靠静态 mpv 源）。Go 侧只加一处「应用内嵌路径探测」，`Status()`/`PickPlayer()` 接口不变。

**Tech Stack:** Go 1.26（`go.mod` 已钉 `go 1.26.3`）、Wails v3.0.0-beta.9、NSIS、nfpm。

**Spec:** 无独立 spec。播放架构背景见 `docs/superpowers/specs/2026-08-25-unbox-m4-playback-design.md`；本次设计结论见本计划「Global Constraints」与各任务。

---

## Global Constraints

- **不引入 CGO**：Windows 构建保持 `CGO_ENABLED=0`（本计划只打包二进制文件，不改变编译方式）。
- **mpv 版本钉死**：Windows 用 `shinchiro/mpv-winbuild-cmake` 便携 7z，tag `20260903`、commit `69e63f425a`，SHA256 见 Task 3（固定 URL `releases/download/20260903/…`，不可变、可复现）。
- **不触碰 macOS**（维持 `brew install mpv`）与 **Linux AppImage**（维持命令兜底，无可靠静态 mpv 源，已明确放弃内嵌）。
- **保留现有按需安装兜底**：`installWindows` / `pluginPath` / `Install()` 逻辑不删除，仅在其前插入内嵌探测。
- **探测优先级（最终）**：应用内嵌 → 用户插件目录 → 系统 `PATH`。
- **开发走新分支** `feat/bundle-mpv`（项目惯例：功能开发新开分支；本计划产物不直接落 master）。
- **许可证**：mpv 以「未修改二进制 + 指向源码」方式分发（mere aggregation），不链接、不修改；分发时在 About/README 已有「开源库」入口外，无需额外动作，但 README 应补一句 mpv 随包分发说明（见 Task 3 末尾可选步骤）。

---

### Task 1: mpvplugin 探测应用内嵌 mpv

**Files:**
- Modify: `internal/player/mpvplugin/manager.go`
- Test: `internal/player/mpvplugin/manager_test.go`

**Interfaces:**
- Consumes: `os.Executable`（经新包级变量 `executablePath` 注入）、现有 `Manager` 结构体字段 `goos`/`root`/`lookPath`。
- Produces: `(*Manager).bundledPath() string`（内嵌 mpv 绝对路径，无则 `""`）；`Status()` 的新优先级「内嵌 → 插件目录 → PATH」。对外 `Status`/`InstallResult`/`NewPlayer` 签名不变。

- [ ] **Step 1: 写失败测试**

在 `manager_test.go` 末尾新增（沿用现有 `newManager` + 注入 `lookPath` 的桩模式；新增包级变量 `executablePath` 注入）：

```go
func TestManagerStatusPrefersBundledExecutable(t *testing.T) {
	exeDir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(filepath.Join(exeDir, "mpv"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundled := filepath.Join(exeDir, "mpv", "mpv.exe")
	if err := os.WriteFile(bundled, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := executablePath
	executablePath = func() (string, error) { return filepath.Join(exeDir, "unbox.exe"), nil }
	t.Cleanup(func() { executablePath = orig })
	// lookPath 返回一个「系统 mpv」，确保内嵌优先于系统。
	m := newManager("windows", t.TempDir(), func(string) (string, error) { return `C:\system\mpv.exe`, nil })
	got := m.Status()
	if !got.Available || got.Path != bundled {
		t.Fatalf("Status = %+v, want bundled %q", got, bundled)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/player/mpvplugin/ -run TestManagerStatusPrefersBundledExecutable -count=1`
Expected: FAIL —— 编译错误 `undefined: executablePath`（或 `Status` 未命中内嵌路径）。

- [ ] **Step 3: 实现 bundledPath + 注入点 + Status 优先级**

在 `manager.go` 的 const 块之后、`Manager` 结构体附近新增包级变量：

```go
// executablePath 是 os.Executable 的可注入替身，供测试覆盖「应用内嵌 mpv」路径。
var executablePath = os.Executable
```

在 `Manager` 的 `pluginPath` 方法旁新增 `bundledPath`（Windows 内嵌 `$INSTDIR\mpv\mpv.exe`；其余平台返回空，交由后续插件目录/PATH 兜底）：

```go
// bundledPath 返回随应用分发的 mpv 路径（Windows NSIS 内嵌在 exe 旁 mpv/ 目录）。
// 非 Windows 或未找到时返回 ""。
func (m *Manager) bundledPath() string {
	if m.goos != "windows" {
		return ""
	}
	exe, err := executablePath()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), "mpv", exeForOS(m.goos))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
```

修改 `Status()`（现为 `pluginPath` → `lookPath` 两级），在开头插入内嵌探测：

```go
func (m *Manager) Status() Status {
	if path := m.bundledPath(); path != "" {
		return Status{Available: true, Path: path}
	}
	if path := m.pluginPath(); path != "" {
		return Status{Available: true, Path: path}
	}
	if path, err := m.lookPath(exeForOS(m.goos)); err == nil {
		return Status{Available: true, Path: path}
	}
	// …… 其余不变（InstallMode / InstallCommand）
}
```

确认新增 import 无需变更（`os`、`path/filepath` 已在）。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/player/mpvplugin/ -count=1` 与 `go test ./internal/player/... ./internal/shell/ -count=1`
Expected: PASS（新用例通过，`pick.go` 依赖的 `Status` 行为不回归）。

- [ ] **Step 5: 提交**

```bash
git add internal/player/mpvplugin/manager.go internal/player/mpvplugin/manager_test.go
git commit -m "feat(mpv): 探测应用内嵌 mpv（Windows NSIS 安装目录）"
```

---

### Task 2: Linux .deb 声明 Depends: mpv

**Files:**
- Modify: `build/linux/nfpm/nfpm.yaml`

**Interfaces:**
- Consumes: 无（纯配置）。
- Produces: `.deb` 安装时自动拉取发行版 `mpv` 包，`exec.LookPath("mpv")` 命中。

- [ ] **Step 1: 在 depends 列表追加 mpv**

在 `build/linux/nfpm/nfpm.yaml` 的 `depends:`（第 28-33 行）末尾加一行：

```yaml
depends:
  - libgtk-4-1
  - libwebkitgtk-6.0-4
  - gstreamer1.0-libav
  - gstreamer1.0-plugins-bad
  - mpv
```

> 说明：`mpv` 包名在 Debian/Ubuntu、Fedora、Arch 三系一致，故只加进基础 `depends:`，无需在 `overrides.rpm`/`overrides.archlinux` 重复声明。

- [ ] **Step 2: 校验配置可解析**

Run（二选一，能跑通即可）:
- `nix run nixpkgs#nfpm -- package -f build/linux/nfpm/nfpm.yaml --packager deb -t /tmp/nfpm-test --target /tmp` 或
- `go run github.com/goreleaser/nfpm/v2/cmd/nfpm@latest package -f build/linux/nfpm/nfpm.yaml --packager deb --target /tmp`

Expected: 无 YAML 解析错误；若环境缺 nfpm，至少用 `python3 -c 'import yaml;yaml.safe_load(open("build/linux/nfpm/nfpm.yaml"))'` 确认语法合法（`${GOARCH}`/`${VERSION}` 是 nfpm 模板变量，YAML 层面仍是合法字符串）。

- [ ] **Step 3: 提交**

```bash
git add build/linux/nfpm/nfpm.yaml
git commit -m "feat(pkg): deb 依赖声明 mpv"
```

---

### Task 3: Windows NSIS 内嵌便携 mpv（amd64 + arm64）

**Files:**
- Create: `build/windows/fetch_mpv.ps1`（下载 + 校验 SHA256 + 解压）
- Modify: `build/windows/Taskfile.yml`（新增 `fetch:mpv` 任务，作为 `create:nsis:installer` 的 deps）
- Modify: `build/windows/nsis/project.nsi`（`Section` 内 `File /r` 安装 mpv 目录）
- Modify: `.github/workflows/release.yml`（Windows 打包步确保 7-Zip 可用；若选 PS 脚本内已含 `Get-FileHash` 则无额外依赖）

**Interfaces:**
- Consumes: Task 1 的 `bundledPath()`（期望安装后 `<exeDir>\mpv\mpv.exe` 存在）。
- Produces: 构建期在 `build/windows/mpv/` 产出可直接运行、结构为「`mpv.exe` + 伴随 DLL（如 `libmpv-2.dll`）同级」的文件集；NSIS 把它们装进 `$INSTDIR\mpv\`。

**钉死的下载源（不可变 URL + SHA256，来自 GitHub API `digest`）：**

| 架构 | 资产 | SHA256 |
|---|---|---|
| amd64 | `mpv-x86_64-20260903-git-69e63f425a.7z` | `418dbfb5feb851cbed33d6c05d8481ba71802621bfd6efe8974522b28d42ac97` |
| arm64 | `mpv-aarch64-20260903-git-69e63f425a.7z` | `1ac2e56fdc990db5d7d448432e95e653c622fd518250fb3f7d8876d4b4f4459f` |

下载基址：`https://github.com/shinchiro/mpv-winbuild-cmake/releases/download/20260903/<asset>`

- [ ] **Step 1: 写 `build/windows/fetch_mpv.ps1`**

脚本职责（参数 `-Arch amd64|arm64`）：按架构选 URL/SHA → `Invoke-WebRequest` 下载到 `build/windows/mpv.7z` → `Get-FileHash -Algorithm SHA256` 与钉死值比对（不匹配即 throw）→ 7-Zip 解压到 `build/windows/mpv/`（`C:\Program Files\7-Zip\7z.exe x -y -o"build\windows\mpv" mpv.7z`）→ 删除 `mpv.7z`。**关键**：解压后把 7z 内顶层目录（形如 `mpv-x86_64-…-git-…\`）的内容**拍平**到 `mpv\` 直接子级，确保 `mpv\mpv.exe` 与其伴随 DLL 同级（`mpv.exe` 链接共享 `libmpv-2.dll`，靠 Windows DLL 同目录搜索解析）。

- [ ] **Step 2: Taskfile 新增 `fetch:mpv` 并接入打包**

在 `build/windows/Taskfile.yml` 新增：

```yaml
fetch:mpv:
  summary: Download, verify, and extract portable mpv for NSIS bundling
  dir: build/windows
  vars:
    ARCH: '{{.ARCH | default ARCH}}'
  cmds:
    - powershell -NoProfile -ExecutionPolicy Bypass -File fetch_mpv.ps1 -Arch '{{.ARCH}}'
```

并把 `create:nsis:installer` 的 `deps` 从 `- task: build` 改为：

```yaml
    deps:
      - task: build
      - task: fetch:mpv
        vars:
          ARCH: '{{.ARCH | default ARCH}}'
```

> 注意：`fetch:mpv` 必须在 makensis 之前执行；`create:nsis:installer` 的 `dir: build/windows/nsis` 与 `fetch:mpv` 的 `dir: build/windows` 相对路径各自成立。

- [ ] **Step 3: NSIS 安装 mpv 目录**

在 `build/windows/nsis/project.nsi` 的 `Section` 中，`!insertmacro wails.files` 之后、`CreateShortcut` 之前，插入：

```nsi
    # 内嵌便携 mpv（mpv.exe + 伴随 DLL），供无系统 mpv 时开箱即用
    SetOutPath "$INSTDIR\mpv"
    File /r "..\mpv\*.*"
```

> makensis 运行于 `build/windows/nsis`（Taskfile `dir`），故 mpv 暂存目录相对路径为 `..\mpv`。卸载段已 `RMDir /r $INSTDIR`，会连带清掉 `mpv\`，无需改动。

- [ ] **Step 4: release.yml 确保 7-Zip 可用**

`windows-latest` runner 已预装 7-Zip（`C:\Program Files\7-Zip\7z.exe`）。若 `fetch_mpv.ps1` 直接以绝对路径调用 7z，则无需改 release.yml；若依赖 PATH，需在 Windows 打包步加一行 `export PATH="/c/Program Files/7-Zip:$PATH"`（与现有 `choco install nsis` 后加 NSIS PATH 的写法一致）。以 CI 日志无 `7z: command not found` 为验收。

- [ ] **Step 5: 本地/CI 验证**

- 单架构本地（有 Windows 环境时）：`wails3 task windows:package`，随后 `build\windows\mpv\mpv.exe --version` 能打印版本号（证明解压结构正确、伴随 DLL 齐全）。
- 无 Windows 环境：仅能验证脚本语法 `powershell -NoProfile -Command "Get-Command"`（不可行则跳过），最终以 CI 出包为准——push 一个 `v*` 测试标签或 `workflow_dispatch`，确认 4 个 Windows 产物（amd64/arm64 installer + exe）均生成且 installer 体积相对之前增大约 `mpv.7z` 解压后大小。
- 真机验收：在无 mpv 的 Windows 上安装 NSIS 包，`MPVStatus()` 应返回 `Available`，HEVC/本地文件可直接播。

- [ ] **Step 6: 提交**

```bash
git add build/windows/fetch_mpv.ps1 build/windows/Taskfile.yml build/windows/nsis/project.nsi .github/workflows/release.yml
git commit -m "feat(pkg): windows NSIS 内嵌便携 mpv（amd64 + arm64）"
```

- [ ] **Step 7:（可选）README 补充 mpv 分发说明**

在 README 的 mpv/播放器相关段落补一句：Windows 安装包内嵌 mpv、Linux .deb 依赖发行版 mpv、macOS 与 AppImage 需自行安装 mpv。并确认 mpv 的 GPL 分发说明（提供未修改二进制 + 指向 https://mpv.io 源码）已覆盖。

---

## 完成后的收尾

三个任务全部提交后，按项目流程用 `superpowers:finishing-a-development-branch` 把 `feat/bundle-mpv` 合入 master（或推 PR 待用户确认）。注意：Task 3 的真机验收依赖 CI 出包，若本地无 Windows 环境，须在合入前跑一次 `workflow_dispatch` 或测试标签，确认产物结构无误。
