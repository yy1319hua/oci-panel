/**
 * @author yy1319hua
 * @name db-probe
 * @team oci-panel
 * @version 1.0.0
 * @description 诊断 BncrDB 读取：列出数据库实例、数据表、键名，验证插件配置到底存在哪、该怎么读。
 * @rule ^oci\s+探测$
 * @priority 200
 * @admin true
 * @public false
 * @classification ["甲骨文"]
 *
 * ⚠️ 一次性排查工具，用完即删。
 * 用途：确认 oci-daily-report.js 到底该用哪种方式读 oci-panel.js 的插件配置。
 * 用法：对机器人发「oci 探测」，把返回内容贴出来即可。
 *
 * ⚠️ 头部注解只能用单空格分隔，@name 必须与文件名一致（db-probe）。
 */

/* ============================================================
 * 常量
 * ============================================================ */

// 微信单条消息别太长，超出会被截断
const MAX_OUT = 2600;
// 目标插件的默认键（相对工作目录的插件路径）
const TARGET = '/plugins/凯尼尔/oci-panel.js';

/* ============================================================
 * 主入口
 * ============================================================ */
module.exports = async s => {
  const out = [];
  const line = (...a) => out.push(a.join(''));
  const cut = (t, n = 300) => {
    t = String(t === undefined ? 'undefined' : t);
    return t.length > n ? `${t.slice(0, n)}…（共${t.length}字符）` : t;
  };
  const dump = v => {
    if (v === undefined) return 'undefined';
    if (v === null) return 'null';
    if (typeof v === 'string') return v;
    try { return JSON.stringify(v); } catch (_) { return String(v); }
  };
  // 值可能包在 userConfig / config 里，也可能就是扁平的
  const flat = v => (v && typeof v === 'object' && (v.userConfig || v.config) ? (v.userConfig || v.config) : v);

  line('🔬 BncrDB 读取诊断');
  line('━━━━━━━━━━━━━━━');

  /* ---------- A. 数据库实例 ---------- */
  const hasDIO = typeof DatabaseInstantiationObject !== 'undefined' && !!DatabaseInstantiationObject;
  line(`A1 DatabaseInstantiationObject: ${hasDIO ? '✅ 存在' : '❌ 不存在（未注入）'}`);
  if (hasDIO) {
    line(`A2 已注册实例: ${Object.keys(DatabaseInstantiationObject).join(', ') || '(空)'}`);
  }

  const info = hasDIO ? DatabaseInstantiationObject['pluginConfig'] : null;
  line(`A3 pluginConfig 实例: ${info ? '✅ 拿到' : '❌ 拿不到'}`);

  /* ---------- B. 用正确实例读 PluginConfig 表 ---------- */
  line('');
  line('【B】正确写法：new BncrDB("PluginConfig", pluginConfig实例)');

  let db = null;
  try {
    db = info ? new BncrDB('PluginConfig', info) : new BncrDB('PluginConfig');
    line(`B1 实例创建: ✅（${info ? '带第二参数' : '⚠️ 退化：无第二参数，可能读错库'}）`);
  } catch (e) {
    line(`B1 实例创建: ❌ ${e.message}`);
  }

  let keys = [];
  if (db) {
    try {
      keys = (await db.keys()) || [];
      line(`B2 表内键数: ${keys.length}`);
    } catch (e) {
      line(`B2 keys() 失败: ${e.message}`);
    }

    // 先找跟 oci / panel 有关的键
    const hit = keys.filter(k => /oci|panel/i.test(k));
    line(`B3 命中 oci/panel 的键: ${hit.length ? hit.join(' | ') : '(无)'}`);
    line(`B4 全部键（最多60个）: ${cut(keys.slice(0, 60).join(' | '), 700) || '(空)'}`);

    // 逐个取值看结构
    const cands = (hit.length ? hit : keys).slice(0, 5);
    for (const k of cands) {
      let v = null;
      try {
        v = await db.get(k);
      } catch (e) {
        line('');
        line(`【${k}】读取失败: ${e.message}`);
        continue;
      }
      const f = flat(v);
      line('');
      line(`【${k}】`);
      line(`  类型: ${Array.isArray(v) ? 'array' : typeof v}${f !== v ? '（值被包裹，已展开）' : ''}`);
      if (v && typeof v === 'object') {
        line(`  字段: ${cut(Object.keys(f || {}).join(','), 240)}`);
      }
      const p = f?.panelUrl;
      const t = f?.apiToken;
      line(`  panelUrl: ${p ? cut(p, 90) : '(无)'}`);
      line(`  apiToken: ${t ? `${String(t).slice(0, 6)}…（共${String(t).length}位）` : '(无)'}`);
    }
  }

  /* ---------- C. 直接读目标键（多种写法对照） ---------- */
  line('');
  line('【C】直接读目标键，写法对照');

  const variants = [
    ['正确：PluginConfig + 实例 · 键=路径', async () => (info ? new BncrDB('PluginConfig', info) : new BncrDB('PluginConfig')).get(TARGET)],
    ['去掉前导斜杠', async () => (info ? new BncrDB('PluginConfig', info) : new BncrDB('PluginConfig')).get(TARGET.replace(/^\//, ''))],
    ['默认库 · 表=PluginConfig · 键=路径', async () => new BncrDB('PluginConfig').get(TARGET)],
    ['❌旧写法：表=路径 · 键=userConfig', async () => new BncrDB(TARGET).get('userConfig')],
    ['❌旧写法：表=路径 · 无键', async () => new BncrDB(TARGET).get()]
  ];

  for (const [name, fn] of variants) {
    try {
      const v = await fn();
      const hit = v === undefined || v === null ? '❌ undefined/null' : `✅ ${cut(dump(v), 160)}`;
      line(`C· ${name}`);
      line(`   → ${hit}`);
    } catch (e) {
      line(`C· ${name}`);
      line(`   → 抛错: ${cut(e.message, 120)}`);
    }
  }

  /* ---------- D. 默认库有哪些表 ---------- */
  line('');
  try {
    const forms = await new BncrDB('anyTable').getAllForm();
    line(`D 默认库的数据表: ${cut((forms || []).join(', '), 400) || '(空)'}`);
  } catch (e) {
    line(`D getAllForm() 失败: ${cut(e.message, 120)}`);
  }

  line('');
  line('—— 把以上内容整段发出来即可');

  await s.reply(out.join('\n').slice(0, MAX_OUT));
};
