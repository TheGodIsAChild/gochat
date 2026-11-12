/**
 * 修复历史遗留的错误内部链接（写文章时手工估的 slug 与实际文件名不一致）。
 * 运行：node blog/scripts/fix-links.js
 */
const fs = require('fs');
const path = require('path');

const POSTS = path.resolve(__dirname, '../source/_posts');
const SOURCE = path.resolve(__dirname, '../source');

// 旧的错误 slug（或误写的旧 slug） → 重构后的新 slug
const alias = {
  '课程导读-从0到1怎么学': '01-01-课程导读-从0到1怎么学',
  '环境准备与项目启动-逐行': '01-03-环境准备与项目启动',
  '一次HTTP请求的完整链路-逐行': '01-04-一次HTTP请求的完整链路',
  '02-一次HTTP请求的完整链路-逐行': '01-04-一次HTTP请求的完整链路',
  '代码地图-每个文件是干什么的': '01-05-代码地图',
  '踩坑集-24个真实坑': '01-06-踩坑集',
  '架构总览-从单体到微服务': '02-01-架构总览',
  '数据库表设计-12张表逐张讲': '02-02-数据库表设计',
  '并发编程实战-7个并发场景': '02-03-并发编程实战',
  'WebSocket全解-握手心跳重连与wss': '02-04-WebSocket全解',
  'Redis在本项目的5种用法': '02-05-Redis用法',
  'MySQL索引与查询优化实战': '02-06-MySQL索引优化',
  '开发模式-Go后端常用的8种模式': '03-01-开发模式',
  '接到需求怎么动手-标准开发流程': '03-02-开发流程',
  '接口设计规范与错误码': '03-03-接口设计规范',
  '错误处理与日志-Go工程化实践': '03-04-错误处理与日志',
  '29-配置管理与优雅降级': '03-05-配置管理与降级',
  '30-配置管理与优雅降级': '03-05-配置管理与降级',
  '30-测试与调试-怎么验证代码是对的': '03-06-测试与调试',
  '31-测试与调试-怎么验证代码是对的': '03-06-测试与调试',
  '认证与好友-双token与事务': '04-02-认证与好友',
  '面试集-72题': '06-01-面试集72题',
  '场景集-30个业务场景': '06-02-场景集30个',
  '项目总结与面试话术': '06-03-项目总结与面试话术',
};

const keys = Object.keys(alias).sort((a, b) => b.length - a.length); // 长的先匹配
let count = 0;

function fix(content) {
  return content.replace(/\(\/blog\/2026\/11\/(?:12|13|14)\/([^/)\s]+)\/\)/g, (all, slug) => {
    if (alias[slug]) {
      count++;
      return `(/blog/2026/11/14/${alias[slug]}/)`;
    }
    return all;
  });
}

const files = fs.readdirSync(POSTS).filter(f => f.endsWith('.md')).map(f => path.join(POSTS, f));
files.push(path.join(SOURCE, 'about/index.md'));

for (const p of files) {
  if (!fs.existsSync(p)) continue;
  const before = fs.readFileSync(p, 'utf8');
  const after = fix(before);
  if (after !== before) fs.writeFileSync(p, after, 'utf8');
}

console.log(`修复 ${count} 处历史错误链接`);
