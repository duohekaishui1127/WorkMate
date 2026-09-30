#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: sudo bash install.sh DOMAIN [EXISTING_ADMIN_DATA_DIRECTORY]"
  echo "First installation requires the existing admin-data used by your client build."
  echo "Updates reuse /var/lib/workmate; no database or license keys are replaced."
}
if [[ "${1:-}" == "--help" || $# == 0 ]]; then usage; exit 0; fi
if [[ $# -gt 2 ]]; then usage >&2; exit 2; fi
domain=${1,,}
if [[ ${#domain} -gt 253 || "$domain" != *.* || "$domain" == example.com || "$domain" == *.example.com || "$domain" == pro.your-domain.com ]]; then
  echo "Provide your real DNS domain, without https://, a path or a port." >&2; exit 2
fi
IFS='.' read -r -a labels <<< "$domain"
for label in "${labels[@]}"; do
  if [[ ! "$label" =~ ^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ ]]; then
    echo "Invalid DNS domain." >&2; exit 2
  fi
done
if [[ "$domain" == *. ]]; then echo "Invalid DNS domain." >&2; exit 2; fi
if [[ $(id -u) -ne 0 ]]; then echo "Run this installer with sudo." >&2; exit 1; fi
for tool in systemctl caddy curl useradd install; do
  command -v "$tool" >/dev/null || { echo "Missing dependency: $tool. See DEPLOY.md." >&2; exit 1; }
done
base=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
data=/var/lib/workmate
source_data=${2:-}
[[ -f "$base/workmate-admin" ]] || { echo "Missing Linux binary; use the complete server release package." >&2; exit 1; }
if [[ -f "$data/workmate.sqlite" ]]; then
  if [[ -n "$source_data" ]]; then echo "Data already exists. Update without a source-data argument; existing orders will be kept." >&2; exit 1; fi
  source_data=$data
elif [[ -z "$source_data" ]]; then
  echo "Copy your stopped backend's entire admin-data folder to this server, then pass its path." >&2; exit 1
fi
for file in workmate.sqlite license-private.key license-public.key; do
  [[ -f "$source_data/$file" && ! -L "$source_data/$file" ]] || { echo "Missing original backend data: $file" >&2; exit 1; }
done
if [[ -f "$base/client-license-public.key" ]] && ! cmp -s "$base/client-license-public.key" "$source_data/license-public.key"; then
  echo "The original backend public key does not match this client build. Restore the matching admin-data." >&2; exit 1
fi
# Validate the copied database and matching key before installing anything.
"$base/workmate-admin" -init -data-dir "$source_data" -listen 127.0.0.1:8090
id workmate >/dev/null 2>&1 || useradd --system --user-group --home-dir "$data" --shell /usr/sbin/nologin workmate
install -d -m 0755 /opt/workmate /etc/workmate /etc/caddy/workmate.d
install -d -m 0700 -o workmate -g workmate "$data"
if [[ "$source_data" != "$data" ]]; then cp -a -- "$source_data/." "$data/"; fi
chown -R --no-dereference workmate:workmate "$data"
chmod 0700 "$data"
chmod 0600 "$data/license-private.key" "$data/workmate.sqlite"
# Stop only WorkMate before replacing its executable.
systemctl stop workmate-admin.service 2>/dev/null || true
install -m 0755 "$base/workmate-admin" /opt/workmate/workmate-admin
install -m 0644 "$base/workmate-admin.service" /etc/systemd/system/workmate-admin.service
# Preserve an optional tracked download target across backend upgrades.
download_url=
if [[ -f /etc/workmate/admin.env ]]; then
  while IFS= read -r line; do
    case "$line" in WORKMATE_DOWNLOAD_URL=*) download_url=${line#WORKMATE_DOWNLOAD_URL=} ;; esac
  done < /etc/workmate/admin.env
fi
umask 077
cat > /etc/workmate/admin.env <<EOF
WORKMATE_LISTEN=127.0.0.1:8090
WORKMATE_DATA_DIR=$data
WORKMATE_PUBLIC_URL=https://$domain
WORKMATE_TRUSTED_PROXIES=127.0.0.1,::1
EOF
if [[ -n "$download_url" ]]; then printf 'WORKMATE_DOWNLOAD_URL=%s\n' "$download_url" >> /etc/workmate/admin.env; fi
sed "s/pro.your-domain.com/$domain/g" "$base/Caddyfile.example" > /etc/caddy/workmate.d/workmate.caddy
chmod 0644 /etc/caddy/workmate.d/workmate.caddy
if [[ ! -f /etc/caddy/Caddyfile ]]; then
  printf 'import /etc/caddy/workmate.d/*.caddy\n' > /etc/caddy/Caddyfile
  chmod 0644 /etc/caddy/Caddyfile
fi
# Keep existing Caddy sites and include only the WorkMate site.
if ! grep -Eq '^[[:space:]]*import[[:space:]]+/etc/caddy/workmate\.d/\*\.caddy[[:space:]]*(#.*)?$' /etc/caddy/Caddyfile; then
  cp -a /etc/caddy/Caddyfile "/etc/caddy/Caddyfile.before-workmate-$(date -u +%Y%m%d%H%M%S)"
  printf '\nimport /etc/caddy/workmate.d/*.caddy\n' >> /etc/caddy/Caddyfile
fi
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl daemon-reload
systemctl enable --now workmate-admin.service
systemctl enable caddy.service
systemctl reload-or-restart caddy.service
ready=0
for attempt in {1..30}; do
  if curl --fail --silent http://127.0.0.1:8090/healthz >/dev/null && systemctl is-active --quiet workmate-admin.service; then ready=1; break; fi
  sleep 1
done
[[ $ready -eq 1 ]] || { echo "Backend did not start; inspect journalctl -u workmate-admin." >&2; exit 1; }
echo "Admin: https://$domain/admin"
echo "Client server_url: https://$domain"
echo "Data and signing keys: $data (kept across updates)"
