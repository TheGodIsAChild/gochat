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

   【localhost 演示说明】
   http://127.0.0.1 属于安全上下文，可以调 getUserMedia；
   同一台机器两个浏览器窗口之间，主机候选（host candidate）就能直连，
   因此这里的 ICE 服务器留空也能通。跨机器演示需要加 STUN/TURN。
   ========================================================= */

const RTC = (() => {
  let pc = null;
  let localStream = null;
  let remoteAudio = null;
  let muted = false;

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
    localStream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
    return localStream;
  }

  /** 创建 PeerConnection */
  async function createPeer() {
    await ensureMedia();
    closePeer();

    // 本机演示不需要 STUN；跨网演示可换成：
    // iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    pc = new RTCPeerConnection({ iceServers: [] });

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
