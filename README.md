# OCI Panel

一个功能强大的 Oracle Cloud Infrastructure (OCI) 管理面板，提供实例管理、自动抢机、密钥管理等功能。

## 功能特性

- **实例管理** - 查看、创建、启动、停止、重启实例
- **自动抢机** - 支持定时任务自动创建实例
- **密钥管理** - 管理 OCI API 密钥配置
- **预设配置** - 保存常用实例配置模板
- **Telegram 通知** - 支持 Telegram Bot 消息推送
- **流量统计** - 实例流量监控与统计
- **安全组管理** - 管理实例安全规则
- **IP 管理** - 公网 IP 分配与管理

## 截图预览

### 仪表盘
![仪表盘](./screenshots/1.jpg)

### 配置管理
![实例管理](./screenshots/2.jpg)

### 密钥配置
![密钥配置](./screenshots/4.jpg)

### 任务管理
![任务管理](./screenshots/3.jpg)

### 预设配置
![预设配置](./screenshots/5.jpg)

### 实例设置
![系统设置](./screenshots/6.jpg)

## 技术栈

### 后端
- Go
- Gin Web Framework
- SQLite

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

配置示例：

```toml
[server]
port = "8999"

[web]
account = "admin"
# 使用 bcrypt 哈希，不要使用 admin 或其他默认密码
password = "$2b$12$replace-with-a-generated-bcrypt-hash"

[database]
dsn = "oci-helper.db"

[logging]
level = "info"
```

### 构建运行

**Linux/macOS:**

```bash
./build.sh
./oci-panel
```

**Windows:**

```bash
build.bat
oci-panel.exe
```

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

## License

[LICENSE](./LICENSE)
