ParallelPath Server for Ubuntu
==============================

Post-quantum secure co-parenting, hosted by you. Built by DarkCipher0x at DarkCipher Systems (AGPL-3.0).

Install
  sudo sh install.sh

The installer checks this machine, asks a few questions and lets you tick what to set up:
  - a sandboxed systemd service under its own 'parallelpath' account, started at boot
  - an encrypted data container (LUKS2) if the disk isn't already encrypted - recommended on a VPS
  - the master key sealed with systemd encrypted credentials (and the TPM, when there is one)
  - a ufw rule for the port, daily sealed backups, and the first invite code
It installs anything missing (cryptsetup) with apt. Needs Ubuntu 24.04 LTS or newer (systemd 250+).

Afterwards
  ppserver status        is it running? fingerprints and addresses
  ppserver invite        another one-time invite code
  ppserver install       upgrade (run the new version's installer) or repair
  ppserver uninstall     remove it (add --purge to delete all data and keys too)
  journalctl -u parallelpath    the server's log

The full administrator guide (certificates, ports, calls, the admin portal, backups, upgrades): ADMIN-GUIDE.md
