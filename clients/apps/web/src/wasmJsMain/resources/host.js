// ParallelPath web app host. Starts the Go engine (WebAssembly) and then the Compose UI (Kotlin/Wasm), and gives
// them what a browser can: an in-memory file system, IndexedDB for the vault (sealed values only), file picking,
// opening files, the clipboard and location. Nothing here sees a key or plaintext beyond what the person opens.
"use strict";

(() => {
  // ---------------------------------------------------------------- in-memory file system (Node fs API for Go)
  // Go's os package on js/wasm calls globalThis.fs. Picked files, the decrypted-file cache and temp files live here,
  // in memory only; the engine wipes the cache when the app locks, and a page reload clears everything.
  const S_IFDIR = 0o040000, S_IFREG = 0o100000;
  const constants = { O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_EXCL: 128, O_TRUNC: 512, O_APPEND: 1024, O_DIRECTORY: 65536 };
  const nodes = new Map([["/", { dir: true, mtime: Date.now(), ino: 1 }]]);
  const fds = new Map();
  let nextFd = 10, nextIno = 2;
  const fail = (code) => { const e = new Error(code); e.code = code; return e; };
  const norm = (p) => {
    const out = [];
    for (const part of String(p).split("/")) {
      if (!part || part === ".") continue;
      if (part === "..") out.pop(); else out.push(part);
    }
    return "/" + out.join("/");
  };
  const parentOf = (p) => p === "/" ? "/" : norm(p + "/..");
  const statOf = (n) => {
    const size = n.dir ? 0 : n.size;
    return { dev: 0, ino: n.ino, mode: n.dir ? (S_IFDIR | 0o700) : (S_IFREG | 0o600), nlink: 1, uid: 0, gid: 0, rdev: 0, size,
      blksize: 4096, blocks: Math.ceil(size / 512), atimeMs: n.mtime, mtimeMs: n.mtime, ctimeMs: n.mtime };
  };
  const ensure = (n, need) => {
    if (need <= n.data.length) return;
    const grown = new Uint8Array(Math.max(need, n.data.length * 2, 4096));
    grown.set(n.data.subarray(0, n.size));
    n.data = grown;
  };
  const dec = new TextDecoder("utf-8");
  let consoleLine = "";
  const memfs = {
    constants,
    writeSync(fd, buf) {
      if (fd === 1 || fd === 2) {
        consoleLine += dec.decode(buf);
        const nl = consoleLine.lastIndexOf("\n");
        if (nl !== -1) { (fd === 2 ? console.warn : console.log)(consoleLine.substring(0, nl)); consoleLine = consoleLine.substring(nl + 1); }
        return buf.length;
      }
      throw fail("EBADF");
    },
    write(fd, buf, off, len, pos, cb) {
      if (fd === 1 || fd === 2) { cb(null, this.writeSync(fd, buf.subarray(off, off + len))); return; }
      const f = fds.get(fd);
      if (!f) { cb(fail("EBADF")); return; }
      const n = f.node;
      if (n.dir) { cb(fail("EISDIR")); return; }
      const at = f.append ? n.size : (pos == null ? f.pos : pos);
      ensure(n, at + len);
      n.data.set(buf.subarray(off, off + len), at);
      n.size = Math.max(n.size, at + len);
      n.mtime = Date.now();
      if (pos == null) f.pos = at + len;
      cb(null, len);
    },
    read(fd, buf, off, len, pos, cb) {
      const f = fds.get(fd);
      if (!f) { cb(fail("EBADF")); return; }
      const n = f.node;
      if (n.dir) { cb(fail("EISDIR")); return; }
      const at = pos == null ? f.pos : pos;
      const cnt = Math.max(0, Math.min(len, n.size - at));
      buf.set(n.data.subarray(at, at + cnt), off);
      if (pos == null) f.pos = at + cnt;
      cb(null, cnt);
    },
    open(path, flags, mode, cb) {
      const p = norm(path);
      let n = nodes.get(p);
      if (!n) {
        if (!(flags & constants.O_CREAT)) { cb(fail("ENOENT")); return; }
        const parent = nodes.get(parentOf(p));
        if (!parent || !parent.dir) { cb(fail("ENOENT")); return; }
        n = { data: new Uint8Array(0), size: 0, mtime: Date.now(), ino: nextIno++ };
        nodes.set(p, n);
      } else if ((flags & constants.O_CREAT) && (flags & constants.O_EXCL)) { cb(fail("EEXIST")); return; }
      if (n.dir && (flags & 3) !== 0) { cb(fail("EISDIR")); return; }
      if (!n.dir && (flags & constants.O_TRUNC)) n.size = 0;
      const fd = nextFd++;
      fds.set(fd, { node: n, path: p, pos: 0, append: !!(flags & constants.O_APPEND) });
      cb(null, fd);
    },
    close(fd, cb) { fds.delete(fd); cb(null); },
    fstat(fd, cb) { const f = fds.get(fd); if (!f) cb(fail("EBADF")); else cb(null, statOf(f.node)); },
    stat(path, cb) { const n = nodes.get(norm(path)); if (!n) cb(fail("ENOENT")); else cb(null, statOf(n)); },
    lstat(path, cb) { this.stat(path, cb); },
    readdir(path, cb) {
      const p = norm(path), n = nodes.get(p);
      if (!n) { cb(fail("ENOENT")); return; }
      if (!n.dir) { cb(fail("ENOTDIR")); return; }
      const prefix = p === "/" ? "/" : p + "/";
      const names = [];
      for (const k of nodes.keys()) if (k !== p && k.startsWith(prefix) && !k.substring(prefix.length).includes("/")) names.push(k.substring(prefix.length));
      cb(null, names);
    },
    mkdir(path, perm, cb) {
      const p = norm(path);
      if (nodes.has(p)) { cb(fail("EEXIST")); return; }
      const parent = nodes.get(parentOf(p));
      if (!parent || !parent.dir) { cb(fail("ENOENT")); return; }
      nodes.set(p, { dir: true, mtime: Date.now(), ino: nextIno++ });
      cb(null);
    },
    unlink(path, cb) {
      const p = norm(path), n = nodes.get(p);
      if (!n) { cb(fail("ENOENT")); return; }
      if (n.dir) { cb(fail("EISDIR")); return; }
      nodes.delete(p);
      cb(null);
    },
    rmdir(path, cb) {
      const p = norm(path), n = nodes.get(p);
      if (!n) { cb(fail("ENOENT")); return; }
      if (!n.dir) { cb(fail("ENOTDIR")); return; }
      for (const k of nodes.keys()) if (k.startsWith(p + "/")) { cb(fail("ENOTEMPTY")); return; }
      nodes.delete(p);
      cb(null);
    },
    rename(from, to, cb) {
      const a = norm(from), b = norm(to);
      if (!nodes.has(a)) { cb(fail("ENOENT")); return; }
      for (const [k, v] of [...nodes]) {
        if (k === a || k.startsWith(a + "/")) { nodes.delete(k); nodes.set(b + k.substring(a.length), v); }
      }
      cb(null);
    },
    ftruncate(fd, len, cb) {
      const f = fds.get(fd);
      if (!f) { cb(fail("EBADF")); return; }
      ensure(f.node, len);
      if (len > f.node.size) f.node.data.fill(0, f.node.size, len);
      f.node.size = len;
      cb(null);
    },
    truncate(path, len, cb) { this.open(path, constants.O_WRONLY, 0, (e, fd) => e ? cb(e) : this.ftruncate(fd, len, (e2) => { fds.delete(fd); cb(e2); })); },
    fsync(fd, cb) { cb(null); },
    chmod(p, m, cb) { cb(null); }, fchmod(fd, m, cb) { cb(null); },
    chown(p, u, g, cb) { cb(null); }, fchown(fd, u, g, cb) { cb(null); }, lchown(p, u, g, cb) { cb(null); },
    utimes(p, a, m, cb) { cb(null); }, futimes(fd, a, m, cb) { cb(null); },
    readlink(p, cb) { cb(fail("EINVAL")); }, link(a, b, cb) { cb(fail("ENOSYS")); }, symlink(a, b, cb) { cb(fail("ENOSYS")); },
  };
  globalThis.fs = memfs;
  // Go's wasm_exec.js adds a minimal `process`; Kotlin/Wasm checks process.release.name to detect Node.js.
  if (globalThis.process && !globalThis.process.release) globalThis.process.release = { name: "browser" };
  const fileBytes = (path) => { const n = nodes.get(norm(path)); return n && !n.dir ? n.data.slice(0, n.size) : null; };
  const putFile = (path, bytes) => {
    const p = norm(path);
    let parent = "";
    for (const part of p.split("/").slice(1, -1)) { parent += "/" + part; if (!nodes.has(parent)) nodes.set(parent, { dir: true, mtime: Date.now(), ino: nextIno++ }); }
    nodes.set(p, { data: bytes, size: bytes.length, mtime: Date.now(), ino: nextIno++ });
  };
  const rand = () => Array.from(crypto.getRandomValues(new Uint8Array(8)), (b) => b.toString(16).padStart(2, "0")).join("");

  // ---------------------------------------------------------------- vault mirror in IndexedDB (sealed values only)
  let db = null;
  const openDB = () => new Promise((resolve, reject) => {
    const req = indexedDB.open("parallelpath", 1);
    req.onupgradeneeded = () => req.result.createObjectStore("vault");
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  globalThis.ppPersist = (dir, bucket, key, val) => {
    if (!db) return;
    const tx = db.transaction("vault", "readwrite");
    const k = dir + "|" + bucket + "|" + key;
    if (val == null) tx.objectStore("vault").delete(k); else tx.objectStore("vault").put(val, k);
  };
  const preloadAll = () => new Promise((resolve, reject) => {
    const req = db.transaction("vault", "readonly").objectStore("vault").openCursor();
    req.onsuccess = () => {
      const c = req.result;
      if (!c) { resolve(); return; }
      const k = String(c.key), i = k.indexOf("|"), j = k.indexOf("|", i + 1);
      globalThis.ppPreload(k.substring(0, i), k.substring(i + 1, j), k.substring(j + 1), c.value);
      c.continue();
    };
    req.onerror = () => reject(req.error);
  });

  // ---------------------------------------------------------------- events and helpers for the UI (Host.kt)
  let sink = null;
  const queued = [];
  const callSubs = new Set();
  globalThis.ppOnEvent = (name, json) => {
    if (name === "call") for (const cb of callSubs) { try { cb(json); } catch (e) { console.warn(e); } }
    if (sink) sink(name, json); else queued.push([name, json]);
  };
  // The call page (an iframe beside the app) listens to call-setup messages and is opened/closed by the app.
  globalThis.ppSubscribeCall = (cb) => { callSubs.add(cb); return () => callSubs.delete(cb); };
  globalThis.ppOpenCall = (url) => {
    globalThis.ppCloseCall();
    const f = document.createElement("iframe");
    f.id = "pp-call";
    f.src = url;
    f.allow = "camera; microphone; display-capture; autoplay; fullscreen";
    f.title = "ParallelPath call";
    document.body.appendChild(f);
  };
  globalThis.ppCloseCall = () => { const f = document.getElementById("pp-call"); if (f) f.remove(); };
  globalThis.ppSetEventSink = (fn) => { sink = fn; for (const [n, j] of queued.splice(0)) fn(n, j); };
  globalThis.ppCall = (method, args) => globalThis.ppGoCall(method, args).then((r) => "1" + r, (e) => "0" + String(e && e.message ? e.message : e));

  globalThis.ppPickFiles = () => new Promise((resolve) => {
    const input = document.createElement("input");
    input.type = "file";
    input.multiple = true;
    input.style.display = "none";
    let done = false;
    const finish = (list) => { if (!done) { done = true; input.remove(); resolve(JSON.stringify(list)); } };
    input.addEventListener("cancel", () => finish([]));
    input.addEventListener("change", async () => {
      const out = [];
      for (const f of input.files) {
        const path = "/picked/" + rand() + "/" + f.name.replace(/[\/\\]/g, "_");
        putFile(path, new Uint8Array(await f.arrayBuffer()));
        out.push({ path, name: f.name, mime: f.type || "" });
      }
      finish(out);
    });
    document.body.appendChild(input);
    input.click();
  });

  const viewable = (mime) => /^(image|video|audio)\//.test(mime) || mime === "application/pdf" || mime === "text/plain";
  globalThis.ppOpenFile = (path, mime) => {
    const bytes = fileBytes(path);
    if (!bytes) return;
    const url = URL.createObjectURL(new Blob([bytes], { type: mime || "application/octet-stream" }));
    const a = document.createElement("a");
    a.href = url;
    a.rel = "noopener";
    if (viewable(mime)) a.target = "_blank"; else a.download = path.substring(path.lastIndexOf("/") + 1);
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 120000);
  };
  globalThis.ppSaveFile = (path, name, mime) => {
    const bytes = fileBytes(path);
    if (!bytes) return false;
    const url = URL.createObjectURL(new Blob([bytes], { type: mime || "application/octet-stream" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = name.replace(/[\/\\]/g, "_");
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 120000);
    return true;
  };
  globalThis.ppReadFile = (path) => {
    const bytes = fileBytes(path);
    if (!bytes) return null;
    let s = "";
    for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
    return btoa(s);
  };
  globalThis.ppCopy = (text) => { navigator.clipboard && navigator.clipboard.writeText(text).catch(() => {}); };
  globalThis.ppCanLocate = () => !!navigator.geolocation;
  globalThis.ppLocate = () => new Promise((resolve) => {
    if (!navigator.geolocation) { resolve(""); return; }
    navigator.geolocation.getCurrentPosition(
      (p) => resolve(JSON.stringify({ lat: p.coords.latitude, lon: p.coords.longitude, accuracy: p.coords.accuracy })),
      () => resolve(""), { enableHighAccuracy: true, timeout: 20000, maximumAge: 0 });
  });
  globalThis.ppGetPref = (k) => { try { return localStorage.getItem("pp." + k); } catch (e) { return null; } };
  globalThis.ppSetPref = (k, v) => { try { localStorage.setItem("pp." + k, v); } catch (e) { /* private mode */ } };
  globalThis.ppOrigin = () => location.origin;
  globalThis.ppTimeZone = () => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"; } catch (e) { return "UTC"; } };
  // Notifications: the browser asks the person once; nothing is shown while the tab is in front.
  globalThis.ppPrepareNotify = () => {
    if ("Notification" in globalThis && Notification.permission === "default") {
      const ask = () => { Notification.requestPermission().catch(() => {}); document.removeEventListener("pointerdown", ask); };
      document.addEventListener("pointerdown", ask); // browsers only allow the prompt after a click or tap
    }
  };
  globalThis.ppNotify = (title, text, tag) => {
    if (!("Notification" in globalThis) || Notification.permission !== "granted" || document.visibilityState === "visible") return;
    try {
      const n = new Notification(title, { body: text, tag: "pp-" + tag, icon: "icon.svg" });
      n.onclick = () => { window.focus(); n.close(); };
    } catch (e) { /* some mobile browsers only notify from a service worker */ }
  };
  globalThis.ppDeviceName = () => {
    const ua = navigator.userAgent;
    const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
    const os = /Windows/.test(ua) ? "Windows" : /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" : /Mac OS X/.test(ua) ? "macOS" : /Linux/.test(ua) ? "Linux" : "";
    return os ? browser + " on " + os : browser;
  };

  // ---------------------------------------------------------------- start-up
  const status = (text, failed) => {
    const el = document.getElementById("pp-status");
    if (el) { el.textContent = text; el.className = failed ? "failed" : ""; }
  };
  const start = async () => {
    if (typeof WebAssembly !== "object" || !window.isSecureContext || !window.indexedDB) {
      status("This browser can't run ParallelPath securely. Use a current version of Chrome, Edge, Firefox or Safari.", true);
      return;
    }
    try {
      db = await openDB();
      const ready = new Promise((resolve) => { globalThis.ppReady = resolve; });
      const go = new Go();
      const { instance } = await WebAssembly.instantiateStreaming(fetch("ppweb.wasm"), go.importObject);
      go.run(instance);
      await ready;
      await preloadAll();
      globalThis.ppStart("/pp");
      status("Opening ParallelPath…");
      const s = document.createElement("script");
      s.src = "parallelpath.js";
      s.onload = () => { const el = document.getElementById("pp-splash"); if (el) setTimeout(() => el.remove(), 400); };
      s.onerror = () => status("ParallelPath couldn't load. Check your connection and reload.", true);
      document.body.appendChild(s);
    } catch (e) {
      console.error(e);
      status("ParallelPath couldn't start: " + (e && e.message ? e.message : e) + ". Your browser may need updating.", true);
    }
  };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start); else start();
})();
