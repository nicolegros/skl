#!/usr/bin/env bash
# End to end check of the skl formula: build from source, brew test, CLI and completions, uninstall.
# Usage: e2e.sh <tap/formula, e.g. skl-ci/local/skl>
set -euo pipefail

fail() {
  echo "e2e FAIL: $*" >&2
  exit 1
}
ok() {
  echo "e2e ok: $*"
}

[[ $# -eq 1 ]] || fail "usage: $0 <tap/formula>"
formula="$1"

prefix="$(brew --prefix)"
bin="$prefix/bin/skl"
completions=(
  "$prefix/etc/bash_completion.d/skl"
  "$prefix/share/zsh/site-functions/_skl"
  "$prefix/share/fish/vendor_completions.d/skl.fish"
)

brew install --build-from-source "$formula" || fail "brew install --build-from-source $formula"
ok "$formula built from source and installed"

brew test "$formula" || fail "brew test $formula"
ok "brew test passed"

hash -r
[[ "$(command -v skl)" == "$bin" ]] ||
  fail "skl on PATH resolves to '$(command -v skl)', expected $bin"
version="$(brew list --versions "$formula" | cut -d ' ' -f 2)"
[[ "$(skl version)" == "$version" ]] ||
  fail "skl version printed '$(skl version)', expected '$version'"
ok "skl on PATH at $bin reports $version"

for file in "${completions[@]}"; do
  [[ -s "$file" ]] || fail "completion $file missing or empty"
done
ok "bash, zsh and fish completions installed"

brew uninstall "$formula" || fail "brew uninstall $formula"
hash -r
[[ ! -e "$bin" ]] || fail "$bin still present after uninstall"
if command -v skl >/dev/null; then
  fail "skl still on PATH at $(command -v skl) after uninstall"
fi
for file in "${completions[@]}"; do
  [[ ! -e "$file" ]] || fail "completion $file still present after uninstall"
done
ok "uninstall removed the binary and completions"
