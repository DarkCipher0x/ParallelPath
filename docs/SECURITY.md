# ParallelPath security overview

What ParallelPath protects, against whom, and how. The full design is in ARCHITECTURE.md, and the latest security
test report is PENTEST-0.0.1.md.

## What's protected

A family's record: messages, files, calendar, expenses, documents, the info bank, journals, names and calls. It must
stay **confidential** (only the family can read it), **intact** (nobody can change or delete it unseen) and
**attributable** (every action provably made by a given person's device).

## Who it's protected against

| Adversary | What they can't do |
|---|---|
| Someone who breaks into the server, or a dishonest operator | Read any family content, or change it unseen. They hold only ciphertext, public keys and metadata; every item and audit entry is signed by family devices, and each family's audit chain is checked by every member's app. |
| A network attacker | Read or alter traffic. TLS 1.3 with hybrid post-quantum key exchange only; certificates are pinned; the content is end-to-end encrypted as well. |
| A future quantum computer recording traffic now | Decrypt it later. Every key exchange and wrap is hybrid ML-KEM + classical, and signatures are hybrid ML-DSA + classical. |
| The other parent (or anyone in the family) | Rewrite history, deny what they sent, read your private journal or a private conversation they're not in, lock you out of the record, or take over the family's keys. |
| Someone who finds an invite code | Do anything beyond waiting for approval: they get no keys and no parent powers until a parent compares safety numbers and approves them. |
| An account holder (any member, a parent included) | Bring new families onto the server: a server invite (admin only) allows exactly one family; more need the operator's allowance, checked by the server. A journal is one per person and never takes members. |
| Someone with a stolen device | Read anything without the passphrase (Argon2id). A revoked device can't sign in, and the family's keys are rotated away from it. |

**Out of scope:** a device that is itself compromised while unlocked (malware on a parent's phone sees what that
parent sees), and metadata the server needs for delivery (who is in which family, sizes, times).

## Cryptography

| Purpose | Algorithm |
|---|---|
| Encrypting keys to a device | HPKE: ML-KEM-1024 + P-384, HKDF-SHA512, AES-256-GCM |
| Signatures (identities, devices, items, audit, reports) | ML-DSA-87 + Ed25519, both must verify |
| Content | AES-256-GCM, a fresh key per item and per file chunk |
| Hashing | SHA3-512 |
| Passphrases | Argon2id |
| Transport | TLS 1.3, X25519MLKEM768 / SecP384r1MLKEM1024 only |
| Calls | WebRTC (DTLS-SRTP), plus end-to-end frame encryption: AES-256-GCM with per-device keys derived from a per-call key that's shared under the family's post-quantum keys |

All of it comes from Go's standard library (the FIPS 140-3-validated code base): `crypto/mlkem`, `crypto/mldsa`,
`crypto/hpke`. The same Go engine runs on Windows, Android and (compiled to WebAssembly) in the browser, so there's
one implementation to get right.

## Key controls

- **Keys never leave devices unencrypted.** The only exception is the person's own recovery kit, which they create
  and which is sealed with Argon2id under a 24-byte random code.
- **Epoch keys** are rotated monthly, when someone is removed, when a device that held the key is revoked, and when
  an account is blocked or a professional's access ends. Only devices that already hold the key can rotate or
  share it, and a rotation never adds a new device.
- **Approval** of every new member or device happens after comparing a 60-digit safety number.
- **Private conversations** are spaces of their own inside a family, with their own epoch keys granted only to
  their participants' devices, and their own audit chain. Nothing about them is written to the family's audit log,
  so other members don't learn they exist. Who may be in one (and call whom) follows the family's permissions, which
  the server checks when a conversation is made, on every post and on every call signal. Leaving the family ends a
  person's conversations, and their keys are rotated.
- **Permissions** (set by the inviting parent, changeable only by them, audited) are enforced by the server: items
  in a section someone may not see are never sent to their devices, posts outside what they may add are refused,
  and calls outside who they may call are refused. Limits: the family's audit log (who did what and when, never
  content) stays visible to every member, because each device verifies the whole chain. Within a family, sections
  share the family's epoch keys, so visibility is enforced by the server, not by separate keys; private
  conversations do have separate keys. Whether a call is video or voice is end-to-end encrypted, so the apps, not
  the server, enforce "voice only".
- **On-device AI only.** The tone check (a small model compiled into the engine: TONE-MODEL.md) and the help
  assistant (search over the app's own guide) run on the device, offline. No text is sent to any service. Google's
  ML Kit (Gemini Nano on Android) was considered and rejected because its SDK sends identifiers and usage data to
  Google.
- **Immutability:** edits and withdrawals are new signed revisions with a reason; the server enforces revision
  order and authorship.
- **Audit chain** per family, signed by devices, chained with SHA3-512 and countersigned by the server; each app
  verifies it and keeps the last head it saw, so a rolled-back or forked chain is detected.
- **Professionals:** read-only, time-limited, and optionally limited to history from a date on. The server hides
  older items, and their devices never get older epoch keys.
- **Certificate changes** are accepted only when signed by the server's long-term key the apps pinned at setup.
- **Server at rest:** the data must be on an encrypted volume (BitLocker or LUKS), and the server's keys are sealed
  with the OS key store (DPAPI, or systemd-creds with the TPM).
- **Admin portal:** Argon2id passphrase + TOTP, lockout, a CSRF token, short sessions, an IP allow-list, and a signed
  audit of every admin action.

## Reporting a vulnerability

Please report security issues privately to DarkCipher Systems (DarkCipher0x) through the contact on the project's
website, not in a public issue. Include the version and the steps to reproduce.
