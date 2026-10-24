# ParallelPath server: administrator guide

This guide is for whoever runs a ParallelPath server: a parent hosting it for their own family, an IT-minded
relative, or an organisation hosting it for many families. Parents and family members have their own guide
(USER-GUIDE.md).

Version 0.0.1.

---

## Contents

1. [What the server does, and what it can't see](#1-what-the-server-does-and-what-it-cant-see)
2. [Requirements](#2-requirements)
3. [Installing on Windows](#3-installing-on-windows)
4. [Installing on Ubuntu](#4-installing-on-ubuntu)
5. [Certificates](#5-certificates)
6. [Network, ports and calls](#6-network-ports-and-calls)
7. [Invites and getting families started](#7-invites-and-getting-families-started)
8. [The admin portal](#8-the-admin-portal)
9. [Backups and restoring](#9-backups-and-restoring)
10. [Upgrading](#10-upgrading)
11. [Command reference](#11-command-reference)
12. [Configuration reference](#12-configuration-reference)
13. [Security hardening checklist](#13-security-hardening-checklist)
14. [Troubleshooting](#14-troubleshooting)
15. [Uninstalling](#15-uninstalling)

---

## 1. What the server does, and what it can't see

The server stores and relays. It **can't read** any family's messages, files, events, expenses, names, journal
entries or calls. They arrive end-to-end encrypted (ML-KEM-1024 + P-384, ML-DSA-87 + Ed25519, AES-256-GCM) by the
families' own devices.

What it does:

- registers people and their devices (public keys only) and signs devices in;
- checks that every item and audit entry is signed by a registered device of a member allowed to make it;
- enforces roles: who may invite, write, share keys or rotate them, and the professionals' access windows;
- keeps each family's audit chain and countersigns every entry, so tampering is detectable;
- stores the ciphertext and the encrypted files;
- relays call set-up messages and, through its TURN relay, encrypted call media;
- serves the web app and the admin portal.

Families can also hold **private conversations** between some of their members. Each is a space of its own with
keys only its participants hold. The admin portal lists them with the families, by ID only. The families'
**permissions** (who sees which sections, who may message and call whom) are set by the parents and enforced by the
server; there's nothing for you to configure. The **tone check** and **Help & FAQ** run inside the apps, on the
families' devices: no AI service, nothing for the server to host.

What the server does hold is metadata: account and device IDs, which family spaces exist and who is in them, the
kind and size of each item, and when it was sent. As the operator you can see that metadata in the admin portal,
but never content. Everything you do in the portal is recorded in the server's own signed audit log.

---

## 2. Requirements

| | Windows | Ubuntu |
|---|---|---|
| OS | Windows 10/11 or Windows Server 2019+ (64-bit) | Ubuntu 24.04 or newer (64-bit, systemd) |
| Disk encryption | The data folder must be on a **BitLocker** drive (a testing-only override exists) | A **LUKS** disk, or the installer's encrypted container (recommended on a VPS) |
| Memory / CPU | 1 GB free RAM, any modern CPU | 1 GB RAM, 1 vCPU |
| Disk | Depends on the families' photos and videos (default quota 50 GB per family) | Same |
| Network | A fixed address or DNS name the apps can reach; the ports in section 6 | Same |

Nothing else needs installing: the server is one self-contained program.

---

## 3. Installing on Windows

Run `ParallelPath-Server-Setup-<version>.exe` (it asks for administrator rights once).

1. **Checks:** Windows version, BitLocker on the data drive, ports free.
2. **Features:** tick what to set up:
   - start with Windows as a locked-down service (`NT SERVICE\ParallelPathServer`, with almost no privileges);
   - a firewall rule, for home and office networks only (private/domain) or for all networks;
   - daily backups (choose the folder);
   - Start-menu shortcuts and the **Server Manager** icon by the clock;
   - **calls** (opens the TURN relay ports);
   - the **certificate**: the server's own certificate authority (default), Let's Encrypt, or your own files (section
     5), and whether to trust the server's CA on this PC;
   - the first invite code.
3. **Finish:** the server's address, certificate fingerprint and first invite code, each with a **Copy** button.
   Give the fingerprint to the families: their apps show it on first connection.

Program files go in `C:\Program Files\DarkCipher Systems\ParallelPath Server`, and data in
`C:\ProgramData\DarkCipher Systems\ParallelPath Server` (only SYSTEM, Administrators and the service can open it).

### Server Manager

The icon by the clock shows the server's state: green running, amber starting or stopping, red stopped. Click it to
open the server's page (pinned to the server's own certificate). Right-click it to open the page in your browser,
copy the address, create an invite, open the admin portal, see the status, or start, stop and restart the service.

---

## 4. Installing on Ubuntu

Copy the `.tar.gz` to the server, then run:

```bash
tar xzf ParallelPath-Server-<version>-linux-x64.tar.gz
```

```bash
cd ParallelPath-Server-<version>-linux-x64
```

```bash
sudo sh install.sh
```

`install.sh` checks the download's checksum, then starts the interactive installer. Use ↑/↓ and space to tick
features:

- a **systemd service**, sandboxed under its own account (`systemd-analyze security` scores it about 1.1);
- an **encrypted data container** (LUKS2) if the disk isn't encrypted. Its key is a systemd credential, sealed to
  the machine (and the TPM if there is one), and it opens at boot;
- a **ufw** rule and **daily backups** (`/var/backups/parallelpath`, keeping 14);
- permission to install missing packages (`cryptsetup-bin`);
- **calls** (TURN ports), the **certificate** choice, and the first invite code.

Data lives in `/var/lib/parallelpath`. The installer ends with a one-time **admin portal setup link** (valid 24
hours).

---

## 5. Certificates

The apps and browsers connect with **TLS 1.3 and post-quantum key exchange only** (X25519MLKEM768 /
SecP384r1MLKEM1024). Classical-only clients are refused.

| Mode (`tls_mode`) | When to use it | Browsers |
|---|---|---|
| `local-ca` (default) | Home and private networks. The server makes its own certificate authority, restricted (X.509 name constraints) to the server's own names, and issues itself 200-day certificates, renewed automatically. | Each device trusts the CA once, through `https://<server>/trust` (steps for Windows, Android, iPhone, Mac and Linux). On Windows, setup can do it for the server PC. |
| `acme` | A server reachable from the internet under a domain name. Certificates come from Let's Encrypt and renew automatically. Port 80 must reach the server. | Trusted everywhere, nothing to do. |
| `files` | You already have certificates (for example from your organisation's CA). Set `cert_file` and `key_file`. | Depends on your CA. |
| `self` | A plain self-signed certificate (kept by servers from early test builds). | Warns. Switch to `local-ca` when convenient (below). |

The **Windows and Android apps pin** the server's CA (or its certificate) the first time. If you **change** the
certificate (switching mode, new files, a new CA), the apps follow automatically: the server signs a statement of
its new certificate with the long-term server key the apps already trust. Nothing else would be accepted, so a
network attacker can't use this.

**Names the certificate covers:** the `hosts` list (and `acme_domains`). If you add a name after the CA was made, the
admin portal warns that it isn't covered by the CA's name constraints.

---

## 6. Network, ports and calls

| Port | Protocol | What | Open to |
|---|---|---|---|
| 8443 (`listen`) | TCP | Apps, web app, admin portal (`/admin/`), `/trust` | The families' devices |
| 8444 (`admin_listen`) | TCP | Local admin links | **This machine only** (loopback) |
| 80 (`http_listen`) | TCP | Let's Encrypt challenges and a redirect to HTTPS | Internet (`acme` mode only) |
| 3478 (`turn_listen`) | UDP and TCP | Call relay (TURN) | The families' devices |
| 49160-49200 (`turn_ports`) | UDP | Relayed call media | The families' devices |

Setup creates the firewall rules on the server. On a home router or a cloud firewall, forward or allow the same
ports. On the same network, calls connect directly and don't need the relay.

The relay only accepts short-lived credentials the server gives signed-in devices, and they stop working for a
blocked account. It refuses to forward to the server's loopback, link-local or multicast addresses, or to any of its
other services. It never sees call content.

`turn_relay_ip` is the address the families' devices reach the relay on: the server's public IP behind NAT. If it
isn't set, the first IP in `hosts` is used, otherwise the server's outbound address.

---

## 7. Invites and getting families started

1. Make a **server invite** (`S.…`): in the admin portal (**Invites**), from **Start › ParallelPath Server › Create
   an invite code** on Windows, or with `sudo ppserver invite`. It works once, for 7 days.
2. Give the first parent the **server address**, the **invite** and the **certificate fingerprint**, separately if
   you can.
3. They set up the app and create their family space. From there, the **parents invite everyone else themselves**
   (`J.…` codes) and approve them after comparing safety numbers. You don't need to do anything per family.

**Only you bring new families onto the server.** A server invite lets its person start exactly **one** family.
Parents can invite people into their own family, but they can't start other families or make server invites - the
server refuses it. If someone genuinely needs another family space (children with a different co-parent, say), open
**Accounts** and raise their **New families** number with **+**; it goes down by one each time they start a family,
and **−** takes an allowance back. Every change is signed into the server's audit log.

---

## 8. The admin portal

`https://<server>:8443/admin/`

### First sign-in

- **Windows:** **Admin** in the Server Manager window, or **Admin portal…** on the icon's menu, opens a one-time
  local link.
- **Ubuntu:** the installer's final link, or `sudo ppserver admin setup-link` for a new one (24 hours).

Create your admin account: a **passphrase** (stored with Argon2id) and an **authenticator app** (TOTP). Each code
works once.

### Pages

| Page | What you can do |
|---|---|
| Overview | Version, disk encryption, server key and certificate fingerprints, certificate mode and uncovered names, counts (accounts, devices, devices signed in now, families, files and their size, unused invites), the last backup. **Restart server** (the service manager starts it again). |
| Invites | Create server invites; see and revoke unused ones. |
| Accounts | Every account, its families and status, and its devices. **New families**: how many more family spaces the account may start (**+**/**−**; only the admin can change it). **Block** an account (signs it out everywhere and stops it signing in; its families' keys are then rotated away from it) or **unblock** it. Per device: **Sign out**, or **Revoke** (lost or stolen: it can never sign in again, and its families rotate their keys automatically). |
| Families | Each family space by its ID: created, members, key version, items and files. No names or content (those are encrypted). |
| Backups | The backups so far, and **Back up now**. |
| Settings | Server name, largest file (MB), storage per family (GB), backups to keep, and the networks allowed to reach the portal. |
| Logs | The server's recent log lines. |
| Audit log | The server's own audit chain (every admin action, signed), and whether it verifies. |
| Admins | Add admin accounts, disable or enable them (not your own), and see admin sessions and end any of them. |

### Portal security

- Sessions end after 30 minutes idle and 12 hours at most. The cookie is host-only and `SameSite=Strict`, and
  every change needs a CSRF token.
- Five wrong sign-ins lock the account.
- **Allowed networks** (Settings) limit the portal to, for example, your LAN. Other addresses don't even see it.
- Every action is signed into the server's audit chain.

---

## 9. Backups and restoring

Daily backups (first one 10 minutes after the server starts) write:

- `parallelpath-YYYYMMDD-HHMMSS.db`: a consistent copy of the database, keeping the newest `backup_keep` (14 by
  default);
- `blobs/`: a copy of every encrypted file (photos, receipts, documents). Files never change, so only new ones are
  copied.

Both stay sealed. The database copy needs this server's master key (sealed with the OS key store, DPAPI or
systemd-creds), and the files are end-to-end encrypted by the families' devices. So the backup folder can be on
another, even unencrypted, drive or a NAS. **Back up now** in the portal makes one immediately.

### Restoring on the same server

1. Stop the service: Server Manager › **Stop**, or `sudo ppserver service stop`.
2. In the data folder, rename `parallelpath.db` (keep it until you're sure), and copy the backup you want there as
   `parallelpath.db`.
3. Copy the backup's `blobs` folder over the data folder's `blobs` folder. This only adds files; it never replaces
   them.
4. Start the service and check the admin portal's **Overview** and **Audit log**.

The families' apps keep the newest state they've seen. Anything written after the backup is still on their devices'
copy of the record, but the server won't have it until it's sent again. A restore is for when the server's own copy
is damaged.

### Moving to new hardware

The master key is sealed to the machine (and its TPM). To move, install the server on the new machine and set up
families again, or contact the project about migration. Don't copy `master.sealed` between machines: it won't
unseal there.

---

## 10. Upgrading

**Windows:** run the new `ParallelPath-Server-Setup-<version>.exe`. It keeps your choices (they're stored in the
registry) and data, re-verifies BitLocker, and restarts the service.

**Ubuntu:** run the new bundle's `sudo sh install.sh`. It detects the installed server and upgrades it in place.

Upgrade the **server first**, then the apps. Newer features (calendar, calls, reports, linking, history windows,
private conversations, permissions and so on) need a server that knows them.

---

## 11. Command reference

| Command | What it does |
|---|---|
| `ppserver install` | Interactive setup (service, firewall, backups, encryption, certificate). |
| `ppserver install --plan FILE` | Unattended setup from a JSON plan. |
| `ppserver uninstall [--purge]` | Removes the server. `--purge` also deletes its data and backups. |
| `ppserver status` | State, fingerprints and addresses. |
| `ppserver service start\|stop\|restart` | Controls the installed service. |
| `ppserver invite` | A one-time server invite. |
| `ppserver admin setup-link` | A one-time link to create an admin account (24 hours). |
| `ppserver admin login-link` | A one-time local sign-in link (2 minutes, this machine only). |
| `ppserver init --data DIR --name NAME --host HOST[,HOST]` | Manual setup without a service. |
| `ppserver run --data DIR` | Runs in the foreground. |
| `ppserver verify-report FILE.json` | Checks a court-ready report's signature and contents. |
| `ppserver version` | The version. |

On Linux, run the commands that change things with `sudo`. On Windows, the Start-menu shortcuts already run as
administrator. `journalctl -u parallelpath` shows the Linux log.

---

## 12. Configuration reference

`config.json` in the data folder. Change it with setup or the portal where you can; edit it by hand only with the
service stopped.

| Key | Default | Meaning |
|---|---|---|
| `name` | from setup | The server name shown in the apps. |
| `listen` | `:8443` | HTTPS address. |
| `hosts` | from setup | Names and IPs the certificate covers. |
| `tls_mode` | `local-ca` | `local-ca`, `acme`, `files` or `self` (section 5). |
| `acme_domains`, `acme_email` | | For `acme`. |
| `cert_file`, `key_file` | | For `files`. |
| `http_listen` | set by setup for acme (`:80`) | Let's Encrypt challenges, and a redirect to HTTPS. |
| `turn_listen`, `turn_relay_ip`, `turn_ports` | `:3478`, auto, `49160-49200` | The call relay (section 6). |
| `admin_listen` | `127.0.0.1:8444` | Local admin links. Keep it on loopback. |
| `admin_allow` | none (any) | Networks allowed to reach the portal, e.g. `192.168.1.0/24`. |
| `max_blob_mb` | `2048` | Largest single file. |
| `space_quota_gb` | `50` | Storage per family space. |
| `backup_dir`, `backup_keep` | from setup, `14` | Backups. |
| `allow_unencrypted_volume` | `false` | Testing only: run without disk encryption. |

---

## 13. Security hardening checklist

- [ ] The data folder is on an encrypted volume (BitLocker or LUKS); the Overview says so.
- [ ] `admin_listen` stays on 127.0.0.1, and `admin_allow` limits the portal to your own networks.
- [ ] Each admin has their own account with an authenticator app. Remove admins who don't need access.
- [ ] Give families the certificate fingerprint through a different channel from the invite.
- [ ] Backups go to a second drive or a NAS. Try a restore now and then.
- [ ] The OS installs security updates automatically. Upgrade ParallelPath when a new version comes out.
- [ ] Only the ports in section 6 are open. On a VPS, use the provider's firewall as well.
- [ ] Watch the portal's **Audit log** for admin actions you don't recognise, and the **Overview** for "uncovered
      names".
- [ ] Revoke lost or stolen devices straight away (Accounts › the device › Revoke).

---

## 14. Troubleshooting

| Symptom | Check |
|---|---|
| Apps say the server can't be reached | Is the service running (`ppserver status`)? Is port 8443 open on the server's firewall and the router? |
| "Classical-only connection refused" in the log | An old browser or proxy without post-quantum TLS. Use a current browser. Proxies that terminate TLS aren't supported. |
| Browser certificate warning | `local-ca`: the device hasn't trusted the CA yet (`/trust`). `acme`: is port 80 open, and does the name point at this server? |
| Calls connect at home but not between networks | TCP/UDP 3478 and UDP 49160-49200 must reach the server, and `turn_relay_ip` must be the server's public address. The log says "call relay (TURN) running" at start; if it says it couldn't start, the reason follows (often the port is in use). |
| The service won't start on Windows | BitLocker must be on for the data drive; setup (Repair) re-verifies it. See `C:\ProgramData\DarkCipher Systems\ParallelPath Server\logs\server.log`. |
| The service won't start on Ubuntu | `journalctl -u parallelpath -e`. If the encrypted container didn't open, check `systemctl status` for the volume unit. |
| "Backup failed" in the log | Is the backup folder still there and writable by the service? Is the disk full? |
| A family says their audit log fails verification | Don't touch the database. Make a backup now, collect the server log and the portal's Audit log page, and report it. |

---

## 15. Uninstalling

- **Windows:** Settings › Apps › ParallelPath Server, or Start › ParallelPath Server › Uninstall. Choose whether to
  keep the data.
- **Ubuntu:** `sudo ppserver uninstall`, or `sudo ppserver uninstall --purge` to delete the data, the encrypted
  container and the backups as well.

Purging can't be undone, and the families' record on the server goes with it (their devices keep their own copies).
