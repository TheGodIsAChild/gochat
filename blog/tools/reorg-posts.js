/**
 * 博客结构重构脚本：
 *   1. 统一文件名：类目号-序号-名称.md（01-入门 / 02-设计 / 03-工程 / 04-里程碑 / 05-源码 / 06-面试）
 *   2. 分类收敛为 6 个
 *   3. 按学习路径重排日期（首页从上到下 = 学习顺序）
 *   4. 自动修复所有内部链接（旧 slug → 新 slug）
 *   5. 生成「文章目录」页
 *
 * 运行：node blog/scripts/reorg-posts.js
 */
const fs = require('fs');
const path = require('path');

const POSTS = path.resolve(__dirname, '../source/_posts');
const SOURCE = path.resolve(__dirname, '../source');

const CATS = {
  '01': '01-入门',
  '02': '02-设计',
  '03': '03-工程',
  '04': '04-里程碑',
  '05': '05-源码',
  '06': '06-面试',
};

// [旧文件名, 新文件名, 类目号]
const map = [
  // 01 入门
  ['00-课程导读-从0到1怎么学.md', '01-01-课程导读-从0到1怎么学.md', '01'],
  ['90-学习路线图.md', '01-02-学习路线图.md', '01'],
  ['01-环境准备与项目启动-逐行.md', '01-03-环境准备与项目启动.md', '01'],
  ['02-一次HTTP请求的完整链路-逐行.md', '01-04-一次HTTP请求的完整链路.md', '01'],
  ['03-代码地图-每个文件是干什么的.md', '01-05-代码地图.md', '01'],
  ['04-踩坑集-24个真实坑.md', '01-06-踩坑集.md', '01'],
  // 02 设计
  ['20-架构总览-从单体到微服务.md', '02-01-架构总览.md', '02'],
  ['21-数据库表设计-12张表逐张讲.md', '02-02-数据库表设计.md', '02'],
  ['22-并发编程实战-7个并发场景.md', '02-03-并发编程实战.md', '02'],
  ['23-WebSocket全解-握手心跳重连与wss.md', '02-04-WebSocket全解.md', '02'],
  ['24-Redis在本项目的5种用法.md', '02-05-Redis用法.md', '02'],
  ['25-MySQL索引与查询优化实战.md', '02-06-MySQL索引优化.md', '02'],
  // 03 工程
  ['26-开发模式-Go后端常用的8种模式.md', '03-01-开发模式.md', '03'],
  ['27-接到需求怎么动手-标准开发流程.md', '03-02-开发流程.md', '03'],
  ['28-接口设计规范与错误码.md', '03-03-接口设计规范.md', '03'],
  ['29-错误处理与日志-Go工程化实践.md', '03-04-错误处理与日志.md', '03'],
  ['30-配置管理与优雅降级.md', '03-05-配置管理与降级.md', '03'],
  ['31-测试与调试-怎么验证代码是对的.md', '03-06-测试与调试.md', '03'],
  // 04 里程碑
  ['01-项目介绍与架构.md', '04-01-项目介绍与架构.md', '04'],
  ['02-认证与好友.md', '04-02-认证与好友.md', '04'],
  ['03-实时消息与WebSocket.md', '04-03-实时消息与WebSocket.md', '04'],
  ['04-群聊设计与权限.md', '04-04-群聊设计与权限.md', '04'],
  ['05-并发加固与优雅退出.md', '04-05-并发加固与优雅退出.md', '04'],
  ['06-WebRTC语音通话.md', '04-06-WebRTC语音通话.md', '04'],
  ['07-微服务与消息队列.md', '04-07-微服务与消息队列.md', '04'],
  ['08-仿微信前端与面试准备.md', '04-08-仿微信前端与面试准备.md', '04'],
  ['91-M1-认证与好友-详细设计.md', '04-09-M1详细设计.md', '04'],
  ['92-M2-单聊与WebSocket-详细设计.md', '04-10-M2详细设计.md', '04'],
  ['93-M3-群聊-详细设计.md', '04-11-M3详细设计.md', '04'],
  ['94-M4-并发加固-详细设计.md', '04-12-M4详细设计.md', '04'],
  ['95-M5-WebRTC语音通话-详细设计.md', '04-13-M5详细设计.md', '04'],
  ['96-M6-微服务化-gRPC-详细设计.md', '04-14-M6详细设计.md', '04'],
  ['97-M7-RabbitMQ与最终一致性-详细设计.md', '04-15-M7详细设计.md', '04'],
  // 06 面试（先放，05 源码在下面批量生成）
  ['98-面试集-72题.md', '06-01-面试集72题.md', '06'],
  ['99-场景集-30个业务场景.md', '06-02-场景集30个.md', '06'],
  ['100-项目总结与面试话术.md', '06-03-项目总结与面试话术.md', '06'],
];

// 05 源码：S00-S21 → 05-00 ~ 05-21
const srcNames = [
  'S00-源码走读总索引', 'S01-源码-main', 'S02-源码-init-config', 'S03-源码-init-mysql',
  'S04-源码-global', 'S05-源码-auth', 'S06-源码-jwt', 'S07-源码-token-store',
  'S08-源码-register-service', 'S09-源码-login-service', 'S10-源码-refresh-service',
  'S11-源码-friend-service', 'S12-源码-message-service', 'S13-源码-group-service',
  'S14-源码-call-service', 'S15-源码-hub', 'S16-源码-client', 'S17-源码-handler',
  'S18-源码-push', 'S19-源码-server', 'S20-源码-mq', 'S21-源码-outbox',
];
srcNames.forEach((n, i) => {
  const no = String(i).padStart(2, '0');
  map.push([`${n}.md`, `05-${no}-${n.replace(/^S\d+-/, '')}.md`, '05']);
});

// ---------- 1. 重命名 + 重写 front-matter ----------
const BASE = new Date('2026-11-14T23:59:00');
const slugMap = {};      // 旧 slug -> 新 slug
const order = [];        // 目录页数据
const HEAD = {};

function rewriteFrontMatter(md, dateStr, cat) {
  const m = md.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?/);
  if (!m) return md;
  let head = m[1];
  head = head.replace(/^date:.*$/m, `date: ${dateStr}`);
  head = head.replace(/categories:\s*\r?\n(?:[ \t]*-[^\r\n]*\r?\n)*/m, `categories:\n  - ${cat}\n`);
  return `---\n${head}\n---\n` + md.slice(m[0].length);
}

map.forEach(([oldName, newName, cat], i) => {
  const oldPath = path.join(POSTS, oldName);
  if (!fs.existsSync(oldPath)) { console.warn('跳过(不存在):', oldName); return; }
  const md = fs.readFileSync(oldPath, 'utf8');

  const d = new Date(BASE.getTime() - i * 60000);
  const pad = n => String(n).padStart(2, '0');
  const dateStr = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;

  const out = rewriteFrontMatter(md, dateStr, CATS[cat]);
  fs.writeFileSync(path.join(POSTS, newName), out, 'utf8');
  if (oldName !== newName) fs.unlinkSync(oldPath);

  const oldSlug = oldName.replace(/\.md$/, '');
  slugMap[oldSlug] = newSlug = newName.replace(/\.md$/, '');

  const titleMatch = md.match(/^title:\s*"?(.*?)"?\s*$/m);
  order.push({ cat, no: newName.slice(3, 5), title: titleMatch ? titleMatch[1] : newName, slug: newName.replace(/\.md$/, '') });
});

// ---------- 2. 修复内部链接 ----------
const files = fs.readdirSync(POSTS).filter(f => f.endsWith('.md'));
const extra = [path.join(SOURCE, 'about/index.md')];
let fixed = 0;

function fixLinks(content) {
  return content.replace(/\(\/blog\/2026\/11\/(?:12|13|14)\/([^/)\s]+)\/\)/g, (all, slug) => {
    const nu = slugMap[slug];
    if (nu) { fixed++; return `(/blog/2026/11/14/${nu}/)`; }
    return all;
  });
}

files.forEach(f => {
  const p = path.join(POSTS, f);
  const before = fs.readFileSync(p, 'utf8');
  const after = fixLinks(before);
  if (after !== before) fs.writeFileSync(p, after, 'utf8');
});
extra.forEach(p => {
  if (!fs.existsSync(p)) return;
  const before = fs.readFileSync(p, 'utf8');
  const after = fixLinks(before);
  if (after !== before) fs.writeFileSync(p, after, 'utf8');
});

// ---------- 3. 生成文章目录页 ----------
const groups = [
  ['01', '01 入门篇 —— 先跑起来，再读懂骨架'],
  ['02', '02 设计篇 —— 架构、库表与关键机制'],
  ['03', '03 工程篇 —— 开发模式与工程实践'],
  ['04', '04 里程碑篇 —— 项目介绍与 M1–M7'],
  ['05', '05 源码篇 —— 核心文件完整源码'],
  ['06', '06 面试篇 —— 面试题、场景题与话术'],
];

let catalog = '> 全站 58 篇文章的完整索引。**按顺序读 = 按学习路径学**。\n\n';
for (const [cat, label] of groups) {
  const items = order.filter(o => o.cat === cat).sort((a, b) => a.no.localeCompare(b.no));
  if (!items.length) continue;
  catalog += `## ${label}\n\n| 序号 | 文章 |\n|---|---|\n`;
  items.forEach(o => {
    catalog += `| ${o.no} | [${o.title}](/blog/2026/11/14/${o.slug}/) |\n`;
  });
  catalog += '\n';
}

fs.mkdirSync(path.join(SOURCE, 'catalog'), { recursive: true });
fs.writeFileSync(path.join(SOURCE, 'catalog/index.md'),
  '---\ntitle: 文章目录\ndate: 2026-11-15 00:00:00\nlayout: page\ncomments: false\n---\n\n' + catalog,
  'utf8');

console.log(`重命名/生成 ${Object.keys(slugMap).length} 篇，修复内部链接 ${fixed} 处，已生成文章目录页`);
