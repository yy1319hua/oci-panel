# OCI Panel

一个 Oracle Cloud Infrastructure (OCI) 管理面板，覆盖实例运维、网络与防火墙配置、流量与成本监控、租户与身份管理，并提供 Telegram 机器人远程操作。

> 本文档描述的是**当前代码的实际功能**。功能清单来自 `internal/router/router.go` 的路由注册与 `frontend/src/router/index.ts` 的页面路由，不包含已移除的历史功能。

## 功能特性

### 实例管理

- 查看实例列表与详情（规格、IP、引导卷、运行状态）
- 启动 / 停止 / 重启 / 终止实例
- 修改实例名称与引导卷配置
- **一键测活**：批量检测实例连通性
- **自动救援**（Auto Rescue）：实例异常时自动尝试恢复
- 为实例挂载 **IPv6** 地址、更换公网 IP

### 网络与防火墙

- VCN / 子网列表查看
- 安全列表（Security List）规则管理：新增、修改、删除
- 防火墙规则**行内编辑与删除**，支持一键释放全部规则
- 安全规则端口范围、协议、来源 / 目标的可视化配置

### 流量与成本监控

- **账号级月度流量汇总**：入站 / 出站总量、每实例明细
- 区分**实际流量**与**计费流量**（Oracle 仅对出站计费，免费额度 10TB/月）
- **每日成本查询**（Usage API），便于第一时间发现超额扣费
- 流量趋势图与实例占比可视化
- 流量查询支持**缓存加速**，避免每次打开首页都实时请求 OCI

### 租户与身份管理

- 租户信息查看（区域列表、创建时间、用户列表）
- 租户用户管理：修改用户信息、重置密码、修改密码过期策略
- 删除用户、删除 MFA 设备、删除 API Key

### 主机与 IP

- 更换实例公网 IP
- 为实例挂载 IPv6 地址

### 系统与安全

- 多配置管理：支持添加多个 OCI 配置并使用不同密钥
- **API 令牌**：便于第三方程序调用面板接口，含调用日志审计
- **双因素认证（TOTP）** 与 **通行密钥（Passkey / WebAuthn）**
- 账号密码修改、登录账号变更、邮箱设置、密码邮件重置
- 运行日志级别在线调整（debug / info / warn / error），即时生效
- **面板日志**页：后端日志与 API 调用日志实时推送，支持搜索、级别筛选、行数筛选、复制与下载
- 数据缓存开关与刷新间隔配置

### Telegram 机器人

- 消息推送通知（配合机器人实时掌握实例状态）
- 按钮菜单：一键测活、每日成本、实例统计、配置列表、版本信息、流量统计
- 文本命令：`/menu` `/traffic` `/cost` `/instances` `/alive` `/configs` `/version`
- 支持配置**自定义 API 反代地址**（国内网络无法直连 Telegram 时使用）
- 命令菜单自动注册与刷新

## 技术栈

### 后端
- Go
- Gin Web Framework
- SQLite（GORM + 纯 Go 驱动 glebarez/sqlite，无需 CGO）
- OCI Go SDK

### 前端
- Vue 3
- TypeScript
- Tailwind CSS
- Vite

## 快速开始

### 环境要求

- Go 1.24+
- Node.js 22.12+（也支持 20.x 的 20.19+，与 Vite 7 的要求一致）

### 配置

复制配置文件并修改：

```bash
cp config.toml.example config.toml
```

配置示例（完整项与注释见 `config.toml.example`）：

```toml
[server]
port = "8999"

[web]
account = "admin"
# 密码：可填明文，也可填 bcrypt 哈希（以 $2a$/$2b$/$2y$ 开头）。
# 强烈建议使用 bcrypt 哈希，不要使用默认密码。
password = "$2b$12$replace-with-a-generated-bcrypt-hash"
# JWT 签名密钥。留空时服务会自动生成随机密钥并持久化到数据库（重启保持不变）。
jwt_secret = ""
# 跨域来源白名单，留空表示仅允许同源（生产同源部署无需配置）。
allow_origins = []

[database]
dsn = "db/oci-helper.db"

[logging]
level = "info"

[passkey]
# WebAuthn 的 Relying Party ID，通常是你的域名（不含协议与端口）。
# 本地开发用 "localhost"，生产环境填实际域名。
rp_id = "localhost"
rp_origins = ["http://localhost:8999"]

[email]
# 邮件服务商（目前支持 resend），用于密码重置邮件。
provider = "resend"
resend_api_key = ""
from = ""
# 面板对外地址，用于拼接密码重置链接。
public_url = "https://panel.example.com"
```

> 关于密码哈希：`htpasswd -bnBC 12 "" 'your-password' | tr -d ':\n'`
> 生成的 `$2y$...` 整体填入 `password` 即可（明文仍然兼容，但不推荐）。

### 构建运行

```bash
# 前端
cd frontend && npm install && npm run build && cd ..

# 后端（版本号由 tag 注入，见下方「版本号」一节）
go build -ldflags "-X github.com/adiecho/oci-panel/internal/version.AppVersion=$(git describe --tags --always)" -o oci-panel main.go

./oci-panel
```

> 监听端口以 `config.toml` 的 `[server].port` 为准（默认 `8999`）。
> 生产环境推荐直接用 GHCR 上的镜像（见 `docker-compose.yml`），无需本地构建。

### 访问面板

启动后访问 `http://localhost:8999`，使用配置文件中的账号密码登录。

### Docker 与 Nginx 反代

`docker-compose.yml` 默认只把服务发布到宿主机 `127.0.0.1:8999`，适合由宿主机 Nginx 反代并在 Nginx 处终止 HTTPS。请保留这个绑定地址，避免绕过 Nginx 直接暴露管理面板；Nginx 反代 WebSocket 时需要转发 `Upgrade` 和 `Connection` 请求头。

## 开发

### 前端开发

```bash
cd frontend
npm install
npm run dev
```

### 后端开发

```bash
go run main.go
```

### 测试

```bash
go test ./...
go test -race ./...   # 并发安全回归
cd frontend && npx vue-tsc --noEmit   # 前端类型检查
```

## 版本号

版本号以 **git tag 为唯一数据源**：构建时通过 `-ldflags` 注入到
`internal/version.AppVersion`，后端接口 `/api/sys/getVersion`、Telegram
机器人与前端界面均读取该值。发布新版本只需打 tag，无需改动代码。

```bash
go build -ldflags "-X github.com/adiecho/oci-panel/internal/version.AppVersion=$(git describe --tags --always)" -o oci-panel main.go
```

## License

[LICENSE](./LICENSE)
