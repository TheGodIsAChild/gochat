/* =========================================================
   webrtc.js —— 浏览器端 WebRTC 语音通话（P2P）
   ---------------------------------------------------------
   服务端只做信令转发（offer/answer/ice），音视频走 P2P。
   本文件负责：
     1. 采集麦克风（getUserMedia）
     2. 创建 RTCPeerConnection
     3. 生成 offer / answer
     4. 交换 ICE candidate
     5. 播放对方音频

   【跨设备演示说明（重要）】
   1) 浏览器只在「安全上下文」允许采集麦克风：
      - http://localhost、http://127.0.0.1 可以
      - 通过 http://<服务器IP> 访问 **不行**，必须上 HTTPS（如 Nginx + 证书）
   2) 两台设备跨网络时，需要 STUN 才能发现公网地址；
      严格 NAT（4G / 企业网）还需要 TURN 中继。
   3) STUN/TURN 由后端 GET /api/v1/rtc/config 下发，换环境不用改前端代码。
   ========================================================= */

const RTC = (() => {
  let pc = null;
  let localStream = null;
  let remoteAudio = null;
  let muted = false;

  // ICE 服务器：默认内置公共 STUN，启动后会被后端配置覆盖
  let iceServers = [
    { urls: ['stun:stun.l.google.com:19302', 'stun:stun.miwifi.com:3478'] },
  ];
  let iceConfigLoaded = false;

  /** 从后端拉取 ICE 配置（只拉一次）；失败时用内置默认值 */
  async function loadIceServers() {
    if (iceConfigLoaded) return iceServers;
    iceConfigLoaded = true; // 避免并发重复请求
    try {
      const res = await fetch('/api/v1/rtc/config');
      const json = await res.json();
      const list = (json && json.data && json.data.ice_servers) || [];
      if (Array.isArray(list) && list.length > 0) {
        iceServers = list;
      }
    } catch (e) {
      console.warn('[RTC] 获取 ICE 配置失败，使用内置 STUN', e);
    }
    return iceServers;
  }

  const callbacks = {
    onState: null,     // (state) => void
    onIce: null,       // (candidate) => void
    onStream: null,    // () => void
  };

  function setCallbacks(cbs) { Object.assign(callbacks, cbs || {}); }

  /** 打开麦克风（只调一次，复用） */
  async function ensureMedia() {
    if (localStream) return localStream;
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
      throw new Error('当前浏览器不支持麦克风采集（需要 localhost 或 HTTPS）');
    }
    // 跨设备用 http://IP 访问时，浏览器会禁用麦克风，这里给出明确提示
    const isLocal = location.hostname === 'localhost' || location.hostname === '127.0.0.1';
    if (location.protocol !== 'https:' && !isLocal) {
      throw new Error('语音通话需要 HTTPS：当前是 http://' + location.hostname +
        '，浏览器禁止麦克风。请用 HTTPS 域名访问（或在本机用 localhost 演示）');
    }
    localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
    return localStream;
  }

  /** 创建 PeerConnection */
  async function createPeer() {
    await ensureMedia();
    closePeer();

    // ICE 服务器来自后端配置（含默认公共 STUN），跨网络通话必需
    const servers = await loadIceServers();
    pc = new RTCPeerConnection({ iceServers: servers });

    localStream.getTracks().forEach(track => pc.addTrack(track, localStream));

    // 交换 ICE
    pc.onicecandidate = (e) => {
      if (e.candidate && callbacks.onIce) {
        callbacks.onIce(e.candidate.toJSON ? e.candidate.toJSON() : e.candidate);
      }
    };

    // 播放对方音频
    pc.ontrack = (e) => {
      if (!remoteAudio) {
        remoteAudio = new Audio();
        remoteAudio.autoplay = true;
      }
      remoteAudio.srcObject = e.streams[0];
      remoteAudio.play().catch(() => { /* 自动播放限制，用户已点击可忽略 */ });
      if (callbacks.onStream) callbacks.onStream();
    };

    // 连接状态
    pc.onconnectionstatechange = () => {
      if (callbacks.onState) callbacks.onState(pc.connectionState);
    };
    pc.oniceconnectionstatechange = () => {
      if (callbacks.onState) callbacks.onState('ice:' + pc.iceConnectionState);
    };

    return pc;
  }

  /** 主叫：生成 offer */
  async function createOffer() {
    if (!pc) await createPeer();
    const offer = await pc.createOffer({ offerToReceiveAudio: true });
    await pc.setLocalDescription(offer);
    return { type: 'offer', sdp: pc.localDescription.sdp };
  }

  /** 被叫：收到 offer → 生成 answer */
  async function handleOffer(sdp) {
    if (!pc) await createPeer();
    await pc.setRemoteDescription(new RTCSessionDescription(sdp));
    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);
    return { type: 'answer', sdp: pc.localDescription.sdp };
  }

  /** 主叫：收到 answer */
  async function handleAnswer(sdp) {
    if (!pc) return;
    await pc.setRemoteDescription(new RTCSessionDescription(sdp));
  }

  /** 收到 ICE candidate */
  async function handleIce(candidate) {
    if (!pc || !candidate) return;
    try {
      await pc.addIceCandidate(new RTCIceCandidate(candidate));
    } catch (e) {
      console.warn('addIceCandidate 失败', e);
    }
  }

  function toggleMute() {
    if (!localStream) return false;
    muted = !muted;
    localStream.getAudioTracks().forEach(t => { t.enabled = !muted; });
    return muted;
  }

  function isMuted() { return muted; }

  function closePeer() {
    if (pc) {
      try { pc.close(); } catch (e) {}
      pc = null;
    }
    if (remoteAudio) {
      try { remoteAudio.pause(); } catch (e) {}
      remoteAudio.srcObject = null;
      remoteAudio = null;
    }
  }

  /** 彻底释放（挂断时调用） */
  function release() {
    closePeer();
    if (localStream) {
      localStream.getTracks().forEach(t => t.stop());
      localStream = null;
    }
    muted = false;
  }

  return {
    setCallbacks, createPeer, createOffer, handleOffer, handleAnswer, handleIce,
    toggleMute, isMuted, release, closePeer,
  };
})();
