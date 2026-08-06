#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel)"
readonly BESKAR="dell@beskar"
readonly KYBER="dell@kyber"
readonly REMOTE_MONITOR_DIR="/home/dell/.local/share/yokai/monitoring"
readonly REMOTE_EXPORTER_DIR="/home/dell/.local/share/yokai/fleet-exporters"
readonly MODEL_CONTAINER="deepseek-v4-flash-vllm-dspark-1"
readonly SSH_OPTIONS=(-o BatchMode=yes -o ConnectTimeout=10)
readonly RUN_STAMP="$(date -u +%Y%m%dT%H%M%SZ)-$$"
readonly REMOTE_NEXT_DIR="${REMOTE_MONITOR_DIR}.next-${RUN_STAMP}"
readonly REMOTE_EXPORTER_NEXT="${REMOTE_EXPORTER_DIR}.next-${RUN_STAMP}"

stage_dir="$(mktemp -d)"
cleanup() {
  chmod -R u+rwX "${stage_dir}" 2>/dev/null || true
  find "${stage_dir}" -mindepth 1 -delete 2>/dev/null || true
  rmdir "${stage_dir}" 2>/dev/null || true
}
trap cleanup EXIT

model_identity() {
  local host="$1"
  ssh "${SSH_OPTIONS[@]}" "${host}" \
    "docker inspect '${MODEL_CONTAINER}' --format '{{.Id}}|{{.State.StartedAt}}|{{.State.Running}}'"
}

echo "Recording DeepSeek container identities..."
beskar_model_before="$(model_identity "${BESKAR}")"
kyber_model_before="$(model_identity "${KYBER}")"

if [[ "${beskar_model_before}" != *"|true" || "${kyber_model_before}" != *"|true" ]]; then
  echo "Refusing to deploy: both DeepSeek workers must already be running." >&2
  exit 1
fi

echo "Building and validating the provisioned dashboard..."
mkdir -p \
  "${stage_dir}/grafana/dashboards" \
  "${stage_dir}/grafana/provisioning" \
  "${stage_dir}/prometheus" \
  "${stage_dir}/secrets"
cp "${SCRIPT_DIR}/docker-compose.yml" "${stage_dir}/docker-compose.yml"
cp -R "${SCRIPT_DIR}/grafana/provisioning/." "${stage_dir}/grafana/provisioning/"
cp -R "${SCRIPT_DIR}/prometheus/." "${stage_dir}/prometheus/"
(
  cd "${REPO_ROOT}"
  go run ./deployments/deepseek-v4-flash-observability/cmd/dashboard
) >"${stage_dir}/grafana/dashboards/deepseek-v4-flash.json"
jq -e '.uid == "deepseek-v4-flash" and (.panels | length >= 50)' \
  "${stage_dir}/grafana/dashboards/deepseek-v4-flash.json" >/dev/null

echo "Collecting existing credentials into a private temporary directory..."
ssh "${SSH_OPTIONS[@]}" "${BESKAR}" \
  'jq -er .token /home/dell/.config/yokai/agent.json' \
  >"${stage_dir}/secrets/beskar-agent-token"
ssh "${SSH_OPTIONS[@]}" "${KYBER}" \
  'jq -er .token /home/dell/.config/yokai/agent.json' \
  >"${stage_dir}/secrets/kyber-agent-token"
ssh "${SSH_OPTIONS[@]}" "${KYBER}" \
  'cd /home/dell/src/github.com/spencerbull/dgx-spark-deepseek-v4-flash && set -a && . ./.env && set +a && test -n "${VLLM_API_KEY}" && printf "%s\n" "${VLLM_API_KEY}"' \
  >"${stage_dir}/secrets/vllm-api-key"
chmod 0700 "${stage_dir}/secrets"
chmod 0600 "${stage_dir}/secrets/"*

echo "Staging Kyber node exporter..."
ssh "${SSH_OPTIONS[@]}" "${KYBER}" \
  "install -d -m 0750 '${REMOTE_EXPORTER_NEXT}'"
scp "${SSH_OPTIONS[@]}" "${SCRIPT_DIR}/kyber-exporter.compose.yml" \
  "${KYBER}:${REMOTE_EXPORTER_NEXT}/docker-compose.yml" >/dev/null
ssh "${SSH_OPTIONS[@]}" "${KYBER}" \
  "cd '${REMOTE_EXPORTER_NEXT}' && docker compose config --quiet"

echo "Staging the Beskar observability hub..."
ssh "${SSH_OPTIONS[@]}" "${BESKAR}" \
  "install -d -m 0750 '${REMOTE_NEXT_DIR}'"
scp -r "${SSH_OPTIONS[@]}" "${stage_dir}/." "${BESKAR}:${REMOTE_NEXT_DIR}/" >/dev/null
ssh "${SSH_OPTIONS[@]}" "${BESKAR}" \
  "chmod 0700 '${REMOTE_NEXT_DIR}/secrets' && chmod 0600 '${REMOTE_NEXT_DIR}/secrets/'* && cd '${REMOTE_NEXT_DIR}' && docker compose config --quiet"
ssh "${SSH_OPTIONS[@]}" "${BESKAR}" \
  "docker run --rm --user 0:0 --entrypoint promtool -v '${REMOTE_NEXT_DIR}/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro' -v '${REMOTE_NEXT_DIR}/prometheus/rules.yml:/etc/prometheus/rules.yml:ro' -v '${REMOTE_NEXT_DIR}/secrets:/etc/prometheus/secrets:ro' prom/prometheus@sha256:e4254400b85610324913f0dc4acf92603d9984e7519414c5a12811aa6146acc3 check config /etc/prometheus/prometheus.yml"

echo "Starting Kyber exporters without touching the model container..."
ssh "${SSH_OPTIONS[@]}" "${KYBER}" bash -s -- \
  "${REMOTE_EXPORTER_DIR}" "${REMOTE_EXPORTER_NEXT}" <<'REMOTE_KYBER'
set -Eeuo pipefail
exporter_dir="$1"
next_dir="$2"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="${exporter_dir}.backup-${stamp}"
if [[ -d "${exporter_dir}" ]]; then
  mv "${exporter_dir}" "${backup_dir}"
fi
if ! mv "${next_dir}" "${exporter_dir}"; then
  [[ -d "${backup_dir}" ]] && mv "${backup_dir}" "${exporter_dir}"
  exit 1
fi
cd "${exporter_dir}"
if ! docker compose up -d --remove-orphans --force-recreate; then
  cd /home/dell
  mv "${exporter_dir}" "${exporter_dir}.failed-${stamp}"
  if [[ -d "${backup_dir}" ]]; then
    mv "${backup_dir}" "${exporter_dir}"
    cd "${exporter_dir}"
    docker compose up -d --force-recreate
  fi
  exit 1
fi
systemctl --user start yokai-agent.service
systemctl --user is-active --quiet yokai-agent.service
REMOTE_KYBER

echo "Replacing the Beskar monitoring files with a recoverable backup..."
ssh "${SSH_OPTIONS[@]}" "${BESKAR}" bash -s -- \
  "${REMOTE_MONITOR_DIR}" "${REMOTE_NEXT_DIR}" <<'REMOTE_BESKAR'
set -Eeuo pipefail
monitor_dir="$1"
next_dir="$2"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="${monitor_dir}.backup-${stamp}"
if [[ -d "${monitor_dir}" ]]; then
  mv "${monitor_dir}" "${backup_dir}"
fi
if ! mv "${next_dir}" "${monitor_dir}"; then
  [[ -d "${backup_dir}" ]] && mv "${backup_dir}" "${monitor_dir}"
  exit 1
fi
cd "${monitor_dir}"
if ! docker compose up -d --remove-orphans --force-recreate; then
  cd /home/dell
  mv "${monitor_dir}" "${monitor_dir}.failed-${stamp}"
  if [[ -d "${backup_dir}" ]]; then
    mv "${backup_dir}" "${monitor_dir}"
    cd "${monitor_dir}"
    docker compose up -d --force-recreate
  fi
  exit 1
fi
REMOTE_BESKAR

echo "Verifying the inference workers were not recreated..."
beskar_model_after="$(model_identity "${BESKAR}")"
kyber_model_after="$(model_identity "${KYBER}")"
if [[ "${beskar_model_before}" != "${beskar_model_after}" || "${kyber_model_before}" != "${kyber_model_after}" ]]; then
  echo "DeepSeek container identity changed unexpectedly; stop and inspect immediately." >&2
  exit 1
fi

echo "Running live Prometheus and Grafana checks..."
"${SCRIPT_DIR}/verify.sh"

echo "Observability is live at http://beskar:3001/d/deepseek-v4-flash"
