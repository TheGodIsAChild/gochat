/**
 * 把 gochat-study 下已有的学习文档转换成 Hexo 博客文章。
 *
 * 为什么用脚本：这些文档已经写得很详细（每篇 7~12KB，含思路/代码导读/注意点/面试点），
 * 直接转换成博客文章即可，避免重复写作、节省时间。
 *
 * 运行：node blog/scripts/convert-docs.js
 */
const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '../..');        // gochat/
const STUDY = path.resolve(ROOT, '../gochat-study');  // gochat-study/
const OUT = path.resolve(__dirname, '../source/_posts');

// 源文档 → 博客文章配置
const items = [
  {
    src: 'learn/00-学习总览.md',
    out: '90-学习路线图.md',
    title: '学习路线图：这个项目该怎么学',
    cats: ['开始之前'], tags: ['学习路线', '阅读方法'],
    date: '2026-11-13 08:00:00',
    img: '/blog/images/layers.svg',
    imgAlt: '分层架构：依赖只能单向向下',
  },
  {
    src: 'learn/m1-认证与好友.md',
    out: '91-M1-认证与好友-详细设计.md',
    title: 'M1 详细设计：认证与好友（双 token / 事务 / 软删除）',
    cats: ['里程碑'], tags: ['JWT', 'Redis', '事务', 'GORM', '好友'],
    date: '2026-11-13 08:10:00',
    img: '/blog/images/auth-flow.svg',
    imgAlt: '双 Token 无感刷新流程',
  },
  {
    src: 'learn/m2-单聊与WebSocket.md',
    out: '92-M2-单聊与WebSocket-详细设计.md',
    title: 'M2 详细设计：单聊与 WebSocket（幂等 / 分页 / 读写泵）',
    cats: ['里程碑'], tags: ['WebSocket', '幂等', '分页', '并发'],
    date: '2026-11-13 08:20:00',
    img: '/blog/images/websocket.svg',
    imgAlt: 'WebSocket 连接管理架构',
  },
  {
    src: 'learn/m3-群聊.md',
    out: '93-M3-群聊-详细设计.md',
    title: 'M3 详细设计：群聊（建群事务 / 权限 / 人数统计）',
    cats: ['里程碑'], tags: ['群聊', '事务', '权限'],
    date: '2026-11-13 08:30:00',
  },
  {
    src: 'learn/m4-并发加固.md',
    out: '94-M4-并发加固-详细设计.md',
    title: 'M4 详细设计：并发加固（优雅退出 / 限流 / goroutine）',
    cats: ['里程碑'], tags: ['并发', '限流', '优雅退出', 'goroutine'],
    date: '2026-11-13 08:40:00',
  },
  {
    src: 'learn/m5-WebRTC语音信令.md',
    out: '95-M5-WebRTC语音通话-详细设计.md',
    title: 'M5 详细设计：WebRTC 语音通话（信令 / SDP / ICE）',
    cats: ['里程碑'], tags: ['WebRTC', '信令', 'P2P'],
    date: '2026-11-13 08:50:00',
    img: '/blog/images/call-flow.svg',
    imgAlt: 'WebRTC 通话信令时序',
  },
  {
    src: 'learn/m6-微服务化-gRPC.md',
    out: '96-M6-微服务化-gRPC-详细设计.md',
    title: 'M6 详细设计：微服务化与 gRPC',
    cats: ['里程碑'], tags: ['gRPC', '微服务', 'protobuf'],
    date: '2026-11-13 09:00:00',
    img: '/blog/images/architecture.svg',
    imgAlt: 'gochat 系统架构',
  },
  {
    src: 'learn/m7-RabbitMQ与最终一致性.md',
    out: '97-M7-RabbitMQ与最终一致性-详细设计.md',
    title: 'M7 详细设计：RabbitMQ 与 Outbox 最终一致性',
    cats: ['里程碑'], tags: ['RabbitMQ', 'Outbox', '最终一致性'],
    date: '2026-11-13 09:10:00',
    img: '/blog/images/outbox.svg',
    imgAlt: 'Outbox 模式异步投递链路',
  },
  {
    src: '面试集.md',
    out: '98-面试集-72题.md',
    title: '面试集：72 道高频题（含简答与代码位置）',
    cats: ['面试准备'], tags: ['面试', '八股'],
    date: '2026-11-13 09:20:00',
  },
  {
    src: '场景集.md',
    out: '99-场景集-30个业务场景.md',
    title: '场景集：30 个业务场景怎么实现',
    cats: ['面试准备'], tags: ['场景题', '设计'],
    date: '2026-11-13 09:30:00',
  },
  {
    src: '项目总结与面试话术.md',
    out: '100-项目总结与面试话术.md',
    title: '项目总结与面试话术：怎么讲这个项目',
    cats: ['面试准备'], tags: ['面试', '话术', '复盘'],
    date: '2026-11-13 09:40:00',
  },
];

function stripFirstHeading(md) {
  // 去掉文档开头的一级标题（Hexo 会用 front-matter 的 title 渲染标题）
  return md.replace(/^\s*#\s+.*\r?\n/, '').replace(/^\s*\r?\n/, '');
}

let ok = 0;
for (const item of items) {
  const srcPath = path.join(STUDY, item.src);
  if (!fs.existsSync(srcPath)) {
    console.warn('跳过（不存在）:', item.src);
    continue;
  }
  let body = fs.readFileSync(srcPath, 'utf8');
  body = stripFirstHeading(body);

  const imgBlock = item.img
    ? `![${item.imgAlt || ''}](${item.img})\n\n> 上图由项目手绘 SVG 说明，配合下文阅读效果更好。\n\n`
    : '';

  const fm = [
    '---',
    `title: ${item.title}`,
    `date: ${item.date}`,
    'categories:',
    ...item.cats.map(c => `  - ${c}`),
    'tags:',
    ...item.tags.map(t => `  - ${t}`),
    '---',
    '',
  ].join('\n');

  fs.writeFileSync(path.join(OUT, item.out), fm + imgBlock + body, 'utf8');
  ok++;
  console.log('生成:', item.out);
}
console.log(`\n完成：${ok} 篇文章`);
