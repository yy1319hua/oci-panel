# Bncr 插件：OCI 面板查询

通过 [Bncr](https://mourgod.github.io/Bncr/) 机器人调用 oci-panel 开放 API，在 QQ / 微信 / Telegram 里查询甲骨文实例状态、流量、成本。

脚本本身**平台无关**（只跟 Bncr 的 `Sender` 抽象打交道），一份代码多平台通用，且**纯只读**，不含任何开关机类写操作。

## 安装

1. 把 `oci-panel.js` 复制到 Bncr 的 `plugins/` 目录
2. 重启 Bncr（或热重载插件）

> ⚠️ **改名时务必同步**：Bncr 要求插件元数据里的 `@name` 必须与**文件名（去掉 `.js` 后缀）完全一致**，否则插件无法被正确识别。
> 例如文件叫 `oci-panel.js`，则注解必须写 `@name oci-panel`；若改成 `OCI面板查询.js`，`@name` 也要同步改成 `OCI面板查询`。

> ⚠️ **头部注解只能用单空格分隔**：`@name` / `@rule` / `@admin` 等字段后**只能有一个空格**，不要用多空格做对齐。Bncr 按单空格切分元数据，多余的空格会被算进字段值里（典型报错就是"加载异常 / name 不一致"）。正确写法：` * @name oci-panel`，错误写法：` * @name        oci-panel`。

## 配置

在 Bncr Web 管理面板 → 插件配置中填写：

| 配置项 | 说明 |
|---|---|
| 面板地址 | 如 `https://oci.158088.xyz`，结尾不要带斜杠 |
| API Token | 面板 → 系统设置 → API Token 生成 |
| 默认配置ID | 留空则自动取第一个配置；发 `oci 配置` 可查所有 ID |
| 超时（秒） | 默认 30。流量接口实时查询较慢，首次可能需调大 |

## 命令

> 🔒 插件设为 `@admin true`，**仅管理员可触发**，其他人发送命令不会响应。

| 命令 | 说明 |
|---|---|
| `oci` / `oci 状态` | 全部实例概况（状态 / 形状 / 公网 IP） |
| `oci 流量` | 本月流量与免费额度占比 |
| `oci 成本` | 近 30 天成本 |
| `oci 配置` | 查看配置 ID |
| `oci 帮助` | 显示帮助 |

流量和成本支持追加配置 ID，例如 `oci 流量 abc123`；不填则用插件配置里的默认 ID。

## ⚠️ Token 权限注意

面板的 API Token 分两种 scope：

- `readonly` —— 仅允许 GET，**只能查状态**
- `full` —— 可用 POST，查流量 / 成本需要它

因为 `/api/oci/traffic/monthly` 与 `/api/oci/traffic/cost` 是 POST 端点，用 readonly Token 调用会返回 403，此时脚本会提示「需要 full 权限的 Token」。

脚本不做任何写操作，因此即便使用 full Token 也不会误触开关机；唯一风险是 Token 泄露，建议设置过期时间。

## 调用的 API

| 端点 | 方法 | 用途 |
|---|---|---|
| `/api/bot/summary` | GET | 全部配置 + 实例概况 |
| `/api/oci/traffic/monthly` | POST | 月度流量 |
| `/api/oci/traffic/cost` | POST | 每日成本 |

鉴权统一走 `Authorization: Bearer <token>` 请求头。
