# 捐助榜单实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在设置页提供捐助榜单弹窗，按累计金额降序展示捐助人头像与昵称，并把「捐助」入口移入该弹窗底部。

**Architecture:** 离线导出脚本生成 `docs/donors.json` 发布到仓库 raw；应用侧「远端拉取 → `store.kv` 缓存（6 小时）→ 内置快照」三级回退，任何失败静默降级；前端在设置页弹窗展示。

**Tech Stack:** Go 1.26（标准库 + go:embed）、Wails v3 beta.9、Vue 3 + TypeScript、Vitest、modernc SQLite KV。

**Spec:** `docs/superpowers/specs/2026-09-20-donation-leaderboard-design.md`

## Global Constraints

- 爱发电 `user_id` / `token` 只经环境变量提供给导出脚本，**不得**写入仓库或编入应用。
- 远端地址常量可配置，默认 `https://raw.githubusercontent.com/teaGod-s/UnBox/master/docs/donors.json`。
- 缓存默认 6 小时；拉取/解析失败一律回退，不向用户报错。
- 界面**不展示金额**；`anonymous: true` 显示默认头像 + 「热心网友」，不露 ID 与头像。
- 排序：以**后端**为准——后端按金额降序、同额按昵称稳定排序；前端只做字段归一化并**原样保留后端顺序**（前端模型不含金额，自行排序会退化成按昵称排序）。
- 提交前：`gofmt`、`go test ./... -count=1`、`go vet ./...`、`CGO_ENABLED=1 go build ./...`、
  前端 `npm test -- --run`、`npm run build` 全绿。
- Wails 绑定用 `mise exec -- env GOCACHE=/tmp/unbox-bindings-cache wails3 generate bindings -f '' -clean=true -ts -i ./...` 生成。

---

### Task 1: 导出脚本

**Files:**
- Create: `cmd/unbox-donors/main.go`、`cmd/unbox-donors/main_test.go`
- Create: `docs/donors.json`（初始空榜单 `{"updatedAt":"","donors":[]}`）

**Interfaces:**
- Produces: `Donor{ID, Name, Avatar string, Amount float64, Anonymous bool}`、
  `Leaderboard{UpdatedAt string, Donors []Donor}`（JSON 字段 `id/name/avatar/amount/anonymous`、
  `updatedAt/donors`）；`normalizeDonors([]rawSponsor) []Donor`、`signParams(token, params, ts, userID) string`

- [ ] **Step 1: 写失败测试**：`normalizeDonors` 按金额降序 + 同额按昵称稳定排序；`anonymous` 条目
  清空 `ID`/`Avatar`；`signParams` 生成 md5 签名与爱发电文档一致。
- [ ] **Step 2: 跑测试确认失败**：`go test ./cmd/unbox-donors -count=1`
- [ ] **Step 3: 实现**：读 `AFDIAN_USER_ID`/`AFDIAN_TOKEN`（缺失则报错退出非零）；分页调用
  `POST https://afdian.com/api/open/query-sponsor`；归一化后按降序输出 JSON 到 stdout。
- [ ] **Step 4: 跑测试确认通过**，并手动 `go run ./cmd/unbox-donors > /tmp/donors.json`（有凭据时）
- [ ] **Step 5: 提交**：`git commit -m "feat(donors): add offline leaderboard export command"`

---

### Task 2: 后端榜单服务（三级回退）

**Files:**
- Create: `internal/shell/donation.go`、`internal/shell/donation_test.go`
- Create: `internal/shell/donors_snapshot.json`（`go:embed` 的内置快照，内容与 `docs/donors.json` 一致）
- Modify: `internal/shell/service.go`（如需共享 store 字段，否则不加）

**Interfaces:**
- Produces:
  - `type DonorInfo struct { ID, Name, Avatar string; Anonymous bool }`、
    `type DonationLeaderboard struct { UpdatedAt string; Donors []DonorInfo }`
  - `func (s *ShellService) GetDonationLeaderboard() DonationLeaderboard`
- 缓存键：`donations.cache`；缓存 TTL `6h`

- [ ] **Step 1: 写失败测试**：缓存命中不发请求；远端成功写缓存并返回；远端失败回退缓存；
  无缓存回退内置快照；非法 JSON 不 panic 且回退；输出按金额降序稳定排序、匿名条目已归一化。
- [ ] **Step 2: 跑测试确认失败**：`go test ./internal/shell -run TestDonation -count=1`
- [ ] **Step 3: 实现**：`fetchLeaderboard(ctx, url)` 拉取并解析；`GetDonationLeaderboard` 先查缓存
  （`store.GetKV` + `updatedAt`/时间戳），未过期直接返回；否则拉取，成功写缓存；失败回退缓存或内置快照。
  测试通过注入 `httpClient` 与 `now` 函数避免真实网络（沿用 `vodCategoryNow` 的可注入模式）。
- [ ] **Step 4: 跑测试 + 生成绑定**，确认 `shellservice.ts` 出现 `GetDonationLeaderboard` 与模型。
- [ ] **Step 5: 提交**：`git commit -m "feat(donors): serve leaderboard with remote, cache and snapshot fallback"`

---

### Task 3: 前端展示

**Files:**
- Modify: `frontend/src/App.vue`、`frontend/public/style.css`
- Test: `frontend/src/donation.test.ts`（纯逻辑）+ `frontend/src/App.playback.test.ts`（源码级接线断言）

**Interfaces:**
- Consumes: Task 2 的 `GetDonationLeaderboard()` 绑定
- Produces: `normalizeLeaderboard(value)`（缺失/非法字段归一化、按金额降序稳定排序）、
  `formatUpdatedAt(value)`；App 中的 `showDonations`、`donationLeaderboard`

- [ ] **Step 1: 写失败测试**：`normalizeLeaderboard` 处理 `null`/缺字段/匿名输入并保留后端顺序；
  `formatUpdatedAt` 对空值与非法值返回「未知」；App 源码含 `showDonations`、弹窗标记、
  `openURL(DONATE_URL)` 且设置页「关于」区不再有独立捐助按钮。
- [ ] **Step 2: 跑测试确认失败**：`cd frontend && npm test -- --run src/donation.test.ts`
- [ ] **Step 3: 实现**：设置页「关于」区改为「捐助榜单」按钮 → 弹窗（`settings-overlay` +
  `settings-panel`）：圆形头像（`@error` 回退默认头像）+ 昵称，空榜文案「还没有捐助记录，
  感谢每一份支持」，底部「数据更新于 X」+ 「捐助」按钮（打开 `DONATE_URL`）；移除设置页原捐助按钮。
- [ ] **Step 4: 跑测试与构建**
- [ ] **Step 5: 提交**：`git commit -m "feat(donors): show leaderboard dialog with donate entry"`

---

### Task 4: 文档与完整验证

**Files:**
- Modify: `docs/HANDOFF.md`、`README.md`（捐助栏目指向榜单说明）

- [ ] **Step 1: 更新文档**：记录导出脚本用法（环境变量、如何替换 `docs/donors.json`）、
  三级回退与缓存 TTL、隐私约定（不展示金额、匿名处理）、以及"更新榜单无需发版"的操作方式。
- [ ] **Step 2: 完整验证**：`gofmt -l .`、`go test ./... -count=1`、`go vet ./...`、
  `CGO_ENABLED=1 go build ./...`、前端 `npm test -- --run`、`npm run build`。
- [ ] **Step 3: 提交**：`git commit -m "docs: document donation leaderboard workflow"`
