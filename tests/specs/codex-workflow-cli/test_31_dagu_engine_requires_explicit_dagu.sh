#!/usr/bin/env bash
# 文件功能目的：验证默认 engine 不依赖 Dagu，但显式 --engine dagu 时必须检查 Dagu，不能静默回退默认状态机。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/31-dagu-engine/missing-dagu"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"

BIN="$TMP/wo"
go build -o "$BIN" "$ROOT/cmd/wo"

FAKEBIN="$TMP/fakebin"
HOME_DIR="$TMP/home"
STATE_HOME="$TMP/state"
DAGU_STATE_HOME="$TMP/dagu-state"
mkdir -p "$FAKEBIN" "$HOME_DIR" "$STATE_HOME" "$DAGU_STATE_HOME"

cat > "$FAKEBIN/oz" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  validate) printf '{"valid":true,"errors":[],"warnings":[],"artifacts":{}}\n' ;;
  status)
    if grep -q '\[x\]' docs/changes/demo/task.md; then
      printf '{"change":"demo","status":"ready","tasks":{"total":1,"done":1}}\n'
    else
      printf '{"change":"demo","status":"incomplete","tasks":{"total":1,"done":0}}\n'
    fi
    ;;
  list) printf '{"changes":[{"name":"demo"}]}\n' ;;
  *) printf 'unexpected oz command: %s\n' "$*" >&2; exit 2 ;;
esac
EOF
chmod +x "$FAKEBIN/oz"

cat > "$FAKEBIN/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
repo="$PWD"
while (($#)); do
  case "$1" in
    --cd) repo="$2"; shift 2 ;;
    *) shift ;;
  esac
done
prompt="$(cat)"
count_file="$repo/.fake-codex-count"
count=0
if [[ -f "$count_file" ]]; then
  count="$(cat "$count_file")"
fi
count=$((count + 1))
printf '%s' "$count" > "$count_file"
printf '{"type":"thread.started","thread_id":"thread-%s"}\n' "$count"
if grep -q '^execution$' <<<"$prompt"; then
  printf -- '- [x] implement demo\n' > "$repo/docs/changes/demo/task.md"
elif grep -q '^archive ' <<<"$prompt"; then
  delivery="$(awk '/^archive /{print $2; exit}' <<<"$prompt")"
  mkdir -p "$(dirname "$delivery")"
  printf 'default engine summary\n' > "$delivery"
  mkdir -p "$repo/docs/changes/archive/2026-06-08-demo"
fi
EOF
chmod +x "$FAKEBIN/codex"

setup_repo() {
  local work="$1"
  mkdir -p "$work"
  (
    cd "$work"
    git init -q
    git config user.email test@example.com
    git config user.name Test
    printf 'demo\n' > README.md
    mkdir -p docs/changes/demo
    cat > docs/changes/demo/task.md <<'EOF'
- [ ] implement demo
EOF
    cat > docs/changes/demo/acceptance.json <<'JSON'
{
  "summary": "demo acceptance",
  "required_tests": [
    {
      "id": "contract-demo",
      "source": "change_contract",
      "path": "docs/changes/demo/tests/demo.acceptance.test.ts",
      "command": "true",
      "purpose": "cover demo contract"
    }
  ],
  "required_evidence": []
}
JSON
    cat > wo.yaml <<'YAML'
wo:
  workflow:
    max_review_iterations: 0
    stages:
      execution:
        cli: codex
      archive:
        cli: codex
    validation:
      commands: []
  prompts:
    execution: "{{.Stage}}\n"
    archive: "{{.Stage}} {{.DeliverySummaryPath}}\n"
YAML
    git add .
    git commit -m init >/dev/null
  )
}

export PATH="$FAKEBIN:/usr/bin:/bin"
export HOME="$HOME_DIR"
export XDG_STATE_HOME="$STATE_HOME"

default_work="$TMP/default-engine"
setup_repo "$default_work"
(
  cd "$default_work"
  "$BIN" run --change demo --json > "$RESULT_DIR/default-run.json"
  state="$(find "$STATE_HOME/wo/repos" -path '*/runs/*/state.json' -print | sort | tail -n 1)"
  test -s "$state"
  cp "$state" "$RESULT_DIR/default-state.json"
  grep -q '"status": "done"' "$RESULT_DIR/default-state.json"
  test -s .fake-codex-count
)

dagu_work="$TMP/dagu-missing"
setup_repo "$dagu_work"
(
  cd "$dagu_work"
  export XDG_STATE_HOME="$DAGU_STATE_HOME"
  "$BIN" graph --change demo --format dagu > "$RESULT_DIR/graph-preview.yaml"
  grep -q 'wo node ' "$RESULT_DIR/graph-preview.yaml"
  if find "$DAGU_STATE_HOME/wo/repos" -path '*/runs/*/state.json' -print -quit 2>/dev/null | grep -q .; then
    echo "wo graph --format dagu created a run" >&2
    exit 1
  fi
  set +e
  "$BIN" run --change demo --engine dagu --json > "$RESULT_DIR/dagu-run.json" 2> "$RESULT_DIR/dagu-run.err"
  status=$?
  set -e
  if [[ "$status" -eq 0 ]]; then
    echo "target behavior missing: --engine dagu succeeded without a dagu executable" >&2
    exit 1
  fi
  if [[ -e .fake-codex-count ]]; then
    echo "target behavior missing: --engine dagu fell back to the default agent flow before checking Dagu" >&2
    exit 1
  fi
  if ! grep -Eiq 'dagu|Dagu' "$RESULT_DIR/dagu-run.err" "$RESULT_DIR/dagu-run.json"; then
    echo "Dagu engine missing-CLI failure did not mention Dagu" >&2
    exit 1
  fi
)

echo "contract passed: default engine avoids Dagu; explicit Dagu engine requires Dagu"
