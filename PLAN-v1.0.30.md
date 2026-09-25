# OCI Panel v1.0.30 功能扩展计划（2026-09-25）

用户需求：除 Web SSH(7) 外全部新增；手机端 UI 优先；避免任何收费；同步 API/无界脚本/TG。

## 功能清单

1. **空闲防回收保活**（keepalive）
   - OCI 空闲判定：CPU<20% + 内存<20% + 网络<20% → 需同时打高 CPU 与网络
   - 实现：SchedulerService 新增保活循环；每实例可开关（新表 keepalive_task 或 sys_setting JSON）
   - 动作：向实例注入 CPU 负载？❌ 无 agent。改用 **OCI API 层保活方案**：
     - 方案A（选定）：定期对实例做监控指标查询 + 产生网络流量（OCI 侧无法直接打流量进实例）
     - 现实可行方案：面板持续查询实例指标不改变 CPU。**真正可行的是「定时轻量操作」不可靠**
     - ✅ 最终方案：通过 **cloud-init/user-data 已有脚本**不现实 → 改为面板定时对实例执行
       RunCommand（OCI Run Command 服务，无需 SSH，实例需 oracle-cloud-agent 默认装）
       执行 `dd if=/dev/zero of=/dev/null` 5 分钟 + curl 下载产生网络流量。RunCommand 免费无 agent。
   - TG 推送保活结果
2. **自动抢机**（grabber）：新表 grab_task{cfg_id, ad, shape, image, ocpus, memory, boot_gb, ssh_key, subnet, interval, enabled, last_status, last_try_at}
   - 循环 LaunchInstance，捕 Out of capacity 错误重试；成功后 TG 通知
3. **流量超额告警**：月度流量 > 阈值%（默认 80%）→ TG 推送，防重复告警（每月每配置一次，state 存 sys_setting）
4. **CPU/内存监控曲线**：oci_monitoring.go 扩展 CpuUtilization + MemoryUtilization 指标查询 API；前端抽屉加趋势图（SVG 折线，手机横滚友好）
5. **卷备份**：List/Create/Delete BootVolumeBackup + BlockVolumeBackup；滚动保留 N（默认3，硬上限5）；⚠️ UI 明确提示免费上限；定时备份任务挂 scheduler
6. **配额总览**：ListAvailabilityDomains + 服务限制 API（limits service）或用 GetResourcesSummary；展示 Always Free 已用/剩余（实例数、OCPUs、卷容量）
8. ~~**CSV 导出**~~（2026-09-25 用户砍掉，不做）
9. /api/oci/traffic/export、cost/export 返回 text/csv；前端下载
9. **Token 权限细分**：api_token 加 scope 字段（readonly / instance / traffic / full）；token_service.go 改造；前端 Settings 加选择；鉴权中间件按路由组校验

## 约束
- 手机优先：所有新 UI 用现有组件库（components/ui），窄屏单列、抽屉内 Tab
- 防收费：备份总数硬校验 ≤5；抢机只抢 Always Free 形状；保活 RunCommand 免费
- bncr 脚本：新增 /api/bot/summary 扩展字段（保活状态、抢机状态、流量告警）；脚本版本号递增但不打 tag（用户规则）
- TG：新增命令 /keepalive /grab /backup 查询与开关

## 版本：v1.0.30（发布前必须征得用户同意推送）
