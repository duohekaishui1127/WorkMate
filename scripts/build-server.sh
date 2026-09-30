#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
target=${1:-amd64}
case "$target" in amd64|arm64) ;; *) echo "Usage: bash scripts/build-server.sh [amd64|arm64]" >&2; exit 2 ;; esac
output="dist/server-linux-$target"
mkdir -p "$output"
CGO_ENABLED=0 GOOS=linux GOARCH="$target" go build -trimpath -buildvcs=false -ldflags="-s -w" -o "$output/workmate-admin" ./cmd/workmate-admin
cp deploy/linux/* "$output/"
cp DEPLOY.md "$output/"
key=${WORKMATE_ADMIN_PUBLIC_KEY_FILE:-admin-data/license-public.key}
[[ -f "$key" ]] || { echo "Missing admin public key; initialize your local backend or set WORKMATE_ADMIN_PUBLIC_KEY_FILE." >&2; exit 1; }
cp "$key" "$output/client-license-public.key"
chmod 0755 "$output/workmate-admin" "$output/install.sh" "$output/backup.sh"
files=(workmate-admin workmate-admin.service admin.env.example Caddyfile.example install.sh backup.sh DEPLOY.md client-license-public.key)
entries=()
for file in "${files[@]}"; do entries+=("server-linux-$target/$file"); done
tar -C dist -czf "dist/WorkMate-Server-Linux-$target.tar.gz" "${entries[@]}"
echo "Created dist/WorkMate-Server-Linux-$target.tar.gz (does not contain private keys or a database)."
