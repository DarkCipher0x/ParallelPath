// The call's end-to-end frame encryption, in a worker. Every encoded audio and video frame is sealed with
// AES-256-GCM under a key derived (HKDF-SHA-256) from the call key for the sending device - the call key itself
// was shared inside a message sealed with the family's post-quantum keys. The first few bytes of each frame stay
// readable so WebRTC can still packetise it (as in Jitsi and the IETF SFrame work); they are authenticated as AAD.
//
// Frame layout: header (n bytes, clear) | ciphertext + GCM tag | IV (12 bytes) | n (1 byte)
"use strict";

let setBase;
const base = new Promise((resolve) => { setBase = resolve; }); // the call key, imported once it arrives
const keys = new Map();
const enc = (s) => new TextEncoder().encode(s);

function keyFor(device) {
  if (!keys.has(device)) {
    keys.set(device, base.then((b) => crypto.subtle.deriveKey(
      { name: "HKDF", hash: "SHA-256", salt: enc("pp-call-frames-v1"), info: enc(device) },
      b, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"])));
  }
  return keys.get(device);
}

// How many bytes stay readable for WebRTC's packetiser (VP8 payload header; Opus TOC byte).
function clearBytes(kind, frame) {
  if (kind === "audio") return 1;
  return frame.type === "key" ? 10 : 3;
}

function encrypter(kind, device) {
  const salt = crypto.getRandomValues(new Uint8Array(4));
  let counter = 0n;
  return new TransformStream({
    async transform(frame, controller) {
      const data = new Uint8Array(frame.data);
      const n = Math.min(clearBytes(kind, frame), data.length);
      const iv = new Uint8Array(12);
      iv.set(salt, 0);
      new DataView(iv.buffer).setBigUint64(4, counter++);
      const key = await keyFor(device);
      const ct = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv, additionalData: data.subarray(0, n) }, key, data.subarray(n)));
      const out = new Uint8Array(n + ct.length + 13);
      out.set(data.subarray(0, n), 0);
      out.set(ct, n);
      out.set(iv, n + ct.length);
      out[out.length - 1] = n;
      frame.data = out.buffer;
      controller.enqueue(frame);
    },
  });
}

function decrypter(device) {
  let failures = 0;
  return new TransformStream({
    async transform(frame, controller) {
      const data = new Uint8Array(frame.data);
      if (data.length < 13 + 16) return; // not one of ours: drop it
      const n = data[data.length - 1];
      if (n > 10 || data.length < n + 13 + 16) return;
      const iv = data.subarray(data.length - 13, data.length - 1);
      try {
        const key = await keyFor(device);
        const plain = new Uint8Array(await crypto.subtle.decrypt({ name: "AES-GCM", iv, additionalData: data.subarray(0, n) },
          key, data.subarray(n, data.length - 13)));
        const out = new Uint8Array(n + plain.length);
        out.set(data.subarray(0, n), 0);
        out.set(plain, n);
        frame.data = out.buffer;
        controller.enqueue(frame);
        failures = 0;
      } catch (e) {
        // A frame that doesn't decrypt is dropped, never played: it isn't from the person it claims to be.
        if (++failures === 50) self.postMessage({ op: "failing", device });
      }
    },
  });
}

function pipe(readable, writable, op, kind, device) {
  const t = op === "encrypt" ? encrypter(kind, device) : decrypter(device);
  readable.pipeThrough(t).pipeTo(writable).catch(() => {});
}

self.onmessage = async (e) => {
  const m = e.data;
  if (m.op === "key") {
    setBase(await crypto.subtle.importKey("raw", m.key, "HKDF", false, ["deriveKey"]));
  } else if (m.op === "stream") {
    pipe(m.readable, m.writable, m.mode, m.kind, m.device);
  }
};

// RTCRtpScriptTransform (Firefox, Safari, newer Chromium).
self.onrtctransform = (e) => {
  const t = e.transformer;
  pipe(t.readable, t.writable, t.options.mode, t.options.kind, t.options.device);
};
