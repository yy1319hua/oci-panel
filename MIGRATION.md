# 迁移说明 / Migration Notes

本文件记录优化过程中**会改变可观察行为或需要重新配置**的改动，按批次组织。
升级后请按对应批次的步骤操作。

---

## Batch A — 正确性 Bug 修复 + 安全加固

> 总体：本批修复一个**数据丢失级 bug** 与一组**高危安全问题**。多数改动向后兼容；
> 少数需要一次性重新登录或确认配置（已在下方标注 ⚠️）。

### 1. ⚠️ JWT 密钥不再有硬编码默认值（需要重新登录一次）

- **变化**：此前未配置 `web.jwt_secret` 时使用一个公开的硬编码字符串，任何人都能伪造登录 token。
  现在：`config.toml` 显式配置优先；否则服务首次启动时用密码学随机数生成一个密钥并**持久化到数据库**（`sys_setting` 表，key=`jwt_secret`），重启后保持不变。
- **影响**：升级后此前签发的 token 立即失效，**所有用户需要重新登录一次**。
- **动作**：无需手动操作；重新登录即可。若想固定密钥（例如多副本部署共享），在 `config.toml` 的 `web.jwt_secret` 显式填写同一随机字符串。

### 2. ⚠️ 登录密码支持 bcrypt（建议改用哈希，明文仍兼容）

- **变化**：登录校验改为常量时间比较（防时序侧信道）；`web.password` 现在**同时支持明文与 bcrypt 哈希**（以 `$2a$/$2b$/$2y$` 开头时按 bcrypt 校验）。
- **影响**：原有明文密码**继续可用**，无强制动作。
- **建议动作**：把 `config.toml` 的 `web.password` 换成 bcrypt 哈希，避免明文泄露：
  ```bash
  htpasswd -bnBC 12 "" 'your-password' | tr -d ':\n'
  # 或（无 htpasswd 时）
  docker run --rm httpd:alpine htpasswd -bnBC 12 "" 'your-password' | tr -d ':\n'
  ```
  把输出（`$2y$...`）整体填入 `password = "..."`。

### 3. ⚠️ CORS 收紧（同源部署不受影响）

- **变化**：移除了 `Access-Control-Allow-Origin: *` 与 `Allow-Credentials: true` 的非法且不安全组合。
  现在仅当请求 `Origin` 命中 `web.allow_origins` 白名单时才回显该 Origin 并允许携带凭证。
- **影响**：
  - **生产同源部署**（前端由本服务托管）：**不受影响**，无需配置。
  - **本地开发**（Vite 代理）：浏览器视角同源，**不受影响**。
  - 仅当你把前端与 API 部署在**不同来源**时，才需要在 `config.toml` 配置：
    ```toml
    [web]
    allow_origins = ["https://panel.example.com"]
    ```

### 4. ⚠️ 实时日志 `/ws/logs` 现在需要鉴权

- **变化**：此前 `/ws/logs` 完全开放，任何人都能订阅日志流（含实例 ID/IP）。现在先通过已鉴权的 API 获取 60 秒、单次使用的 ticket，再升级 WebSocket，并收紧了 `CheckOrigin`（仅同源或白名单来源）。
- **影响**：前端已自动获取 ticket，**正常使用无感知**。任何外部脚本若直接连 `/ws/logs` 需先调用 `/api/sys/wsTicket` 获取 ticket。

### 5. NLB 误删修复（行为修正，无需配置）

- **变化**：`Disable500Mbps` 此前会列出并删除 compartment 内的**所有**网络负载均衡器；`Enable500Mbps` 会先删除所有 active NLB。现在两者都**只操作由本工具为当前实例创建**（带 FreeformTag `oci-panel-managed=true` 且 `oci-panel-instance=<实例OCID>`）的 NLB。
- **影响**：不再误删租户内无关的负载均衡器。
- ⚠️ **历史遗留**：升级**之前**由旧版本创建的 500Mbps NLB（名为 `nlb-<时间戳>`）没有受管标签，新代码不会自动清理它们。如需移除，请在 OCI 控制台手动删除一次；之后通过本工具开启的 NLB 都会带标签并被正确管理。

### 6. 其他（无行为变化）

- WebSocket 广播改用写锁（修正锁纪律）。
- 所有后台 fire-and-forget goroutine 增加 panic 恢复——单个后台任务 panic 不再导致整个进程崩溃。
- `ParseToken` 显式锁定 HS256 签名算法（防算法混淆）。

### 已知残留风险（经安全/代码评审确认，留待 Batch B 跟踪，非阻塞）

- **WS ticket 经查询参数传递**：`?ticket=<one-time-ticket>` 可能出现在 gin 访问日志/反代日志中，但 ticket 仅 60 秒有效且只能使用一次，不能直接调用 API。
- **bcrypt 72 字节上限**：bcrypt 只取密码前 72 字节，超长密码多余部分被忽略（单管理员场景影响极小）。
- **`allow_origins = ["*"]` 会被拒绝启动**：为避免凭证跨站泄露，配置加载会拒绝通配符来源；请填写具体的 `https://...` 来源。
- **Go 工具链 CVE**：`govulncheck` 报告若干 Go 标准库（`crypto/tls`/`net/url` 等，Go 1.25.x）漏洞，与本批代码无关。建议单独升级构建用的 Go 工具链。

---

## Batch A — 手动冒烟验证清单

> 自动化已通过：`go build ./...`、`go vet ./...`、`go test ./...`、前端 `vue-tsc -b && vite build` 全绿。
> 以下为针对真实 OCI/部署的人工确认项：

- [ ] **登录**：用现有账号密码可登录（明文兼容）；把密码换成 bcrypt 哈希后仍可登录。
- [ ] **错误密码**被拒绝。
- [ ] **JWT 持久化**：重启服务后，已登录会话仍需重新登录一次（首次升级），之后重启不再要求重登（密钥已持久化）。检查数据库 `sys_setting` 出现 `jwt_secret` 行。
- [ ] **实时日志**：登录后「实时日志」页可正常连接并收到日志；未带 ticket 直连 `ws://host/ws/logs` 被拒（401）。
- [ ] **CORS**：同源访问正常；（如适用）配置 `allow_origins` 后跨域前端可访问。
- [ ] **500Mbps（关键）**：在一个有**多个**负载均衡器的 compartment 中，对某 AMD 实例执行「开启500Mbps」→ 仅新建带 `oci-panel-nlb-` 前缀的 NLB；执行「关闭500Mbps」→ **仅删除该实例对应的受管 NLB**，其余 NLB 原样保留。
- [ ] **后台任务健壮性**：触发自动救援/缓存刷新等后台任务，进程不因偶发错误退出（panic 仅记录日志）。

---

## Batch B — 性能优化（行为基本不变，少数可观察差异已标注 ⚠️）

> 总体：本批为高价值性能优化，**不改变业务语义**。两处操作性变化需注意：
> SQLite 改用 WAL（备份方式需含 sidecar 文件），以及列表接口现在返回**全部分页**结果。

### 1. ⚠️ SQLite 启用 WAL + 放开单连接（操作性变化：备份方式）

- **变化**：此前 `SetMaxOpenConns(1)` 把所有请求与后台任务串行化到单连接。现在：
  - 通过 DSN 注入 PRAGMA：`journal_mode(WAL)`、`busy_timeout(5000)`、`synchronous(NORMAL)`、`foreign_keys(1)`；
  - 连接池放开到 `max(4, NumCPU×2)`，并设置空闲连接与最大空闲时长。
- **影响**：并发吞吐显著提升；数据库文件旁会新增 **`oci.db-wal`** 与 **`oci.db-shm`** 两个 sidecar 文件。
- ⚠️ **备份动作**：冷备份/复制数据库时必须**同时包含** `-wal`/`-shm`，或先执行 `PRAGMA wal_checkpoint(TRUNCATE);` 后再复制 `.db`。只复制 `.db` 可能丢失最近写入。
- **回滚**：如需旧行为，可在 DSN 显式带 `_pragma=journal_mode(DELETE)`——代码检测到调用方自带 `_pragma=` 时不再覆盖。

### 2. ⚠️ 列表接口返回全部分页（消除结果截断）

- **变化**：`ListInstances`、`ListBootVolumes`、`ListVCNs`（含子网）、`ListImages`、租户用户列表此前**只取首页**（忽略 `OpcNextPage`），现在循环抓取**全部分页**。
- **影响**：当某 compartment 的实例/镜像/卷/用户数量超过单页上限时，列表与计数会比之前**更完整**。这是修正而非回归；镜像列表可能明显变长。
- **未改动**：每实例的 VNIC/引导卷附件查询（单实例作用域、数量有限）仍按单页处理。

### 3. OCI 客户端并发 + 缓存（结果不变，更快）

- `GetInstanceDetails` 内部 6~8 次 SDK 调用并发化；列实例与调度器缓存刷新对多实例并发（上限 `instanceDetailConcurrency=6`），结果顺序与字段保持不变。
- `UserPage` 实时统计并发（上限 8）+ 30s 超时。
- Telegram「测活 / 实例统计 / 流量统计」对每个配置并发（上限 `telegramConcurrency=5`）+ 各自超时；输出顺序与此前一致。
- monitoring 客户端纳入 `clientCache`；`*ForRegion`（compute/vnic/identity）客户端按 (user, region) 缓存，不再每次重读磁盘私钥。凭证变更仍由现有 `InvalidateClientCache` 失效。
- ⚠️ **OCI 限流**：实例非常多时并发会提高瞬时 API 速率。已设并发上限缓解；若仍触发 429，可下调上述常量。

### 4. 前端（轻微 UX 变化）

- 配置/密钥搜索框 `@input` 改为 **300ms 防抖**：停止输入后才发请求，不再逐键请求（共享 `src/lib/utils.ts` 的 `debounce`）。
- 实时日志缓冲上限 **1000 条**，超出丢弃最旧条目（防 DOM/内存无界增长）；移除每条日志的逐行入场动画（大量日志时更流畅）。
- 批量创建实例改为**并发**提交（`Promise.allSettled`）并**逐项汇总**：全部成功显示成功数，部分失败则列出失败配置名。

---

## Batch B — 手动冒烟验证清单

> 自动化已通过：`go build ./...`、`go vet ./...`、`go test ./...`（含 `paginate` 与 WAL 单测）、
> 前端 `vue-tsc -b && vite build` 与 `eslint` 全绿。以下为针对真实 OCI/部署的人工确认项：

- [ ] **WAL 生效**：服务启动后数据库目录出现 `*.db-wal`/`*.db-shm`；多标签页并发刷新无 "database is locked" 错误。
- [ ] **列表完整**：在实例/镜像数量超过单页的 compartment 下，列表与计数显示完整，不再被截断。
- [ ] **实例详情速度 + 正确性**：打开含多实例/多 VNIC 的配置详情，加载明显更快；镜像名/引导卷大小/公私网 IP/IPv6 与此前一致，VNIC 顺序稳定。
- [ ] **Telegram**：测活、实例统计、流量统计内容与此前一致，但多配置时返回更快。
- [ ] **多区域**：查询不同 region 的镜像等，结果正确（客户端缓存命中）。
- [ ] **前端搜索防抖**：连续输入时仅在停顿后发一次请求（浏览器网络面板确认）。
- [ ] **实时日志**：长时间运行页面不卡顿，日志条数稳定在上限附近。
- [ ] **批量创建**：选多个配置批量创建，全部成功显示成功数；个别失败时提示「成功 N 个，失败 M 个：<配置名>」。

---

## Batch C（部分）— 架构去重 C1–C4（纯重构，无可观察行为变化）

> 本批为内部重构，**不改变任何对外行为/接口/响应**，无需迁移动作。仅记录范围与一处可忽略的内部差异。

- **C1**：抽取 `OciController.loadUser(c, configID)`，消除 `oci_controller.go` 中 19 处「取 DB → 查配置 → 404」复制粘贴前导。
- **C2**：抽取泛型 `waitForState(maxAttempts, interval, getState, isReady)`（含 4 项单测），替换 `oci_compute.go` 中 VCN / Internet 网关 / 子网创建的 3 个等待轮询循环。
  - ⚠️ **范围说明（非全量）**：`oci_rescue.go` / `oci_nlb.go` 中的 `for {}` 轮询**有意保留未改**——它们语义不同（获取出错即**立即失败返回**并带各自错误信息，且依赖 10 分钟 `context` 超时而非固定次数），位于救援 / 500Mbps 等**破坏性流程**且无法离线验证。强行并入同一 helper 会改变其错误/超时行为，风险高于收益，故按「逐项确认、保守重构」原则跳过。
- **C3**：抽取 `database.UpsertSysSetting(key, value)`，消除 sys / passkey / scheduler / telegram 中 7 处 SysSetting upsert 重复。
  - 内部差异（可忽略）：新建设置行的主键 ID 统一改为随机 UUID（此前 MFA/Passkey 用固定串如 `mfa_secret_id`）。查询与更新均以 `key` 为准，ID 仅作占位，**无功能影响**；已存在的行不受影响。
- **C4**：删除死代码 `services/volume_service.go`、`services/security_service.go`（无任何调用方，真实逻辑在 `OCIService`/`oci_*.go`）及 `router.go` 中被丢弃的 `_ = NewVolumeService(...)`。

**验证**：`go build` / `go vet` / `go test`（含 `-race`）通过，`gofmt` clean，无前端改动。

---

## Batch C（续）— 按域拆分 oci_controller.go（C5，纯文件重组，无可观察行为变化）

> 本项为**纯文件级重组**：所有 handler 仍是 `package controllers` 中 `*OciController` 的方法，
> 方法集、签名、路由注册、请求/响应结构体**全部不变**，不改变任何对外行为。无需迁移动作。

- **C5**：把原 1093 行的 `oci_controller.go` 按业务域拆分为「核心 + 7 个域文件」，便于定位与维护：
  - `oci_controller.go`（36 行）— 核心：`OciController` 结构体、`NewOciController`、共享的 `loadUser` 辅助。
  - `oci_controller_config.go` — 配置（账号凭据）列表/增删改 + API 私钥上传。
  - `oci_controller_resource.go` — 单配置资源读取：配置详情、租户信息、实例/卷/VCN 列表、缓存刷新（共享「先读缓存再实时拉取」模式）。
  - `oci_controller_task.go` — 实例创建任务的提交与分页查询。
  - `oci_controller_traffic.go` — 流量统计及其查询条件（区域/实例/VNIC）。
  - `oci_controller_iam.go` — 租户 IAM 用户管理（密码过期策略、用户信息、删除/重置/清除 MFA·API 密钥）。
  - `oci_controller_vcn.go` — VCN 与安全规则（查询/添加/一键放行/删除 VCN）。
  - `oci_controller_images.go` — 可用镜像列表（带 区域+架构 维度数据库缓存）。
  - 共 28 个 `*OciController` 方法，拆分前后逐一核对**无遗漏、无重复**。

**验证**：`gofmt -l` 无待格式化文件；`go build ./...` / `go vet ./...` / `go test -race ./...` 全绿；方法清单 28 项与拆分前一致。

---

## Batch C（续）— 前端类型化 API 层（C6，纯重构，无可观察行为变化）

> 本批新建 `frontend/src/api/` 类型化 API 层，把此前散落在各 view/store 中的裸 `api.post/api.get('/url', body)`
> 调用收敛为**按域分组、带请求/响应 TS 类型**的函数。**运行时与改造前完全等价**（内部仍调用原
> `src/lib/api.ts` 的 axios 实例，拦截器逻辑不变），无需迁移动作。

- **新增** `src/api/`：`http.ts`（`ApiResponse`/`PageResult`/`ValueLabel` 与类型化 `post`/`get` 包装，
  吸收「响应拦截器已解包 body」这一事实）、按域模块 `sys/passkey/telegram/preset/key/task/bootVolume/instance/oci`（其中
  `oci.ts` 含 `ociApi` 与 `vcnApi` 两组）、`index.ts` 汇总导出。
- **迁移** 13 个调用方（`stores/auth.ts`、`views/{Configs,Dashboard,Keys,Login,Presets,Settings,Tasks}.vue`、
  `views/configs/components/{CloudShell,EditInstance,SecurityList,VolumeEdit}Modal.vue`）：
  `import api from '@/lib/api'` → `import { xxxApi } from '@/api'`，所有调用改为类型化函数；`api.post/api.get`
  现仅存在于 `src/api/http.ts` 内部实现。
- 内部差异（可忽略，行为等价）：
  - `stores/auth.ts` 登录成功分支 `token/username` 改用 `|| ''` 兜底——类型化后暴露其在 MFA/Passkey 分支为可选；
    成功分支必有值，空串与 `undefined` 同为 falsy，`isAuthenticated` 行为不变。
  - `/key/standalone` 因 Configs（`id:number`）与 Presets（`id:string`）对返回项 id 的类型期望冲突，该端点
    保留 `any[]` 返回类型以零回归（其余端点均精确类型化）。

**验证**：`vue-tsc -b` 类型检查 0 错误；`eslint` 0 problems；`vite build` 成功（1920 模块）。

---

## Batch C（续）— 前端共享原语：webauthn / 选择·分页 composable / Modal（C8，纯重构，无可观察行为变化）

> 本批把前端散落的重复逻辑收敛为可复用原语。**运行时行为与改造前完全等价**，无需迁移动作。

- **C8a｜passkey 凭据编解码下沉** `src/lib/webauthn.ts`：把 `Login.vue` / `Settings.vue` 中各自内联的
  WebAuthn 凭据 base64url 编解码抽成 `getPasskeyCredential(publicKey)` / `createPasskeyCredential(publicKey)`。
  两处调用点收敛为 `begin → 取/建凭据 → finish` 三步，删除各自重复的 base64 helper。
- **C8b｜选择 + 分页 composable**：
  - `composables/useSelection.ts`：`useSelection<T>(items, getId)` → `{ selectedIds, isAllSelected,
    isIndeterminate, toggle, toggleAll, clear }`，吸收「全选/半选/切换」样板。
  - `composables/usePagination.ts`：`usePagination(pageSize=10)` → `{ currentPage, pageSize, totalPages,
    applyResult }`，`applyResult(PageResult)` 统一回填页码与总页数。
  - 接入 `Tasks.vue`（任务多选 + 日志分页）、`Keys.vue`、`Configs.vue`（配置多选 + 列表分页）；
    模板经别名解构（`selectedIds: selectedConfigIds` 等）保持逐字节不变。
- **C8c｜统一 Modal 原语** `components/ui/modal/Modal.vue`：抽出 4 个详情子弹窗各自复制的
  「`Teleport` + 遮罩 + 面板 + 标题栏 + X 关闭 + `.fade` 过渡」脚手架；props `open/title/maxWidth/
  panelClass/zClass`、`#title` 插槽、`update:open` 事件。`CloudShellModal` / `VolumeEditModal` /
  `EditInstanceModal` / `SecurityListModal` 改用 `<Modal>`（`SecurityListModal` 含主弹窗 + 嵌套添加规则
  弹窗两个 `<Modal>`），删除重复脚手架与 `X` 图标导入。

**验证**：`vue-tsc -b` 0 错误；`eslint` 0 problems；`vite build` 成功。

---

## Batch C（续）— 拆分 Configs.vue（C7，纯重构，无可观察行为变化）

> 把原 **1923 行**的 `Configs.vue` 巨型组件按职责拆分：列表/新建留在 `Configs.vue`，配置详情整体下沉为
> `ConfigDetailsDrawer` 编排组件 + 5 个纯展示标签页 + 用户卡片。所有逻辑（加载时序、懒加载 watch、
> 刷新派发、实例操作、用户管理、流量查询、4 个子弹窗）**逐字迁移、行为不变**，无需迁移动作。

- **`Configs.vue`（1923 → 853 行）**：仅保留配置列表、搜索/分页/多选、新增·编辑配置弹窗、创建实例弹窗、
  批量创建弹窗。`viewConfigDetails(config)` 收敛为「设置选中配置 + 打开抽屉」两行，详情逻辑全部移出。
- **新增 `views/configs/`**：
  - `types.ts`：抽出 `Config` / `Instance` 接口（形状取自原文件）。
  - `components/ConfigDetailsDrawer.vue`（编排）：`v-model:open` 控制显隐，持有 `configDetails`、5 个标签页
    数据、实例操作锁、流量查询状态、用户编辑与 4 个子弹窗状态；`initDetails`（原 `viewConfigDetails` 主体）
    经 `watch(() => props.open)` 在打开时触发、关闭时 `resetDetails`——与原「打开即加载、关闭即清空」时序一致；
    `watch(activeTab)` 懒加载、`watch(trafficForm.instanceId)` 加载 VNIC 均逐字保留。
  - `components/tabs/{BasicInfoTab,InstancesTab,VolumesTab,VcnsTab,TrafficTab}.vue`：5 个纯展示标签页，
    数据经 props 传入、动作经 typed emits 上抛由抽屉处理。`BasicInfoTab` 因密码过期编辑为局部交互，连同其
    `ociApi.updatePwdEx` 调用一并下沉（成功后就地更新共享的 `tenant` 对象）；`TrafficTab` 查询表单经 props
    传入并就地 `v-model`（与抽屉共享同一响应式对象，故抽屉的 `watch(instanceId)` 仍触发）。
  - `components/UserListCard.vue`：用户列表卡片，5 个用户操作经 emit 上抛。
- 设计要点：编排组件（抽屉）独占全部副作用与 API 调用并逐字搬运；子组件纯展示 + typed emits，使
  `vue-tsc` 能静态校验 props/事件接线。本项目 eslint 未启用 `vue/no-mutating-props`，故 `TrafficTab`/
  `BasicInfoTab` 对传入响应式对象的 `v-model`/就地更新合法且与原内联实现等价。`formatDate` 等日期/文件
  大小格式化已存在于 `src/lib/utils.ts`，未重复抽取。

**验证**：`vue-tsc -b` 0 错误；`eslint` 0 problems；`vite build` 成功（1937 模块）；原 95 个顶层符号逐一
核对均落位于新组件图（列表项留 `Configs.vue`、详情项入抽屉/标签页），开/关/初始化/重置时序与 API 调用
参数逐项比对一致。
