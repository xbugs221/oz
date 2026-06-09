#!/usr/bin/env bash
# Verifies release automation cannot build or publish without the local CLI test gate.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
workflow="$repo_root/.github/workflows/release.yml"
ci_workflow="$repo_root/.github/workflows/ci.yml"

test -s "$workflow"
test -s "$ci_workflow"
for gate in "$workflow" "$ci_workflow"; do
    grep -qF "Build local CLIs" "$gate"
    grep -qF 'go build -o "$install_dir/oz" ./cmd/oz' "$gate"
    grep -qF 'go build -o "$install_dir/wo" ./cmd/wo' "$gate"
    ! grep -qF "github.com/xbugs221/oz/releases/latest/download" "$gate"
    grep -qF '>> "$GITHUB_PATH"' "$gate"
    grep -qF '/oz" --version' "$gate"
    grep -qF '/wo" --version' "$gate"
done
grep -qF "go test ./..." "$workflow"
grep -qF "for script in tests/*.sh" "$workflow"
grep -qF 'bash "$script"' "$workflow"
for gate in "$workflow" "$ci_workflow"; do
    grep -qF '::group::$script' "$gate"
    grep -qF '::endgroup::' "$gate"
done

awk '
  /^  build:/ { in_build=1; in_release=0 }
  /^  release:/ { in_release=1; in_build=0 }
  /^  [[:alnum:]_-]+:/ && !/^  build:/ && !/^  release:/ { in_build=0; in_release=0 }
  in_build && /needs: test/ { build_needs_test=1 }
  in_release && /- test/ { release_needs_test=1 }
  END { exit !(build_needs_test && release_needs_test) }
' "$workflow"

echo "PASS"
