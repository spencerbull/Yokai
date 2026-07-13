#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../deploy/gcp/setup-common.sh
source "${REPO_ROOT}/deploy/gcp/setup-common.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

gcloud() { return 1; }
if verify_spark_billing test-project 0 2>/dev/null; then
  fail 'billing lookup failure was accepted'
fi

gcloud() { printf 'True\n'; }
if verify_spark_billing test-project 0 2>/dev/null; then
  fail 'billed project was accepted without override'
fi
verify_spark_billing test-project 1 || fail 'explicit billed-project override was rejected'

gcloud() { printf 'False\n'; }
verify_spark_billing test-project 0 || fail 'unbilled project was rejected'

apps='{"result":[{"appId":"unrelated","displayName":"Other App"},{"appId":"wanted","displayName":"Yokai CLI development"}]}'
[[ "$(firebase_app_id "${apps}" 'Yokai CLI development')" == "wanted" ]] || fail 'selected the wrong Firebase app'

duplicates='{"result":[{"appId":"one","displayName":"Yokai CLI development"},{"appId":"two","displayName":"Yokai CLI development"}]}'
if firebase_app_id "${duplicates}" 'Yokai CLI development' >/dev/null 2>&1; then
  fail 'duplicate Firebase apps were accepted'
fi

printf 'setup-common tests passed\n'
