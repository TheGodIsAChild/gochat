/**
 * 站内链接检查：扫描 public 下的 HTML，验证所有 /blog/... 链接都有对应文件。
 * 运行：node blog/scripts/check-links.js
 */
const fs = require('fs');
const path = require('path');

const PUB = path.resolve(__dirname, '../public');

const files = [];
(function walk(d) {
  for (const f of fs.readdirSync(d, { withFileTypes: true })) {
    const p = path.join(d, f.name);
    if (f.isDirectory()) walk(p);
    else if (p.endsWith('.html')) files.push(p);
  }
})(PUB);

const bad = new Map();

for (const f of files) {
  const html = fs.readFileSync(f, 'utf8');
  const re = /href="(\/blog\/[^"#?]*)"/g;
  let m;
  while ((m = re.exec(html))) {
    let url = m[1];
    if (/\.(css|js|svg|png|jpe?g|gif|xml|json|ico|woff2?|ttf)$/i.test(url)) continue;
    let decoded;
    try { decoded = decodeURIComponent(url); } catch (e) { decoded = url; }

    const rel = decoded.replace(/^\/blog\/?/, '');
    let target = rel ? path.join(PUB, rel) : PUB;
    if (fs.existsSync(target) && fs.statSync(target).isDirectory()) {
      target = path.join(target, 'index.html');
    }
    if (!fs.existsSync(target)) {
      bad.set(decoded, (bad.get(decoded) || 0) + 1);
    }
  }
}

console.log('检查页面数:', files.length);
if (bad.size === 0) {
  console.log('✅ 所有站内链接有效');
} else {
  console.log(`❌ 失效链接 ${bad.size} 个：`);
  [...bad.entries()].slice(0, 40).forEach(([u, c]) => console.log('  ', u, 'x' + c));
  process.exitCode = 1;
}
