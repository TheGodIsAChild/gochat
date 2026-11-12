/**
 * 最终整理脚本：
 *   1. 按文件名顺序重排所有文章的 date（首页从上到下 = 01-01 → 06-03）
 *   2. 生成「文章目录」页（带难度、时长、适合人群，面向小白）
 *
 * 运行：node tools/finalize.js
 */
const fs = require('fs');
const path = require('path');

const POSTS = path.resolve(__dirname, '../source/_posts');
const SOURCE = path.resolve(__dirname, '../source');

// ---------- 1. 重排日期 ----------
const files = fs.readdirSync(POSTS).filter(f => f.endsWith('.md')).sort();
const BASE = new Date('2026-11-14T23:59:00');
const pad = n => String(n).padStart(2, '0');

const meta = [];  // { file, slug, title, cat, no }

files.forEach((f, i) => {
  const p = path.join(POSTS, f);
  let md = fs.readFileSync(p, 'utf8');
  const d = new Date(BASE.getTime() - i * 60000);
  const dateStr = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
                  `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;

  md = md.replace(/^date:.*$/m, `date: ${dateStr}`);
  fs.writeFileSync(p, md, 'utf8');

  const catMatch = md.match(/categories:\s*\r?\n\s*-\s*([^\r\n]+)/);
  const titleMatch = md.match(/^title:\s*"?(.*?)"?\s*$/m);
  meta.push({
    file: f,
    slug: f.replace(/\.md$/, ''),
    title: titleMatch ? titleMatch[1] : f,
    cat: catMatch ? catMatch[1].trim() : '未分类',
    no: f.slice(3, 5),
  });
});

// ---------- 2. 分类元信息（面向小白） ----------
const CAT_META = {
  '01-入门': { emoji: '①', label: '入门篇', diff: '⭐ 入门', time: '15–40 分钟', who: '第一次接触本项目，先跑通再理解' },
  '02-设计': { emoji: '②', label: '设计篇', diff: '⭐⭐ 进阶', time: '30–50 分钟', who: '已经能跑通，想搞懂架构与关键机制' },
  '03-工程': { emoji: '③', label: '工程篇', diff: '⭐⭐ 进阶', time: '25–40 分钟', who: '想知道「真实项目怎么开发」' },
  '04-里程碑': { emoji: '④', label: '里程碑篇', diff: '⭐⭐ 进阶', time: '40–70 分钟', who: '按 M1–M7 逐个击破' },
  '05-源码': { emoji: '⑤', label: '源码篇', diff: '⭐⭐⭐ 深入', time: '按需查阅', who: '要逐行读实现的阶段' },
  '06-面试': { emoji: '⑥', label: '面试篇', diff: '⭐⭐ 进阶', time: '20–40 分钟', who: '准备面试 / 复盘项目' },
};

// 入门篇各篇的一句话说明
const INTRO_DESC = {
  '01-01': '学习地图：每阶段读什么、怎么判断学会了',
  '01-02': '速查表：接口、表、端口、命令一页看完',
  '01-03': '环境搭建：Docker + 配置 + 启动 + 第一个请求',
  '01-04': '**核心**：一条请求从 curl 到 MySQL 的 9 个环节（逐行）',
  '01-05': '代码地图：每个文件是干什么的',
  '01-06': '踩坑集：24 个真实坑 + 排错方法',
};

const lines = [];
lines.push('> **第一次来？按下面 4 步走，2 周能读懂这个项目。**');
lines.push('');
lines.push('| 步骤 | 做什么 | 看哪篇 |');
lines.push('|---|---|---|');
lines.push('| 第 1 步 | 先把项目跑起来，亲手发一条消息 | [环境准备与项目启动](/blog/2026/11/14/01-03-环境准备与项目启动/) |');
lines.push('| 第 2 步 | 搞懂一条请求怎么走完全程 | [一次 HTTP 请求的完整链路](/blog/2026/11/14/01-04-一次HTTP请求的完整链路/) |');
lines.push('| 第 3 步 | 按 M1–M7 逐个理解模块 | [里程碑篇](#4-里程碑篇) |');
lines.push('| 第 4 步 | 练面试表达 | [面试篇](#6-面试篇) |');
lines.push('');
lines.push('---');
lines.push('');

const groups = ['01-入门', '02-设计', '03-工程', '04-里程碑', '05-源码', '06-面试'];

for (const cat of groups) {
  const m = CAT_META[cat];
  const items = meta.filter(x => x.cat === cat).sort((a, b) => a.no.localeCompare(b.no));
  if (!items.length) continue;

  lines.push(`## ${m.emoji} ${m.label}`);
  lines.push('');
  lines.push(`**难度**：${m.diff}　**单篇时长**：${m.time}　**适合**：${m.who}`);
  lines.push('');

  if (cat === '05-源码') {
    lines.push('> 源码篇是**参考手册**，不需要从头读到尾。建议先读 `05-00 总索引`，再按需查具体文件。');
    lines.push('');
  }

  lines.push('| # | 文章 | 说明 |');
  lines.push('|---|---|---|');
  for (const it of items) {
    const desc = INTRO_DESC[it.no] || (cat === '04-里程碑' ? '里程碑详解' : cat === '05-源码' ? '完整源码（参考）' : '');
    lines.push(`| ${it.no} | [${it.title}](/blog/2026/11/14/${it.slug}/) | ${desc} |`);
  }
  lines.push('');
}

lines.push('---');
lines.push('');
lines.push('## 学习节奏建议');
lines.push('');
lines.push('| 周 | 每天 1.5 小时做什么 |');
lines.push('|---|---|');
lines.push('| 第 1 周 | 入门篇全部 + 里程碑 M1/M2 |');
lines.push('| 第 2 周 | 里程碑 M3–M7 + 设计篇 |');
lines.push('| 第 3 周 | 工程篇 + 源码篇（挑重点）+ 面试篇 |');
lines.push('');
lines.push('> 判断「学会了」的标准不是「看完了」，而是能合上电脑回答：');
lines.push('> **这个模块解决什么问题？怎么解决的？有什么坑？**');

fs.mkdirSync(path.join(SOURCE, 'catalog'), { recursive: true });
fs.writeFileSync(
  path.join(SOURCE, 'catalog/index.md'),
  '---\ntitle: 文章目录\ndate: 2026-11-15 00:00:00\nlayout: page\ncomments: false\n---\n\n' + lines.join('\n') + '\n',
  'utf8'
);

console.log(`重排日期 ${files.length} 篇，已生成目录页（${groups.length} 个分类）`);
