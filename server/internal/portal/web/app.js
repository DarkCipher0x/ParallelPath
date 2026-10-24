// ParallelPath Server admin portal. Plain JavaScript, no libraries. Everything shown is inserted as text (never as
// HTML), because some of it - device names - comes from people's devices.
'use strict';

const app = document.getElementById('app');
let me = null;      // { name, csrf, local, id }
let serverName = '';

// ---- DOM helper: h('div', {class: 'x', onclick: fn}, 'text', child, ...)
function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === null || v === undefined || v === false) continue;
    if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (k === 'class') el.className = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat()) {
    if (kid === null || kid === undefined || kid === false) continue;
    el.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return el;
}

// ---- API
async function api(method, path, body) {
  const opts = { method, headers: {}, credentials: 'same-origin' };
  if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
  if (method !== 'GET' && me) opts.headers['X-PP-CSRF'] = me.csrf;
  const res = await fetch('/admin/api/' + path, opts);
  let data = null;
  try { data = await res.json(); } catch (_) { /* empty */ }
  if (res.status === 401 && me) { me = null; start(); throw new Error('Signed out'); }
  if (!res.ok) throw new Error((data && data.error) || ('Error ' + res.status));
  return data;
}

// ---- small UI pieces
function toast(text) {
  const t = h('div', { class: 'toast' }, text);
  document.body.append(t);
  setTimeout(() => t.remove(), 3200);
}

function confirmBox(title, text, yes, danger) {
  return new Promise(resolve => {
    const close = v => { scrim.remove(); resolve(v); };
    const scrim = h('div', { class: 'scrim' },
      h('div', { class: 'card dialog' }, h('h3', {}, title), h('p', {}, text),
        h('div', { class: 'row', style: 'justify-content:flex-end' },
          h('button', { class: 'btn', onclick: () => close(false) }, 'Cancel'),
          h('button', { class: 'btn ' + (danger ? 'danger' : 'primary'), onclick: () => close(true) }, yes))));
    document.body.append(scrim);
  });
}

function showLink(title, text, link) {
  const scrim = h('div', { class: 'scrim' },
    h('div', { class: 'card dialog' }, h('h3', {}, title), h('p', {}, text), h('div', { class: 'key' }, link),
      h('div', { class: 'row', style: 'justify-content:flex-end;margin-top:14px' },
        h('button', { class: 'btn', onclick: () => copy(link) }, 'Copy'),
        h('button', { class: 'btn primary', onclick: () => scrim.remove() }, 'Done'))));
  document.body.append(scrim);
}

async function copy(text) {
  try { await navigator.clipboard.writeText(text); toast('Copied'); } catch (_) { toast('Select the text and press Ctrl+C'); }
}

const portOf = listen => 'port ' + String(listen).split(':').pop();
const when = t => t && !String(t).startsWith('0001') ? new Date(t).toLocaleString() : '-';
const fp = id => (id || '').slice(0, 16).toUpperCase().replace(/(.{4})(?!$)/g, '$1 ');
function bytes(n) {
  const u = ['bytes', 'KB', 'MB', 'GB', 'TB']; let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i ? n.toFixed(1) : n) + ' ' + u[i];
}
function since(seconds) {
  const d = Math.floor(seconds / 86400), hr = Math.floor(seconds % 86400 / 3600), m = Math.floor(seconds % 3600 / 60);
  return d ? `${d} d ${hr} h` : hr ? `${hr} h ${m} min` : `${m} min`;
}
const wordmark = (sub) => h('div', { class: 'wordmark' }, h('img', { src: '/admin/logo.svg', alt: '' }),
  h('div', {}, h('span', {}, 'Parallel'), h('span', { class: 'path' }, 'Path'), h('small', {}, sub)));

// ---- start: setup link, local link, sign in, or the portal
async function start() {
  const hash = new URLSearchParams(location.hash.slice(1));
  let state;
  try { state = await api('GET', 'state'); } catch (e) { app.replaceChildren(h('div', { class: 'center' }, e.message)); return; }
  serverName = state.name;
  if (hash.get('local')) {
    history.replaceState(null, '', '/admin/');
    try { await api('POST', 'local-login', { token: hash.get('local') }); return start(); }
    catch (e) { return signInView(state, e.message); }
  }
  if (hash.get('setup')) return setupView(hash.get('setup'));
  if (state.me) { me = state.me; return shell(); }
  signInView(state);
}

function signInView(state, message) {
  const name = h('input', { autocomplete: 'username', autofocus: true });
  const pass = h('input', { type: 'password', autocomplete: 'current-password' });
  const code = h('input', { class: 'code', inputmode: 'numeric', autocomplete: 'one-time-code', maxlength: 7, placeholder: '000000' });
  const err = h('div', { class: 'error' }, message || '');
  const go = async (e) => {
    e.preventDefault(); err.textContent = '';
    try { await api('POST', 'login', { name: name.value, password: pass.value, code: code.value }); start(); }
    catch (x) { err.textContent = x.message; code.value = ''; }
  };
  const noAdmins = !state.has_admins ? h('div', { class: 'banner warm' },
    'There are no admin accounts yet. Create the first one on the server: on Linux run ',
    h('span', { class: 'mono' }, 'sudo ppserver admin setup-link'),
    ', on Windows open ParallelPath Server Manager and choose Admin.') : null;
  app.replaceChildren(h('div', { class: 'center' }, h('div', { class: 'auth' },
    wordmark('Server admin'),
    h('h1', {}, 'Sign in'), h('p', {}, serverName),
    noAdmins,
    h('form', { class: 'card', onsubmit: go },
      h('label', {}, 'Name'), name, h('label', {}, 'Passphrase'), pass,
      h('label', {}, 'Code from your authenticator app'), code,
      h('div', { class: 'row', style: 'margin-top:18px' }, h('button', { class: 'btn primary', type: 'submit' }, 'Sign in')), err),
    h('p', { class: 'hint', style: 'margin-top:14px' }, h('span', { class: 'badge' }, 'Post-quantum protected'),
      ' The connection to this server uses post-quantum TLS.'))));
}

async function setupView(token) {
  let info;
  try { info = await api('POST', 'setup/begin', { token }); }
  catch (e) { history.replaceState(null, '', '/admin/'); return signInView({ has_admins: true }, e.message); }
  const name = h('input', { autocomplete: 'username', value: '' });
  const p1 = h('input', { type: 'password', autocomplete: 'new-password' });
  const p2 = h('input', { type: 'password', autocomplete: 'new-password' });
  const code = h('input', { class: 'code', inputmode: 'numeric', autocomplete: 'one-time-code', maxlength: 7, placeholder: '000000' });
  const err = h('div', { class: 'error' });
  const go = async (e) => {
    e.preventDefault(); err.textContent = '';
    if (p1.value !== p2.value) { err.textContent = 'The passphrases are different.'; return; }
    try {
      await api('POST', 'setup/finish', { token, name: info.reset ? '' : name.value, password: p1.value, code: code.value });
      history.replaceState(null, '', '/admin/'); start();
    } catch (x) { err.textContent = x.message; }
  };
  app.replaceChildren(h('div', { class: 'center' }, h('div', { class: 'auth', style: 'max-width:540px' },
    wordmark('Server admin'),
    h('h1', {}, info.reset ? 'Reset ' + info.reset : 'Create your admin account'),
    h('p', {}, 'This link works once. Nobody else will know your passphrase or your authenticator.'),
    h('form', { class: 'card', onsubmit: go }, h('ol', { class: 'steps' },
      h('li', {}, h('b', {}, 'Add ParallelPath Server to your authenticator app'),
        h('p', { class: 'hint' }, 'In Microsoft Authenticator, Google Authenticator, 1Password or similar, add an account and choose to enter a setup key:'),
        h('div', { class: 'key' }, info.setup_key),
        h('div', { class: 'row', style: 'margin-top:8px' }, h('button', { class: 'btn small', type: 'button', onclick: () => copy(info.setup_key.replace(/ /g, '')) }, 'Copy key'),
          h('a', { href: info.uri, class: 'btn small' }, 'Open in authenticator (on a phone)'))),
      h('li', {}, h('b', {}, 'Choose a name and passphrase'),
        info.reset ? null : [h('label', {}, 'Name'), name],
        h('label', {}, 'Passphrase'), p1, h('div', { class: 'hint' }, 'At least 12 characters - a few random words is best.'),
        h('label', {}, 'Passphrase again'), p2),
      h('li', {}, h('b', {}, 'Enter the code your app shows'), h('div', { style: 'margin-top:8px' }, code))),
    h('button', { class: 'btn primary', type: 'submit' }, info.reset ? 'Reset account' : 'Create account'), err))));
}

// ---- the portal
const pages = [
  ['overview', 'Overview', overviewPage], ['invites', 'Invites', invitesPage], ['accounts', 'Accounts', accountsPage],
  ['families', 'Families', familiesPage], ['backups', 'Backups', backupsPage], ['settings', 'Settings', settingsPage],
  ['logs', 'Logs', logsPage], ['audit', 'Audit log', auditPage], ['admins', 'Admins', adminsPage],
];

function shell() {
  const page = (location.hash.slice(1) || 'overview');
  const current = pages.find(p => p[0] === page) || pages[0];
  const main = h('main', { class: 'main' }, h('div', { class: 'spinner' }));
  const nav = h('nav', { class: 'nav' }, pages.map(([id, label]) =>
    h('a', { href: '#' + id, class: id === current[0] ? 'on' : null }, label)));
  const out = async () => { await api('POST', 'logout').catch(() => {}); me = null; start(); };
  app.replaceChildren(h('div', { class: 'shell' },
    h('aside', { class: 'side' }, wordmark(serverName), nav,
      h('div', { class: 'who' }, h('b', {}, me.name), 'Signed in', h('div', { style: 'margin-top:8px' }, h('button', { class: 'btn small', onclick: out }, 'Sign out')))),
    main));
  current[2](main).catch(e => main.replaceChildren(h('div', { class: 'banner bad' }, e.message)));
}
window.addEventListener('hashchange', () => { if (me) shell(); });

const head = (title, sub, ...actions) => h('div', { class: 'spread' },
  h('div', {}, h('h1', {}, title), h('p', { class: 'sub' }, sub)), h('div', { class: 'row' }, actions));

async function overviewPage(main) {
  const o = await api('GET', 'overview');
  const c = o.counts, s = o.status;
  const restart = async () => {
    if (!await confirmBox('Restart the server?', 'Families are disconnected for a few seconds; their apps reconnect by themselves.', 'Restart')) return;
    await api('POST', 'restart'); toast('Restarting...'); setTimeout(() => location.reload(), 6000);
  };
  const stat = (n, l) => h('div', { class: 'card stat' }, h('div', { class: 'n' }, n), h('div', { class: 'l' }, l));
  main.replaceChildren(
    head('Overview', o.name, h('button', { class: 'btn', onclick: restart }, 'Restart server')),
    o.unencrypted_allowed ? h('div', { class: 'banner warm' }, 'Testing mode: the data folder is allowed on a drive without encryption. Don\'t use this server for real families.') : null,
    o.portal_limited ? null : h('div', { class: 'banner' }, 'Tip: in Settings you can limit this portal to your own networks.'),
    h('div', { class: 'grid' },
      stat(c.people - c.people_blocked, 'accounts' + (c.people_blocked ? ` (+${c.people_blocked} blocked)` : '')),
      stat(c.devices - c.devices_revoked, 'devices'), stat(c.signed_in, 'signed in now'), stat(c.spaces, 'families'),
      stat(bytes(c.file_bytes), `${c.files} files`), stat(c.invites, 'unused invites')),
    h('div', { class: 'card' }, h('dl', { class: 'kv' },
      h('dt', {}, 'Status'), h('dd', {}, h('span', { class: 'pill ok' }, 'Running'), ' for ' + since(o.uptime_seconds)),
      h('dt', {}, 'Version'), h('dd', {}, s.version + ' (' + s.os + ')'),
      h('dt', {}, 'Encryption at rest'), h('dd', {}, s.at_rest),
      h('dt', {}, 'Reached at'), h('dd', {}, o.hosts.join(', ') + '  ·  ' + portOf(o.listen)),
      h('dt', {}, 'Certificate'), h('dd', {}, ({ 'local-ca': 'This server\'s own certificate authority - devices trust it once (see /trust)',
        acme: 'Let\'s Encrypt (renewed automatically)', files: 'Your own certificate files', self: 'Self-signed (browsers warn; the apps pin it)' })[s.tls_mode] || s.tls_mode),
      h('dt', {}, 'Certificate fingerprint'), h('dd', { class: 'mono' }, s.tls_cert_sha256 ? s.tls_cert_sha256.toUpperCase().replace(/(.{4})/g, '$1 ') : '(publicly trusted - nothing to pin)'),
      (s.uncovered || []).length ? h('dt', {}, 'Not covered') : null,
      (s.uncovered || []).length ? h('dd', {}, s.uncovered.join(', ') + ' - the certificate authority was made before these names were added, so browsers won\'t trust them. Making a new authority means every device trusts it again.') : null,
      h('dt', {}, 'Calls'), h('dd', {}, o.calls && o.calls.relay_port ? `Relay running on port ${o.calls.relay_port} (UDP/TCP) · media ports ${o.calls.ports || '49160-49200'} · everything it carries is end-to-end encrypted` : 'Relay off - calls connect directly only (set turn_listen in config.json)'),
      h('dt', {}, 'Server key'), h('dd', { class: 'mono' }, s.server_key.toUpperCase().replace(/(.{4})/g, '$1 ')),
      h('dt', {}, 'Database'), h('dd', {}, bytes(o.database_bytes) + ' · ' + c.items + ' encrypted items · ' + c.audit_entries + ' server audit entries'),
      h('dt', {}, 'Backups'), h('dd', {}, o.backups.on ? (o.backups.last ? 'Last: ' + when(o.backups.last.time) : 'None yet') + ` · keeping ${o.backups.keep}` : 'Off'))));
}

async function invitesPage(main) {
  const list = (await api('GET', 'invites')) || [];
  const create = async () => {
    const r = await api('POST', 'invites');
    showLink('New invite code', 'Give this to the person who will set up their family in the ParallelPath app. It works once, until ' + when(r.expires) + '.', r.code);
    invitesPage(main);
  };
  const revoke = async (i) => {
    if (!await confirmBox('Cancel this invite?', 'Nobody will be able to use it.', 'Cancel invite', true)) return;
    await api('POST', `invites/${i.id}/revoke`); invitesPage(main);
  };
  main.replaceChildren(head('Invites', 'One-time codes that let someone create an account. Family members invite each other from the app.',
    h('button', { class: 'btn primary', onclick: create }, 'Create invite code')),
  h('div', { class: 'card' }, list.length === 0 ? h('div', { class: 'empty' }, 'No unused invites.') :
    h('table', {}, h('tr', {}, ['Kind', 'Made by', 'Made', 'Expires', ''].map(t => h('th', {}, t))),
      list.map(i => h('tr', {}, h('td', {}, i.kind === 'server' ? 'New account' : 'Join a family (' + i.role + ')'),
        h('td', {}, i.created_by === 'admin' ? 'Admin' : 'Account ' + fp(i.created_by)), h('td', {}, when(i.created)), h('td', {}, when(i.expires)),
        h('td', {}, h('button', { class: 'btn small danger', onclick: () => revoke(i) }, 'Cancel')))))));
}

async function accountsPage(main) {
  const people = (await api('GET', 'people')) || [];
  const act = async (path, title, text, yes, danger) => {
    if (!await confirmBox(title, text, yes, danger)) return;
    await api('POST', path); toast('Done'); accountsPage(main);
  };
  // Only the operator decides who may bring a new family onto the server.
  const families = async (p, left) => {
    if (left < 0 || left > 20) return;
    await api('POST', `people/${p.id}/families`, { left }); toast(left === 1 ? 'May start 1 more family' : `May start ${left} more families`); accountsPage(main);
  };
  const rows = [];
  for (const p of people) {
    rows.push(h('tr', {}, h('td', { class: 'mono' }, fp(p.id)), h('td', {}, when(p.created)), h('td', {}, p.spaces),
      h('td', {}, h('div', { class: 'row', title: 'How many more families this account may start' },
        h('button', { class: 'btn small', 'aria-label': 'One fewer', disabled: p.families_left < 1, onclick: () => families(p, p.families_left - 1) }, '−'),
        h('span', { class: 'mono' }, String(p.families_left)),
        h('button', { class: 'btn small', 'aria-label': 'One more', onclick: () => families(p, p.families_left + 1) }, '+'))),
      h('td', {}, h('span', { class: 'pill ' + (p.blocked ? 'bad' : 'ok') }, p.blocked ? 'Blocked' : 'Active')),
      h('td', {}, p.blocked
        ? h('button', { class: 'btn small', onclick: () => act(`people/${p.id}/unblock`, 'Unblock this account?', 'They can sign in again.', 'Unblock') }, 'Unblock')
        : h('button', { class: 'btn small danger', onclick: () => act(`people/${p.id}/block`, 'Block this account?', 'All their devices are signed out and they can\'t sign in. Nothing in their families is changed or deleted.', 'Block', true) }, 'Block'))));
    for (const d of p.devices || []) {
      rows.push(h('tr', { class: 'sub' }, h('td', {}, '↳ ' + (d.name || 'Device') + ' (' + d.kind + ')'), h('td', {}, 'Added ' + when(d.added)),
        h('td', {}, 'Last sign-in ' + when(d.last_sign_in)), h('td', {}),
        h('td', {}, d.revoked ? h('span', { class: 'pill mute' }, 'Revoked') : h('span', { class: 'pill ok' }, 'OK')),
        h('td', {}, d.revoked ? null : h('div', { class: 'row' },
          h('button', { class: 'btn small', onclick: () => act(`devices/${d.id}/signout`, 'Sign out this device?', 'It signs in again with its own keys next time it\'s used.', 'Sign out') }, 'Sign out'),
          h('button', { class: 'btn small danger', onclick: () => act(`devices/${d.id}/revoke`, 'Revoke this device?', 'For a lost or stolen device: it can never sign in again. Its family should then rotate their keys (the app asks the parents).', 'Revoke', true) }, 'Revoke')))));
    }
  }
  main.replaceChildren(head('Accounts', 'People with an account on this server, and their devices. Only you can bring new families onto the server: a "New account" invite lets its person start one family, and "New families" here lets someone start more. Parents can only invite people into their own family.'),
    h('div', { class: 'banner' }, 'Names aren\'t shown: they\'re end-to-end encrypted inside each family, so the server never sees them. Accounts are shown by fingerprint - people can see theirs in the app\'s Settings.'),
    h('div', { class: 'card' }, people.length === 0 ? h('div', { class: 'empty' }, 'No accounts yet.') :
      h('table', {}, h('tr', {}, ['Account', 'Created', 'Spaces', 'New families', 'Status', ''].map(t => h('th', {}, t))), rows)));
}

async function familiesPage(main) {
  const list = (await api('GET', 'spaces')) || [];
  main.replaceChildren(head('Families', 'Family spaces on this server. Everything inside them is end-to-end encrypted.'),
    h('div', { class: 'card' }, list.length === 0 ? h('div', { class: 'empty' }, 'No families yet.') :
      h('table', {}, h('tr', {}, ['Family', 'Created', 'Members', 'Key version', 'Items', 'Files'].map(t => h('th', {}, t))),
        list.map(s => h('tr', {}, h('td', { class: 'mono' }, fp(s.id)), h('td', {}, when(s.created)), h('td', {}, s.members),
          h('td', {}, s.epoch), h('td', {}, s.items), h('td', {}, s.files + ' · ' + bytes(s.file_bytes)))))));
}

async function backupsPage(main) {
  const b = await api('GET', 'backups');
  const now = async () => { const r = await api('POST', 'backups'); toast('Backup made: ' + r.file); backupsPage(main); };
  main.replaceChildren(head('Backups', b.on ? `Daily, into ${b.dir}, keeping the newest ${b.keep}.` : 'Backups are off.',
    b.on ? h('button', { class: 'btn primary', onclick: now }, 'Back up now') : null),
  h('div', { class: 'banner' }, 'Backups are copies of the database, still sealed with this server\'s keys and with family content end-to-end encrypted - unreadable on their own.'),
  b.on ? null : h('div', { class: 'banner warm' }, 'Turn backups on with setup: ParallelPath Server Setup on Windows, or sudo ppserver install on Linux.'),
  h('div', { class: 'card' }, (b.files || []).length === 0 ? h('div', { class: 'empty' }, 'No backups yet.') :
    h('table', {}, h('tr', {}, ['File', 'Made', 'Size'].map(t => h('th', {}, t))),
      b.files.map(f => h('tr', {}, h('td', { class: 'mono' }, f.name), h('td', {}, when(f.time)), h('td', {}, bytes(f.size)))))));
}

async function settingsPage(main) {
  const s = await api('GET', 'settings');
  const e = s.editable, f = s.fixed;
  const name = h('input', { value: e.name, maxlength: 80 });
  const blob = h('input', { type: 'number', min: 1, max: 16384, value: e.max_blob_mb });
  const quota = h('input', { type: 'number', min: 1, max: 100000, value: e.space_quota_gb });
  const keep = h('input', { type: 'number', min: 1, max: 365, value: e.backup_keep || 14, disabled: !f.backup_dir });
  const allow = h('input', { value: (e.admin_allow || []).join(', '), placeholder: 'Anywhere' });
  const err = h('div', { class: 'error' });
  const save = async (ev) => {
    ev.preventDefault(); err.textContent = '';
    try {
      await api('PUT', 'settings', { name: name.value, max_blob_mb: +blob.value, space_quota_gb: +quota.value, backup_keep: +keep.value,
        admin_allow: allow.value.split(/[ ,]+/).filter(Boolean) });
      if (await confirmBox('Saved', 'The changes take effect when the server restarts. Restart now?', 'Restart'))
        { await api('POST', 'restart'); toast('Restarting...'); setTimeout(() => location.reload(), 6000); }
    } catch (x) { err.textContent = x.message; }
  };
  main.replaceChildren(head('Settings', 'What can be changed from here.'),
    h('form', { class: 'card', onsubmit: save },
      h('label', {}, 'Server name'), name, h('div', { class: 'hint' }, 'Shown in the apps.'),
      h('label', {}, 'Largest file (MB)'), blob,
      h('label', {}, 'Storage per family (GB)'), quota,
      h('label', {}, 'Backups to keep'), keep,
      h('label', {}, 'Admin portal reachable from'), allow,
      h('div', { class: 'hint' }, 'Networks or addresses, e.g. 192.168.1.0/24, 203.0.113.7 - empty means anywhere. This computer can always reach it.'),
      h('div', { class: 'row', style: 'margin-top:18px' }, h('button', { class: 'btn primary', type: 'submit' }, 'Save')), err),
    h('div', { class: 'section' }, h('h2', {}, 'Set by setup'),
      h('p', { class: 'sub' }, 'These need administrator rights on the machine: change them with ParallelPath Server Setup (Windows) or sudo ppserver install (Linux).'),
      h('div', { class: 'card' }, h('dl', { class: 'kv' },
        h('dt', {}, 'Address and port'), h('dd', {}, f.hosts.join(', ') + '  ·  ' + portOf(f.listen)),
        h('dt', {}, 'Data folder'), h('dd', { class: 'mono' }, f.data_dir),
        h('dt', {}, 'Backup folder'), h('dd', { class: 'mono' }, f.backup_dir || 'Backups off'),
        h('dt', {}, 'Encryption at rest'), h('dd', {}, f.unencrypted_allowed ? 'Not required (testing only)' : 'Required'),
        h('dt', {}, 'Certificate'), h('dd', {}, f.cert_file ? 'Your own certificate files' : 'The server\'s own (pinned by the apps)')))));
}

async function logsPage(main) {
  const lines = (await api('GET', 'logs')) || [];
  const box = h('div', { class: 'logs' }, lines.length ? lines.join('\n') : 'Nothing logged since the server started.');
  main.replaceChildren(head('Logs', 'Recent lines from the server (newest at the bottom).', h('button', { class: 'btn', onclick: () => logsPage(main) }, 'Refresh')), box);
  box.scrollTop = box.scrollHeight;
}

async function auditPage(main) {
  const a = await api('GET', 'audit');
  main.replaceChildren(head('Audit log', 'Every administrator action and server event, signed by the server and hash-chained.'),
    h('div', { class: 'banner ' + (a.verified ? 'ok' : 'bad') }, a.verified ? 'The whole chain verifies: nothing has been changed or removed.' : 'The chain does NOT verify - entries have been changed or removed.'),
    h('div', { class: 'card' }, h('table', {}, h('tr', {}, ['#', 'Time', 'What'].map(t => h('th', {}, t))),
      (a.entries || []).map(x => h('tr', {}, h('td', {}, x.seq), h('td', {}, when(x.time)), h('td', {}, x.detail))))));
}

async function adminsPage(main) {
  const [admins, sess] = await Promise.all([api('GET', 'admins'), api('GET', 'sessions')]);
  const list = admins || [], sessions = (sess && sess.sessions) || [];
  const add = async () => {
    const r = await api('POST', 'admins');
    showLink('Link for a new admin', 'Send this to the person (over a private channel). It works once, within 24 hours; they choose their own passphrase and set up their authenticator.', r.link);
  };
  const reset = async (a) => {
    if (!await confirmBox('Reset ' + a.name + '?', 'You get a link that lets them choose a new passphrase and authenticator. Their current ones keep working until then.', 'Make link')) return;
    const r = await api('POST', `admins/${a.id}/reset`); showLink('Reset link for ' + a.name, 'It works once, within 24 hours.', r.link);
  };
  const toggle = async (a) => { await api('POST', `admins/${a.id}/${a.disabled ? 'enable' : 'disable'}`); adminsPage(main); };
  const end = async (s) => { await api('POST', `sessions/${s.id}/end`); adminsPage(main); };
  main.replaceChildren(head('Admins', 'People who can sign in to this portal.', h('button', { class: 'btn primary', onclick: add }, 'Add an admin')),
    h('div', { class: 'card' }, list.length === 0 ? h('div', { class: 'empty' }, 'No admin accounts yet - only this computer\'s administrator can sign in.') :
      h('table', {}, h('tr', {}, ['Name', 'Created', 'Last sign-in', 'Status', ''].map(t => h('th', {}, t))),
        list.map(a => h('tr', {}, h('td', {}, a.name), h('td', {}, when(a.created)), h('td', {}, when(a.last_login)),
          h('td', {}, h('span', { class: 'pill ' + (a.disabled ? 'mute' : 'ok') }, a.disabled ? 'Disabled' : 'Active')),
          h('td', {}, h('div', { class: 'row' }, h('button', { class: 'btn small', onclick: () => reset(a) }, 'Reset'),
            h('button', { class: 'btn small ' + (a.disabled ? '' : 'danger'), onclick: () => toggle(a) }, a.disabled ? 'Enable' : 'Disable'))))))),
    h('div', { class: 'section' }, h('h2', {}, 'Signed in now'), h('div', { class: 'card' },
      h('table', {}, h('tr', {}, ['Who', 'From', 'Since', 'Last active', ''].map(t => h('th', {}, t))),
        sessions.map(s => h('tr', {}, h('td', {}, s.name), h('td', {}, s.ip), h('td', {}, when(s.created)), h('td', {}, when(s.seen)),
          h('td', {}, s.id === sess.me ? h('span', { class: 'pill ok' }, 'You') : h('button', { class: 'btn small danger', onclick: () => end(s) }, 'Sign out'))))))));
}

start();
