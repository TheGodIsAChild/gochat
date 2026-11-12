/* =========================================================
   ws.js —— WebSocket 客户端
   看点：
   1. 浏览器 WebSocket 不能自定义 header，所以 token 走 query 参数
   2. 心跳 ping（服务端 60s 无 pong 会踢连接）
   3. 断线自动重连（指数退避，避免雪崩）
   4. 事件分发：on(type, handler)
   ========================================================= */

const WS = (() => {
  let sock = null;
  let heartbeatTimer = null;
  let reconnectTimer = null;
  let reconnectDelay = 1000;
  let manualClose = false;
  const handlers = {};   // type -> [fn]
  let statusHandler = null;

  function on(type, fn) {
    (handlers[type] = handlers[type] || []).push(fn);
  }

  function emit(type, payload) {
    (handlers[type] || []).forEach(fn => {
      try { fn(payload); } catch (e) { console.error('WS handler error', type, e); }
    });
    (handlers['*'] || []).forEach(fn => {
      try { fn(type, payload); } catch (e) { console.error(e); }
    });
  }

  function setStatusHandler(fn) { statusHandler = fn; }
  function status(s) { if (statusHandler) statusHandler(s); }

  function connect(token) {
    manualClose = false;
    if (sock && (sock.readyState === WebSocket.OPEN || sock.readyState === WebSocket.CONNECTING)) return;

    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = proto + '://' + location.host + '/ws?token=' + encodeURIComponent(token);
    sock = new WebSocket(url);

    sock.onopen = () => {
      reconnectDelay = 1000;
      status('connected');
      startHeartbeat();
      emit('open');
    };

    sock.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      if (!msg || !msg.type) return;
      emit(msg.type, msg);
    };

    sock.onclose = () => {
      stopHeartbeat();
      status('disconnected');
      emit('close');
      if (!manualClose) scheduleReconnect();
    };

    sock.onerror = () => { /* onclose 会处理 */ };
  }

  function scheduleReconnect() {
    if (reconnectTimer) return;
    const delay = reconnectDelay;
    reconnectDelay = Math.min(reconnectDelay * 2, 15000); // 指数退避，上限 15s
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      emit('reconnect');
    }, delay);
  }

  function startHeartbeat() {
    stopHeartbeat();
    heartbeatTimer = setInterval(() => {
      send({ type: 'ping' });
    }, 25000);
  }

  function stopHeartbeat() {
    if (heartbeatTimer) { clearInterval(heartbeatTimer); heartbeatTimer = null; }
  }

  function send(obj) {
    if (!sock || sock.readyState !== WebSocket.OPEN) return false;
    sock.send(JSON.stringify(obj));
    return true;
  }

  function close() {
    manualClose = true;
    stopHeartbeat();
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    if (sock) { try { sock.close(); } catch (e) {} sock = null; }
  }

  return { connect, close, send, on, setStatusHandler };
})();
