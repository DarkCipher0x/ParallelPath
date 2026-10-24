# ParallelPath user guide

ParallelPath is a private place for separated parents to organise their children's lives: messages, the parenting
calendar, shared expenses, documents, the children's details, calls and court-ready reports. Everything is
end-to-end encrypted with post-quantum cryptography, and nothing can be quietly changed or deleted.

This guide is for parents, other family members, children and professionals (mediators, solicitors, guardians).
Whoever runs your ParallelPath server has a separate guide (ADMIN-GUIDE.md).

Version 0.0.1.

---

## Contents

1. [How ParallelPath protects you](#1-how-parallelpath-protects-you)
2. [Getting the app](#2-getting-the-app)
3. [Setting up](#3-setting-up)
4. [Your family space](#4-your-family-space)
5. [Messages](#5-messages)
6. [Calendar and parenting time](#6-calendar-and-parenting-time)
7. [Expenses](#7-expenses)
8. [Documents](#8-documents)
9. [Info bank](#9-info-bank)
10. [My journal](#10-my-journal)
11. [Voice and video calls](#11-voice-and-video-calls)
12. [Audit log and court-ready reports](#12-audit-log-and-court-ready-reports)
13. [Notifications and reminders](#13-notifications-and-reminders)
14. [Your devices, passphrase and recovery code](#14-your-devices-passphrase-and-recovery-code)
15. [Roles: who can do what](#15-roles-who-can-do-what)
16. [Settings](#16-settings)
17. [Help & FAQ](#17-help--faq)
18. [Troubleshooting](#18-troubleshooting)

---

## 1. How ParallelPath protects you

- **Only your family can read what you share.** Messages, files, events, expenses and names are encrypted on your
  device before they leave it, and only the devices of people in your family can decrypt them. The server stores
  ciphertext. Neither its owner nor anyone who breaks into it can read your family's content.
- **Post-quantum.** The encryption uses ML-KEM-1024 and ML-DSA-87 (the newest NIST standards) together with classical
  P-384 and Ed25519. It stays secure even against future quantum computers, as long as either half holds.
- **Nothing disappears.** You can't delete a message or record. You can edit or withdraw one, but you must give a
  reason, and the earlier version stays in the history for everyone in the family to see.
- **Everything is on the record.** Every action is signed by the device that did it and chained into a tamper-evident
  audit log: sending, reading, editing, opening a file, starting a call, making a report. If anyone (including the
  server) tried to rewrite history, every family member's app would detect it.
- **You control your keys.** They're created on your device and protected by your passphrase. Nobody can reset that
  passphrase for you, so keep it safe and make a recovery code (section 14).

---

## 2. Getting the app

| Device | How |
|---|---|
| Windows 10 or 11 | Run `ParallelPath-Setup-<version>.exe`. It installs for your Windows account only; no administrator rights needed. |
| Android 10 or newer | Install `ParallelPath-<version>-android.apk` (allow installs from your browser or file manager when asked). |
| Any device with a browser | Open your server's address (for example `https://family.example.org:8443/`) and choose **Open ParallelPath in this browser**, or go to `/app/`. Use a current Chrome, Edge, Firefox or Safari. You can install it to your home screen from the browser's menu. |

All three are the same app, with the same encryption. The web app does all its cryptography inside your browser.

**Certificate warning in the browser?** Your server uses its own certificate authority. Open
`https://<your server>/trust` once on that device and follow the steps for it. Check that the fingerprint shown
matches the one your server's owner gave you. The Windows and Android apps don't need this.

---

## 3. Setting up

You need the **server address** and an **invite code**:

- `S.…`: from the server's owner. It lets you start **one** new family space.
- `J.…`: from the other parent. It lets you join their family space.

Only the server's owner (its administrator) can bring a new family onto the server. Parents invite people into
their own family; they can't start other families or hand out server invites. If you need a second family space
(say, for children with a different co-parent), ask the server's owner to allow it.

Steps:

1. Enter the server address and the invite code, then **Continue**.
2. The app shows the server's **certificate fingerprint** and **signing key**. Check that the fingerprint matches what
   the server's owner gave you, then choose **It matches - continue**. From then on, the app refuses any other
   server pretending to be yours.
3. Enter your name (the family sees it; the server never does), a name for this device, and a **passphrase** of at
   least 8 characters. A few random words is best. Choose **Create my keys**.

If you joined with a `J.` code, you'll see **Waiting for approval**. A parent approves you after you've compared
**safety numbers** (section 4).

**Already use ParallelPath on another device?** Choose **I already use ParallelPath** and see section 14.
**Lost all your devices?** Choose **Recover my account** and see section 14.

---

## 4. Your family space

A family space (for example "Mia & Leo") holds everything for one family. Open it to see the tabs: **Messages**,
**Calendar**, **Expenses**, **Documents**, **Info bank**, **My journal**, **Family** and **Audit log**. A number on a
tab means something is waiting for you.

### Inviting someone

**Family › Invite someone**: choose the role and **Create invite code**, then send the code privately. It works once,
for 7 days.

| Role | For |
|---|---|
| Parent | The other parent: full access. |
| Family member | Grandparents, step-parents: can read and add, not manage the family. |
| Professional | Mediators, solicitors, guardians: **read-only**, and access **ends automatically**. Choose how long (30-365 days) and how much history they see (**All history**, **Last year**, **Last 3 months** or **From today**). Older history stays hidden from them, and their devices never get the keys for it. |
| Child | Reads the family chat and calendar; messages and calls the parents (see permissions below). |

### What someone can do (permissions)

When you invite anyone other than a parent, **What they can do** shows their permissions, starting from sensible
defaults for the role. Choose **Change…** to set:

- **They see:** which parts of the family's record they receive (family chat, calendar, expenses, documents, info
  bank). The server doesn't even send them the rest.
- **They can add to:** which of those they can add to. Expenses, parenting schedules and change requests are
  always between parents.
- **Private messages with:** anyone in the family, the parents, chosen people, or nobody.
- **Calls with:** the same choices, plus **video calls** (or voice only) and whether they can **ring the whole
  family** at once.

Only **you, the parent who invited them**, can change their permissions later: **Family › their name ›
Permissions…**. Changes take effect at once and are recorded in the audit log. Everyone can see anyone's
permissions, but nobody else can change them. Parents always have every permission.

Defaults: family members see and add to everything except expenses, and can message and call anyone. Children see the
family chat and calendar, can't add to them, and can message and call only the parents. Professionals see
everything, read-only, and can't message or call.

### Approving someone: compare safety numbers

When someone joins, they show as **waiting for approval**. Choose **Approve…** and you'll see a 60-digit
**safety number**. Call them or meet in person, and ask them to read the safety number shown on their Family tab.

- **It matches:** choose **It matches - approve**. Their devices get the family's keys and can read the history
  their role allows.
- **It doesn't match:** don't approve. Someone may be impersonating them.

### Removing someone

Parents can remove family members, professionals and children, and a parent who joined but was never approved.
The family's encryption keys are rotated straight away, so nothing new can be read with anything the removed person
still holds. What they could already see stays in the record. Neither approved parent can remove the other; if
that's ever genuinely needed, the server's owner can block an account.

### Children

If children are members, parents see **Children can send messages**. On, every child in the family can write in the
family chat (messages and reactions, nothing else); off, only children whose own permissions allow it can. Either
parent can change it, and the change is recorded. Children can always message and call whoever their permissions
allow (by default, the parents) in private conversations.

### Names

**Family › Names** sets your name and the family's name. Both are stored encrypted inside the family space, and
changes are kept in the history.

---

## 5. Messages

The **Messages** tab has the **family chat** (everyone who may see it) and your **private conversations**, along the
top.

### Private conversations

**New private conversation** › tick one or more people › **Start**. Only those people (and you) can read it: it has
its own encryption keys, given only to their devices, so the rest of the family and the server can't. Starting a
conversation with the same people again opens the existing one. The family's permissions decide who you may talk to.
If a permission is taken away, posting there stops at once. If someone leaves the family, the conversation's keys
change so they can't read anything new.

Inside a conversation, **Voice call** and **Video call** call just its people (section 11).

### Writing

- Type and press **Enter** to send (**Shift+Enter** for a new line). Use the paperclip for photos, videos and files,
  and the smiley for emoji.
- **Reply**, **react**, **edit** or **withdraw** from a message's menu. Editing and withdrawing need a reason. The
  original stays in the message's **history**.
- **Read receipts** show who has read a message, and when. They come from the signed audit log, so they can't be
  faked.
- **Tone check:** before a message (or an edit, a reason, a decline comment or a change-request note) goes out,
  ParallelPath points out wording that may come across as hostile and suggests a calmer way to put it. A small
  model built into the app catches insults, threats and hostility even in ordinary words; co-parenting rules catch
  blame ("you always"), legal threats, dismissiveness, shouting in capitals and "!!!". Choose **Use the calmer
  version** (it goes back into the box for you to check), **Change it myself**, or **Send as it is**. It runs on
  your device in a fraction of a millisecond: nothing is sent anywhere, and there's no cloud AI. Turn it off in
  **Settings**. Details: TONE-MODEL.md.
- **Tone bar:** as you type, a thin bar under the message box shows how the draft may come across: green
  **Calm**, amber **Could come across badly**, red **May read as hostile**. It updates a moment after you stop
  typing and is worked out on your device, like the tone check.
- **Advice:** tap **Advice** on the tone bar (or the light-bulb button beside the message box) for what may come
  across badly and why, a calmer version you can **Use**, short tips for writing to a co-parent, and an **Ask for
  advice** box ("how do I reply to an angry message?", "how do I say no politely?") answered from this guide and the
  FAQ, on your device. It's general guidance, not legal advice.
- Calls appear in the conversation too: who called, when, and for how long.

---

## 6. Calendar and parenting time

The **Calendar** tab has three views:

- **Calendar:** the month, each day coloured by who has the children, split on handover days. Tap a day for its
  events and check-ins.
- **Requests:** schedule proposals and change requests, and which ones need your answer.
- **Schedule:** the agreed parenting schedule.

### A parenting schedule

**Schedule › Propose a new schedule**. Choose a common pattern (alternating weeks, 2-2-3, 2-2-5-5, 3-4-4-3, every
other weekend, two weeks each) or **Custom**. With Custom you build the cycle step by step, for example 5 days with
one parent and 2 with the other. Choose who starts, the start date and the handover time. Nothing changes until the
other parent agrees.

### Events

Tap a day, then **Add event**: a title, a category (school, medical, activity…), times or all-day, repeats, which
children, notes and a **reminder**. Editing or cancelling an event needs a reason, and its history is kept.

### Asking for a change

Tap a day, then **Ask for a change**: who has the children, from when until when, and why. To ask for several dates
at once (two Saturdays, say), choose **Ask for another period in the same request**. The other parent agrees or
declines, and declining needs a reason.

### Check-ins

From today's day panel, **Check in**, for example at a handover. On a phone you can tick **Include my location**. It's
only sent when ticked, and it's end-to-end encrypted like everything else.

---

## 7. Expenses

1. **Add expense:** what it was, the amount and currency, the date, a category, which children, your note, the share
   you're asking for (50% by default) and receipts (photos or PDFs).
2. The other parent **agrees** or **declines** (with a reason).
3. Once agreed, they **record their payment**: how and a reference.
4. You **confirm it arrived**.

The **balance** at the top shows who owes whom. An expense can't be changed once someone has paid towards it.

---

## 8. Documents

A full file explorer for the family's files. Everything is encrypted on your device before it's uploaded, every
version is kept, and each opening is recorded.

**Finding your way**

- The **folder tree** on the left, with **Recent** (the latest files), **Shared in messages** (every photo and file
  sent in the family's messages and your private conversations) and **Removed** (kept for the record).
- The **address bar** (Documents › School › Reports): click a part to go there. **Back**, **Forward** and **Up**
  work as in Windows Explorer.
- **Search** looks at names in every folder.
- **Details** view with columns you can sort by (click a column: Name, Date modified, Type, Size, Added by) or
  **Large icons** with picture thumbnails. The **details pane** on the right shows the selected file's versions, who
  opened it and its history.

**Doing things**

- **Right-click** anything (on a phone, **press and hold**) for everything you can do with it: Open, Open in another
  app, Save a copy…, Send in a message, Cut, Copy, Move to…, Copy to…, Rename…, Upload a new version…, Edit note…,
  Copy name/path, Show in folder, Change history, Properties, Remove… or Restore. Right-click empty space for New
  folder, Upload files, Paste, View, Sort by, Show removed, Select all and Refresh.
- **Select several**: Ctrl+click, Shift+click, Ctrl+A (on a phone: press and hold, then **Select**). Then move,
  copy, remove or save copies of all of them at once.
- **Move or copy**: cut/copy and paste, **drag** files onto a folder (in the list, the tree or the address bar), or
  use Move to…/Copy to…. A folder moves or copies with everything in it. **Copy to** can put a copy into **another
  family** you're in (it's encrypted again for that family).
- **Upload**: the Upload button, or **drag files in from your computer** (Windows).
- **Save a copy…** puts a normal (decrypted) copy where you choose - your Downloads folder, a USB stick. Saving a
  copy counts as opening it, so it's recorded.
- **Shared in messages**: right-click a file to **Save to Documents** into a folder of your choice.
- Only the person who added something can rename, move, replace or remove it (others can copy it). A move, rename or
  removal needs a short reason, which stays in its history. Removed things can be **restored** by whoever added them.

**Keyboard** (Windows and the web app): Enter open · Alt+Enter properties · F2 rename · Del remove · Ctrl+C/X/V copy,
cut, paste · Ctrl+A select all · Ctrl+S save a copy · Ctrl+F search · Ctrl+Shift+N new folder · Ctrl+U upload ·
Backspace or Alt+Up up a level · Alt+Left/Right back/forward · F5 refresh · arrows (with Shift) to select · Alt+P
details pane.

**Right-click menus everywhere else too**

Messages, attachments, expenses, info bank entries, journal entries, calendar events and family members all have a
right-click menu (press and hold on a phone) with what you can do: reply, copy, edit, open or save attachments,
change, withdraw, history and more.

---

## 9. Info bank

The children's details in one place: the GP, school, allergies, medication, sizes, contacts. Add each child, then
their details. Parents and family members can update an entry; the reason and every earlier version are kept. If you both change
the same entry at once, both versions are shown, so nothing is silently lost.

---

## 10. My journal

Private notes that **only you** can read: not the other parent, not the server. They can't even tell your journal
exists. Each entry has a date, an optional mood and tags, and can have attachments.

**Share with the family** posts a copy of an entry into the messages. Your journal itself stays private.

---

## 11. Voice and video calls

Use the **phone** or **camera** button at the top of a family and choose **who to call**:

- **Everyone in the family**: for parents, and for people a parent allowed to ring everyone.
- **Chosen people**: tick one or more. The call happens in your private conversation with exactly them, so **only
  their devices ring** and only they get the call's key.

In a private conversation, **Voice call** and **Video call** call its people directly. Who you can call, and whether
video is allowed, follows your permissions (by default, children call their parents). The camera button only shows
when video is allowed.

- Calls are HD (up to 1080p at 30 fps for video; full-band audio).
- Every audio and video frame is encrypted end to end, with a key exchanged under the family's post-quantum keys.
  When the server relays a call between networks, it only forwards what it can't read.
- On the call screen: **mute**, **camera off**, **switch camera** (phones) and **share your screen** (computers).
- If the other side's connection disappears for good (battery, no signal), the call ends on its own after about 30
  seconds.
- On Windows the call opens in a separate Edge window; on Android, a full-screen call screen; in a browser, on top of
  the app. The first time, allow the camera and microphone.

---

## 12. Audit log and court-ready reports

**Audit log** lists every signed action in the family, with the time, who and which device, and whether the whole
chain verifies. A tick means nothing has been altered.

**Audit log › Court-ready report…** makes three files on your device, for any dates and sections (messages,
calendar, expenses, documents, info bank, audit log, your journal):

- a **PDF** to read and print;
- a **signed JSON file**: the evidence, signed with your identity key and tied to the family's audit chain;
- a **spreadsheet (CSV)** for sorting and totals. It's a convenience copy; the signed file is the evidence.

Choose **My private conversations** to include your private conversations in the family. They're marked as
private, with who could read them.

**Verify a report…** checks any report file: whether it's intact, who signed it, and whether it matches the family's
audit log. Anyone else (a court, a solicitor) can check it with `ppserver verify-report file.json`.

Making a report is itself recorded in the audit log (that it was made, by whom and when; not what's in it).

---

## 13. Notifications and reminders

ParallelPath tells you about new messages, requests that need your answer, expenses, documents and check-ins, and
reminds you before events that have a reminder set.

- **Windows:** in the notification centre, when ParallelPath isn't the window in front.
- **Android:** as notifications. Android asks for permission once.
- **Browser:** as browser notifications, when the tab isn't in front. The browser asks for permission once.

By default notifications say who and what ("Sam · Mia & Leo: New message") but **not the message itself**, because
they can appear on a locked screen. Turn on **Show message text in notifications** in Settings if you prefer.

Notifications and reminders come from the app while it's unlocked (on Android, while the app is running).

---

## 14. Your devices, passphrase and recovery code

### Adding another device

1. On a device you already use: **Settings › Link another device…**. It shows a code (`L-…`).
2. On the new device: choose **I already use ParallelPath**, enter the server address and that code, and set a
   passphrase for the new device.
3. Back on the first device, check the new device's name and **confirm**.

The new device gets everything, including your journal.

### Your passphrase

It protects your keys on that device (Argon2id). **Nobody can reset it**, not even the server's owner. If you forget
it, link the device again from another of yours, or use your recovery code.

On Windows and Android, ParallelPath locks itself after a period without use. Use the lock button to lock it
straight away.

### Recovery code

**Settings › Make a recovery code** shows a code (`R-…`). Write it down and keep it somewhere safe and offline. It
holds an encrypted copy of your identity and your journal keys.

If you lose every device: on a new one, choose **Recover my account** and enter the code. Your old devices are signed
out, and each family's parents approve the new device after comparing safety numbers (your safety number doesn't
change). The family's keys are then rotated automatically, so the lost devices can't read anything new.

Anyone with your recovery code could do the same, so treat it like a house key.

---

## 15. Roles: who can do what

| | Parent | Family member | Professional | Child |
|---|---|---|---|---|
| Read the family's record | Everything | By default everything but expenses | Read-only, within their history window and access period | By default family chat and calendar |
| Family chat | Yes | Yes (default) | No | If a parent allows it |
| Calendar events, check-ins, documents, info bank | Yes | Yes (default) | No | No (default) |
| Propose schedules, request changes, expenses | Yes | No | No | No |
| Private conversations | Anyone | Anyone (default) | Nobody (default) | Parents (default) |
| Calls | Anyone, or the whole family | Anyone (default) | Nobody (default) | Parents (default) |
| Invite and approve people, family settings | Yes (once approved) | No | No | No |
| Reports | Yes | Yes | Yes | Yes |
| Private journal | Everyone has their own | | | |

Everything marked "default" can be changed by the parent who invited them (section 4).

---

## 16. Settings

- **Appearance:** System, Dark or Light, and high contrast.
- **Notifications and messages:** notifications on or off, message text in notifications, and the tone check.
- **Families:** join another family with an invite code. Creating another family space only shows when the
  server's owner has allowed you to.
- **Your devices:** link another device; see your devices.
- **Recovery:** make or replace your recovery code.
- **Security:** the cryptography in use, and your server's details.

---

## 17. Help & FAQ

The **?** button at the top opens **Help & FAQ**. Ask in your own words ("how do I message just Dad?", "I forgot my
password") and it shows the best answers from this guide and the FAQ, with where they come from. It works on your
device, offline, with no AI service: the answers are always the guide's own words, so it never makes anything up.

## 18. Troubleshooting

| Problem | What to do |
|---|---|
| "The server's certificate has changed" | If your server's owner changed its certificate, the app normally follows on its own: the server proves the change with a key your app already trusts. If it can't, ask the server's owner before going further. It could be an attack. |
| The browser warns about the certificate | Open `https://<server>/trust` and follow the steps for your device (section 2). |
| Waiting for approval | Ask a parent to approve you, after you've compared safety numbers. |
| "Some frames couldn't be decrypted" on a call | Hang up and call again. If it keeps happening, update the app on both sides. |
| A call doesn't ring | Make sure the other person's app is unlocked and online (the dot at the top left is green). |
| No notifications | Check they're on in Settings, and that Windows, Android or your browser allows them for ParallelPath. |
| Forgot your passphrase | Link the device again from another of yours, or use your recovery code (section 14). |
| "The family's audit log failed verification" | Don't ignore it: something in the record doesn't add up. Tell the other parent and the server's owner, and make a report as it stands. |
