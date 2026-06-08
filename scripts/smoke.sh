#!/usr/bin/env bash
# smoke.sh — end-to-end guard for the native semantic chain. Onboards a tiny
# fixture repo with the given columbus binary and asserts that a natural-language
# query (no token overlap with the code) returns a vector-backed hit.
#
# Usage: scripts/smoke.sh /path/to/columbus
set -euo pipefail

bin="${1:?usage: smoke.sh <columbus-binary>}"
bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"

work="$(mktemp -d)"
export COLUMBUS_DATA_DIR="$(mktemp -d)"
cleanup() { rm -rf "$work" "$COLUMBUS_DATA_DIR"; }
trap cleanup EXIT

cd "$work"
git init -q
git config user.email smoke@test.local
git config user.name smoke

cat > auth.go <<'EOF'
package auth

// New builds an authenticator that validates bearer tokens against the store.
func New() *Authenticator { return &Authenticator{} }

type Authenticator struct{}

// Check reports whether the supplied credential is acceptable.
func (a *Authenticator) Check(token string) bool { return token != "" }
EOF
git add -A

echo "==> doctor"
"$bin" doctor || true   # informational; must not gate the smoke result

echo "==> install"
"$bin" install

echo "==> search (semantic, zero token overlap)"
out="$("$bin" search "how are credentials verified" --kind code --json)"
echo "$out"

# A vector-backed result is tagged "semantic match" in the why field. Its
# presence proves onnxruntime loaded, the model embedded the query, and vec0
# returned a nearest neighbor.
if echo "$out" | grep -q "semantic match"; then
  echo "PASS: semantic (vector) hit present"
else
  echo "FAIL: no semantic hit — search fell back to keyword (runtime chain broken)" >&2
  exit 1
fi
