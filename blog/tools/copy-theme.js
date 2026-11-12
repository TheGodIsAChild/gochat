/**
 * 把 npm 安装的 hexo-theme-fluid 复制到 blog/themes/fluid。
 *
 * 为什么需要：Hexo 只会从 `themes/<name>/` 加载主题，不会直接读 node_modules。
 * 而主题目录属于第三方代码（168 个文件），不适合提交到自己的仓库，
 * 所以用 npm 的 postinstall 钩子在 `npm install` 后自动复制。
 *
 * 运行：npm install（自动触发）或 node tools/copy-theme.js
 */
const fs = require('fs');
const path = require('path');

const BLOG = path.resolve(__dirname, '..');
const SRC = path.join(BLOG, 'node_modules', 'hexo-theme-fluid');
const DEST = path.join(BLOG, 'themes', 'fluid');

function copyDir(src, dest) {
  fs.mkdirSync(dest, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const s = path.join(src, entry.name);
    const d = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      copyDir(s, d);
    } else {
      fs.copyFileSync(s, d);
    }
  }
}

if (!fs.existsSync(SRC)) {
  console.warn('[copy-theme] 未找到 node_modules/hexo-theme-fluid，请先运行 npm install');
  process.exit(0);
}

// 已存在且文件数一致时跳过，避免每次 install 都重复复制
if (fs.existsSync(DEST)) {
  const count = (p) => fs.readdirSync(p, { withFileTypes: true })
    .reduce((n, e) => n + (e.isDirectory() ? count(path.join(p, e.name)) : 1), 0);
  try {
    if (count(SRC) === count(DEST)) {
      console.log('[copy-theme] 主题已就绪，跳过');
      process.exit(0);
    }
  } catch (e) { /* 忽略，继续覆盖 */ }
  fs.rmSync(DEST, { recursive: true, force: true });
}

copyDir(SRC, DEST);
console.log('[copy-theme] 已复制 hexo-theme-fluid 到 themes/fluid');
