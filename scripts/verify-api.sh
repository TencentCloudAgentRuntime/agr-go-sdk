#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
actual="$(mktemp)"
trap 'rm -f "$actual"' EXIT
go_cmd="${GO:-go}"
channel="${API_CHANNEL:-}"

if [[ -z "$channel" ]]; then
  channel="$(cd "$repo_root" && GOTOOLCHAIN=local "$go_cmd" run ./internal/cmd/controlcontract current-channel)"
fi
case "$channel" in
  stable|preview) ;;
  *) echo "unsupported API_CHANNEL: $channel" >&2; exit 2 ;;
esac
snapshot="$repo_root/contracts/public-api-$channel.txt"

(
  cd "$repo_root"
  GOTOOLCHAIN=local "$go_cmd" doc -all .
  GOTOOLCHAIN=local "$go_cmd" doc -all ./sandbox
) >"$actual"

diff -u <(perl -0777 -pe 's/\s+\z/\n/' "$snapshot") <(perl -0777 -pe 's/\s+\z/\n/' "$actual")

if [[ "$channel" == "stable" ]]; then
  diff -u \
    <(perl -0777 -pe 's/\s+\z/\n/' "$repo_root/contracts/public-api-stable.txt") \
    <(perl -0777 -pe 's/\s+\z/\n/' "$repo_root/contracts/public-api-preview.txt")
fi
