/* =========================================================
   api.js —— HTTP 客户端
   核心点：
   1. access token 放内存 + localStorage，refresh token 持久化
   2. 请求遇到 401 → 自动用 refresh 换新 token → 重试一次（无感刷新）
   3. 刷新做「单飞」（single-flight）：并发 401 只发一次刷新请求，
      避免 rotation 导致旧 refresh 被重复使用而被服务端判定盗用
   ========================================================= */

const API = (() => {
  const ACCESS_KEY = 'gochat_access';
  const REFRESH_KEY = 'gochat_refresh';

  let access = localStorage.getItem(ACCESS_KEY) || '';
  let refresh = localStorage.getItem(REFRESH_KEY) || '';
  let refreshingPromise = null;

  function setTokens(a, r) {
    access = a || '';
    refresh = r || '';
    if (access) localStorage.setItem(ACCESS_KEY, access); else localStorage.removeItem(ACCESS_KEY);
    if (refresh) localStorage.setItem(REFRESH_KEY, refresh); else localStorage.removeItem(REFRESH_KEY);
  }

  function clearTokens() { setTokens('', ''); }
  function hasLogin() { return !!(access || refresh); }
  function getAccess() { return access; }
  function getRefresh() { return refresh; }

  /** 用 refresh token 换新 token 对 */
  async function doRefresh() {
    if (!refresh) throw new Error('无 refresh token');
    const res = await fetch('/api/v1/token/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refresh }),
    });
    if (!res.ok) throw new Error('刷新登录态失败');
    const j = await res.json();
    setTokens(j.data.access_token, j.data.refresh_token);
  }

  /** 单飞刷新：并发请求共享同一个 Promise */
  function refreshOnce() {
    if (!refreshingPromise) {
      refreshingPromise = doRefresh().finally(() => { refreshingPromise = null; });
    }
    return refreshingPromise;
  }

  /**
   * 发起请求
   * @returns data 部分；失败抛 Error(message)
   */
  async function request(method, path, body, opts) {
    opts = opts || {};
    const auth = opts.auth !== false;
    const retry = opts.retry !== false;

    const headers = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (auth && access) headers['Authorization'] = 'Bearer ' + access;

    let res;
    try {
      res = await fetch(path, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
      });
    } catch (e) {
      throw new Error('网络错误：' + e.message);
    }

    // 401 → 尝试无感刷新后重试一次
    if (res.status === 401 && auth && retry && refresh) {
      try {
        await refreshOnce();
        return request(method, path, body, { auth, retry: false });
      } catch (e) {
        clearTokens();
        throw e;
      }
    }

    let json = null;
    try { json = await res.json(); } catch (e) { /* 空响应 */ }

    if (!res.ok) {
      const msg = (json && json.message) ? json.message : ('请求失败 HTTP ' + res.status);
      throw new Error(msg);
    }
    return json ? json.data : null;
  }

  /** 登录成功后的统一处理 */
  async function afterLogin(data) {
    setTokens(data.access_token, data.refresh_token);
    return data.user;
  }

  return {
    setTokens, clearTokens, hasLogin, getAccess, getRefresh, request,
    // ---- 认证 ----
    register: (username, password, nickname) => request('POST', '/api/v1/register', { username, password, nickname }, { auth: false }),
    login: async (username, password) => {
      const d = await request('POST', '/api/v1/login', { username, password }, { auth: false });
      return afterLogin(d);
    },
    logout: (refreshToken) => request('POST', '/api/v1/logout', { refresh_token: refreshToken }).catch(() => {}),

    // ---- 用户 ----
    me: () => request('GET', '/api/v1/users/me'),
    updateMe: (patch) => request('PUT', '/api/v1/users/me', patch),
    searchUsers: (keyword) => request('GET', '/api/v1/users/search?keyword=' + encodeURIComponent(keyword)),
    getSettings: () => request('GET', '/api/v1/users/settings'),
    updateSettings: (patch) => request('PUT', '/api/v1/users/settings', patch),

    // ---- 好友 ----
    sendFriendRequest: (toUserId, message) => request('POST', '/api/v1/friends/requests', { to_user_id: toUserId, message: message || '' }),
    listFriendRequests: () => request('GET', '/api/v1/friends/requests'),
    acceptFriendRequest: (id) => request('POST', '/api/v1/friends/requests/' + id + '/accept'),
    rejectFriendRequest: (id) => request('POST', '/api/v1/friends/requests/' + id + '/reject'),
    listFriends: () => request('GET', '/api/v1/friends'),
    deleteFriend: (friendId) => request('DELETE', '/api/v1/friends/' + friendId),
    setRemark: (friendId, remark) => request('PUT', '/api/v1/friends/' + friendId + '/remark', { remark }),
    listBlacklist: () => request('GET', '/api/v1/blacklist'),
    addBlacklist: (userId) => request('POST', '/api/v1/blacklist', { user_id: userId }),
    removeBlacklist: (userId) => request('DELETE', '/api/v1/blacklist/' + userId),

    // ---- 会话 / 消息 ----
    listConversations: () => request('GET', '/api/v1/conversations'),
    createConversation: (peerUserId) => request('POST', '/api/v1/conversations', { peer_user_id: peerUserId }),
    listMessages: (convId, beforeId, limit) => {
      let url = '/api/v1/conversations/' + convId + '/messages?limit=' + (limit || 30);
      if (beforeId) url += '&before_id=' + beforeId;
      return request('GET', url);
    },
    sendMessageHttp: (convId, clientMsgId, content, type) =>
      request('POST', '/api/v1/conversations/' + convId + '/messages',
        { client_msg_id: clientMsgId, type: type || 1, content }),
    markRead: (convId, messageId) => request('POST', '/api/v1/conversations/' + convId + '/read', { message_id: messageId }),
    onlineUsers: (ids) => request('GET', '/api/v1/online?ids=' + ids.join(',')),

    // ---- 群聊 ----
    createGroup: (name, memberIds, announcement) =>
      request('POST', '/api/v1/groups', { name, member_ids: memberIds, announcement: announcement || '' }),
    listGroups: () => request('GET', '/api/v1/groups'),
    getGroup: (id) => request('GET', '/api/v1/groups/' + id),
    listMembers: (id) => request('GET', '/api/v1/groups/' + id + '/members'),
    addGroupMembers: (id, memberIds) => request('POST', '/api/v1/groups/' + id + '/members', { member_ids: memberIds }),
    removeGroupMember: (id, userId) => request('DELETE', '/api/v1/groups/' + id + '/members/' + userId),
    updateGroup: (id, patch) => request('PUT', '/api/v1/groups/' + id, patch),
    dissolveGroup: (id) => request('POST', '/api/v1/groups/' + id + '/dissolve'),

    // ---- 通话 ----
    listCalls: (limit) => request('GET', '/api/v1/calls?limit=' + (limit || 30)),
    getCall: (id) => request('GET', '/api/v1/calls/' + id),
  };
})();
