#!/usr/bin/env bash
# Runs every root shell business test as the same gate used by CI and release.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)

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

for script in "$repo_root"/tests/*.sh; do
    if is_gate_acceptance_script "$script"; then
        continue
    fi
    bash "$script"
done

echo "PASS"
