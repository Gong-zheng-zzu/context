#!/usr/bin/env bash
# Execute a reproducible full evaluation only after explicit operator approval.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$SCRIPT_DIR/run_configured_eval.sh"
REPORT_BUILDER="$SCRIPT_DIR/report_builder.py"
CONFIGS_TEXT="${EVAL_CONFIGS:-baseline_vanilla_llm baseline_naive_rag baseline_rag_with_filter full_system}"
read -r -a CONFIGS <<< "$CONFIGS_TEXT"

if [[ "${EVAL_ALLOW_FULL_RUN:-}" != "true" ]]; then
    echo "Refusing to run a full evaluation without EVAL_ALLOW_FULL_RUN=true." >&2
    exit 2
fi
if [[ "${EVAL_ALLOW_RETRIEVAL_COMPARISON:-}" != "true" ]]; then
    echo "Retrieval comparison is blocked until trace evidence is approved; set EVAL_ALLOW_RETRIEVAL_COMPARISON=true only after the release gate passes." >&2
    exit 2
fi
if [[ "${EVAL_ALLOW_DESTRUCTIVE_UNLEARNING:-}" != "true" ]]; then
    echo "Machine unlearning is destructive; set EVAL_ALLOW_DESTRUCTIVE_UNLEARNING=true only for the isolated evaluation user." >&2
    exit 2
fi
if [[ "${EVAL_UNLEARNING_USER_ID:-}" != "eval_user_001" ]]; then
    echo "EVAL_UNLEARNING_USER_ID must be eval_user_001 for this runner." >&2
    exit 2
fi

for config in "${CONFIGS[@]}"; do
    "$RUNNER" "$config" "python3 security_eval.py --samples 600 --config-label $config --config-evidence \$EVAL_RUNTIME_CONFIG_HASH"
    "$RUNNER" "$config" "python3 causal_eval.py --samples 20 --run-label $config"
    "$RUNNER" "$config" "python3 retrieval_eval.py --queries 30 --configs $config --sample-mode all"
done

"$RUNNER" "full_system" "python3 unlearning_eval.py --runtime-label full_system"

python3 "$REPORT_BUILDER" \
    --mode full \
    --require security causal retrieval unlearning \
    --output "$SCRIPT_DIR/../results/reports/full_report.html"

echo "Report published: $SCRIPT_DIR/../results/reports/full_report.html"
