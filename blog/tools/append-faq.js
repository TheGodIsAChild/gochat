/**
 * 在 4 篇核心文章末尾（"相关/下一篇" 之前）追加「小白常见疑问」小节。
 * 运行：node tools/append-faq.js
 */
const fs = require('fs');
const path = require('path');

const POSTS = path.resolve(__dirname, '../source/_posts');

const blocks = {
  '01-03-环境准备与项目启动.md': [
    '## 十、小白最容易卡住的 5 个点',
    '',
    '### ❓ 为什么我的 `curl` 命令行为很怪？',
    '',
    '**Windows PowerShell 里 `curl` 是 `Invoke-WebRequest` 的别名**，参数和真 curl 完全不同。',
    '解决办法二选一：① 用 `curl.exe`（显式带扩展名）；② 在 CMD 里执行。',
    '',
    '### ❓ 装了 Docker Desktop，但 `docker` 命令提示找不到',
    '',
    '装完必须**重启终端**（新开的窗口才会加载 PATH）。Windows 还要确认 Docker Desktop 已启动（托盘图标变绿）。',
    '',
    '### ❓ 怎么确认 MySQL 真的能连上（不只是容器 healthy）',
    '',
    '```bash',
    'docker exec -it gochat-mysql mysql -ugohat -pgochat123 -e "SELECT 1;"',
    '```',
    '能输出 `1` 就说明账号密码和库都没问题。（本项目用户是 `gochat`，密码 `gochat123`）',
    '',
    '### ❓ 改了 `config.yaml` 需要重启吗',
    '',
    '**需要**。本项目配置只在启动时读取一次，没有热更新。改完 `Ctrl+C` 再 `go run . -db`。',
    '',
    '### ❓ 启动日志里 `RabbitMQ 连接失败` 是不是必须修',
    '',
    '**不是**。RabbitMQ 是可选依赖，连不上会自动降级为同步推送，功能正常。',
    '只有 MySQL/Redis 连不上才必须修。',
    '',
  ],
  '01-04-一次HTTP请求的完整链路.md': [
    '## 十六、小白读到这里的 5 个疑问',
    '',
    '### ❓ `Authorization: Bearer xxx` 里的 Bearer 是什么意思',
    '',
    '它是 HTTP 的一种认证方案名（RFC 6750），直译"持有者"。含义是：**谁持有这个 token，谁就是该用户**。',
    '它只是个固定前缀，`Bearer` 后面必须有一个空格。',
    '',
    '### ❓ `global.DB` 是在哪初始化的？我怎么找不到',
    '',
    '三步定位：',
    '1. `global/global.go` 里有 `var DB *gorm.DB`（只是声明）',
    '2. 搜 `global.DB =` → 在 `core/db/init_mysql.go` 里赋值',
    '3. `main.go` 调用 `core.InitBaseServices()` 时执行',
    '',
    'VS Code 里 `Ctrl+点击` 变量名能直接跳转定义。',
    '',
    '### ❓ bcrypt 到底慢到什么程度？慢不是缺点吗',
    '',
    '一次哈希约 **50–100 毫秒**。登录时你只算一次，几乎无感；',
    '但如果攻击者拿到数据库想暴力破解，每个密码都要 50ms，破解成本被放大上百万倍——**慢正是它的安全性来源**。',
    '',
    '### ❓ 为什么事务里 `tx.Create(user)` 之后 `user.ID` 就有值了',
    '',
    'GORM 在 `Create` 成功后会把数据库生成的自增主键**回填**到结构体字段。',
    '所以下一行 `settings := &UserSettings{UserID: user.ID}` 能直接用。如果自己写 `database/sql` 就没这么方便。',
    '',
    '### ❓ 中间件 `c.Set` 存的值，后面的 handler 怎么拿到的',
    '',
    '因为整条中间件链 + handler 共享**同一个 `*gin.Context` 对象**。',
    '`c.Set` 是往这个对象里写，handler 用同样的 key `c.GetUint64(...)` 就能读出来。',
    '这也解释了为什么 key 必须两边一致——写错一个字母就读不到（返回零值，不报错）。',
    '',
  ],
  '02-03-并发编程实战.md': [
    '## 十三、小白读到这里的 5 个疑问',
    '',
    '### ❓ `struct{}{}` 是什么东西？为什么拿它当信号量',
    '',
    '`struct{}{}` 是「**空结构体**」，占用 **0 字节**内存。',
    '信号量只需要表达"有没有令牌"，不关心内容，所以用 0 字节的类型最省内存。',
    '`chan struct{}` 是 Go 里表示"纯信号"的惯用写法。',
    '',
    '### ❓ `wg.Add(1)` / `wg.Done()` / `wg.Wait()` 分别干嘛',
    '',
    '- `Add(1)`：告诉 WaitGroup「我还要等 1 个任务」',
    '- `Done()`：任务完成，计数 -1（通常 `defer wg.Done()`）',
    '- `Wait()`：阻塞直到计数归零',
    '少写一个 `Done()` 就会永远卡在 `Wait()`——这是新手最常见的死锁原因。',
    '',
    '### ❓ RWMutex 可以多次 RLock 吗',
    '',
    '可以，多个读可以同时持有读锁；但**读锁和写锁互斥**。',
    '注意：如果一个 goroutine 持有读锁再去请求写锁，会死锁（读锁没释放，写锁永远等不到）。',
    '',
    '### ❓ `defer recover()` 为什么能兜住 panic',
    '',
    '`recover()` 只在 `defer` 的函数里调用才有效，它能捕获当前 goroutine 的 panic 并让程序继续。',
    '所以 `Enqueue` 里加 `defer func(){ _ = recover() }()`，即使连接已关闭、channel 已关闭也不会让整个进程崩。',
    '',
    '### ❓ Go 1.22 之后，闭包还会捕获错误的循环变量吗',
    '',
    'Go 1.22 起每个循环迭代都有独立变量，**不再有这个 bug**。',
    '但显式传参 `go func(uid uint64){...}(uid)` 仍然更好——代码意图更清晰，也兼容老版本。',
    '',
  ],
  '02-04-WebSocket全解.md': [
    '## 十三、小白读到这里的 5 个疑问',
    '',
    '### ❓ `CheckOrigin` 是什么？为什么本地能连、部署到线上可能 403',
    '',
    'WebSocket 握手是 HTTP 请求，浏览器会带 `Origin` 头。`Upgrader.CheckOrigin` 用来决定"允不允许这个来源连"。',
    '默认策略只允许**同源**。本地 `127.0.0.1:8080` 打开页面、连同一个地址，同源所以能连；',
    '一旦前后端不同域（比如前端 3000、后端 8080），默认就会被拒，需要显式放行。',
    '生产环境不要写成"永远返回 true"，那等于允许任意网站连你的 WS（CSWSH 劫持）。',
    '',
    '### ❓ 应用层的 `{"type":"ping"}` 和协议层的 Ping 帧有什么区别',
    '',
    '| | 协议层 Ping/Pong（opcode 9/10） | 应用层 `type:ping` |',
    '|---|---|---|',
    '| 谁处理 | 浏览器/服务端底层库自动回 | 你的业务代码自己处理 |',
    '| 用途 | 保活、探测连接是否存活 | 业务自定义（本项目前端也发了它） |',
    '',
    '本项目**两种都有**：服务端 WritePump 发协议层 Ping 做保活；客户端还会发应用层 ping 让服务端回 pong，双向确认。',
    '',
    '### ❓ `SetReadLimit(4096)` 会不会太小？发图片怎么办',
    '',
    '4KB 只限制**单帧**大小。文本消息足够；图片/语音的正确做法是**先上传拿到 URL，再发 URL 文本**，不走 WS 传二进制。',
    '如果不设上限，恶意客户端发一个 1GB 的帧就能把服务内存打爆。',
    '',
    '### ❓ WebSocket 握手为什么用 GET 而不是 POST',
    '',
    '因为它本质是「**请求把这条连接升级成 WebSocket 协议**」，不涉及提交数据体。',
    '协议规定握手必须是 GET，而且不能带 body。',
    '',
    '### ❓ 怎么在浏览器里看到 WebSocket 帧',
    '',
    'F12 → Network（网络）→ 筛选 `WS` → 点开那条连接 → Messages（消息）标签页，',
    '能看到收发的每一帧 JSON，调试消息格式时非常有用。',
    '',
  ],
};

let done = 0;
for (const [file, lines] of Object.entries(blocks)) {
  const p = path.join(POSTS, file);
  if (!fs.existsSync(p)) { console.warn('跳过(不存在):', file); continue; }
  let md = fs.readFileSync(p, 'utf8');

  if (md.includes('小白读到这里的') || md.includes('小白最容易卡住的 5 个点')) {
    console.log('已存在，跳过:', file);
    continue;
  }

  const block = lines.join('\n') + '\n';
  const marker = md.lastIndexOf('\n---\n');
  if (marker >= 0) {
    md = md.slice(0, marker) + '\n' + block + '---\n' + md.slice(marker + 5);
  } else {
    md += '\n' + block;
  }
  fs.writeFileSync(p, md, 'utf8');
  done++;
  console.log('已追加:', file);
}
console.log(`\n完成：${done} 篇`);
