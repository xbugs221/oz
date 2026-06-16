#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 对外暴露 run-acceptance runner 命令和机器能力声明。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/37-执行验收合同测试并汇总结果/surface"
LOG="$RESULT_DIR/contract.log"
TMP="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP"
}

fail() {
  printf 'FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

note() {
  printf '%s\n' "$*" | tee -a "$LOG"
}

trap cleanup EXIT
rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

note "build real oz binary"
OZ_BIN="$TMP/oz"
(cd "$ROOT" && go build -o "$OZ_BIN" ./cmd/oz) >>"$LOG" 2>&1

note "check oz flow help exposes run-acceptance"
"$OZ_BIN" flow --help >"$RESULT_DIR/help.txt" 2>"$RESULT_DIR/help.err" || fail "oz flow --help failed"
grep -q 'oz flow run-acceptance --change <change-name> --json' "$RESULT_DIR/help.txt" || fail "help missing run-acceptance JSON command"

note "check runner contract exposes run-acceptance capability"
"$OZ_BIN" flow contract --json >"$RESULT_DIR/contract.json" 2>"$RESULT_DIR/contract.err" || fail "contract --json failed"
python3 - "$RESULT_DIR/contract.json" <<'PY' >>"$LOG" 2>&1
import json
import sys

path = sys.argv[1]
payload = json.loads(open(path, encoding="utf-8").read())
if payload.get("json") is not True:
    raise SystemExit("contract json flag must be true")
capabilities = payload.get("capabilities") or []
if "run-acceptance" not in capabilities:
    raise SystemExit(f"missing run-acceptance capability: {capabilities!r}")
print("run-acceptance capability is present")
PY

note "surface contract passed; evidence: test-results/37-执行验收合同测试并汇总结果/surface/contract.log"

