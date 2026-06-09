#!/usr/bin/env bash
# Verifies root shell business tests can run from a non-developer checkout path.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

is_gate_acceptance_script() {
    # Returns success when the script is a gate self-test, not a business test.
    case "$(basename "$1")" in
        *test_all_root_business_tests_pass.sh | \
        *test_ci_shell_tests_are_portable.sh | \
        *test_release_workflow_runs_business_tests.sh)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

portable_repo="$tmp/portable-wo"
mkdir -p "$portable_repo"
tar --exclude .git -C "$repo_root" -cf - . | tar -C "$portable_repo" -xf -
cp -R "$repo_root/.git" "$portable_repo/.git"

forbidden_path="/home/zzl/projects"
forbidden_path="$forbidden_path/wo"
if rg -n --glob '!*test_ci_shell_tests_are_portable.sh' "$forbidden_path" "$portable_repo/tests" "$portable_repo/.github"; then
    echo "FAIL: shell tests or workflows still reference the developer checkout path" >&2
    exit 1
fi

cd "$portable_repo"
for script in tests/*.sh; do
    if is_gate_acceptance_script "$script"; then
        continue
    fi
    bash "$script"
done

echo "PASS"
