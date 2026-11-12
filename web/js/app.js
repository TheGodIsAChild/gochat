/* =========================================================
   app.js —— 仿微信前端主逻辑
   ---------------------------------------------------------
   功能覆盖：
   1. 登录/注册/登出（双 token 无感刷新）
   2. 会话列表 + 未读数
   3. 聊天：历史分页、发消息（WebSocket）、实时接收、已读
   4. 通讯录：好友列表、新朋友（申请）、用户搜索加好友、备注、删除、拉黑
   5. 群聊：建群、群详情、成员列表、拉人、踢人、退群、改资料、解散
   6. 通话：通话记录 + WebRTC 语音通话（信令走 WebSocket）
   7. 设置：资料修改、隐私开关、黑名单、登出
   ========================================================= */

const App = (() => {
  // ==================== 状态 ====================
  const S = {
    me: null,
    panel: 'chats',
    conversations: [],
    currentConv: null,     // 当前会话对象
    messages: [],          // 当前会话消息
    friends: [],
    requests: [],
    groups: [],
    calls: [],
    settings: null,
    blacklist: [],
    online: new Set(),
    userNameCache: {},     // userId -> 用户信息（群消息显示发送者昵称）
    loadingMore: false,
    noMore: false,
    // 通话
    call: null,            // { callId, peerId, peerName, direction, state }
    callTimer: null,
    callStartAt: 0,
    autoRefreshTimer: null,
  };

  // ==================== 小工具 ====================
  const $ = (sel, root) => (root || document).querySelector(sel);
  const $$ = (sel, root) => Array.from((root || document).querySelectorAll(sel));

  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function initial(name) {
    const s = String(name || '?').trim();
    return s ? s.slice(-2) : '?';
  }

  /** 头像 HTML：有 URL 用图片，否则用昵称首字 */
  function avatarHtml(name, url, cls) {
    cls = cls || 'avatar';
    if (url) return '<div class="' + cls + '"><img src="' + esc(url) + '" alt="" onerror="this.parentNode.textContent=\'' + esc(initial(name)) + '\'"></div>';
    return '<div class="' + cls + '">' + esc(initial(name)) + '</div>';
  }

  /** 时间戳归一化：服务端 HTTP 返回 ISO 字符串，MQ 推送返回 Unix 秒 */
  function toDate(v) {
    if (v == null) return new Date();
    if (typeof v === 'number') return new Date(v * 1000);
    return new Date(v);
  }

  function fmtClock(d) {
    if (!d || isNaN(d)) return '';
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    return hh + ':' + mm;
  }

  function fmtListTime(v) {
    const d = toDate(v);
    if (!v || isNaN(d)) return '';
    const now = new Date();
    const sameDay = d.toDateString() === now.toDateString();
    if (sameDay) return fmtClock(d);
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return month + '-' + day;
  }

  function fmtDuration(sec) {
    sec = Math.max(0, Math.floor(sec || 0));
    const m = Math.floor(sec / 60), s = sec % 60;
    return m + '分' + String(s).padStart(2, '0') + '秒';
  }

  function uid() {
    return 'c-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
  }

  // ==================== Toast / Modal ====================
  function toast(msg, isErr) {
    const div = document.createElement('div');
    div.className = 'toast' + (isErr ? ' error' : '');
    div.textContent = msg;
    $('#toast-root').appendChild(div);
    setTimeout(() => div.remove(), 2600);
  }

  let modalStack = [];
  function openModal(title, bodyHtml, footHtml) {
    const mask = document.createElement('div');
    mask.className = 'modal-mask';
    mask.innerHTML =
      '<div class="modal">' +
        '<div class="modal-head"><h3>' + esc(title) + '</h3>' +
          '<button class="icon-btn" data-close>✕</button></div>' +
        '<div class="modal-body">' + bodyHtml + '</div>' +
        (footHtml ? '<div class="modal-foot">' + footHtml + '</div>' : '') +
      '</div>';
    $('#modal-root').appendChild(mask);
    const close = () => { mask.remove(); modalStack = modalStack.filter(m => m !== close); };
    mask.addEventListener('click', (e) => {
      if (e.target === mask || e.target.hasAttribute('data-close')) close();
    });
    modalStack.push(close);
    return { mask, close };
  }

  function closeAllModals() { modalStack.slice().forEach(fn => fn()); }

  // ==================== 登录 / 注册 ====================
  function bindLoginView() {
    $$('.tab').forEach(btn => {
      btn.addEventListener('click', () => {
        $$('.tab').forEach(b => b.classList.toggle('active', b === btn));
        const isLogin = btn.dataset.tab === 'login';
        $('#login-form').hidden = !isLogin;
        $('#register-form').hidden = isLogin;
      });
    });

    $('#login-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const btn = e.target.querySelector('button[type=submit]');
      btn.disabled = true;
      try {
        await API.login($('#login-username').value.trim(), $('#login-password').value);
        await bootAfterLogin();
      } catch (err) {
        toast(err.message, true);
      } finally {
        btn.disabled = false;
      }
    });

    $('#register-form').addEventListener('submit', async (e) => {
      e.preventDefault();
      const btn = e.target.querySelector('button[type=submit]');
      btn.disabled = true;
      try {
        const username = $('#reg-username').value.trim();
        const nickname = $('#reg-nickname').value.trim();
        const password = $('#reg-password').value;
        await API.register(username, password, nickname);
        await API.login(username, password);
        await bootAfterLogin();
      } catch (err) {
        toast(err.message, true);
      } finally {
        btn.disabled = false;
      }
    });
  }

  // ==================== 启动 ====================
  async function init() {
    bindLoginView();
    bindNav();
    bindWsEvents();

    if (API.hasLogin()) {
      try {
        await bootAfterLogin();
        return;
      } catch (e) {
        API.clearTokens();
      }
    }
    $('#login-view').hidden = false;
    $('#app-view').hidden = true;
  }

  async function bootAfterLogin() {
    S.me = await API.me();
    $('#me-avatar').textContent = initial(S.me.nickname || S.me.username);
    if (S.me.avatar) {
      $('#me-avatar').innerHTML = '<img src="' + esc(S.me.avatar) + '" alt="">';
    }
    $('#login-view').hidden = true;
    $('#app-view').hidden = false;

    connectWs();
    await Promise.all([
      loadConversations(),
      loadFriends(),
      loadRequests(),
      loadGroups(),
    ]);
    refreshOnlineLoop();
    switchPanel('chats');
  }

  function connectWs() {
    WS.setStatusHandler((st) => {
      const dot = $('#conn-dot');
      dot.classList.toggle('online', st === 'connected');
      dot.title = st === 'connected' ? '已连接' : '连接中断，重连中…';
    });
    WS.on('reconnect', () => {
      // 重连后用最新 token
      WS.connect(API.getAccess());
    });
    WS.connect(API.getAccess());
  }

  async function doLogout() {
    try { await API.logout(API.getRefresh()); } catch (e) {}
    WS.close();
    if (S.autoRefreshTimer) clearInterval(S.autoRefreshTimer);
    API.clearTokens();
    location.reload();
  }

  // ==================== 面板切换 ====================
  // 左侧导航按钮绑定：点击切换 聊天/通讯录/通话/设置
  function bindNav() {
    $$('.nav-btn').forEach(btn => {
      btn.addEventListener('click', () => switchPanel(btn.dataset.panel));
    });
  }

  function switchPanel(panel) {
    S.panel = panel;
    $$('.nav-btn').forEach(b => b.classList.toggle('active', b.dataset.panel === panel));
    if (panel === 'calls') loadCalls();
    if (panel === 'settings') loadSettings();
    if (panel !== 'chats') S.currentConv = null;
    renderList();
    renderMain();
  }

  // ==================== 中间列表渲染 ====================
  function renderList() {
    const head = $('#list-head');
    const body = $('#list-body');

    if (S.panel === 'chats') {
      head.innerHTML =
        '<div class="title">聊天</div>' +
        '<div class="head-actions">' +
          '<button class="icon-btn" id="btn-new-group" title="发起群聊">＋</button>' +
        '</div>';
      $('#btn-new-group').onclick = openCreateGroupModal;
      body.innerHTML = S.conversations.length
        ? S.conversations.map(convItemHtml).join('')
        : '<div class="list-empty">还没有会话<br>去通讯录找个好友聊聊吧</div>';
      $$('.conv-item', body).forEach(el => {
        el.onclick = () => {
          const conv = S.conversations.find(c => String(c.conversation_id) === el.dataset.id);
          if (conv) openConversation(conv);
        };
      });
    } else if (S.panel === 'contacts') {
      head.innerHTML =
        '<div class="title">通讯录</div>' +
        '<div class="head-actions">' +
          '<button class="icon-btn" id="btn-add-friend" title="添加好友">🔍</button>' +
          '<button class="icon-btn" id="btn-new-group2" title="发起群聊">＋</button>' +
        '</div>';
      $('#btn-add-friend').onclick = openAddFriendModal;
      $('#btn-new-group2').onclick = openCreateGroupModal;

      const pending = S.requests.length;
      let html = '';
      html += '<div class="list-subhead">新朋友' + (pending ? '（' + pending + '）' : '') + '</div>';
      html += S.requests.length
        ? S.requests.map(reqItemHtml).join('')
        : '<div class="list-item" id="requests-empty"><div class="meta"><div class="sub">暂无新的好友申请</div></div></div>';
      html += '<div class="list-subhead">好友（' + S.friends.length + '）</div>';
      html += S.friends.length
        ? S.friends.map(friendItemHtml).join('')
        : '<div class="list-empty">还没有好友</div>';
      html += '<div class="list-subhead">群聊（' + S.groups.length + '）</div>';
      html += S.groups.length
        ? S.groups.map(groupItemHtml).join('')
        : '<div class="list-empty">还没有群聊</div>';
      body.innerHTML = html;

      $$('.req-accept', body).forEach(b => b.onclick = (e) => { e.stopPropagation(); acceptRequest(Number(b.dataset.id)); });
      $$('.req-reject', body).forEach(b => b.onclick = (e) => { e.stopPropagation(); rejectRequest(Number(b.dataset.id)); });
      $$('.friend-item', body).forEach(el => {
        el.onclick = () => openFriendMenu(el.dataset.id);
      });
      $$('.group-item', body).forEach(el => {
        el.onclick = () => openGroupDetail(el.dataset.id);
      });
    } else if (S.panel === 'calls') {
      head.innerHTML = '<div class="title">通话</div>';
      body.innerHTML = S.calls.length
        ? S.calls.map(callItemHtml).join('')
        : '<div class="list-empty">暂无通话记录</div>';
    } else if (S.panel === 'settings') {
      head.innerHTML = '<div class="title">设置</div>';
      body.innerHTML =
        '<div class="list-item" id="set-refresh"><div class="meta"><div class="name">刷新数据</div><div class="sub">重新拉取会话/好友/群</div></div></div>' +
        '<div class="list-item" id="set-logout"><div class="meta"><div class="name" style="color:#fa5151">退出登录</div><div class="sub">清除本地登录态</div></div></div>';
      $('#set-refresh').onclick = async () => {
        await Promise.all([loadConversations(), loadFriends(), loadRequests(), loadGroups()]);
        toast('已刷新');
      };
      $('#set-logout').onclick = () => { if (confirm('确定退出登录？')) doLogout(); };
    }
  }

  function convItemHtml(c) {
    const isGroup = c.type === 2;
    const name = isGroup ? (c.group_name || '群聊') : (c.peer_nickname || '用户');
    const avatarUrl = isGroup ? c.group_avatar : c.peer_avatar;
    const online = !isGroup && S.online.has(c.peer_user_id);
    const lastTime = fmtListTime(c.last_message_at);
    const active = S.currentConv && S.currentConv.conversation_id === c.conversation_id;
    return '' +
      '<div class="list-item conv-item' + (active ? ' active' : '') + '" data-id="' + c.conversation_id + '">' +
        avatarHtml(name, avatarUrl) +
        '<div class="meta">' +
          '<div class="name">' + esc(name) +
            (isGroup ? ' <span style="font-size:11px;color:#999">(' + (c.member_count || 0) + ')</span>' : '') +
            (online ? ' <span class="online-mark"></span>' : '') +
          '</div>' +
          '<div class="sub">' + (c.last_message_id ? '有新消息' : '点击开始聊天') + '</div>' +
        '</div>' +
        '<div class="right">' +
          '<div class="time">' + lastTime + '</div>' +
          (c.unread ? '<div class="unread">' + (c.unread > 99 ? '99+' : c.unread) + '</div>' : '') +
        '</div>' +
      '</div>';
  }

  function reqItemHtml(r) {
    return '' +
      '<div class="list-item">' +
        avatarHtml(r.nickname || r.username, r.avatar) +
        '<div class="meta">' +
          '<div class="name">' + esc(r.nickname || r.username) + '</div>' +
          '<div class="sub">' + esc(r.message || '请求添加你为好友') + '</div>' +
        '</div>' +
        '<div class="right" style="flex-direction:row;gap:6px">' +
          '<button class="btn btn-green req-accept" data-id="' + r.id + '">接受</button>' +
          '<button class="btn req-reject" data-id="' + r.id + '">拒绝</button>' +
        '</div>' +
      '</div>';
  }

  function friendItemHtml(f) {
    return '' +
      '<div class="list-item friend-item" data-id="' + f.friend_id + '">' +
        avatarHtml(f.remark || f.nickname || f.username, f.avatar) +
        '<div class="meta">' +
          '<div class="name">' + esc(f.remark || f.nickname || f.username) + '</div>' +
          '<div class="sub">@' + esc(f.username) + '</div>' +
        '</div>' +
        '<div class="right">' + (S.online.has(f.friend_id) ? '<span class="online-mark"></span>' : '') + '</div>' +
      '</div>';
  }

  function groupItemHtml(g) {
    return '' +
      '<div class="list-item group-item" data-id="' + g.id + '">' +
        avatarHtml(g.name, g.avatar) +
        '<div class="meta">' +
          '<div class="name">' + esc(g.name) + '</div>' +
          '<div class="sub">' + g.member_count + ' 人</div>' +
        '</div>' +
      '</div>';
  }

  function callItemHtml(c) {
    const meId = S.me ? S.me.id : 0;
    const isCaller = c.caller_id === meId;
    const peerId = isCaller ? c.callee_id : c.caller_id;
    const peer = S.userNameCache[peerId] || {};
    const statusMap = { 1: '呼叫中', 2: '通话中', 3: '已拒绝', 4: '已取消', 5: '已结束', 6: '未接' };
    const name = peer.nickname || peer.username || ('用户 ' + peerId);
    return '' +
      '<div class="list-item call-item" data-peer="' + peerId + '">' +
        avatarHtml(name, peer.avatar) +
        '<div class="meta">' +
          '<div class="name">' + esc(name) + '</div>' +
          '<div class="sub">' + (isCaller ? '呼出' : '呼入') + ' · ' + (statusMap[c.status] || '') +
            (c.duration ? ' · ' + fmtDuration(c.duration) : '') + '</div>' +
        '</div>' +
        '<div class="right"><div class="time">' + fmtListTime(c.created_at) + '</div></div>' +
      '</div>';
  }

  // ==================== 数据加载 ====================
  async function loadConversations() {
    try {
      const d = await API.listConversations();
      S.conversations = d.conversations || [];
      if (S.panel === 'chats') renderList();
    } catch (e) { console.warn(e); }
  }

  async function loadFriends() {
    try {
      const d = await API.listFriends();
      S.friends = d.friends || [];
      S.friends.forEach(f => { S.userNameCache[f.friend_id] = { username: f.username, nickname: f.nickname, avatar: f.avatar }; });
      if (S.panel === 'contacts') renderList();
    } catch (e) { console.warn(e); }
  }

  async function loadRequests() {
    try {
      const d = await API.listFriendRequests();
      S.requests = d.requests || [];
      if (S.panel === 'contacts') renderList();
    } catch (e) { console.warn(e); }
  }

  async function loadGroups() {
    try {
      const d = await API.listGroups();
      S.groups = d.groups || [];
      if (S.panel === 'contacts') renderList();
    } catch (e) { console.warn(e); }
  }

  async function loadCalls() {
    try {
      const d = await API.listCalls(30);
      S.calls = d.calls || [];
      // 批量解析对方昵称（走 gRPC gateway）
      const ids = Array.from(new Set(S.calls.map(c => c.caller_id === S.me.id ? c.callee_id : c.caller_id)));
      const unknown = ids.filter(id => !S.userNameCache[id]);
      if (unknown.length) {
        try {
          const r = await fetch('/api/v1/rpc/users?ids=' + unknown.join(','), {
            headers: { 'Authorization': 'Bearer ' + API.getAccess() },
          });
          const j = await r.json();
          (j.data && j.data.users || []).forEach(u => { S.userNameCache[u.id] = u; });
        } catch (e) { /* 忽略 */ }
      }
      if (S.panel === 'calls') renderList();
    } catch (e) { console.warn(e); }
  }

  async function loadSettings() {
    try {
      S.settings = await API.getSettings();
      renderMain();
    } catch (e) { console.warn(e); }
  }

  async function refreshOnlineLoop() {
    const tick = async () => {
      const ids = S.friends.map(f => f.friend_id);
      if (!ids.length) return;
      try {
        const d = await API.onlineUsers(ids);
        S.online = new Set(d.online_ids || []);
        if (S.panel === 'contacts' || S.panel === 'chats') renderList();
      } catch (e) { /* 忽略 */ }
    };
    tick();
    S.autoRefreshTimer = setInterval(tick, 20000);
  }

  // ==================== 右侧主区域 ====================
  function renderMain() {
    const main = $('#main-panel');
    if (S.panel === 'settings') { renderSettingsMain(main); return; }
    if (S.panel === 'calls') { renderCallsMain(main); return; }
    if (S.panel === 'contacts') { renderContactsMain(main); return; }
    if (!S.currentConv) {
      main.innerHTML =
        '<div class="empty-state">' +
          '<div class="empty-icon">💬</div>' +
          '<p>选择一个会话开始聊天</p>' +
        '</div>';
      return;
    }
    renderChatMain(main);
  }

  function renderContactsMain(main) {
    main.innerHTML =
      '<div class="section">' +
        '<h3>通讯录</h3>' +
        '<div class="card">' +
          '<p style="font-size:13px;color:#666;line-height:1.8">' +
            '左侧列表可以：<br>' +
            '· 处理「新朋友」里的好友申请<br>' +
            '· 点击好友查看详情（发消息 / 设备注 / 删除 / 拉黑）<br>' +
            '· 点击群聊查看群详情（成员 / 拉人 / 踢人 / 改资料 / 退群 / 解散）<br>' +
            '右上角 🔍 搜索用户加好友，＋ 发起群聊' +
          '</p>' +
        '</div>' +
      '</div>';
  }

  function renderSettingsMain(main) {
    if (!S.me) return;
    const s = S.settings || {};
    const row = (label, inputHtml) => '<div class="form-row"><label>' + label + '</label>' + inputHtml + '</div>';
    const sw = (key, label, on) =>
      '<div class="form-row"><label>' + label + '</label>' +
      '<div class="switch' + (on ? ' on' : '') + '" data-setting="' + key + '"></div></div>';

    main.innerHTML =
      '<div class="section">' +
        '<h3>我的资料</h3>' +
        '<div class="card">' +
          row('昵称', '<input id="set-nickname" value="' + esc(S.me.nickname || '') + '" />') +
          row('头像 URL', '<input id="set-avatar" value="' + esc(S.me.avatar || '') + '" placeholder="https://..." />') +
          row('签名', '<input id="set-signature" value="' + esc(S.me.signature || '') + '" />') +
          row('地区', '<input id="set-region" value="' + esc(S.me.region || '') + '" />') +
          '<div class="btn-row"><button class="btn btn-green" id="btn-save-profile">保存资料</button></div>' +
        '</div>' +
      '</div>' +
      '<div class="section">' +
        '<h3>隐私设置</h3>' +
        '<div class="card">' +
          sw('allow_search_by_phone', '允许通过手机号搜索', s.allow_search_by_phone) +
          sw('allow_friend_request', '允许他人添加我为好友', s.allow_friend_request) +
          sw('need_verify', '加好友需要验证', s.need_verify) +
          sw('show_read_receipt', '显示已读回执', s.show_read_receipt) +
          sw('notification_enabled', '消息通知', s.notification_enabled) +
        '</div>' +
      '</div>' +
      '<div class="section">' +
        '<h3>黑名单</h3>' +
        '<div class="card">' +
          '<div id="blacklist-box" style="font-size:13px;color:#666">加载中…</div>' +
        '</div>' +
      '</div>';

    $('#btn-save-profile').onclick = saveProfile;
    $$('.switch', main).forEach(el => {
      el.onclick = () => toggleSetting(el);
    });
    loadBlacklist();
  }

  async function saveProfile() {
    try {
      await API.updateMe({
        nickname: $('#set-nickname').value.trim(),
        avatar: $('#set-avatar').value.trim(),
        signature: $('#set-signature').value.trim(),
        region: $('#set-region').value.trim(),
      });
      S.me = await API.me();
      $('#me-avatar').textContent = initial(S.me.nickname || S.me.username);
      if (S.me.avatar) $('#me-avatar').innerHTML = '<img src="' + esc(S.me.avatar) + '" alt="">';
      toast('资料已保存');
    } catch (e) { toast(e.message, true); }
  }

  async function toggleSetting(el) {
    const key = el.dataset.setting;
    const on = !el.classList.contains('on');
    try {
      await API.updateSettings({ [key]: on });
      el.classList.toggle('on', on);
      S.settings[key] = on;
    } catch (e) { toast(e.message, true); }
  }

  async function loadBlacklist() {
    try {
      const d = await API.listBlacklist();
      S.blacklist = d.blacklist || [];
      const box = $('#blacklist-box');
      if (!box) return;
      box.innerHTML = S.blacklist.length
        ? S.blacklist.map(b =>
            '<div style="display:flex;align-items:center;gap:8px;padding:6px 0">' +
              '<span style="flex:1">' + esc(b.nickname || b.username) + '</span>' +
              '<button class="btn" data-unblock="' + b.user_id + '">移出黑名单</button>' +
            '</div>').join('')
        : '暂无黑名单用户';
      $$('[data-unblock]', box).forEach(btn => {
        btn.onclick = async () => {
          try { await API.removeBlacklist(Number(btn.dataset.unblock)); await loadBlacklist(); toast('已移出'); }
          catch (e) { toast(e.message, true); }
        };
      });
    } catch (e) { console.warn(e); }
  }

  function renderCallsMain(main) {
    main.innerHTML =
      '<div class="section">' +
        '<h3>通话</h3>' +
        '<div class="card"><p style="font-size:13px;color:#666;line-height:1.8">' +
          '左侧是通话记录。在单聊里点击顶部 📞 可以发起 WebRTC 语音通话。<br>' +
          '提示：同一台机器开两个浏览器窗口即可体验 P2P 语音（localhost 属于安全上下文）。' +
        '</p></div>' +
      '</div>';
  }

  // ==================== 聊天 ====================
  async function openConversation(conv) {
    S.currentConv = conv;
    S.messages = [];
    S.noMore = false;
    renderList();
    renderMain();
    await loadMessages(conv.conversation_id);
  }

  async function loadMessages(convId, beforeId) {
    try {
      const d = await API.listMessages(convId, beforeId, 30);
      const list = d.messages || [];
      if (!beforeId) {
        S.messages = list;
      } else {
        S.messages = list.concat(S.messages);
      }
      if (list.length < 30) S.noMore = true;
      renderMessages();
      // 标记已读
      const last = list[list.length - 1];
      if (last && !beforeId) {
        API.markRead(convId, last.id).catch(() => {});
        const c = S.conversations.find(x => x.conversation_id === convId);
        if (c) { c.unread = 0; renderList(); }
      }
    } catch (e) { toast(e.message, true); }
  }

  function convTitle(conv) {
    if (!conv) return '';
    return conv.type === 2 ? (conv.group_name || '群聊') : (conv.peer_nickname || '用户');
  }

  function convSubTitle(conv) {
    if (!conv) return '';
    if (conv.type === 2) return (conv.member_count || 0) + ' 人';
    return S.online.has(conv.peer_user_id) ? '在线' : '离线';
  }

  function renderChatMain(main) {
    const conv = S.currentConv;
    const isGroup = conv.type === 2;
    main.innerHTML =
      '<div class="chat-view">' +
        '<div class="chat-head">' +
          avatarHtml(convTitle(conv), isGroup ? conv.group_avatar : conv.peer_avatar, 'avatar') +
          '<div>' +
            '<div class="peer-name">' + esc(convTitle(conv)) + '</div>' +
            '<div class="peer-sub">' + esc(convSubTitle(conv)) + '</div>' +
          '</div>' +
          '<div class="spacer"></div>' +
          (isGroup
            ? '<button class="icon-btn" id="btn-group-info" title="群详情">ℹ️</button>'
            : '<button class="icon-btn" id="btn-call" title="语音通话">📞</button>' +
              '<button class="icon-btn" id="btn-friend-info" title="好友详情">ℹ️</button>') +
        '</div>' +
        '<div class="messages" id="messages"></div>' +
        '<div class="chat-input">' +
          '<textarea id="msg-input" placeholder="输入消息，Enter 发送，Shift+Enter 换行"></textarea>' +
          '<div class="input-foot"><button class="send-btn" id="btn-send">发送</button></div>' +
        '</div>' +
      '</div>';

    const input = $('#msg-input');
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });
    $('#btn-send').onclick = sendMessage;
    if (isGroup) {
      $('#btn-group-info').onclick = () => openGroupDetail(conv.group_id);
    } else {
      $('#btn-call').onclick = () => startVoiceCall(conv);
      $('#btn-friend-info').onclick = () => openFriendMenu(conv.peer_user_id);
    }

    const box = $('#messages');
    box.addEventListener('scroll', () => {
      if (box.scrollTop < 40 && !S.loadingMore && !S.noMore) {
        S.loadingMore = true;
        const first = S.messages[0];
        loadMessages(conv.conversation_id, first ? first.id : 0).finally(() => { S.loadingMore = false; });
      }
    });

    renderMessages();
    input.focus();
  }

  function renderMessages() {
    const box = $('#messages');
    if (!box || !S.currentConv) return;
    const isGroup = S.currentConv.type === 2;
    const meId = S.me ? S.me.id : 0;

    if (!S.messages.length) {
      box.innerHTML = '<div class="msg-system"><span>还没有消息，打个招呼吧</span></div>';
      return;
    }

    box.innerHTML = S.messages.map(m => {
      const me = m.sender_id === meId;
      const senderName = me ? '我' : senderDisplayName(m.sender_id);
      const time = fmtClock(toDate(m.created_at));
      return '' +
        '<div class="msg-row' + (me ? ' me' : '') + '">' +
          (me ? '' : avatarHtml(senderName)) +
          '<div class="msg-body">' +
            (!me && isGroup ? '<div class="msg-sender">' + esc(senderName) + '</div>' : '') +
            '<div class="bubble">' + esc(m.content) + '</div>' +
            '<div class="msg-time">' + time + '</div>' +
          '</div>' +
          (me ? avatarHtml(S.me.nickname || S.me.username, S.me.avatar) : '') +
        '</div>';
    }).join('');
    box.scrollTop = box.scrollHeight;
  }

  function senderDisplayName(senderId) {
    const f = S.friends.find(x => x.friend_id === senderId);
    if (f) return f.remark || f.nickname || f.username;
    const c = S.userNameCache[senderId];
    if (c) return c.nickname || c.username;
    return '用户' + senderId;
  }

  function appendMessage(m, scroll) {
    // 去重（同一 message id 或同一 client_msg_id）
    if (S.messages.some(x => x.id === m.id || (x.client_msg_id && x.client_msg_id === m.client_msg_id && x.sender_id === m.sender_id))) return;
    S.messages.push(m);
    const box = $('#messages');
    if (!box) return;
    const wasBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 80;
    renderMessages();
    if (scroll || wasBottom) box.scrollTop = box.scrollHeight;
  }

  async function sendMessage() {
    const input = $('#msg-input');
    if (!input || !S.currentConv) return;
    const content = input.value.trim();
    if (!content) return;
    input.value = '';

    const convId = S.currentConv.conversation_id;
    const clientMsgId = uid();

    // 乐观渲染：先显示"发送中"
    const optimistic = {
      id: -Date.now(),
      conversation_id: convId,
      sender_id: S.me.id,
      client_msg_id: clientMsgId,
      type: 1,
      content,
      created_at: new Date().toISOString(),
      pending: true,
    };
    S.messages.push(optimistic);
    renderMessages();

    // 优先走 WebSocket；未连接则降级 HTTP
    const sent = WS.send({ type: 'text', conversation_id: convId, client_msg_id: clientMsgId, content });
    if (!sent) {
      try {
        const m = await API.sendMessageHttp(convId, clientMsgId, content, 1);
        replaceOptimistic(clientMsgId, m);
      } catch (e) {
        toast(e.message, true);
        removeOptimistic(clientMsgId);
      }
    }
  }

  function replaceOptimistic(clientMsgId, real) {
    const optIdx = S.messages.findIndex(m => m.client_msg_id === clientMsgId && m.pending);
    const existsIdx = S.messages.findIndex(m => m.id === real.id);

    if (optIdx >= 0) {
      if (existsIdx >= 0 && existsIdx !== optIdx) {
        // MQ 推送可能先于 ack 到达，已经有一条真实消息了 → 删掉乐观占位
        S.messages.splice(optIdx, 1);
      } else {
        S.messages[optIdx] = real;
      }
    } else if (existsIdx < 0) {
      S.messages.push(real);
    }
    renderMessages();
    loadConversations();
  }

  function removeOptimistic(clientMsgId) {
    S.messages = S.messages.filter(m => !(m.client_msg_id === clientMsgId && m.pending));
    renderMessages();
  }

  // ==================== 好友 / 群聊 弹窗 ====================
  function openAddFriendModal() {
    const { mask, close } = openModal('添加好友',
      '<div class="form-row"><input id="search-kw" placeholder="输入用户名或昵称搜索" style="flex:1;height:34px;border:1px solid #e4e4e4;border-radius:5px;padding:0 10px" /></div>' +
      '<div id="search-result" style="margin-top:10px"></div>',
      '<button class="btn" data-close>关闭</button><button class="btn btn-green" id="btn-do-search">搜索</button>');

    const doSearch = async () => {
      const kw = $('#search-kw').value.trim();
      if (!kw) return;
      try {
        const d = await API.searchUsers(kw);
        const users = (d.users || []).filter(u => u.id !== S.me.id);
        $('#search-result').innerHTML = users.length
          ? users.map(u =>
              '<div class="list-item">' + avatarHtml(u.nickname || u.username, u.avatar) +
                '<div class="meta"><div class="name">' + esc(u.nickname || u.username) + '</div>' +
                '<div class="sub">@' + esc(u.username) + '</div></div>' +
                '<button class="btn btn-green" data-add="' + u.id + '">添加</button></div>').join('')
          : '<div class="list-empty">没有找到用户</div>';
        $$('[data-add]', $('#search-result')).forEach(btn => {
          btn.onclick = async () => {
            try {
              await API.sendFriendRequest(Number(btn.dataset.add), '你好，我是 ' + (S.me.nickname || S.me.username));
              toast('好友申请已发送');
              btn.disabled = true;
              btn.textContent = '已申请';
            } catch (e) { toast(e.message, true); }
          };
        });
      } catch (e) { toast(e.message, true); }
    };
    $('#btn-do-search').onclick = doSearch;
    $('#search-kw').addEventListener('keydown', e => { if (e.key === 'Enter') doSearch(); });
  }

  async function acceptRequest(id) {
    try { await API.acceptFriendRequest(id); toast('已接受'); await Promise.all([loadRequests(), loadFriends(), loadConversations()]); }
    catch (e) { toast(e.message, true); }
  }

  async function rejectRequest(id) {
    try { await API.rejectFriendRequest(id); toast('已拒绝'); await loadRequests(); }
    catch (e) { toast(e.message, true); }
  }

  function openFriendMenu(friendId) {
    friendId = Number(friendId);
    const f = S.friends.find(x => x.friend_id === friendId) || S.userNameCache[friendId] || {};
    const name = f.remark || f.nickname || f.username || ('用户' + friendId);
    const { close } = openModal('好友：' + name,
      '<div class="card" style="box-shadow:none;padding:0">' +
        '<div class="form-row"><label>备注</label><input id="friend-remark" value="' + esc(f.remark || '') + '" /></div>' +
      '</div>',
      '<button class="btn" id="btn-chat">发消息</button>' +
      '<button class="btn btn-green" id="btn-save-remark">保存备注</button>' +
      '<button class="btn btn-danger" id="btn-del-friend">删除好友</button>' +
      '<button class="btn btn-danger" id="btn-block">拉黑</button>');

    $('#btn-chat').onclick = async () => {
      close();
      try {
        await openOrCreateConversation(friendId, name);
      } catch (e) { toast(e.message, true); }
    };
    $('#btn-save-remark').onclick = async () => {
      try { await API.setRemark(friendId, $('#friend-remark').value.trim()); toast('备注已保存'); await loadFriends(); close(); }
      catch (e) { toast(e.message, true); }
    };
    $('#btn-del-friend').onclick = async () => {
      if (!confirm('确定删除好友？')) return;
      try { await API.deleteFriend(friendId); toast('已删除'); close(); await Promise.all([loadFriends(), loadConversations()]); }
      catch (e) { toast(e.message, true); }
    };
    $('#btn-block').onclick = async () => {
      try { await API.addBlacklist(friendId); toast('已拉黑'); close(); }
      catch (e) { toast(e.message, true); }
    };
  }

  async function openOrCreateConversation(peerId, peerName) {
    let conv = S.conversations.find(c => c.type === 1 && c.peer_user_id === peerId);
    if (!conv) {
      const d = await API.createConversation(peerId);
      await loadConversations();
      conv = S.conversations.find(c => c.conversation_id === d.conversation_id) || {
        conversation_id: d.conversation_id, type: 1, peer_user_id: peerId, peer_nickname: peerName,
      };
    }
    switchPanelKeep('chats');
    await openConversation(conv);
  }

  function switchPanelKeep(panel) {
    S.panel = panel;
    $$('.nav-btn').forEach(b => b.classList.toggle('active', b.dataset.panel === panel));
    renderList();
  }

  function openCreateGroupModal() {
    if (!S.friends.length) { toast('先加几个好友吧', true); return; }
    const { close } = openModal('发起群聊',
      '<div class="form-row"><label>群名称</label><input id="group-name" placeholder="例如：Go 学习小组" /></div>' +
      '<div class="list-subhead">选择成员</div>' +
      '<div class="check-list">' +
        S.friends.map(f =>
          '<label class="check-item"><input type="checkbox" value="' + f.friend_id + '" />' +
          avatarHtml(f.remark || f.nickname || f.username, f.avatar) +
          '<span>' + esc(f.remark || f.nickname || f.username) + '</span></label>').join('') +
      '</div>',
      '<button class="btn" data-close>取消</button><button class="btn btn-green" id="btn-create-group">创建</button>');

    $('#btn-create-group').onclick = async () => {
      const name = $('#group-name').value.trim();
      if (!name) { toast('请输入群名称', true); return; }
      const ids = $$('.check-list input:checked').map(x => Number(x.value));
      if (!ids.length) { toast('至少选一个成员', true); return; }
      try {
        await API.createGroup(name, ids);
        toast('群聊已创建');
        close();
        await Promise.all([loadGroups(), loadConversations()]);
        switchPanel('chats');
      } catch (e) { toast(e.message, true); }
    };
  }

  async function openGroupDetail(groupId) {
    if (!groupId) return;
    try {
      const g = await API.getGroup(groupId);
      const d = await API.listMembers(groupId);
      const members = d.members || [];
      const isOwner = g.owner_id === S.me.id;
      const { close } = openModal('群：' + g.name,
        '<div class="card" style="box-shadow:none;padding:0">' +
          '<div class="form-row"><label>群名称</label><input id="g-name" value="' + esc(g.name) + '" ' + (isOwner ? '' : 'disabled') + ' /></div>' +
          '<div class="form-row"><label>群公告</label><input id="g-ann" value="' + esc(g.announcement || '') + '" ' + (isOwner ? '' : 'disabled') + ' /></div>' +
          '<div class="sub" style="font-size:12px;color:#999">群主：' + (g.owner_id === S.me.id ? '我' : '用户' + g.owner_id) + ' · ' + members.length + ' 人</div>' +
        '</div>' +
        '<div class="list-subhead">成员</div>' +
        '<div class="check-list">' +
          members.map(m =>
            '<div class="check-item"><span style="flex:1">' + esc(m.nickname || m.username) +
              (m.role === 1 ? ' 👑' : '') + '</span>' +
              (isOwner && m.user_id !== g.owner_id
                ? '<button class="btn btn-danger" data-kick="' + m.user_id + '">移出</button>' : '') +
            '</div>').join('') +
        '</div>',
        (isOwner ? '<button class="btn btn-danger" id="btn-dissolve">解散群</button>' : '<button class="btn btn-danger" id="btn-leave">退出群</button>') +
        (isOwner ? '<button class="btn btn-green" id="btn-add-members">拉人</button>' : '') +
        '<button class="btn btn-green" id="btn-save-group">保存</button>' +
        '<button class="btn" data-close>关闭</button>');

      if (isOwner) {
        $('#btn-save-group').onclick = async () => {
          try {
            await API.updateGroup(groupId, { name: $('#g-name').value.trim(), announcement: $('#g-ann').value.trim() });
            toast('已保存'); close(); await loadGroups();
          } catch (e) { toast(e.message, true); }
        };
        $('#btn-add-members').onclick = () => openAddMembersModal(groupId, members);
        $('#btn-dissolve').onclick = async () => {
          if (!confirm('解散后不可恢复，确定？')) return;
          try { await API.dissolveGroup(groupId); toast('群已解散'); close(); await Promise.all([loadGroups(), loadConversations()]); }
          catch (e) { toast(e.message, true); }
        };
      } else {
        $('#btn-leave').onclick = async () => {
          if (!confirm('确定退出群？')) return;
          try { await API.removeGroupMember(groupId, S.me.id); toast('已退出'); close(); await Promise.all([loadGroups(), loadConversations()]); }
          catch (e) { toast(e.message, true); }
        };
      }
      $$('[data-kick]', $('#modal-root')).forEach(btn => {
        btn.onclick = async () => {
          try { await API.removeGroupMember(groupId, Number(btn.dataset.kick)); toast('已移出'); close(); openGroupDetail(groupId); }
          catch (e) { toast(e.message, true); }
        };
      });
    } catch (e) { toast(e.message, true); }
  }

  function openAddMembersModal(groupId, currentMembers) {
    const currentIds = new Set(currentMembers.map(m => m.user_id));
    const candidates = S.friends.filter(f => !currentIds.has(f.friend_id));
    if (!candidates.length) { toast('没有可邀请的好友了', true); return; }
    const { close } = openModal('拉人进群',
      '<div class="check-list">' +
        candidates.map(f =>
          '<label class="check-item"><input type="checkbox" value="' + f.friend_id + '" />' +
          avatarHtml(f.remark || f.nickname || f.username, f.avatar) +
          '<span>' + esc(f.remark || f.nickname || f.username) + '</span></label>').join('') +
      '</div>',
      '<button class="btn" data-close>取消</button><button class="btn btn-green" id="btn-add-ok">邀请</button>');
    $('#btn-add-ok').onclick = async () => {
      const ids = $$('.check-list input:checked').map(x => Number(x.value));
      if (!ids.length) { toast('请选择好友', true); return; }
      try { await API.addGroupMembers(groupId, ids); toast('已邀请'); close(); openGroupDetail(groupId); }
      catch (e) { toast(e.message, true); }
    };
  }

  // ==================== WebSocket 事件 ====================
  function bindWsEvents() {
    WS.on('open', () => {
      // 重连后重新拉数据
      if (S.me) { loadConversations(); loadRequests(); }
    });

    // 新消息推送（来自 MQ 消费者）
    WS.on('message', (msg) => {
      const m = msg.data;
      if (!m) return;
      const convId = m.conversation_id;
      if (S.currentConv && S.currentConv.conversation_id === convId) {
        appendMessage(m, true);
        API.markRead(convId, m.id).catch(() => {});
        const c = S.conversations.find(x => x.conversation_id === convId);
        if (c) { c.last_message_id = m.id; c.last_message_at = m.created_at; }
      } else {
        const c = S.conversations.find(x => x.conversation_id === convId);
        if (c) {
          c.unread = (c.unread || 0) + 1;
          c.last_message_id = m.id;
          c.last_message_at = m.created_at;
        } else {
          loadConversations();
        }
        if (S.panel === 'chats') renderList();
      }
      // 更新列表排序
      loadConversations();
    });

    // 发送 ack：把「发送中」替换成真实消息
    WS.on('ack', (msg) => {
      replaceOptimistic(msg.client_msg_id, {
        id: msg.message_id,
        conversation_id: msg.conversation_id,
        sender_id: S.me.id,
        client_msg_id: msg.client_msg_id,
        type: 1,
        content: (S.messages.find(x => x.client_msg_id === msg.client_msg_id) || {}).content || '',
        created_at: new Date().toISOString(),
      });
    });

    WS.on('error', (msg) => toast(msg.message || '操作失败', true));

    // ---------- 通话信令 ----------
    WS.on('call_ack', (msg) => {
      if (!S.call) return;
      S.call.callId = msg.call_id;
      setCallStatus('正在等待对方接听…');
    });

    WS.on('call_incoming', (msg) => {
      S.call = {
        callId: msg.call_id,
        peerId: msg.from_user_id,
        peerName: friendName(msg.from_user_id),
        direction: 'incoming',
        state: 'incoming',
      };
      renderCallOverlay();
    });

    WS.on('call_accepted', async (msg) => {
      if (!S.call) return;
      if (S.call.direction === 'outgoing') {
        // 主叫：收到接听 → 创建 offer 并发给被叫
        setCallStatus('接通中…');
        try {
          const offer = await RTC.createOffer();
          WS.send({ type: 'webrtc_offer', call_id: S.call.callId, to_user_id: S.call.peerId, sdp: offer });
        } catch (e) { toast('建立通话失败：' + e.message, true); hangupCall(); }
      }
      S.call.state = 'connecting';
      startCallTimer();
      renderCallOverlay();
    });

    WS.on('webrtc_offer', async (msg) => {
      if (!S.call) S.call = {
        callId: msg.call_id, peerId: msg.from_user_id, peerName: friendName(msg.from_user_id),
        direction: 'incoming', state: 'connecting',
      };
      try {
        const answer = await RTC.handleOffer(msg.sdp);
        WS.send({ type: 'webrtc_answer', call_id: msg.call_id, to_user_id: msg.from_user_id, sdp: answer });
        S.call.state = 'connecting';
        startCallTimer();
        renderCallOverlay();
      } catch (e) { toast('建立通话失败：' + e.message, true); hangupCall(); }
    });

    WS.on('webrtc_answer', async (msg) => {
      try { await RTC.handleAnswer(msg.sdp); } catch (e) { console.warn(e); }
    });

    WS.on('webrtc_ice', async (msg) => {
      try { await RTC.handleIce(msg.candidate); } catch (e) { console.warn(e); }
    });

    WS.on('call_rejected', () => { toast('对方已拒绝'); endCallUI(); });
    WS.on('call_canceled', () => { toast('对方已取消'); endCallUI(); });
    WS.on('call_ended', (msg) => {
      toast('通话已结束' + (msg.duration ? '（' + fmtDuration(msg.duration) + '）' : ''));
      endCallUI();
      loadCalls();
    });
  }

  function friendName(userId) {
    const f = S.friends.find(x => x.friend_id === userId);
    if (f) return f.remark || f.nickname || f.username;
    const c = S.userNameCache[userId];
    if (c) return c.nickname || c.username;
    return '用户' + userId;
  }

  // ==================== 通话 UI ====================
  RTC.setCallbacks({
    onIce: (candidate) => {
      if (S.call && S.call.callId) {
        WS.send({ type: 'webrtc_ice', call_id: S.call.callId, to_user_id: S.call.peerId, candidate });
      }
    },
    onState: (state) => {
      if (!S.call) return;
      if (state === 'connected' || state === 'ice:connected' || state === 'ice:completed') {
        S.call.state = 'active';
        renderCallOverlay();
      }
    },
  });

  async function startVoiceCall(conv) {
    if (S.call) { toast('当前已有通话', true); return; }
    try {
      // 先申请麦克风（用户手势内，避免被浏览器拦截）
      await RTC.createPeer();
    } catch (e) {
      toast('无法获取麦克风：' + e.message, true);
      return;
    }
    S.call = {
      callId: null,
      peerId: conv.peer_user_id,
      peerName: conv.peer_nickname || '用户',
      direction: 'outgoing',
      state: 'outgoing',
    };
    renderCallOverlay();
    WS.send({ type: 'call_invite', to_user_id: conv.peer_user_id, call_type: 1 });
  }

  async function acceptCall() {
    if (!S.call) return;
    try {
      await RTC.createPeer();   // 申请麦克风
    } catch (e) {
      toast('无法获取麦克风：' + e.message, true);
      hangupCall();
      return;
    }
    WS.send({ type: 'call_accept', call_id: S.call.callId });
    S.call.state = 'connecting';
    renderCallOverlay();
  }

  function rejectCall() {
    if (!S.call) return;
    WS.send({ type: 'call_reject', call_id: S.call.callId });
    endCallUI();
  }

  function hangupCall() {
    if (!S.call) return;
    if (S.call.direction === 'outgoing' && S.call.state === 'outgoing') {
      WS.send({ type: 'call_cancel', call_id: S.call.callId });
    } else {
      WS.send({ type: 'call_end', call_id: S.call.callId });
    }
    endCallUI();
  }

  function endCallUI() {
    stopCallTimer();
    RTC.release();
    S.call = null;
    const overlay = $('#call-overlay');
    overlay.hidden = true;
    overlay.innerHTML = '';
  }

  function setCallStatus(text) {
    if (S.call) S.call.statusText = text;
    renderCallOverlay();
  }

  function startCallTimer() {
    if (S.callStartAt) return;
    S.callStartAt = Date.now();
    stopCallTimer();
    S.callTimer = setInterval(() => renderCallOverlay(), 1000);
  }

  function stopCallTimer() {
    if (S.callTimer) { clearInterval(S.callTimer); S.callTimer = null; }
    S.callStartAt = 0;
  }

  function renderCallOverlay() {
    const overlay = $('#call-overlay');
    if (!S.call) { overlay.hidden = true; overlay.innerHTML = ''; return; }
    overlay.hidden = false;

    const statusMap = {
      outgoing: S.call.statusText || '正在呼叫…',
      incoming: '邀请你语音通话',
      connecting: '接通中…',
      active: S.callStartAt ? fmtDuration((Date.now() - S.callStartAt) / 1000) : '通话中',
    };
    const status = statusMap[S.call.state] || '';

    let actions = '';
    if (S.call.state === 'incoming') {
      actions =
        '<button class="call-btn reject" id="call-reject">拒绝</button>' +
        '<button class="call-btn accept" id="call-accept">接听</button>';
    } else if (S.call.state === 'outgoing' || S.call.state === 'connecting' || S.call.state === 'active') {
      actions =
        '<button class="call-btn mute" id="call-mute">静音</button>' +
        '<button class="call-btn hangup" id="call-hangup">挂断</button>';
    }

    overlay.innerHTML =
      '<div class="call-card">' +
        avatarHtml(S.call.peerName, '', 'call-avatar') +
        '<div class="call-name">' + esc(S.call.peerName) + '</div>' +
        '<div class="call-status">' + esc(status) + '</div>' +
        '<div class="call-actions">' + actions + '</div>' +
      '</div>';

    const acc = $('#call-accept');
    if (acc) acc.onclick = acceptCall;
    const rej = $('#call-reject');
    if (rej) rej.onclick = rejectCall;
    const hang = $('#call-hangup');
    if (hang) hang.onclick = hangupCall;
    const mute = $('#call-mute');
    if (mute) mute.onclick = () => {
      const m = RTC.toggleMute();
      mute.textContent = m ? '取消静音' : '静音';
    };
  }

  // ==================== 启动 ====================
  document.addEventListener('DOMContentLoaded', init);

  return { init };
})();
