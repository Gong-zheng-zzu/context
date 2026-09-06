#!/usr/bin/env bash
# Run exactly one evaluator against one temporary runtime configuration.
# Usage: ./run_configured_eval.sh <config_name> <eval_command>

set -Eeuo pipefail

if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <config_name> <eval_command>" >&2
    exit 2
fi

CONFIG_NAME=$1
EVAL_COMMAND=$2
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUNTIME_ENV_DIR="$PROJECT_ROOT/experiments/runtime_env"
CONFIG_ENV_FILE="$RUNTIME_ENV_DIR/${CONFIG_NAME}.env"
TARGET_ENV_FILE="$PROJECT_ROOT/config/.env"
RESULT_DIR="$PROJECT_ROOT/experiments/results/raw"
RUNS_DIR="$PROJECT_ROOT/experiments/results/runs"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)_${CONFIG_NAME}_$$"
RUN_DIR="$RUNS_DIR/$RUN_ID"
BACKUP_ENV_FILE="$RUN_DIR/base.env.backup"
MERGED_ENV_FILE="$RUN_DIR/effective.env"
RUN_MARKER="$RUN_DIR/evaluation.started"
RUN_LOG="$RUN_DIR/evaluation.log"
RUN_MANIFEST="$RUN_DIR/manifest.json"
STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
OUTCOME="initializing"
ERROR_MESSAGE=""
SMOKE_STATUS="not_run"
EVAL_EXIT_CODE=""
RESULT_FILE=""
EFFECTIVE_ENV_HASH=""

mkdir -p "$RUN_DIR" "$RESULT_DIR"

if [[ ! -f "$TARGET_ENV_FILE" || ! -f "$CONFIG_ENV_FILE" ]]; then
    echo "Required environment file is missing." >&2
    exit 2
fi

BASE_ENV_HASH="$(sha256sum "$TARGET_ENV_FILE" | awk '{print $1}')"
OVERLAY_ENV_HASH="$(sha256sum "$CONFIG_ENV_FILE" | awk '{print $1}')"
DATASET_TREE_HASH="$(python3 - "$PROJECT_ROOT/experiments/datasets" <<'PY'
import hashlib
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
digest = hashlib.sha256()
for path in sorted(candidate for candidate in root.rglob('*') if candidate.is_file()):
    digest.update(path.relative_to(root).as_posix().encode('utf-8'))
    digest.update(b'\0')
    digest.update(hashlib.sha256(path.read_bytes()).digest())
print(digest.hexdigest())
PY
)"
COMMAND_HASH="$(printf '%s' "$EVAL_COMMAND" | sha256sum | awk '{print $1}')"
PYTHON_VERSION="$(python3 --version 2>&1 || true)"
DOCKER_COMPOSE_VERSION="$(docker-compose version 2>&1 || true)"
PLATFORM="$(uname -srm 2>/dev/null || true)"

export RUN_ID STARTED_AT OUTCOME ERROR_MESSAGE SMOKE_STATUS EVAL_EXIT_CODE RESULT_FILE
export CONFIG_NAME EVAL_COMMAND COMMAND_HASH BASE_ENV_HASH OVERLAY_ENV_HASH EFFECTIVE_ENV_HASH
export DATASET_TREE_HASH PYTHON_VERSION DOCKER_COMPOSE_VERSION PLATFORM RUN_LOG RUN_MANIFEST

cp "$TARGET_ENV_FILE" "$BACKUP_ENV_FILE"

write_manifest() {
    local ended_at
    ended_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    export ENDED_AT="$ended_at"
    python3 - "$RUN_MANIFEST" <<'PY'
import json
import os
from pathlib import Path

payload = {
    "schema_version": 1,
    "run_id": os.environ["RUN_ID"],
    "started_at": os.environ["STARTED_AT"],
    "ended_at": os.environ["ENDED_AT"],
    "outcome": os.environ["OUTCOME"],
    "error": os.environ["ERROR_MESSAGE"],
    "smoke_status": os.environ["SMOKE_STATUS"],
    "evaluator_exit_code": os.environ["EVAL_EXIT_CODE"],
    "result_file": os.environ["RESULT_FILE"],
    "command": os.environ["EVAL_COMMAND"],
    "command_sha256": os.environ["COMMAND_HASH"],
    "configuration": {
        "name": os.environ["CONFIG_NAME"],
        "base_env_sha256": os.environ["BASE_ENV_HASH"],
        "overlay_env_sha256": os.environ["OVERLAY_ENV_HASH"],
        "effective_env_sha256": os.environ["EFFECTIVE_ENV_HASH"],
    },
    "data": {"datasets_tree_sha256": os.environ["DATASET_TREE_HASH"]},
    "environment": {
        "python": os.environ["PYTHON_VERSION"],
        "docker_compose": os.environ["DOCKER_COMPOSE_VERSION"],
        "platform": os.environ["PLATFORM"],
        "health_url": "http://localhost:8088/health",
    },
    "artifacts": {"evaluation_log": os.environ["RUN_LOG"]},
}
Path(os.environ["RUN_MANIFEST"]).write_text(json.dumps(payload, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
PY
}

restore_environment() {
    local restore_hash
    if [[ -f "$BACKUP_ENV_FILE" ]]; then
        cp "$BACKUP_ENV_FILE" "$TARGET_ENV_FILE"
        restore_hash="$(sha256sum "$TARGET_ENV_FILE" | awk '{print $1}')"
        if [[ "$restore_hash" != "$BASE_ENV_HASH" ]]; then
            OUTCOME="restore_failed"
            ERROR_MESSAGE="base environment hash changed during restoration"
        fi
    fi
    write_manifest
    rm -f "$BACKUP_ENV_FILE" "$MERGED_ENV_FILE"
    cd "$PROJECT_ROOT"
    docker-compose up -d --force-recreate --no-deps context-keeper >/dev/null 2>&1 || true
}
record_unexpected_error() {
    local line=$1
    local status=$2
    if [[ "$OUTCOME" == "initializing" || "$OUTCOME" == "succeeded" ]]; then
        OUTCOME="runner_error"
        ERROR_MESSAGE="unexpected runner failure at line $line (exit $status)"
    fi
    if [[ -z "$EVAL_EXIT_CODE" ]]; then
        EVAL_EXIT_CODE="$status"
    fi
}
trap 'record_unexpected_error "$LINENO" "$?"' ERR
trap restore_environment EXIT

python3 - "$TARGET_ENV_FILE" "$CONFIG_ENV_FILE" "$MERGED_ENV_FILE" <<'PY'
import pathlib
import sys

base_path, overlay_path, output_path = map(pathlib.Path, sys.argv[1:])

def key_of(line):
    stripped = line.strip()
    if not stripped or stripped.startswith('#') or '=' not in stripped:
        return None
    return stripped.split('=', 1)[0].strip()

overlay = {}
for raw in overlay_path.read_text(encoding='utf-8').splitlines():
    key = key_of(raw)
    if key:
        overlay[key] = raw

merged, seen = [], set()
for raw in base_path.read_text(encoding='utf-8').splitlines():
    key = key_of(raw)
    if key in overlay:
        merged.append(overlay[key])
        seen.add(key)
    else:
        merged.append(raw)
for raw in overlay_path.read_text(encoding='utf-8').splitlines():
    key = key_of(raw)
    if key and key not in seen:
        merged.append(raw)
        seen.add(key)
output_path.write_text('\n'.join(merged) + '\n', encoding='utf-8')
PY

EFFECTIVE_ENV_HASH="$(sha256sum "$MERGED_ENV_FILE" | awk '{print $1}')"
cp "$MERGED_ENV_FILE" "$TARGET_ENV_FILE"
cd "$PROJECT_ROOT"
docker-compose up -d --force-recreate --no-deps context-keeper

for _ in $(seq 1 60); do
    if curl -fsS http://localhost:8088/health >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
if ! curl -fsS http://localhost:8088/health >/dev/null 2>&1; then
    OUTCOME="health_check_failed"
    ERROR_MESSAGE="service did not become healthy within 60 seconds"
    exit 1
fi

cd "$PROJECT_ROOT/experiments/scripts"
if python3 smoke_test.py --quick >"$RUN_DIR/smoke.log" 2>&1; then
    SMOKE_STATUS="passed"
else
    SMOKE_STATUS="failed"
    OUTCOME="smoke_failed"
    ERROR_MESSAGE="smoke test failed; see $RUN_DIR/smoke.log"
    exit 1
fi

touch "$RUN_MARKER"
export EVAL_RUNTIME_CONFIG_NAME="$CONFIG_NAME"
export EVAL_RUNTIME_CONFIG_FILE="$CONFIG_ENV_FILE"
export EVAL_RUNTIME_CONFIG_HASH="$EFFECTIVE_ENV_HASH"
export EVAL_SERVICE_START_TIME="$STARTED_AT"
export EVAL_RUN_ID="$RUN_ID"
export EVAL_DATASETS_TREE_HASH="$DATASET_TREE_HASH"

set +e
eval "$EVAL_COMMAND" 2>&1 | tee "$RUN_LOG"
EVAL_EXIT_CODE=${PIPESTATUS[0]}
set -e

mapfile -t NEW_RESULTS < <(find "$RESULT_DIR" -maxdepth 1 -type f -name '*.json' -newer "$RUN_MARKER" -print | sort)
if [[ ${#NEW_RESULTS[@]} -ne 1 ]]; then
    OUTCOME="result_artifact_invalid"
    ERROR_MESSAGE="expected exactly one new raw JSON result, found ${#NEW_RESULTS[@]}"
    exit 1
fi
RESULT_FILE="${NEW_RESULTS[0]}"

if [[ "$EVAL_EXIT_CODE" -ne 0 ]]; then
    OUTCOME="evaluator_failed"
    ERROR_MESSAGE="evaluator exited with code $EVAL_EXIT_CODE"
else
    OUTCOME="succeeded"
fi

export RUN_MANIFEST
python3 - "$RESULT_FILE" <<'PY'
import json
import os
import sys
from pathlib import Path

result_path = Path(sys.argv[1])
manifest_path = Path(os.environ["RUN_MANIFEST"])
outcome = os.environ["OUTCOME"]
error = os.environ["ERROR_MESSAGE"]
try:
    result = json.loads(result_path.read_text(encoding="utf-8"))
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"result is unreadable: {exc}")
if not isinstance(result, dict):
    raise SystemExit("result JSON must be an object")

result["runner_provenance"] = {
    "run_id": os.environ["RUN_ID"],
    "manifest": str(manifest_path),
    "outcome": outcome,
    "report_eligible": outcome == "succeeded",
    "error": error or None,
    "configuration": {
        "name": os.environ["CONFIG_NAME"],
        "base_env_sha256": os.environ["BASE_ENV_HASH"],
        "overlay_env_sha256": os.environ["OVERLAY_ENV_HASH"],
        "effective_env_sha256": os.environ["EFFECTIVE_ENV_HASH"],
    },
    "data": {"datasets_tree_sha256": os.environ["DATASET_TREE_HASH"]},
    "environment": {
        "python": os.environ["PYTHON_VERSION"],
        "docker_compose": os.environ["DOCKER_COMPOSE_VERSION"],
        "platform": os.environ["PLATFORM"],
    },
    "evaluator_exit_code": int(os.environ["EVAL_EXIT_CODE"]),
    "smoke_status": os.environ["SMOKE_STATUS"],
}
result_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

if [[ "$EVAL_EXIT_CODE" -ne 0 ]]; then
    exit "$EVAL_EXIT_CODE"
fi

echo "Eligible result: $RESULT_FILE"
