#!/usr/bin/env bash
# tx-flow.sh — Full transaction lifecycle: build → sign → submit → register → status
#
# Usage: ./scripts/tx-flow.sh <endpoint> <body-json> <skey-path> <tx-type> [instance-id]
#
# Examples:
#   ./scripts/tx-flow.sh /v2/tx/global/user/access-token/mint \
#     '{"alias":"dev1","initiator_data":"addr_test1..."}' \
#     ./payment.skey access_token_mint
#
#   ./scripts/tx-flow.sh /v2/tx/instance/owner/course/create \
#     '{"alias":"dev1","name":"My Course"}' \
#     ./payment.skey course_create <course_id>

set -euo pipefail

if [ $# -lt 4 ]; then
  echo "Usage: $0 <endpoint> <body-json> <skey-path> <tx-type> [instance-id]" >&2
  exit 1
fi

ENDPOINT="$1"
BODY="$2"
SKEY="$3"
TX_TYPE="$4"
INSTANCE_ID="${5:-}"

echo "=== Build ===" >&2
RESULT=$(andamio tx build "$ENDPOINT" --body "$BODY" --output json)
UNSIGNED_TX=$(echo "$RESULT" | jq -r '.unsigned_tx')
echo "Unsigned TX received ($(echo -n "$UNSIGNED_TX" | wc -c | tr -d ' ') hex chars)" >&2

echo "=== Sign ===" >&2
SIGNED=$(andamio tx sign --tx "$UNSIGNED_TX" --skey "$SKEY" --output json)
TX_HASH=$(echo "$SIGNED" | jq -r '.tx_hash')
SIGNED_TX=$(echo "$SIGNED" | jq -r '.signed_tx')

# Always echo hash before submit so it's never lost on partial failure
echo "Transaction hash: $TX_HASH" >&2

echo "=== Submit ===" >&2
andamio tx submit --tx "$SIGNED_TX" --output json

echo "=== Register ===" >&2
REGISTER_ARGS=(--tx-hash "$TX_HASH" --tx-type "$TX_TYPE")
if [ -n "$INSTANCE_ID" ]; then
  REGISTER_ARGS+=(--instance-id "$INSTANCE_ID")
fi
andamio tx register "${REGISTER_ARGS[@]}" --output json

echo "=== Status ===" >&2
andamio tx status "$TX_HASH"
