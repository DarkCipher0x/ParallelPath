package api

// homePage is shown at the server's root: what this is, and the way into the web app. Styled with the ParallelPath
// theme tokens; no scripts, no external resources. The styles are a separate file (homeCSS at /style.css) because the
// server's Content-Security-Policy allows no inline styles.
const homePage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>ParallelPath</title>
<link rel="stylesheet" href="/style.css">
</head><body><main>
<span class="badge">Post-quantum protected</span>
<h1>Parallel<span>Path</span></h1>
<p>This is a private ParallelPath server. Families use it from the ParallelPath app for Windows or Android, or
right here in a web browser on any device, with the invite they were given.</p>
<p><a class="button" href="/app/">Open ParallelPath in this browser</a></p>
<p class="small">Seeing a certificate warning? <a href="/trust">Trust this server on your device</a>.</p>
<p>Every message, photo and record is end-to-end encrypted with ML-KEM-1024 and signed with ML-DSA-87.
The connection to this server uses post-quantum TLS. Nothing can be changed or deleted without a reason,
and everything is in a tamper-evident audit log.</p>
<small>Built by DarkCipher0x at <a href="https://darkciphersystems.com">DarkCipher Systems</a> - free and open source (AGPL-3.0).</small>
</main></body></html>`

const homeCSS = `:root{--bg:#0b1517;--panel:#132427;--line:#22393d;--fg:#e8f3f2;--dim:#a8c1bf;--accent:#3dd6d0;--warm:#f2a65a}
@media (prefers-color-scheme:light){:root{--bg:#f4f8f8;--panel:#fff;--line:#d6e3e3;--fg:#11292b;--dim:#3f5d5f;--accent:#0f9e98}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.6 Inter,"Segoe UI",system-ui,sans-serif;display:grid;place-items:center;min-height:100vh}
main{max-width:560px;margin:24px;padding:32px;background:var(--panel);border:1px solid var(--line);border-radius:16px}
h1{margin:0 0 8px;font-size:1.6rem}h1 span{color:var(--accent)}p{color:var(--dim)}small{color:var(--dim)}a{color:var(--accent)}
.button{display:inline-block;background:var(--accent);color:#052826;text-decoration:none;font-weight:600;padding:10px 20px;border-radius:10px}
code{font-family:"JetBrains Mono",Consolas,monospace;font-size:.85rem;overflow-wrap:anywhere}h2{font-size:1.05rem;margin:20px 0 4px}ol{color:var(--dim);padding-left:20px}.small,.fp{font-size:.9rem}main{max-width:640px}
.badge{display:inline-block;border:1px solid var(--accent);color:var(--accent);border-radius:999px;padding:2px 12px;font-size:.8rem}
`
