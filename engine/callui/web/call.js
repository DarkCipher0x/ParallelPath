// ParallelPath call page. WebRTC carries the media (directly between devices where possible, else through the
// server's TURN relay); frames.js encrypts every audio and video frame end to end with keys from the call key.
// Call setup (offers, answers, ICE candidates) goes through the engine, which seals each message with the family's
// keys before the server relays it.
//
// Runs in three places: the web app (an iframe next to the app: the engine is window.parent), the Windows app
// (an Edge app window) and the Android app (a WebView) - those two talk to the device's engine over loopback.
"use strict";

(() => {
  const hash = new URLSearchParams(location.hash.slice(1));
  const token = new URLSearchParams(location.search).get("t") || "";
  const space = hash.get("space"), callId = hash.get("call");
  const role = hash.get("role") === "callee" ? "callee" : "caller";
  const host = window.parent !== window && typeof window.parent.ppCall === "function" ? window.parent : null;

  const $ = (id) => document.getElementById(id);
  const status = (t) => { $("status").textContent = t; };
  const notice = (t, ms = 5000) => { const n = $("notice"); n.textContent = t; n.hidden = false; clearTimeout(notice.t); notice.t = setTimeout(() => { n.hidden = true; }, ms); };

  // ---- the engine, wherever it is
  async function rpc(method, args) {
    if (host) {
      const r = await host.ppCall(method, JSON.stringify(args || {}));
      if (r[0] === "1") return JSON.parse(r.slice(1));
      throw new Error(r.slice(1));
    }
    const res = await fetch("api/rpc", { method: "POST", headers: { "X-PP-Token": token, "Content-Type": "application/json" },
      body: JSON.stringify({ method, args: args || {} }) });
    const j = await res.json();
    if (j.error) throw new Error(j.error);
    return j.result;
  }
  function listen(cb) {
    if (host) return host.ppSubscribeCall((json) => cb(JSON.parse(json)));
    const es = new EventSource("api/events?t=" + encodeURIComponent(token));
    es.onmessage = (e) => cb(JSON.parse(e.data));
    return () => es.close();
  }
  const signal = (type, to, extra) => rpc("call_signal", Object.assign({ space, call: callId, type, to_device: to || "" }, extra || {}))
    .catch((e) => console.warn("signal", type, e));

  // ---- state
  let session, ice = [], local = null, camera = null, screen = null, started = 0, timer = 0, ended = false;
  const peers = new Map(); // device -> {pc, name, tile, pending: [], senders}
  const worker = new Worker("frames.js");
  const legacyTransform = typeof RTCRtpSender !== "undefined" && "createEncodedStreams" in RTCRtpSender.prototype;
  const scriptTransform = !legacyTransform && typeof RTCRtpScriptTransform !== "undefined";
  worker.onmessage = (e) => { if (e.data.op === "failing") notice("Some frames from the other side couldn't be decrypted and were dropped."); };

  function protect(sender, kind, device) {
    if (scriptTransform) sender.transform = new RTCRtpScriptTransform(worker, { mode: "encrypt", kind, device });
    else { const s = sender.createEncodedStreams(); worker.postMessage({ op: "stream", mode: "encrypt", kind, device, readable: s.readable, writable: s.writable }, [s.readable, s.writable]); }
  }
  function unprotect(receiver, kind, device) {
    if (scriptTransform) receiver.transform = new RTCRtpScriptTransform(worker, { mode: "decrypt", kind, device });
    else { const s = receiver.createEncodedStreams(); worker.postMessage({ op: "stream", mode: "decrypt", kind, device, readable: s.readable, writable: s.writable }, [s.readable, s.writable]); }
  }

  // ---- media: HD video (up to 1080p30) and full-band audio
  // A synthetic camera and microphone (#test=1): a moving pattern and a quiet tone, for testing on machines without
  // devices. Everything else - encryption included - is exactly as in a real call.
  function testMedia(video) {
    const out = new MediaStream();
    const ac = new AudioContext();
    const osc = ac.createOscillator(), gain = ac.createGain(), dst = ac.createMediaStreamDestination();
    osc.frequency.value = 440; gain.gain.value = 0.05;
    osc.connect(gain).connect(dst); osc.start();
    out.addTrack(dst.stream.getAudioTracks()[0]);
    if (video) {
      const c = document.createElement("canvas"); c.width = 1280; c.height = 720;
      const g = c.getContext("2d");
      let f = 0;
      setInterval(() => {
        g.fillStyle = "#0b1517"; g.fillRect(0, 0, c.width, c.height);
        g.fillStyle = "#3dd6d0"; g.fillRect((f * 8) % c.width, 200, 160, 160);
        g.fillStyle = "#f2a65a"; g.font = "48px sans-serif"; g.fillText("ParallelPath test camera " + f++, 60, 600);
      }, 33);
      out.addTrack(c.captureStream(30).getVideoTracks()[0]);
    }
    return out;
  }

  async function openMedia(video) {
    if (hash.get("test") === "1") return testMedia(video);
    const audio = { echoCancellation: true, noiseSuppression: true, autoGainControl: true, channelCount: 1, sampleRate: 48000 };
    const cam = { width: { ideal: 1920, max: 1920 }, height: { ideal: 1080, max: 1080 }, frameRate: { ideal: 30, max: 30 }, facingMode: "user" };
    try {
      return await navigator.mediaDevices.getUserMedia({ audio, video: video ? cam : false });
    } catch (e) {
      if (video) {
        try { notice("Your camera isn't available - joining with sound only."); return await navigator.mediaDevices.getUserMedia({ audio }); } catch (e2) { /* fall through */ }
      }
      notice("Your microphone and camera aren't available - you can still see and hear the others.", 8000);
      return new MediaStream();
    }
  }

  function tune(pc) {
    for (const t of pc.getTransceivers()) {
      const kind = t.sender.track ? t.sender.track.kind : (t.receiver.track && t.receiver.track.kind);
      if (kind === "video" && t.setCodecPreferences && RTCRtpReceiver.getCapabilities) {
        // VP8 keeps the end-to-end frame encryption compatible with every browser's packetiser.
        const caps = RTCRtpReceiver.getCapabilities("video").codecs;
        const vp8 = caps.filter((c) => /vp8/i.test(c.mimeType)), rest = caps.filter((c) => !/vp8/i.test(c.mimeType) && /rtx|red|ulpfec/i.test(c.mimeType));
        if (vp8.length) try { t.setCodecPreferences(vp8.concat(rest)); } catch (e) { /* not supported */ }
      }
    }
  }

  async function hd(sender) {
    if (!sender.track) return;
    const p = sender.getParameters();
    if (!p.encodings || !p.encodings.length) p.encodings = [{}];
    if (sender.track.kind === "video") {
      p.encodings[0].maxBitrate = 4_000_000;
      p.encodings[0].maxFramerate = 30;
      p.degradationPreference = "balanced";
    } else {
      p.encodings[0].maxBitrate = 128_000;
      p.encodings[0].priority = "high";
    }
    try { await sender.setParameters(p); } catch (e) { /* older browsers */ }
  }

  // ---- peers
  function tileFor(device, name) {
    const tile = document.createElement("div");
    tile.className = "remote";
    tile.innerHTML = '<video autoplay playsinline></video><div class="novideo"><div class="avatar"></div></div><div class="name"></div>';
    tile.querySelector(".name").textContent = name || "";
    tile.querySelector(".avatar").textContent = initials(name);
    $("remotes").appendChild(tile);
    layout();
    return tile;
  }
  function layout() {
    const n = Math.min(peers.size, 4);
    $("remotes").className = n > 1 ? "n" + n : "";
    $("waiting").classList.toggle("hidden", [...peers.values()].some((p) => p.connected));
  }
  const initials = (name) => (name || "?").split(/[\s-]+/).filter(Boolean).slice(0, 2).map((w) => w[0].toUpperCase()).join("") || "?";

  function newPeer(device, name) {
    if (peers.has(device)) return peers.get(device);
    const pc = new RTCPeerConnection(Object.assign({ iceServers: ice, bundlePolicy: "max-bundle" }, legacyTransform ? { encodedInsertableStreams: true } : {}));
    const p = { pc, name, pending: [], tile: tileFor(device, name), connected: false };
    peers.set(device, p);
    const stream = new MediaStream();
    p.tile.querySelector("video").srcObject = stream;
    for (const track of local.getTracks()) protect(pc.addTrack(track, local), track.kind, session.device);
    if (!local.getVideoTracks().length && session.video) pc.addTransceiver("video", { direction: "recvonly" });
    if (!local.getAudioTracks().length) pc.addTransceiver("audio", { direction: "recvonly" });
    tune(pc);
    pc.ontrack = (e) => {
      unprotect(e.receiver, e.track.kind, device);
      stream.addTrack(e.track);
      if (e.track.kind === "video") {
        const nv = p.tile.querySelector(".novideo"), v = p.tile.querySelector("video");
        const show = (on) => { nv.style.display = on ? "none" : ""; };
        v.addEventListener("loadeddata", () => show(true));
        v.addEventListener("resize", () => show(true));
        e.track.addEventListener("mute", () => show(false));
        e.track.addEventListener("unmute", () => show(true));
        show(!e.track.muted);
      }
    };
    pc.onicecandidate = (e) => { if (e.candidate) signal("ice", device, { candidate: e.candidate.toJSON() }); };
    pc.onconnectionstatechange = () => {
      const s = pc.connectionState;
      if (s === "connected") {
        p.connected = true;
        clearTimeout(p.lost);
        p.lost = 0;
        if (!started) startTimer();
        status("Connected");
        for (const snd of pc.getSenders()) hd(snd);
      } else if (s === "disconnected" || s === "failed") {
        if (s === "failed") {
          notice(name + "'s connection failed - trying again…");
          pc.restartIce && pc.restartIce();
        }
        // Someone whose device vanished (no hang-up: battery, network gone) is dropped after 30 s, and the call
        // ends when nobody is left - rather than showing their last frame forever.
        if (!p.lost) p.lost = setTimeout(() => { if (pc.connectionState !== "connected") dropPeer(device, name + " lost the connection."); }, 30000);
      } else if (s === "closed") {
        p.connected = false;
      }
      layout();
    };
    return p;
  }

  function dropPeer(device, why) {
    const p = peers.get(device);
    if (!p) return;
    p.pc.close();
    p.tile.remove();
    peers.delete(device);
    layout();
    if (why) notice(why);
    if (started && peers.size === 0) finish(false);
  }

  async function offerTo(device, name) {
    const p = newPeer(device, name);
    const offer = await p.pc.createOffer();
    await p.pc.setLocalDescription(offer);
    signal("offer", device, { sdp: p.pc.localDescription.sdp });
  }

  async function onEvent(ev) {
    if (ev.call !== callId || ended) return;
    const from = ev.from_device;
    switch (ev.type) {
      case "accept": // someone picked up (the caller connects to them, and tells them who else is here)
        if (session.outgoing) {
          clearTimeout(ringTimer);
          const others = [...peers.keys()];
          status("Connecting to " + ev.from_name + "…");
          await offerTo(from, ev.from_name);
          if (others.length) signal("join", from, { peers: others });
        }
        break;
      case "join": // the caller listed who's already here: connect to each of them too
        for (const d of ev.peers || []) if (d !== session.device && !peers.has(d)) await offerTo(d, "");
        break;
      case "offer": {
        const p = newPeer(from, ev.from_name);
        await p.pc.setRemoteDescription({ type: "offer", sdp: ev.sdp });
        for (const c of p.pending.splice(0)) await p.pc.addIceCandidate(c).catch(() => {});
        await p.pc.setLocalDescription(await p.pc.createAnswer());
        signal("answer", from, { sdp: p.pc.localDescription.sdp });
        break;
      }
      case "answer": {
        const p = peers.get(from);
        if (p && p.pc.signalingState === "have-local-offer") {
          await p.pc.setRemoteDescription({ type: "answer", sdp: ev.sdp });
          for (const c of p.pending.splice(0)) await p.pc.addIceCandidate(c).catch(() => {});
        }
        break;
      }
      case "ice": {
        const p = peers.get(from);
        if (!p || !ev.candidate) break;
        if (p.pc.remoteDescription) await p.pc.addIceCandidate(ev.candidate).catch(() => {});
        else p.pending.push(ev.candidate);
        break;
      }
      case "decline":
        if (session.outgoing && peers.size === 0) { status(ev.from_name + " declined the call"); setTimeout(() => finish(false), 2500); }
        break;
      case "busy":
        if (session.outgoing && peers.size === 0) { status(ev.from_name + " is on another call"); setTimeout(() => finish(false), 2500); }
        break;
      case "hangup":
        if (peers.has(from)) dropPeer(from, (ev.from_name || "Someone") + " left the call");
        else if (!session.outgoing && peers.size === 0) { status("The call ended"); setTimeout(() => finish(false), 1500); }
        break;
    }
  }

  // ---- controls
  function setButton(id, off, offTitle, onTitle) {
    const b = $(id);
    b.classList.toggle("off", off);
    b.title = off ? offTitle : onTitle;
    b.setAttribute("aria-label", b.title);
  }
  $("mic").onclick = () => {
    const t = local.getAudioTracks()[0];
    if (!t) return;
    t.enabled = !t.enabled;
    setButton("mic", !t.enabled, "Unmute microphone", "Mute microphone");
  };
  $("cam").onclick = () => {
    const t = camera && camera.getVideoTracks()[0];
    if (!t) return;
    t.enabled = !t.enabled;
    $("local").classList.toggle("off", !t.enabled);
    setButton("cam", !t.enabled, "Turn camera on", "Turn camera off");
  };
  let facing = "user";
  $("flip").onclick = async () => {
    facing = facing === "user" ? "environment" : "user";
    try {
      const s = await navigator.mediaDevices.getUserMedia({ video: { facingMode: facing, width: { ideal: 1920 }, height: { ideal: 1080 }, frameRate: { ideal: 30 } } });
      const t = s.getVideoTracks()[0];
      await replaceVideo(t);
      const old = camera.getVideoTracks()[0];
      camera.removeTrack(old); old.stop(); camera.addTrack(t);
      $("local").style.transform = facing === "user" ? "scaleX(-1)" : "none";
    } catch (e) { notice("Couldn't switch camera"); }
  };
  async function replaceVideo(track) {
    for (const p of peers.values()) for (const s of p.pc.getSenders()) if (s.track && s.track.kind === "video") await s.replaceTrack(track);
    $("local").srcObject = new MediaStream([track]);
  }
  $("share").onclick = async () => {
    if (screen) { stopShare(); return; }
    try {
      screen = await navigator.mediaDevices.getDisplayMedia({ video: { width: { ideal: 1920 }, height: { ideal: 1080 }, frameRate: { ideal: 30 } }, audio: false });
      const t = screen.getVideoTracks()[0];
      t.contentHint = "detail";
      t.onended = stopShare;
      await replaceVideo(t);
      $("share").classList.add("on");
      $("local").style.transform = "none";
    } catch (e) { screen = null; }
  };
  async function stopShare() {
    if (!screen) return;
    screen.getTracks().forEach((t) => t.stop());
    screen = null;
    $("share").classList.remove("on");
    const t = camera && camera.getVideoTracks()[0];
    if (t) await replaceVideo(t);
    $("local").style.transform = facing === "user" ? "scaleX(-1)" : "none";
  }
  $("hangup").onclick = () => finish(false);

  function startTimer() {
    started = Date.now();
    timer = setInterval(() => {
      const s = Math.floor((Date.now() - started) / 1000);
      $("timer").textContent = (s >= 3600 ? Math.floor(s / 3600) + ":" : "") + String(Math.floor(s / 60) % 60).padStart(2, "0") + ":" + String(s % 60).padStart(2, "0");
    }, 1000);
    setInterval(quality, 3000);
  }

  async function quality() {
    let res = "", fps = 0, rtt = 0, relayed = false;
    for (const p of peers.values()) {
      const stats = await p.pc.getStats();
      stats.forEach((r) => {
        if (r.type === "inbound-rtp" && r.kind === "video" && r.frameHeight) { res = r.frameHeight + "p"; fps = Math.round(r.framesPerSecond || 0); }
        if (r.type === "candidate-pair" && r.state === "succeeded" && r.nominated) {
          rtt = Math.round((r.currentRoundTripTime || 0) * 1000);
          const lc = stats.get(r.localCandidateId);
          relayed = relayed || (lc && lc.candidateType === "relay");
        }
      });
    }
    const hdLabel = res ? (parseInt(res) >= 1080 ? "Full HD " : parseInt(res) >= 720 ? "HD " : "") + res + (fps ? " · " + fps + " fps" : "") : "";
    $("quality").textContent = [hdLabel, rtt ? rtt + " ms" : "", relayed ? "via your server" : (rtt ? "direct" : "")].filter(Boolean).join(" · ");
  }

  let ringTimer = 0;
  async function finish(missed) {
    if (ended) return;
    ended = true;
    clearInterval(timer);
    const seconds = started ? Math.round((Date.now() - started) / 1000) : 0;
    for (const p of peers.values()) p.pc.close();
    peers.clear();
    if (local) local.getTracks().forEach((t) => t.stop());
    if (screen) screen.getTracks().forEach((t) => t.stop());
    await rpc("call_end", { space, call: callId, seconds, missed }).catch(() => {});
    if (host && host.ppCloseCall) { host.ppCloseCall(); return; }
    document.body.innerHTML = '<div id="waiting"><div class="who">Call ended</div><div class="status">You can close this window.</div></div>';
    setTimeout(() => window.close(), 300);
  }
  window.addEventListener("pagehide", () => { if (!ended) finish(false); });

  // ---- start
  (async () => {
    if (!space || !callId) { status("This call link is incomplete."); return; }
    if (!scriptTransform && !legacyTransform) {
      status("This browser can't encrypt call media end to end, so the call can't start here. Use a current Chrome, Edge, Firefox or Safari.");
      return;
    }
    try {
      session = await rpc("call_session", { space, call: callId });
    } catch (e) { status("This call has ended."); return; }
    const raw = Uint8Array.from(atob(session.key), (c) => c.charCodeAt(0));
    worker.postMessage({ op: "key", key: raw.buffer }, [raw.buffer]);
    $("family").textContent = session.family || "";
    const other = session.outgoing ? (session.family || "your family") : session.caller;
    $("who").textContent = other;
    $("who-initials").textContent = initials(other);
    try { ice = ((await rpc("call_turn", {})) || {}).ice_servers || []; } catch (e) { ice = []; }

    status(session.video ? "Starting your camera…" : "Starting your microphone…");
    local = await openMedia(session.video);
    camera = local;
    if (local.getVideoTracks().length) {
      $("local").srcObject = new MediaStream(local.getVideoTracks());
      local.getVideoTracks()[0].contentHint = "motion";
      $("flip").hidden = !/Android|iPhone|iPad/.test(navigator.userAgent);
    } else {
      $("local").classList.add("off");
      $("cam").hidden = true;
    }
    $("share").hidden = !(navigator.mediaDevices && navigator.mediaDevices.getDisplayMedia) || /Android|iPhone|iPad/.test(navigator.userAgent);

    listen(onEvent);
    if (session.outgoing) {
      status("Calling…");
      ringTimer = setTimeout(() => { if (peers.size === 0) { status("No answer"); setTimeout(() => finish(true), 2000); } }, 45000);
    } else {
      status("Connecting…");
      signal("accept", "");
    }
  })();
})();
