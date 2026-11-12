/**
 * 在指定文章末尾（"相关/下一篇" 之前）插入「面试怎么答」小节。
 * 运行：node tools/append-interview.js
 */
const fs = require('fs');
const path = require('path');

const POSTS = path.resolve(__dirname, '../source/_posts');

const blocks = {
  '01-04-一次HTTP请求的完整链路.md': `## 十五、面试怎么答（背这 3 句）

**问：一次请求在你们项目里经过哪几层？**

> handler 只做协议转换（解析请求、参数校验、写响应）；service 放业务逻辑和事务；model 只描述表结构。
> service 层不依赖 gin，所以 M6 拆 gRPC 时能直接复用。

**问：为什么参数校验要分两层？**

> handler 校验**格式**（必填、长度、类型），service 校验**业务规则**（用户名是否重复、是否好友、是否群主）。
> 因为 service 可能被 gRPC、定时任务、CLI 调用，它不应该依赖 HTTP 的校验规则。

**问：注册为什么要开事务？**

> 要同时写 \`users\` 和 \`user_settings\`，它们 1:1 必须成对；不开事务可能只成功一半，产生「有用户没设置」的脏数据。
> 事务回调里必须用 \`tx\`，不能用 \`global.DB\`——那是独立事务，回滚管不到。

`,
  '02-02-数据库表设计.md': `## 十三、面试怎么答（背这 3 句）

**问：好友关系为什么存两行？**

> 因为查「我的好友」要能走索引。存两行后 \`WHERE user_id = ?\` 直接命中 \`idx_user\`；
> 只存一行就得 \`WHERE user_id = ? OR friend_id = ?\`，\`OR\` 用不上索引，数据量大就全表扫。
> 代价是写两条，所以必须用事务保证成对。

**问：未读数怎么算？为什么不用已读表？**

> 在 \`conversation_members\` 存 \`last_read_message_id\`，未读数 = \`id > last_read_message_id AND sender != 我\` 的消息数。
> 建已读表是「消息数 × 成员数」的量级（200 人 × 10 万条 = 2000 万行），当前方案只要 200 行。

**问：软删除和唯一索引冲突怎么解？**

> 软删只是改标记、行还在，唯一索引仍占位。解法两种：
> ① 用 \`Unscoped()\` 查旧行，存在就恢复（重置 status）；
> ② 不需要历史的数据（如黑名单）直接物理删除。

`,
  '02-03-并发编程实战.md': `## 十二、面试怎么答（背这 3 句）

**问：你们项目的并发点在哪？**

> 主要在 WebSocket：Hub 管理所有连接（读多写少，用 RWMutex）、每个连接读写双泵、群消息扇出限 16 并发、
> 慢消费者用 non-blocking 入队隔离；另外业务层的并发控制交给数据库（唯一索引 + 条件更新），不加锁。

**问：为什么说锁里不能做 IO？**

> 持锁做 IO 会把锁持有时间放大到不可控。项目里 \`Hub.SendToUser\` 先持读锁复制连接列表，**立刻释放锁**，再在锁外 \`Enqueue\`。
> 如果持锁调用 \`Enqueue\`，而它可能触发 \`Close()\`（要拿写锁），就会死锁。

**问：怎么限制并发数？**

> 用带缓冲 channel 当信号量：\`sem := make(chan struct{}, 16)\`，起 goroutine 前 \`sem <- struct{}{}\`，结束时 \`<-sem\`。
> 项目里群消息扇出就是这样：小群（≤10 人）顺序推，大群并发但限制同时 16 个。

`,
  '02-04-WebSocket全解.md': `## 十二、面试怎么答（背这 3 句）

**问：WebSocket 握手是怎么完成的？**

> 客户端先发一个普通 HTTP 请求，带 \`Upgrade: websocket\`、\`Connection: Upgrade\`、\`Sec-WebSocket-Key\`；
> 服务端返回 \`101 Switching Protocols\` 和由 Key 算出的 \`Sec-WebSocket-Accept\`。
> 之后同一条 TCP 连接上开始传 WebSocket 帧，不再走 HTTP。

**问：为什么必须读写分离？心跳怎么做？**

> \`gorilla/websocket\` 不允许并发写，两个 goroutine 同时写会 panic，所以 ReadPump 唯一读、WritePump 唯一写，其他 goroutine 只能往 send channel 塞。
> 心跳：服务端每 54 秒发 ping，60 秒收不到 pong 就断开（ping 间隔 < 超时，留网络抖动余量），用来清理「假死」连接。

**问：多实例部署怎么给不在本机的用户推消息？**

> 单机 Hub 只管本进程连接，跨实例要：Redis Pub/Sub 广播，或 MQ 扇出让每个实例推自己的在线用户（本项目 M7 用了这个思路）。
> 在线状态也要从内存改成 Redis + TTL。

`,
};

let done = 0;
for (const [file, block] of Object.entries(blocks)) {
  const p = path.join(POSTS, file);
  if (!fs.existsSync(p)) { console.warn('跳过(不存在):', file); continue; }
  let md = fs.readFileSync(p, 'utf8');

  if (md.includes('面试怎么答')) { console.log('已存在，跳过:', file); continue; }

  // 在最后一个分隔线（后面接「相关」或「下一篇」）之前插入
  const marker = md.lastIndexOf('\n---\n');
  if (marker >= 0) {
    md = md.slice(0, marker) + '\n' + block + '---\n' + md.slice(marker + 5);
  } else {
    md += '\n' + block;
  }
  fs.writeFileSync(p, md, 'utf8');
  done++;
  console.log('已插入:', file);
}
console.log(`\n完成：${done} 篇`);
