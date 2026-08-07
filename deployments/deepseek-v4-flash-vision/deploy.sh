#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly COLEMAN="dell@coleman.trout-rooster.ts.net"
readonly REMOTE_DIR="/home/dell/.local/share/deepseek-vision-gateway"
readonly REMOTE_SECRET_DIR="/home/dell/.config/deepseek-vision"
readonly LOCAL_GATEWAY_KEY_FILE="/home/sbull/.config/deepseek-spark-vision.key"
readonly LOCAL_DEEPSEEK_KEY_FILE="/home/sbull/.config/deepseek-spark.key"
readonly MODEL_REPO="/home/dell/.cache/huggingface/hub/models--Qwen--Qwen3-VL-30B-A3B-Instruct-FP8"
readonly MODEL_REVISION="d9748a51ae66354c4dad665aab2c71f26cf2c8cd"
readonly VLLM_IMAGE="nvcr.io/nvidia/vllm@sha256:95c498a475142c20c989c65e5d223348c09fed83ba17ddf44f117610c0bd3268"
readonly RUN_STAMP="$(date -u +%Y%m%dT%H%M%SZ)-$$"
readonly REMOTE_NEXT="${REMOTE_DIR}.next-${RUN_STAMP}"
readonly REMOTE_SECRET_NEXT="${REMOTE_SECRET_DIR}.next-${RUN_STAMP}"
readonly REMOTE_BACKUP="${REMOTE_DIR}.backup-${RUN_STAMP}"
readonly REMOTE_SECRET_BACKUP="${REMOTE_SECRET_DIR}.backup-${RUN_STAMP}"
rollback_armed=false

remote() {
  tailscale ssh "${COLEMAN}" "$@"
}

cleanup_remote_transients() {
  remote "bash -s -- '${REMOTE_NEXT}' '${REMOTE_SECRET_NEXT}' '${REMOTE_BACKUP}' '${REMOTE_SECRET_BACKUP}' '${REMOTE_DIR}.failed-${RUN_STAMP}' '${REMOTE_SECRET_DIR}.failed-${RUN_STAMP}'" <<'REMOTE_CLEANUP'
set -Eeuo pipefail

remove_tree() {
  local path="$1"
  if [[ -d "${path}" ]]; then
    chmod -R u+rwX "${path}"
    find "${path}" -mindepth 1 -delete
    rmdir "${path}"
  fi
}

for path in "$@"; do
  remove_tree "${path}"
done
REMOTE_CLEANUP
}

rollback() {
  echo "Rolling back Coleman vision deployment..." >&2
  remote "bash -s -- '${REMOTE_DIR}' '${REMOTE_SECRET_DIR}' '${REMOTE_BACKUP}' '${REMOTE_SECRET_BACKUP}' '${RUN_STAMP}'" <<'REMOTE_ROLLBACK'
set -Eeuo pipefail
deploy_dir="$1"
secret_dir="$2"
backup_dir="$3"
secret_backup_dir="$4"
stamp="$5"

remove_tree() {
  local path="$1"
  if [[ -d "${path}" ]]; then
    chmod -R u+rwX "${path}"
    find "${path}" -mindepth 1 -delete
    rmdir "${path}"
  fi
}

is_current_candidate() {
  local path="$1"
  [[ -f "${path}/.deployment-stamp" ]] \
    && [[ "$(<"${path}/.deployment-stamp")" == "${stamp}" ]]
}

if [[ -d "${secret_backup_dir}" ]]; then
  if [[ -e "${secret_dir}" ]]; then
    mv "${secret_dir}" "${secret_dir}.failed-${stamp}"
  fi
  mv "${secret_backup_dir}" "${secret_dir}"
fi
if [[ -d "${backup_dir}" ]]; then
  if [[ -f "${deploy_dir}/docker-compose.yml" ]]; then
    docker compose -f "${deploy_dir}/docker-compose.yml" down --remove-orphans || true
  fi
  if [[ -e "${deploy_dir}" ]]; then
    mv "${deploy_dir}" "${deploy_dir}.failed-${stamp}"
  fi
  mv "${backup_dir}" "${deploy_dir}"
  docker compose -f "${deploy_dir}/docker-compose.yml" up -d --remove-orphans
elif [[ -f "${deploy_dir}/docker-compose.yml" ]]; then
  if is_current_candidate "${deploy_dir}"; then
    docker compose -f "${deploy_dir}/docker-compose.yml" down --remove-orphans || true
    remove_tree "${deploy_dir}"
  else
    docker compose -f "${deploy_dir}/docker-compose.yml" up -d --remove-orphans
  fi
fi
if [[ ! -d "${secret_backup_dir}" ]] && is_current_candidate "${secret_dir}"; then
  remove_tree "${secret_dir}"
fi
REMOTE_ROLLBACK
}

on_exit() {
  status=$?
  trap - EXIT
  cleanup_allowed=true
  if [[ "${rollback_armed}" == "true" ]]; then
    if ! rollback; then
      status=1
      cleanup_allowed=false
      echo "Rollback failed; preserving run-stamped backup paths for manual recovery." >&2
    fi
  fi
  if [[ "${cleanup_allowed}" == "true" ]]; then
    cleanup_remote_transients || status=1
  fi
  exit "${status}"
}
trap on_exit EXIT

for key_file in "${LOCAL_GATEWAY_KEY_FILE}" "${LOCAL_DEEPSEEK_KEY_FILE}"; do
  if [[ ! -r "${key_file}" ]]; then
    echo "Missing local API key file: ${key_file}" >&2
    exit 1
  fi
done
gateway_api_key="$(tr -d '\r\n' <"${LOCAL_GATEWAY_KEY_FILE}")"
deepseek_api_key="$(tr -d '\r\n' <"${LOCAL_DEEPSEEK_KEY_FILE}")"
if [[ ! "${gateway_api_key}" =~ ^[A-Za-z0-9._-]+$ || ! "${deepseek_api_key}" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "An API key contains unsupported characters" >&2
  exit 1
fi
if [[ "${gateway_api_key}" == "${deepseek_api_key}" ]]; then
  echo "Gateway and DeepSeek API keys must be distinct" >&2
  exit 1
fi

echo "Checking Coleman identity, model cache, image, and coexistence workload..."
remote "test \"\$(tailscale ip -4)\" = 100.104.98.40"
remote "test -f '${MODEL_REPO}/snapshots/${MODEL_REVISION}/model.safetensors.index.json'"
remote "test -z \"\$(find -L '${MODEL_REPO}/snapshots/${MODEL_REVISION}' -type l -print -quit)\""
remote "docker image inspect '${VLLM_IMAGE}' >/dev/null"
remote "systemctl --user is-active --quiet qwen36-27b-mtp.service"
llama_before="$(remote 'systemctl --user show qwen36-27b-mtp.service -p ActiveState -p MainPID --value 2>/dev/null | tr "\n" "|"')"

echo "Staging versioned deployment files and separate protected API keys..."
remote "install -d -m 0750 '${REMOTE_NEXT}' '${REMOTE_SECRET_NEXT}'"
tar -C "${SCRIPT_DIR}" \
  --exclude TODO.md \
  --exclude README.md \
  -cf - . | remote "tar -xf - -C '${REMOTE_NEXT}'"
remote "printf '%s\n' 'MOONBRIDGE_RELEASE_TAG=c9ae8a8-${RUN_STAMP}' >'${REMOTE_NEXT}/.env'; printf '%s\n' '${RUN_STAMP}' >'${REMOTE_NEXT}/.deployment-stamp'; chmod 0640 '${REMOTE_NEXT}/.env' '${REMOTE_NEXT}/.deployment-stamp'"
printf '%s\n' "${gateway_api_key}" | remote "umask 077; tee '${REMOTE_SECRET_NEXT}/gateway-api-key' >/dev/null"
printf '%s\n' "${deepseek_api_key}" | remote "umask 077; tee '${REMOTE_SECRET_NEXT}/deepseek-api-key' >/dev/null"
unset gateway_api_key deepseek_api_key
remote "umask 077; openssl rand -hex 32 >'${REMOTE_SECRET_NEXT}/qwen-api-key'; printf '%s\n' '${RUN_STAMP}' >'${REMOTE_SECRET_NEXT}/.deployment-stamp'; chmod 0600 '${REMOTE_SECRET_NEXT}/'* '${REMOTE_SECRET_NEXT}/.deployment-stamp'"

echo "Validating Compose and building pinned Moon Bridge..."
remote "cd '${REMOTE_NEXT}' && docker compose config --quiet"
remote "cd '${REMOTE_NEXT}' && docker compose build --pull=false moonbridge"

echo "Activating Coleman vision services with recoverable rollback..."
rollback_armed=true
remote "bash -s -- '${REMOTE_DIR}' '${REMOTE_NEXT}' '${REMOTE_SECRET_DIR}' '${REMOTE_SECRET_NEXT}' '${REMOTE_BACKUP}' '${REMOTE_SECRET_BACKUP}'" <<'REMOTE_ACTIVATE'
set -Eeuo pipefail
deploy_dir="$1"
next_dir="$2"
secret_dir="$3"
secret_next_dir="$4"
backup_dir="$5"
secret_backup_dir="$6"

if [[ -e "${backup_dir}" || -e "${secret_backup_dir}" ]]; then
  echo "Refusing activation because a target backup path already exists" >&2
  exit 1
fi

if [[ -d "${deploy_dir}" ]]; then
  cd "${deploy_dir}"
  docker compose down --remove-orphans
  mv "${deploy_dir}" "${backup_dir}"
fi
if [[ -d "${secret_dir}" ]]; then
  mv "${secret_dir}" "${secret_backup_dir}"
fi

mv "${secret_next_dir}" "${secret_dir}"
mv "${next_dir}" "${deploy_dir}"
cd "${deploy_dir}"
docker compose up -d --remove-orphans
REMOTE_ACTIVATE

echo "Waiting for Qwen3-VL and Moon Bridge health..."
for attempt in $(seq 1 120); do
  status="$(remote "docker inspect deepseek-vision-qwen3-vl deepseek-vision-moonbridge --format '{{.Name}}={{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' 2>/dev/null" || true)"
  if grep -q 'deepseek-vision-qwen3-vl=healthy' <<<"${status}" \
      && grep -q 'deepseek-vision-moonbridge=healthy' <<<"${status}"; then
    break
  fi
  if (( attempt == 120 )); then
    echo "Vision services did not become healthy" >&2
    remote "cd '${REMOTE_DIR}' && docker compose ps && docker compose logs --tail=200 qwen3-vl moonbridge"
    exit 1
  fi
  sleep 10
done

llama_after="$(remote 'systemctl --user show qwen36-27b-mtp.service -p ActiveState -p MainPID --value 2>/dev/null | tr "\n" "|"')"
if [[ "${llama_before}" != "${llama_after}" ]]; then
  echo "Existing qwen36-27b-mtp service identity changed unexpectedly" >&2
  exit 1
fi

"${SCRIPT_DIR}/verify.sh"
rollback_armed=false

echo "Coleman vision gateway is live at http://coleman.trout-rooster.ts.net:38440/v1"
