#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "--help" ]]; then echo "Usage: sudo bash backup.sh [BACKUP_DIRECTORY]"; exit 0; fi
[[ $(id -u) -eq 0 ]] || { echo "Run with sudo." >&2; exit 1; }
data=/var/lib/workmate
[[ -f "$data/workmate.sqlite" && -f "$data/license-private.key" ]] || { echo "No existing WorkMate data found." >&2; exit 1; }
umask 077
destination=${1:-./backups}
mkdir -p -- "$destination"
destination=$(cd -- "$destination" && pwd)
[[ "$destination" != "$data" && "$destination" != "$data/"* ]] || { echo "Choose a backup directory outside the data directory." >&2; exit 1; }
was_active=0
if systemctl is-active --quiet workmate-admin.service; then was_active=1; fi
restore_service() {
  if [[ $was_active -eq 1 ]]; then systemctl start workmate-admin.service; fi
}
trap restore_service EXIT
if [[ $was_active -eq 1 ]]; then systemctl stop workmate-admin.service; fi
archive="$destination/workmate-data-$(date -u +%Y%m%dT%H%M%SZ)-$$.tar.gz"
tar -C /var/lib -czf "$archive" workmate
chmod 0600 "$archive"
echo "Backup: $archive (contains the database and private signing key; keep it private)"
