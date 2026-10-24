# ParallelPath: architecture

ParallelPath is a self-hosted co-parenting platform. Every piece of family content is
end-to-end encrypted with post-quantum cryptography. Everything that happens is recorded in a tamper-evident,
signed audit log, and nothing can be changed without a reason.

ParallelPath 1 (Python, `Projects\ParallelPath`) is the reference for features and behaviour. It isn't changed by
this project.

Built by DarkCipher0x at DarkCipher Systems. AGPL-3.0.

## Platforms

| Part | Platforms | Technology |
|---|---|---|
| Server | Windows and Ubuntu Linux (24.04+) | Go: one self-contained binary, no runtime to install |
| Web client | Any modern browser, served by the server | Light HTML/JS UI over the **same Go core compiled to WebAssembly** |
| Client apps | Windows and Android | Kotlin + Compose Multiplatform UI over the **same Go core** |
| Later (on hold) | macOS, iPhone | The same Compose UI and the same Go core (gomobile builds for iOS) |

**One crypto and protocol core**, written once in Go, runs everywhere:
- the server;
- Android (as a gomobile library);
- Windows (as a separate local engine process, so keys never live in the UI process);
- the browser (WebAssembly).

So every client follows exactly the same security rules: there's no second implementation to drift.

## Post-quantum cryptography

Everything uses Go's standard library, the same code base validated under FIPS 140-3. ML-KEM and ML-DSA come
from `crypto/mlkem`, `crypto/mldsa` and `crypto/hpke`. Every public-key operation is **hybrid** (post-quantum and
classical together), so it stays secure if either half holds.

| Purpose | Algorithm | Level |
|---|---|---|
| Encrypting keys to a person or device | HPKE **ML-KEM-1024 + P-384** (`MLKEM1024-P384`), HKDF-SHA512, AES-256-GCM | NIST category 5 |
| Signing (identities, messages, audit) | **ML-DSA-87 + Ed25519** composite: both signatures must verify | NIST category 5 |
| Content encryption | AES-256-GCM with a fresh random key per item and per file chunk, and context-bound AAD | 256-bit (128-bit against quantum search) |
| Hashing (audit chain, content IDs) | SHA3-512 | 256-bit collision resistance against quantum attack |
| Key derivation | HKDF-SHA512 with domain-separated labels | |
| Passwords and unlock | Argon2id (64 MiB, t=3, p=4 minimum), giving a key that wraps the device keys | |
| Transport | TLS 1.3 only, key exchange **X25519MLKEM768 only** (classical-only clients refused), HSTS | |
| Server secrets at rest | Sealed with the OS key store (DPAPI/TPM on Windows; systemd-creds/TPM on Linux), then AES-256-GCM | |

Calls: browser WebRTC media itself uses classical DTLS-SRTP. ParallelPath adds an end-to-end media layer whose
keys come from the hybrid ML-KEM exchange, so recorded call traffic can't be opened even if DTLS is broken
later (per-device frame keys derived from a per-call key shared under the family's keys).

## End-to-end encryption model

- **Identity:**
  - Each person has a hybrid identity signing key.
  - Each device has its own hybrid signing and KEM keys, certified by the person's identity key.
  - Keys are generated on the device and never leave it unencrypted. A recovery kit (an encrypted backup of the
    identity key under a recovery phrase) is optional and created only by the user.
- **Family space:**
  - A family space has a space key, rotated whenever someone is removed and at least monthly.
  - Each epoch key is HPKE-wrapped to every member device.
  - Professionals (mediator, solicitor, guardian) get time-limited, read-only grants: the epoch keys for the
    period they're allowed to see.
- **Items** (messages, events, expenses, files, journal entries and so on):
  - Each item is encrypted with its own random content key, wrapped by the epoch key.
  - The author signs the ciphertext together with its header: space, kind, item ID, previous revision and time.
- **The server stores ciphertext, signatures and the minimum metadata** needed for delivery and audit: who, when,
  which item, item size and kind. It can't read content. Administrators can't either.
- **Reports** (court-ready PDF/CSV/JSON) are generated on the client from decrypted data. They're signed by the
  person's identity key and stamped with audit-chain heads, so a court or professional can verify them.

## Private conversations, permissions and on-device AI

- **Private conversations** are spaces whose `parent` is a family. They have their own epoch keys (granted only to
  their participants' devices) and their own audit chain. The server allows them, and every post and call signal in
  them, only while the family's permissions allow each participant to reach the others.
- **Permissions** (`Perms`: sections seen and added to, who to message, who to call, video, whole-family calls) live
  on each family membership, are set by the inviting parent and changed only by them (audited), and are enforced by
  the server (items filtered by kind, posts and signals checked).
- **Tone model** (`engine/tone`, docs/TONE-MODEL.md) and **help** (`engine/help`: BM25 search over the guide and
  FAQ) run inside the engine on every platform; nothing leaves the device.

## Immutability and audit

- **Nothing is ever deleted or overwritten.**
  - An edit is a new signed revision that points at the previous one and **must carry a reason**.
  - A retraction is the same, with an empty body and a reason.
  - Every revision stays readable to the people allowed to see the item.
- **Every action** (send, read receipt, reaction, edit, file view or download, call, invite, role change, sign-in)
  becomes an audit entry. Each entry is:
  - signed by the actor's device;
  - chained per space as `SHA3-512(prev || entry)`;
  - countersigned by the server.
- Any client can verify the whole chain. A server that hides or reorders entries is caught, because clients keep
  the last chain head they saw and compare it.
- Admin actions on the server (setup, backups, user management) go to a separate server audit chain, with the same
  signing.

## Repository layout (small projects, one-way dependencies)

```
ParallelPath2/
  go.work
  core/                   module pp/core    - shared by server, engine and wasm; no I/O, no globals
    pqc/                  hybrid KEM, hybrid signatures, AEAD, KDF, password hashing
    wire/                 the canonical, deterministic encoding every signature covers
    envelope/             encrypted + signed items and revisions
    audit/                audit entries, chains, verification
    keys/                 identity, device and space key structures, certificates, rotation
  server/                 module pp/server  - the ppserver binary (Windows + Linux)
    cmd/ppserver/
    internal/{config,store,auth,api,push,blobs,admin,seal,tlsx}
  engine/                 module pp/engine  - the client engine: vault, sync, E2EE operations
    cmd/ppengine/         local engine process for the Windows app (talks to the app over a private pipe)
    mobile/               gomobile facade for Android (and iOS later)
    wasm/                 WebAssembly build for the web client
  web/                    the light web client served by ppserver
  clients/                Gradle, Kotlin + Compose Multiplatform
    build-logic/
    core/{designsystem,model,engine-bridge}
    feature/{onboarding,chat,calendar,files,expenses,records,audit,settings}
    apps/{android,desktop}
  docs/                   ARCHITECTURE, SECURITY, FEATURES (parity), PLAN, RESUME
  tools/                  build, test, release scripts
```

Dependencies only point down: `apps -> feature -> core`, `server -> core`, `engine -> core`. Each Go package has
one job and its own tests.

## Theme

The ParallelPath theme (not the DarkCipher one) is used on every surface. It's the same tokens as ParallelPath 1:
dark `#0b1517` / panel `#132427` / accent `#3dd6d0`, warm `#f2a65a`, violet `#a970ff`, a light theme, and
high-contrast variants. The type is Inter and JetBrains Mono, with 16/10 px radii. The tokens live in
`clients/core/designsystem` and `web/` from a single source file.

## Milestones

1. **Foundations and secure chat** (first build to test):
   - the core crypto;
   - ppserver for Windows and Linux;
   - accounts, devices and family spaces, with one-time admin setup and invites;
   - E2EE chat: emoji, reactions, photos and videos, read receipts, audited edits/retractions with reasons;
   - the audit log viewer;
   - the Windows and Android apps.
2. Calendar and parenting-time schedules, swap requests, check-ins.
3. Documents (Explorer-style, versioned, access-logged), expenses, info bank and journal.
4. Calls (with the PQ media layer), court-ready reports, professionals' grants.
5. The web client at full parity, the installer and service packaging for both OSes, encrypted backups,
   recovery kits, and the release pipeline.
