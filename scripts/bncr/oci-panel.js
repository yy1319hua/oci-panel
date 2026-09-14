/**
 * @author yy1319hua
 * @name oci-panel
 * @team oci-panel
 * @version 1.0.1
 * @description 调用 oci-panel 开放 API 查询甲骨文实例状态、流量、成本。平台无关：QQ / 微信 / Telegram 均可触发。
 * @rule ^oci$
 * @rule ^oci\s+(状态|status)$
 * @rule ^oci\s+流量$
 * @rule ^oci\s+成本$
 * @rule ^oci\s+帮助$
 * @rule ^oci\s+配置$
 * @priority 100
 * @admin true
 * @public true
 * @classification ["甲骨文"]
 */

/* ============================================================
 * 插件配置：在 Bncr Web 管理面板填写，脚本启动时自动读取
 * ============================================================ */
const ConfigDB = new BncrPluginConfig(
  BncrCreateSchema.object({
    panelUrl: BncrCreateSchema.string()
      .setTitle('面板地址')
      .setDescription('例如 https://你的面板域名，结尾不要带斜杠')
      .setDefault(''),
    apiToken: BncrCreateSchema.string()
      .setTitle('API Token')
      .setDescription('面板 → 系统设置 → API Token 生成。查流量/成本需要 full 权限，readonly 只能查状态')
      .setDefault(''),
    configId: BncrCreateSchema.string()
      .setTitle('默认配置ID')
      .setDescription('留空则取第一个配置。发「oci 配置」可查看所有ID')
      .setDefault(''),
    timeout: BncrCreateSchema.number()
      .setTitle('超时（秒）')
      .setDescription('流量接口实时查询较慢，建议 30 秒以上')
      .setDefault(30)
  })
);

/* ============================================================
 * 工具函数
 * ============================================================ */

/** 字节转人类可读单位 */
function fmtBytes(bytes) {
  if (bytes === null || bytes === undefined || isNaN(bytes)) return '-';
  const b = Number(bytes);
  if (b === 0) return '0 B';
  const k = 1024;
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.min(Math.floor(Math.log(Math.abs(b)) / Math.log(k)), units.length - 1);
  return parseFloat((b / Math.pow(k, i)).toFixed(2)) + ' ' + units[i];
}

/** 实例状态转中文 + 图标 */
const STATE_MAP = {
  RUNNING: '🟢 运行中',
  STOPPED: '🔴 已停止',
  STOPPING: '🟡 停止中',
  STARTING: '🟡 启动中',
  TERMINATED: '⚫ 已删除',
  TERMINATING: '🟡 删除中',
  PROVISIONING: '🟡 创建中',
  FAULTY: '🔴 故障'
};
const fmtState = s => STATE_MAP[s] || `⚪ ${s || '未知'}`;

/** 统一请求：拼接 URL、带 Bearer 头、超时控制、解包统一的 {code,message,data} 信封 */
async function api(path, options = {}) {
  const { panelUrl, apiToken, timeout } = ConfigDB.userConfig;
  const url = `${panelUrl.replace(/\/+$/, '')}/api${path}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), (timeout || 30) * 1000);

  try {
    const res = await fetch(url, {
      method: options.method || 'GET',
      headers: {
        Authorization: `Bearer ${apiToken}`,
        'Content-Type': 'application/json'
      },
      ...(options.body ? { body: JSON.stringify(options.body) } : {}),
      signal: controller.signal
    });

    // 401：Token 无效或过期
    if (res.status === 401) throw new Error('Token 无效或已过期，请到面板重新生成');
    // 403：readonly token 调用 POST 接口
    if (res.status === 403)
      throw new Error('权限不足：该接口需要 full 权限的 Token（readonly 仅允许 GET）');

    const json = await res.json();
    // 业务层错误：响应信封为 {code, message, data}，成功码是 200（不是 0）
    if (json.code !== 200) throw new Error(json.message || `接口返回 code=${json.code}`);
    return json.data;
  } catch (e) {
    if (e.name === 'AbortError') throw new Error(`请求超时（${timeout || 30}s），可尝试调大超时时间`);
    throw e;
  } finally {
    clearTimeout(timer);
  }
}

/** 解析出用户想查的配置ID：优先命令参数，其次插件配置，最后取第一个 */
async function resolveConfigId(arg) {
  if (arg) return arg;
  const { configId } = ConfigDB.userConfig;
  if (configId) return configId;
  // 未提供则从摘要里取第一个配置
  const summary = await api('/bot/summary');
  const first = summary.configs?.[0];
  if (!first) throw new Error('面板中没有任何 OCI 配置');
  return first.id;
}

/* ============================================================
 * 各命令的渲染逻辑
 * ============================================================ */

/** oci / oci 状态 —— 全部配置的实例概况 */
async function renderSummary() {
  const d = await api('/bot/summary');
  if (!d.configs?.length) return '面板中还没有配置任何 OCI 账号';

  const lines = [
    `☁️ OCI 面板概况`,
    `━━━━━━━━━━━━━━━`,
    `实例总数：${d.totalInstances}　运行中：${d.runningInstances}`,
    `更新时间：${d.time}`,
    ''
  ];

  for (const cfg of d.configs) {
    lines.push(`👤 ${cfg.username || '(未命名)'}　📍 ${cfg.region}`);
    lines.push(`　实例 ${cfg.instanceCount} 台，运行 ${cfg.runningCount} 台`);
    if (cfg.instances?.length) {
      for (const inst of cfg.instances) {
        const ip = inst.publicIps?.length ? inst.publicIps.join(', ') : '无公网IP';
        lines.push(`　• ${inst.name || '(无名称)'}`);
        lines.push(`　　${fmtState(inst.state)}　${inst.shape || ''}`);
        lines.push(`　　🌐 ${ip}`);
        if (inst.ipv6) lines.push(`　　🌐 ${inst.ipv6}`);
      }
    } else {
      lines.push('　（该配置下没有实例）');
    }
    lines.push('');
  }

  lines.push(`ID 对照：${d.configs.map(c => `${c.username || '未命名'}=${c.id}`).join(' | ')}`);
  return lines.join('\n');
}

/** oci 流量 —— 账号级月度流量 */
async function renderTraffic(arg) {
  const configId = await resolveConfigId(arg);
  const d = await api('/oci/traffic/monthly', {
    method: 'POST',
    body: { configId, forceRefresh: false }
  });

  // 免费额度只针对「出站」：入站免费，出站超出额度后才计费。
  // 所以「已用」必须拿**出站流量**去除额度，不能用 billableTraffic ——
  // 后者是超出额度之后才产生的计费量，没超额时恒为 0，会出现
  // 「明明用了十几 G、却显示 0%」的错。
  const out = d.outboundTraffic || 0;
  const free = d.freeAllowance || 0;
  const billable = d.billableTraffic || 0;
  const pct = free > 0 ? (out / free) * 100 : 0;

  const lines = [
    `📊 月度流量（${d.instanceCount || 0} 台实例）`,
    `━━━━━━━━━━━━━━━`,
    `↓ 入站（免费）：${fmtBytes(d.inboundTraffic)}`,
    `↑ 出站（计费方向）：${fmtBytes(out)}`,
    `🎁 免费额度：${fmtBytes(free)}`,
    `📈 已用：${pct.toFixed(2)}%`,
    `💸 超额计费流量：${fmtBytes(billable)}`,
    ''
  ];

  // 按实例拆分（接口返回 instances：displayName / inbound / outbound）
  if (d.instances?.length) {
    lines.push('按实例拆分（出站 / 入站）：');
    for (const i of d.instances) {
      lines.push(`• ${i.displayName || i.instanceId || '(未命名)'}`);
      lines.push(`　↑ ${fmtBytes(i.outbound)}　↓ ${fmtBytes(i.inbound)}`);
    }
    lines.push('');
  }

  return lines.join('\n');
}

/** oci 成本 —— 近 N 天每日成本 */
async function renderCost(arg) {
  const configId = await resolveConfigId(arg);
  const d = await api('/oci/traffic/cost', {
    method: 'POST',
    body: { configId, days: 30 }
  });

  const cur = d.currency || '';
  const lines = [
    `💵 成本（近 ${d.daysCount || 0} 天）`,
    `━━━━━━━━━━━━━━━`,
    `本月累计：${(d.monthToDate || 0).toFixed(4)} ${cur}`,
    ''
  ];

  if (d.days?.length) {
    // 只展示有花费的日期，全 0 的行太多没意义
    const charged = d.days.filter(x => Number(x.amount) > 0);
    if (charged.length) {
      lines.push('产生费用的日期：');
      for (const x of charged.slice(0, 15)) {
        lines.push(`• ${x.date}　${Number(x.amount).toFixed(4)} ${x.currency || cur}`);
      }
      if (charged.length > 15) lines.push(`…（另有 ${charged.length - 15} 天）`);
    } else {
      lines.push('✅ 近 30 天没有产生费用，均在免费额度内');
    }
  }
  return lines.join('\n');
}

/** oci 配置 —— 列出所有配置ID，方便填到插件配置里 */
async function renderConfigs() {
  const d = await api('/bot/summary');
  if (!d.configs?.length) return '面板中还没有配置任何 OCI 账号';
  return [
    '⚙️ 配置列表',
    '━━━━━━━━━━━━━━━',
    ...d.configs.map(
      c => `• ${c.username || '(未命名)'}\n　ID: ${c.id}\n　区域: ${c.region}　实例: ${c.instanceCount}`
    )
  ].join('\n');
}

const HELP = `☁️ OCI 面板查询助手

━━━━━━━━━━━━━━━
oci / oci 状态 —— 全部实例概况
oci 流量 —— 本月流量与免费额度
oci 成本 —— 近 30 天成本
oci 配置 —— 查看配置ID
oci 帮助 —— 显示本说明

━━━━━━━━━━━━━━━
提示：流量和成本可跟配置ID，例如
「oci 流量 abc123」；不填则用插件配置里的默认ID。

⚠️ 若提示权限不足，说明 Token 是
readonly，到面板换成 full 权限即可。`;

/* ============================================================
 * 主入口
 * ============================================================ */
module.exports = async s => {
  await ConfigDB.get();
  const { panelUrl, apiToken } = ConfigDB.userConfig;

  // 配置未填写：给出明确引导，而不是抛一堆栈
  if (!panelUrl || !apiToken) {
    return await s.reply(
      '❌ 插件尚未配置\n请在 Bncr 管理面板 → 插件配置中填写：\n1. 面板地址（如 https://你的面板域名）\n2. API Token（面板系统设置里生成）'
    );
  }

  const raw = (s.getMsg() || '').trim();
  // 拆出子命令与可选参数：oci 流量 <configId>
  const m = raw.match(/^oci\s*(\S+)?\s*(\S+)?\s*$/);
  const sub = m?.[1] || '';
  const arg = m?.[2] || '';

  try {
    let text;
    switch (sub) {
      case '':
      case '状态':
      case 'status':
        text = await renderSummary();
        break;
      case '流量':
        text = await renderTraffic(arg);
        break;
      case '成本':
        text = await renderCost(arg);
        break;
      case '配置':
        text = await renderConfigs();
        break;
      case '帮助':
      case 'help':
        text = HELP;
        break;
      default:
        text = `❓ 未知子命令：${sub}\n\n${HELP}`;
    }
    await s.reply(text);
  } catch (e) {
    BncrJSLogger.error('[oci-panel] 查询失败:', e.message);
    await s.reply(`❌ 查询失败\n${e.message}\n\n请检查：面板地址是否可访问、Token 是否有效`);
  }
};
