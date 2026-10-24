# ParallelPath

Calm, organised co-parenting with a record you can trust. ParallelPath gives separated parents one place to
message, plan parenting time, share documents, split costs and call each other - end-to-end encrypted with
post-quantum cryptography, on a server they (or someone they trust) run.

- Apps for **Windows**, **Android** and the **web**.
- A self-hosted **server** for Windows or Ubuntu, with a setup wizard or a terminal installer and a web admin portal.
- Messages that can't be changed unseen, a shared calendar with parenting schedules, expenses, a file explorer with
  versions and an access log, an info bank, a private journal, HD calls, signed court-ready reports.
- An on-device tone check, tone bar and help: no cloud AI, no telemetry.

Free and open source under the GNU AGPL-3.0. Beta software, provided as is - see docs/DISCLAIMER.md.

## Documentation

| | |
|---|---|
| docs/USER-GUIDE.md | For parents, family members and professionals |
| docs/ADMIN-GUIDE.md | Installing and running a server |
| docs/FAQ.md | Common questions |
| docs/FEATURES.md | Everything it does |
| docs/SECURITY.md | The design: keys, cryptography and what each party can and can't do |
| docs/TONE-MODEL.md | The on-device tone model and how it was trained |
| docs/PENTEST-*.md | The latest security test report |
| docs/PRIVACY.md, docs/DISCLAIMER.md | Privacy and the disclaimer |
| CHANGELOG.md | What changed in each build |

## Layout

| Folder | What's there |
|---|---|
| core | Shared Go code: keys, post-quantum cryptography, envelopes, the audit chain |
| server | The server (ppserver): API, rules, storage, TURN relay, admin portal, installers |
| engine | The client engine (Go), used by every app: encryption, sync, calls, reports, the tone model, help |
| clients | The apps (Kotlin Multiplatform, Compose): Windows, Android and the web |
| installer | Windows setup wizards (WPF) and the Ubuntu installer |
| tools | Build scripts |

## Building

Needs Go 1.27, JDK 21, the Android SDK and NDK with gomobile (for Android), the .NET 8 SDK (Windows setups) and
Microsoft Edge (to print the guides as PDF). `tools\env.ps1` puts a portable toolchain on PATH.

```
powershell -File tools\build-release.ps1
```

builds and tests everything into `dist\<version>\`. The web app is built first (`tools\build-web.ps1`) into
`server\internal\webapp\dist`, which the server embeds; run it before building the server on its own. Android
release builds are signed with your own key (`tools\new-release-key.ps1`).

Built by DarkCipher0x at DarkCipher Systems - https://darkciphersystems.com
