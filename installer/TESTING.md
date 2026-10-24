# ParallelPath 0.0.1 - test build

The first release of ParallelPath: messages with a **tone check**, a live **tone bar** and **Advice**; **private
conversations**; the shared calendar with **custom schedules** and change requests; expenses; **Documents as a full
file explorer** with **right-click menus** everywhere; the info bank and a private journal; HD voice and video calls
where you **choose who to call**; court-ready signed reports; **permissions per person**; professionals limited to a
stretch of history; children's chat (if parents allow it); automatic key rotation; **Help & FAQ** answered on the
device; the web app; linking devices and recovery codes; notifications and reminders; installers for every platform
and the admin portal. Only the server's owner can bring new families onto a server.

The **user and administrator guides**, the FAQ, privacy, the disclaimer and the security test report are in
`guides\` (Markdown and PDF).

| Download | For |
|---|---|
| `ParallelPath-Server-Setup-0.0.1.exe` | The server on Windows: a setup wizard (asks for administrator rights once) |
| `ParallelPath-Server-0.0.1-linux-x64.tar.gz` | The server on Ubuntu 24.04 or newer: an interactive installer in the terminal |
| `ParallelPath-Setup-0.0.1.exe` | The app on Windows: a setup wizard, installs just for you (no administrator rights) |
| `ParallelPath-0.0.1-android.apk` | The app on Android phones (signed release build) |
| `ParallelPath-0.0.1-android-emulator.apk` | The same, for x86-64 Android emulators |
| `SHA256SUMS.txt` | Checksums of all of the above |
| `android-signing-certificate.txt` | The Android signing certificate's fingerprint |
| `guides\` | The user guide, the administrator guide, the FAQ, the security overview, the tone model, the feature list, privacy, the disclaimer and the security test report (Markdown and PDF) |

## 1. Install the server

**Windows:** run `ParallelPath-Server-Setup-0.0.1.exe`. The wizard checks the PC and lets you tick what to set up:

- start with Windows, as a locked-down service;
- a firewall rule, either for home and office networks only or for all networks;
- daily backups;
- Start-menu shortcuts;
- the first invite code.

It needs nothing else installed. It ends with the server's address, certificate fingerprint and your first invite code,
each with a Copy button.

**ParallelPath Server Manager** (the "Icon by the clock" option) starts when you sign in:

- The dot on its icon shows the server's state: green running, amber starting or stopping, red stopped.
- Clicking it opens the server's page in a ParallelPath window. The window uses the Edge engine built into Windows and
  trusts only this server's own certificate, so there's no certificate warning.
- Right-click it to open the page in your browser, copy the address, create an invite code, see the status, or start,
  stop or restart the server.

Program files go in `C:\Program Files\DarkCipher Systems\ParallelPath Server`, and data in
`C:\ProgramData\DarkCipher Systems\ParallelPath Server`. Running setup over an older install moves the program there;
the data stays where it was.

- The data folder must be on a BitLocker drive. On a PC without BitLocker, the Features page offers a testing-only
  option.
- More invite codes: Start › ParallelPath Server › Create an invite code.
- Status and uninstall are in the same Start-menu folder, and the server also appears in Settings › Apps.

**Ubuntu:** copy the `.tar.gz` to the server, then run:

```bash
tar xzf ParallelPath-Server-0.0.1-linux-x64.tar.gz
cd ParallelPath-Server-0.0.1-linux-x64
sudo sh install.sh
```

The installer checks the download and the machine, then asks a few questions. Use ↑/↓ and space to tick features:

- a systemd service, sandboxed under its own account;
- an encrypted data container (LUKS2) when the disk isn't encrypted, which is recommended on a VPS;
- a ufw rule and daily backups;
- permission to install missing packages (cryptsetup);
- the first invite code.

Afterwards, these commands are available:

- `ppserver status` - is it running, plus fingerprints and addresses.
- `ppserver invite` - another one-time invite code.
- `ppserver uninstall` - remove it.
- `journalctl -u parallelpath` - the server's log.

**Admin portal.** The server has a web admin portal at `https://<server>:8443/admin/` for people, devices,
invites, backups, logs and the server's audit log. Every admin action is recorded in that audit log.

- Windows: click Admin in the Manager window, or Admin portal… on the icon's menu. The first time, set a passphrase
  and add the portal to an authenticator app (TOTP).
- Ubuntu: the installer ends with a one-time setup link (valid 24 hours). Later, `sudo ppserver admin setup-link`
  makes a new one.

## 2. Install the apps

- **Windows:** run `ParallelPath-Setup-0.0.1.exe`. It installs for your Windows account only, with desktop and
  Start-menu shortcuts. It brings its own runtime, so nothing else is needed. Remove it in Settings › Apps.
- **Android:** install `ParallelPath-0.0.1-android.apk` (allow installs from your file manager or browser when
  asked). It's signed with the ParallelPath release key; later versions will update it in place.

- **Any device, in a browser:** open the server's address (for example `https://family.home.arpa:8443/`) and choose
  **Open ParallelPath in this browser** - or go straight to `/app/`. It's the same app; all encryption happens in the
  browser, which needs to be a current Chrome, Edge, Firefox or Safari (they all speak post-quantum TLS). You can
  install it to the home screen or desktop from the browser's menu.

### Certificates (no browser warnings)

New servers make their own **certificate authority** (the default). It can only vouch for the server's own names.
Trust it once per device and browsers open the server with no warning:

- On the server PC (Windows), setup does it for you ("Trust it on this PC now").
- On other devices, open `https://<server>:8443/trust` and follow the steps for that device (Windows, Android, iPhone,
  Mac, Linux). Check the fingerprint shown there matches the one the server setup showed.
- The ParallelPath apps for Windows and Android don't need this.

For a server reachable from the internet with a domain name, choose **Let's Encrypt** in setup instead (ports 80 and
the server's port must be reachable): browsers everywhere trust it automatically. Or give your own certificate files.

## 3. Try it

1. In the app, enter the server address and the first invite code (`S.…`). Check the certificate fingerprint matches
   the one the server setup showed, then create your keys.
2. In Family, invite the other parent and others. Each gets a `J.…` code. Approve them after comparing safety numbers.
3. Chat with messages, emoji, reactions, photos and videos. Edits and withdrawals need a reason, and their history
   stays. The Audit log tab shows every signed action and whether the chain verifies.
4. In Calendar › Schedule, propose a parenting schedule (alternating weeks, 2-2-3 and others). It goes on the
   calendar only when the other parent agrees, and each day's bar then shows who has the children (split on
   handover days).
5. Tap a day to add an event or ask for a change. The other parent sees the request under Requests (and a count on
   the Calendar tab) and agrees or declines; declining needs a reason. Editing or cancelling an event, or changing a
   proposal, also needs a reason, and the history is kept.
6. Check in from today's day panel, for example at a handover. On Android you can tick "Include my location"; the app
   asks for location permission the first time and only sends a location when that's ticked.
7. Expenses: add something you paid, with a receipt, and the share you're asking for. The other parent agrees or
   declines (with a reason), records their payment, and you confirm it arrived. The balance shows who owes whom.
8. Documents: folders and files, Explorer-style. Upload a new version with a reason and the old one stays. Each file
   shows who opened which version and when.
9. Info bank: add each child, then details such as the GP, school, allergies and sizes. Either parent can update an
   entry; the reason and every earlier version are kept.
10. My journal: private notes only you can read. The other parent can't see them, or even that they exist. "Share
    with the family" posts a copy into the messages.
11. Calls: the phone and camera buttons at the top of a family start a voice or HD video call (up to 1080p, 30 fps).
    The other parent's devices ring. Every audio and video frame is encrypted end to end with a key shared under the
    family's post-quantum keys; the server's relay (for calls across the internet) only ever forwards what it can't
    read. Mute, camera off, switch camera (phones) and screen sharing (computers) are on the call screen; the call
    appears in the conversation afterwards. On Windows the call opens in an Edge window.
12. Reports: Audit log › **Court-ready report…** makes a PDF to read and a signed .json file (the evidence) for any
    dates and sections. **Verify a report…** checks one (anyone can also run `ppserver verify-report file.json`).
13. Your devices: Settings › **Link another device…** shows a code; on the new device choose "I already use
    ParallelPath" and enter it, then confirm on the first device. The new device gets everything, including your
    journal.
14. Settings › **Make a recovery code** - write it down. If you lose every device, choose "Recover my account" when
    setting up a new one: your old devices are signed out, and each family's parents approve the new one. The
    family's keys rotate away from the lost devices on their own.
15. Write a message with something like "You ALWAYS do this!!!" - the **tone check** suggests a calmer version (turn
    it off in Settings).
16. Leave the app in the background and have the other parent send a message: a **notification** says who wrote
    (the text itself only if you turn that on in Settings). Add an event with a reminder a few minutes ahead.
17. Family › invite a **Professional**, choosing "From today" under "They see": once approved they see only what's
    written from then on. Invite a **Child** and try **Children can send messages**.
18. Calendar › Schedule › Propose › **Custom** to build your own cycle; Calendar › a day › Ask for a change › **Ask
    for another period in the same request**.
19. Make a report: besides the PDF and signed file there's **Open as a spreadsheet**.
20. Messages › **New private conversation** › choose the other parent: only you two can read it. Invite a **Child**
    (the invite shows **What they can do**; try **Change…**). Once approved, the child sees the family chat and
    calendar, and in Messages can start a conversation only with the parents.
21. On the child's device, the phone button offers only the parents. Calling Mum rings only Mum's devices. Then, as
    the parent who invited the child, open **Family › the child › Permissions…** and set calls to Nobody: the
    child's next call is refused.
22. Type something sharp, like "Pick-up is at 5. You useless idiot!!!": the tone check offers **Use the calmer
    version** ("Pick-up is at 5."). It also checks edits, reasons, decline comments and change-request notes.
23. The **?** button at the top: ask "how do I message just the other parent?" or "I forgot my password".
24. Type slowly in the message box: the **tone bar** under it moves from green (Calm) to amber or red as the wording
    gets sharper. Tap **Advice** (or the light bulb) for what may come across badly, a calmer version to **Use**,
    and an **Ask for advice** box - try "how do I reply to an angry message?".
25. Start a call on Windows: the call window shows no browser warnings, and after you allow
    the camera and microphone once, the next call doesn't ask again.
26. **Documents explorer** (Windows): right-click a file, a folder and empty space - each shows its menu. Make a
    folder, drag a file onto it (give a reason), then right-click › Move to… to move it back. Select two
    files with Ctrl+click and choose Copy, open another folder, Ctrl+V. Drag a file in from File Explorer: it uploads.
    Right-click a file › Save a copy… to your Downloads. Try Recent, Shared in messages and Removed in the tree;
    restore something you removed. On the phone, press and hold a file for the same menu, and Select for several.
27. Right-click (or press and hold) a message with a photo: Save a copy… and Save to Documents. Right-click an
    expense, an info bank entry, a journal entry, a calendar event and a person on the Family tab.
28. As a parent, Settings › Families doesn't offer a new family space. In the admin portal, Accounts › New
    families › **+** for that account: the app's Settings now offers it once; after one family it's gone again.

## Known limits

- The Windows setups aren't code-signed yet, so Windows SmartScreen may say "Windows protected your PC". Choose
  More info › Run anyway. On a PC with **Smart App Control** turned on, Windows blocks unsigned programs outright;
  test there only once the setups are signed. A code-signing certificate comes with the public release.
- Android's APK signature can't be post-quantum: Android only supports RSA and EC. It only proves who built the app;
  everything the app protects uses ParallelPath's own post-quantum cryptography.
- Calls need the server's relay ports open for calls across the internet: TCP/UDP 3478 and UDP 49160-49200 (setup adds
  the firewall rules; open them on your router or cloud firewall too). On the same network they connect directly.
- A forgotten passphrase can't be reset (nobody else has it): link the device again from another of yours, or use your
  recovery code.
- The web app downloads about 15 MB the first time (the emoji font loads in the background); after that the browser
  keeps it.
- Calls between Firefox or Safari and the apps or Chromium browsers aren't tested yet (Chromium-based ones are).
- Notifications and reminders come while the app is unlocked (on Android, while it's running).
