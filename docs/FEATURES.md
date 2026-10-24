# ParallelPath features (0.0.1)

Everything below is end-to-end encrypted with post-quantum cryptography and kept in a signed, tamper-evident record.

## Communication

- Messages with emoji, reactions, replies, photos, videos and files.
- Read receipts from the signed audit log ("read by Sam, 18:02").
- Edits and withdrawals only with a reason; every version stays visible.
- **Private conversations** with one or more people in the family, with their own keys: the rest of the family and
  the server can't read them.
- **Tone check** before anything goes into the record (messages, edits, reasons, comments, notes): a 1.8 MB
  on-device model (trained on 1.8 million rated comments) plus co-parenting rules, with a **calmer version** to
  use. It answers in microseconds, offline, and nothing leaves the device. A live **tone bar** under the message box,
  and **Advice** on any draft (tips, a calmer version, and questions answered from the built-in help).
- HD voice and video calls (up to 1080p30) with end-to-end encrypted frames, screen sharing, camera switching, and
  the call logged in the conversation. Calls are never recorded.
- **Choose who to call:** the whole family, or chosen people (only their devices ring). Children can call their mum
  or dad directly.
- Notifications for messages, requests, expenses, documents and check-ins. Message text in notifications is off by
  default.

## Parenting time

- Shared calendar, coloured by who has the children, with handover days split.
- Schedule templates (alternating weeks, 2-2-3, 2-2-5-5, 3-4-4-3, every other weekend, two weeks each) and fully
  **custom cycles**.
- Schedules apply only once the other parent agrees.
- **Change requests**, including several periods in one request; declining needs a reason.
- Events with categories, repeats, children, notes and **reminders**; edits and cancellations need a reason.
- Check-ins at handovers, with an optional encrypted location (phones).

## Money

- Expenses with receipts, categories, children and a requested share.
- Agree or decline; record a payment; the payer confirms it arrived.
- A running balance of who owes whom.

## Records

- **Documents:** a full file explorer - folder tree, Recent, Shared in messages and Removed views; details and icon
  views with sortable columns and thumbnails; search across folders; multi-select; right-click (long-press) menus;
  cut/copy/paste, drag to move, drag files in from Windows to upload; moving, copying (also into another family) and
  removing folders with their contents; restore; save copies to the device; send a file in a message; keyboard
  shortcuts; a details pane. Versions with reasons, an access log (who opened which version), in-app previews.
- **Right-click menus** (long-press on phones) on messages, attachments, expenses, info bank entries, journal
  entries, calendar events and family members.
- **New families are the server owner's decision:** a server invite starts one family; the admin portal allows more.
- **Info bank:** each child's details (GP, school, allergies, sizes…), versioned with reasons, with conflict display.
- **Private journal:** readable only by its author (the server can't even tell it exists); entries can be shared
  into the messages.
- **Audit log:** every action, signed and chained, verified on every device.
- **Court-ready reports:** a PDF, a signed JSON (the evidence) and a CSV, for any dates and sections; verifiable in
  the app or with `ppserver verify-report`.

## People and access

- Roles: parents, family members, professionals and children.
- **Permissions per person**, set by the parent who invited them: which sections they see and add to, who they may
  message privately and call, video or voice only, and whether they can ring the whole family. Only that parent can
  change them, and every change is in the audit log.
- Professionals: read-only, with access that ends automatically and an optional limit on how far back they see.
- Children: by default they read the family chat and calendar and can message and call their parents; parents can
  widen or narrow that per child.
- Safety-number approval for every new member and device.
- Several devices per person (linking), and a recovery code if every device is lost.
- Several family spaces per person (a new family needs the server owner's allowance).

## Help

- **Help & FAQ** in the app: ask in your own words; answers come from ParallelPath's own guide and FAQ, found on the
  device, offline, never made up.

## Platforms

- Windows app, Android app, and the full app in any modern browser (served by the family's own server).
- A self-hosted server for Windows or Ubuntu: a setup wizard or terminal installer, a service, firewall rules,
  encrypted storage, daily backups, its own certificate authority (or Let's Encrypt), a call relay, and a web admin
  portal.

## Compared with typical co-parenting apps

Hosted co-parenting services commonly offer messaging with tone checking, a shared calendar with swap requests,
expense tracking, an info bank, a journal, call features and exportable records. ParallelPath covers the same
ground. Where it differs by design:

- **The family's content is unreadable to the service.** Hosted apps generally hold readable copies of what
  parents write; ParallelPath's server holds only ciphertext.
- **Post-quantum** throughout: transport, key exchange, signatures and call keys.
- **Self-hosted:** the family (or someone they trust) runs the server; there's no central provider.
- **Verifiable records:** reports are signed and tied to a hash-chained audit log that anyone can check offline.
- **Calls aren't recorded**, by design: recording would put call content on the server.
- **Free and open source** (AGPL-3.0).
