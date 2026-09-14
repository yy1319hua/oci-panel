/**
 * @author yy1319hua
 * @name oci-daily-report
 * @team oci-panel
 * @version 1.2.0
 * @description 每天定时把 oci-panel 的成本与流量汇总推送到微信等渠道。平台无关：QQ / 微信 / Telegram 均可。
 * @rule ^oci\s+日报$
 * @rule ^oci\s+日报\s+(\S+)$
 * @cron 0 0 9 * * *
 * @priority 100
 * @admin true
 * @public true
 * @classification ["甲骨文"]
 *
 * ┌─ 关于 @cron（定时）────────────────────────────────────────┐
 * │ 这一行决定每天几点推送，格式是「秒 分 时 日 月 周」六位：      │
 * │   @cron 0 0 9 * * *      → 每天 09:00:00                  │
 * │   @cron 0 30 8 * * *     → 每天 08:30:00                  │
 * │   @cron 0 0 9,21 * * *   → 每天 09:00 和 21:00 各一次      │
 * │ 如果你的无界/Bncr 要求五位（分 时 日 月 周），把秒位去掉：     │
 * │   @cron 0 9 * * *        → 每天 09:00                     │
 * │ 改完需要重启无界（docker restart bncr）才生效。              │
 * │ 若平台不认 @cron，插件仍可手动触发：发「oci 日报」，           │
 * │ 或用无界自带「定时任务」插件定时发这句命令。                   │
 * └──────────────────────────────────────────────────────────┘
 *
 * 【凭据来源】面板地址与 API Token 一律从 oci-panel.js 的插件配置里读
 * （数据库键 /plugins/凯尼尔/oci-panel.js），本插件不再重复填写、也不做任何兜底：
 * oci-panel.js 配置好的话，这里一定读得到，读不到就是键填错了。
 *
 * ⚠️ 头部注解只能用单空格分隔：@name / @rule / @cron 等字段后只能有一个空格，
 *    不要用多空格做对齐，否则 Bncr 会把多余空格算进字段值导致加载异常。
 */

/* ============================================================
 * 插件配置：在无界/Bncr Web 管理面板 → 插件配置中填写
 * ============================================================ */
const ConfigDB = new BncrPluginConfig(
  BncrCreateSchema.object({
    sharedConfigKey: BncrCreateSchema.string()
      .setTitle('复用插件配置的数据库键')
      .setDescription('读取 oci-panel.js 的配置。默认 /plugins/凯尼尔/oci-panel.js；插件放在别的目录就改这里')
      .setDefault('/plugins/凯尼尔/oci-panel.js'),
    configId: BncrCreateSchema.string()
      .setTitle('配置ID')
      .setDescription('留空则推送全部配置；填写则只推该配置。发「oci 配置」可查看所有ID')
      .setDefault(''),
    pushPlatform: BncrCreateSchema.string()
      .setTitle('推送平台')
      .setDescription('微信填 wechaty；Telegram 机器人填 tgBot，人形 TG 填 HumanTG；QQ 填 qq。留空则由系统决定')
      .setDefault('wechaty'),
    pushUserId: BncrCreateSchema.string()
      .setTitle('推送目标：用户ID')
      .setDescription('定时推送必填之一。私聊自己就填自己的微信ID；发「我的id」可查看。与群ID二选一')
      .setDefault(''),
    pushGroupId: BncrCreateSchema.string()
      .setTitle('推送目标：群ID')
      .setDescription('定时推送必填之一。填了优先发群；不推群就留空。与用户ID二选一')
      .setDefault(''),
    timeout: BncrCreateSchema.number()
      .setTitle('超时（秒）')
      .setDescription('流量接口实时查询较慢，配置多时建议 60 秒以上')
      .setDefault(60)
  })
);

/* ============================================================
 * 凭据：只从 oci-panel.js 的插件配置里读，无兜底
 * ============================================================ */

// 解析后的实际凭据，由 resolveCreds() 填充
let CRED = { panelUrl: '', apiToken: '' };
// 上一次读取共享配置的结果，供「oci 日报 调试」查看
let SHARED_DEBUG = { key: '', ok: false, via: '', error: '', shape: '' };

/**
 * 从 Bncr 数据库读取另一个插件的配置。
 * BncrDB 的构造参数是「表名」，get 的第一个参数是键；但插件配置到底存成
 * 「表名=插件路径」还是「表名=目录、键=文件名」，各版本不一致，
 * 所以这里按常见写法依次尝试，成功即用。
 */
async function readSharedConfig(key) {
  if (!key) return null;
  const tries = [
    ['表名=完整路径 · 键=userConfig', async () => await new BncrDB(key).get('userConfig')],
    ['表名=完整路径 · 键=config', async () => await new BncrDB(key).get('config')],
    ['表名=完整路径 · 无键', async () => await new BncrDB(key).get()],
    ['表名=目录 · 键=文件名', async () => {
      const i = key.lastIndexOf('/');
      if (i <= 0) return null;
      return await new BncrDB(key.slice(0, i)).get(key.slice(i + 1));
    }]
  ];

  const errors = [];
  for (const [via, fn] of tries) {
    try {
      const v = await fn();
      if (v && typeof v === 'object') return { value: v, via, errors };
      if (typeof v === 'string' && v.trim().startsWith('{')) {
        try {
          return { value: JSON.parse(v), via: via + '（JSON 字符串）', errors };
        } catch (_) { /* 不是 JSON，继续 */ }
      }
    } catch (e) {
      errors.push(`${via}: ${e.message}`);
    }
  }
  return { value: null, via: '', errors };
}

/** 从不同结构的配置对象里抠出面板地址与 Token */
function pickCreds(v) {
  if (!v || typeof v !== 'object') return null;
  const cands = [v, v.userConfig, v.config, v.data, v.value];
  for (const c of cands) {
    if (c && typeof c === 'object' && (c.panelUrl || c.apiToken)) {
      return { panelUrl: c.panelUrl || '', apiToken: c.apiToken || '' };
    }
  }
  return null;
}

/** 读取凭据：只认数据库里 oci-panel.js 的配置，没有第二套方案 */
async function resolveCreds() {
  const key = (ConfigDB.userConfig.sharedConfigKey || '').trim();
  SHARED_DEBUG = { key, ok: false, via: '', error: '', shape: '' };
  CRED = { panelUrl: '', apiToken: '' };

  if (!key) {
    SHARED_DEBUG.error = '未填写数据库键';
    return CRED;
  }

  let res = null;
  try {
    res = await readSharedConfig(key);
  } catch (e) {
    SHARED_DEBUG.error = e.message;
    return CRED;
  }

  if (res?.value) {
    SHARED_DEBUG.shape = Object.keys(res.value).slice(0, 8).join(',');
    const creds = pickCreds(res.value);
    if (creds && (creds.panelUrl || creds.apiToken)) {
      CRED = creds;
      SHARED_DEBUG.ok = true;
      SHARED_DEBUG.via = res.via;
      return CRED;
    }
    SHARED_DEBUG.error = `读到数据但没找到面板地址/Token（字段：${SHARED_DEBUG.shape}）`;
  } else if (res?.errors?.length) {
    SHARED_DEBUG.error = res.errors.join(' | ');
  } else {
    SHARED_DEBUG.error = '数据库中没有该键';
  }

  return CRED;
}

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

/** 今天的日期，用于日报标题 */
function today() {
  const d = new Date();
  const p = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Token 脱敏：只留前 6 位 + 长度，便于排查又不泄露 */
const maskToken = t => (!t ? '(空)' : `${String(t).slice(0, 6)}…（共 ${String(t).length} 位）`);

/** 统一请求：拼 URL、带 Bearer 头、超时控制、解包 {code,message,data} 信封 */
async function api(path, options = {}) {
  const { timeout } = ConfigDB.userConfig;
  const url = `${CRED.panelUrl.replace(/\/+$/, '')}/api${path}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), (timeout || 60) * 1000);

  try {
    const res = await fetch(url, {
      method: options.method || 'GET',
      headers: {
        Authorization: `Bearer ${CRED.apiToken}`,
        'Content-Type': 'application/json'
      },
      ...(options.body ? { body: JSON.stringify(options.body) } : {}),
      signal: controller.signal
    });

    if (res.status === 401) throw new Error('Token 无效或已过期，请到面板重新生成');
    if (res.status === 403)
      throw new Error('权限不足：该接口需要 full 权限的 Token（readonly 仅允许 GET）');

    const json = await res.json();
    // 业务层错误：响应信封是 {code, message, data}，成功码是 200（不是 0）
    if (json.code !== 200) throw new Error(json.message || `接口返回 code=${json.code}`);
    return json.data;
  } catch (e) {
    if (e.name === 'AbortError') throw new Error(`请求超时（${timeout || 60}s），可尝试调大超时时间`);
    throw e;
  } finally {
    clearTimeout(timer);
  }
}

/* ============================================================
 * 日报正文
 * ============================================================ */

/** 单个配置的「成本 + 流量」小节 */
async function renderOne(cfg) {
  const lines = [`👤 ${cfg.username || '(未命名)'}${cfg.region ? '　📍 ' + cfg.region : ''}`];

  // ---- 成本 ----
  try {
    const cost = await api('/oci/traffic/cost', {
      method: 'POST',
      body: { configId: cfg.id, days: 30 }
    });
    const cur = cost.currency || '';
    lines.push(`💵 本月累计：${(cost.monthToDate || 0).toFixed(4)} ${cur}`);

    // 只列最近 3 天有花费的，日报没必要铺满 30 行
    const charged = (cost.days || []).filter(x => Number(x.amount) > 0);
    if (charged.length) {
      for (const x of charged.slice(0, 3)) {
        lines.push(`　· ${x.date}　${Number(x.amount).toFixed(4)} ${x.currency || cur}`);
      }
    } else {
      lines.push('　✅ 近期无费用，均在免费额度内');
    }
  } catch (e) {
    lines.push(`💵 成本：查询失败（${e.message}）`);
  }

  // ---- 流量 ----
  try {
    const t = await api('/oci/traffic/monthly', {
      method: 'POST',
      body: { configId: cfg.id, forceRefresh: false }
    });
    // 免费额度只针对「出站」：入站免费，出站超出额度后才计费。
    // 所以已用占比必须拿出站流量去除额度，不能用 billableTraffic（没超额时恒为 0）
    const out = t.outboundTraffic || 0;
    const free = t.freeAllowance || 0;
    const pct = free > 0 ? (out / free) * 100 : 0;
    lines.push(`📊 出站已用：${fmtBytes(out)} / ${fmtBytes(free)}（${pct.toFixed(2)}%）`);
    lines.push(`　↑ 出站 ${fmtBytes(out)}　↓ 入站 ${fmtBytes(t.inboundTraffic)}`);
    if (t.billableTraffic > 0) {
      lines.push(`　⚠️ 超额计费流量：${fmtBytes(t.billableTraffic)}`);
    }
  } catch (e) {
    lines.push(`📊 流量：查询失败（${e.message}）`);
  }

  lines.push('');
  return lines.join('\n');
}

/** 组装整份日报 */
async function buildReport(onlyId) {
  const summary = await api('/bot/summary');
  if (!summary.configs?.length) return '面板中还没有配置任何 OCI 账号';

  let list = summary.configs;
  if (onlyId) {
    list = list.filter(c => c.id === onlyId);
    if (!list.length) return `❌ 找不到配置ID：${onlyId}\n可发送「oci 配置」查看所有ID`;
  }

  const head = [
    `📅 OCI 日报 · ${today()}`,
    `━━━━━━━━━━━━━━━`,
    `实例 ${summary.totalInstances} 台，运行中 ${summary.runningInstances} 台`,
    ''
  ];

  // 串行查询：并发打 OCI 接口容易被限流，且流量接口本身较慢
  const parts = [];
  for (const cfg of list) {
    parts.push(await renderOne(cfg));
  }

  return head.join('\n') + parts.join('') + `—— 由 oci-panel 定时推送`;
}

/* ============================================================
 * 发送
 * ============================================================ */

/**
 * 统一出口：
 * - 填了推送目标 → 用 sysMethod.push 主动推（不依赖 sender，定时场景必须走这条）
 * - 老版本没有 sysMethod.push → 退回 s.reply 并覆盖 userId/groupId
 * - 没填目标 → 直接回当前会话（手动敲命令的场景）
 */
async function send(s, text) {
  const { pushPlatform, pushUserId, pushGroupId } = ConfigDB.userConfig;
  const hasTarget = !!(pushUserId || pushGroupId);
  if (!hasTarget) {
    await s.reply(text);
    return 'reply 当前会话';
  }

  const target = pushGroupId ? { groupId: pushGroupId } : { userId: pushUserId };

  if (typeof sysMethod !== 'undefined' && typeof sysMethod.push === 'function') {
    await sysMethod.push({
      ...(pushPlatform ? { platform: pushPlatform } : {}),
      ...target,
      msg: text,
      type: 'text'
    });
    return `sysMethod.push → ${JSON.stringify(target)}`;
  }

  await s.reply({ type: 'text', msg: text, ...target });
  return `s.reply 覆盖目标 → ${JSON.stringify(target)}（未找到 sysMethod.push）`;
}

/* ============================================================
 * 主入口：命令触发与 @cron 定时触发共用
 * ============================================================ */
module.exports = async s => {
  await ConfigDB.get();
  await resolveCreds();

  // 调试：查看凭据到底从哪来的
  const raw = (s.getMsg() || '').trim();
  const m = raw.match(/^oci\s+日报\s*(\S+)?\s*$/);
  const arg = m?.[1] || '';

  if (arg === '调试') {
    return await s.reply([
      '🔍 日报插件调试信息',
      '━━━━━━━━━━━━━━━',
      `数据库键：${SHARED_DEBUG.key || '(未填)'}`,
      `读取结果：${SHARED_DEBUG.ok ? '✅ 成功' : '❌ 失败'}`,
      `读取方式：${SHARED_DEBUG.via || '-'}`,
      `面板地址：${CRED.panelUrl || '(空)'}`,
      `Token：${maskToken(CRED.apiToken)}`,
      `配置ID：${ConfigDB.userConfig.configId || '(空=全部配置)'}`,
      `推送平台：${ConfigDB.userConfig.pushPlatform || '(系统默认)'}`,
      `推送目标：${ConfigDB.userConfig.pushGroupId ? '群 ' + ConfigDB.userConfig.pushGroupId : ConfigDB.userConfig.pushUserId ? '用户 ' + ConfigDB.userConfig.pushUserId : '(未填=回当前会话)'}`,
      SHARED_DEBUG.error ? `错误：${SHARED_DEBUG.error}` : '',
      SHARED_DEBUG.shape ? `读到字段：${SHARED_DEBUG.shape}` : ''
    ].filter(Boolean).join('\n'));
  }

  if (!CRED.panelUrl || !CRED.apiToken) {
    return await s.reply([
      '❌ 数据库里没读到面板地址或 Token',
      '',
      `共享配置键：${SHARED_DEBUG.key || '(未填)'}`,
      SHARED_DEBUG.error ? `原因：${SHARED_DEBUG.error}` : '',
      SHARED_DEBUG.shape ? `读到字段：${SHARED_DEBUG.shape}` : '',
      '',
      '处理办法：',
      '1. 确认 oci-panel.js 已经配置好面板地址与 API Token',
      '2. 确认本插件的「复用插件配置的数据库键」填的是 oci-panel.js 的实际路径',
      '　默认 /plugins/凯尼尔/oci-panel.js；插件放在别的目录就跟着改',
      '3. 改完发「oci 日报 调试」看读取结果'
    ].filter(Boolean).join('\n'));
  }

  const onlyId = arg || ConfigDB.userConfig.configId || '';

  try {
    const text = await buildReport(onlyId);
    await send(s, text);
  } catch (e) {
    BncrJSLogger.error('[oci-daily-report] 生成日报失败:', e.message);
    try {
      await send(s, `❌ 日报生成失败\n${e.message}\n\n请检查：面板地址是否可访问、Token 是否有效`);
    } catch (_) { /* 发送失败就不再递归，交给日志 */ }
  }
};
