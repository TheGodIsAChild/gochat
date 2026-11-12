/**
 * 生成「源码走读」系列文章。
 *
 * 思路：项目源码里本身就有大量【思路】【注意点】【面试点】中文注释，
 * 把每个核心文件的完整源码搬上博客，读者可以逐行对照阅读。
 * 这样既做到「事无巨细」，又不需要重复写作。
 *
 * 运行：node blog/scripts/gen-source-posts.js
 */
const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '../..');       // gochat/
const OUT = path.resolve(__dirname, '../source/_posts');
const BASE_DATE = new Date('2026-11-12T18:00:00');

// 顺序、源文件、一句话职责
const files = [
  ['S01', 'main.go', '程序入口：加载配置 → 初始化服务 → 启动 HTTP/gRPC/MQ → 优雅退出'],
  ['S02', 'core/init_config.go', '配置加载：命令行参数、读 YAML、初始化/关闭所有基础服务'],
  ['S03', 'core/db/init_mysql.go', 'GORM 连接 MySQL：DSN、连接池、AutoMigrate 建表'],
  ['S04', 'global/global.go', '全局状态：Config / DB / Redis / MQ 的位置'],
  ['S05', 'internal/middleware/auth.go', 'JWT 鉴权中间件：解析 Bearer、校验 type、写 context'],
  ['S06', 'internal/auth/jwt.go', '双 Token 签发与解析：Claims、HS256、jti、算法校验'],
  ['S07', 'internal/auth/token_store.go', 'Refresh token 的 Redis 存储：GETDEL 原子消费、SCAN 批量删'],
  ['S08', 'internal/service/user_service/register_service.go', '注册：查重 + bcrypt + 事务（用户 + 默认设置）'],
  ['S09', 'internal/service/user_service/login_service.go', '登录：校验密码 → 签发双 token → 存 jti → 更新登录时间'],
  ['S10', 'internal/service/user_service/refresh_service.go', '刷新：解析 refresh → GETDEL 消费 → rotation → 复用检测'],
  ['S11', 'internal/service/friend_service/friend_service.go', '好友全业务：申请、接受（事务+条件更新）、列表、删除、黑名单'],
  ['S12', 'internal/service/chat_service/message_service.go', '消息：幂等发送（client_msg_id）、事务、游标分页'],
  ['S13', 'internal/service/group_service/group_service.go', '群聊：建群事务、成员管理、群主权限、解散'],
  ['S14', 'internal/service/call_service/call_service.go', '通话状态机：invite/accept/reject/cancel/end + 时长计算'],
  ['S15', 'internal/ws/hub.go', 'WebSocket 连接管理：userID → 多连接、RWMutex、优雅关闭'],
  ['S16', 'internal/ws/client.go', '单个连接：读泵/写泵/心跳/慢消费者隔离/通话信令处理'],
  ['S17', 'internal/ws/handler.go', 'WebSocket 握手：token 鉴权 → 升级连接 → 启动读写 goroutine'],
  ['S18', 'internal/ws/push.go', '消息扇出：给会话成员推送（信号量限制并发）'],
  ['S19', 'internal/grpcx/server.go', 'gRPC 微服务：服务端分发、客户端 Invoke、生命周期'],
  ['S20', 'internal/mq/mq.go', 'MQ：Outbox worker 轮询发布 + Consumer 消费推送'],
  ['S21', 'internal/outbox/outbox.go', '本地消息表写入：在业务事务里写事件记录'],
];

function escCode(code) {
  // 防止源码里出现 ``` 破坏 Markdown 代码块（Go 源码一般不会有，保险起见处理）
  return code.replace(/```/g, '\\`\\`\\`');
}

const index = [];

files.forEach(([no, src, desc], i) => {
  const srcPath = path.join(ROOT, src);
  if (!fs.existsSync(srcPath)) {
    console.warn('跳过（不存在）:', src);
    return;
  }
  const code = fs.readFileSync(srcPath, 'utf8');
  const lineCount = code.split('\n').length;
  const slug = `${no}-源码-${path.basename(src, '.go').replace(/_/g, '-')}`;
  const date = new Date(BASE_DATE.getTime() - i * 60000);

  const body = [
    `> **文件路径**：\`${src}\`　**行数**：${lineCount} 行`,
    `> **职责**：${desc}`,
    '',
    '本文是项目的**完整源码**（未删减）。项目源码里保留了',
    '**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，',
    '遇到不理解的函数名，回看对应里程碑的设计文章。',
    '',
    '---',
    '',
    '```go',
    escCode(code),
    '```',
    '',
  ].join('\n');

  const fm = [
    '---',
    `title: "源码走读 ${no}：${path.basename(src)}"`,
    `date: ${date.toISOString().slice(0, 10)} ${date.toTimeString().slice(0, 8)}`,
    'categories:',
    '  - 源码走读',
    'tags:',
    '  - 源码',
    '---',
    '',
  ].join('\n');

  fs.writeFileSync(path.join(OUT, `${slug}.md`), fm + body, 'utf8');
  index.push(`| ${no} | [${src}](/blog/2026/11/12/${slug}/) | ${lineCount} 行 | ${desc} |`);
  console.log('生成:', slug + '.md');
});

// 生成索引文章
if (index.length) {
  const idxBody = [
    '> 本页汇总了项目的**全部核心源码**（每个文件都是完整未删减的版本）。',
    '',
    '项目源码里保留了 **【思路】【注意点】【面试点】** 三类中文注释。',
    '建议阅读顺序：先看 [一次 HTTP 请求的完整链路](/blog/2026/11/13/02-一次HTTP请求的完整链路-逐行/)，',
    '再按下面的顺序读源码。',
    '',
    '| 序号 | 文件 | 规模 | 职责 |',
    '|---|---|---|---|',
    ...index,
    '',
    '---',
    '',
    '## 怎么读源码',
    '',
    '1. **先读函数签名**：这个函数接收什么、返回什么',
    '2. **再读注释**：`//` 后面的【思路】【注意点】【面试点】',
    '3. **最后读实现**：一行一行看它怎么做',
    '4. **带着问题读**：比如「这里为什么要用事务？」「这里为什么要加锁？」',
    '',
    '遇到不熟的语言特性（goroutine、defer、channel、interface），',
    '回看 [Go 语法速查](/blog/2026/11/13/90-学习路线图/) 或直接问。',
  ].join('\n');

  const fm = [
    '---',
    'title: "源码走读总索引：核心文件全览"',
    'date: 2026-11-12 19:00:00',
    'categories:',
    '  - 源码走读',
    'tags:',
    '  - 源码',
    '  - 索引',
    '---',
    '',
  ].join('\n');

  fs.writeFileSync(path.join(OUT, 'S00-源码走读总索引.md'), fm + idxBody, 'utf8');
  console.log('生成: S00-源码走读总索引.md');
}

console.log(`\n完成：${index.length} 个源码文件 + 1 个索引`);
