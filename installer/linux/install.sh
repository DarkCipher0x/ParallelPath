#!/bin/sh
# ParallelPath Server for Ubuntu (x86-64). Run:  sudo sh install.sh
# Checks the download, then starts the interactive installer (service, encryption, firewall, backups).
# Unattended:  sudo sh install.sh --plan plan.json
set -eu
here=$(cd "$(dirname "$0")" && pwd)
cd "$here"
if [ "$(uname -m)" != "x86_64" ]; then
  echo "This download is for 64-bit Intel/AMD (x86-64); this machine is $(uname -m)." >&2
  exit 1
fi
if [ "$(id -u)" != "0" ]; then
  exec sudo sh "$here/install.sh" "$@"
fi
if ! sha256sum --check --quiet SHA256SUMS; then
  echo "The download is damaged (checksum mismatch). Please download it again." >&2
  exit 1
fi
chmod 755 ppserver
exec ./ppserver install "$@"
