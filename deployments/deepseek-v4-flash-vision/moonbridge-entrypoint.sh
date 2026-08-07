#!/bin/sh
set -eu

readonly template_file="/opt/moonbridge/config.yml.template"
readonly runtime_config="/run/moonbridge/config.yml"

read_secret() {
  secret_file="$1"
  secret_name="$2"
  if [ ! -r "${secret_file}" ]; then
    echo "Moon Bridge ${secret_name} is unavailable" >&2
    exit 1
  fi
  secret_value="$(tr -d '\r\n' <"${secret_file}")"
  case "${secret_value}" in
    ''|*[!A-Za-z0-9._-]*)
      echo "Moon Bridge ${secret_name} contains unsupported characters" >&2
      exit 1
      ;;
  esac
  printf '%s' "${secret_value}"
}

export MOONBRIDGE_GATEWAY_API_KEY
export MOONBRIDGE_DEEPSEEK_API_KEY
export MOONBRIDGE_QWEN_API_KEY
MOONBRIDGE_GATEWAY_API_KEY="$(read_secret /run/secrets/gateway-api-key 'gateway API key')"
MOONBRIDGE_DEEPSEEK_API_KEY="$(read_secret /run/secrets/deepseek-api-key 'DeepSeek API key')"
MOONBRIDGE_QWEN_API_KEY="$(read_secret /run/secrets/qwen-api-key 'Qwen API key')"
su-exec 65532:65532 sh -c '
  umask 077
  sed \
    -e "s/__GATEWAY_API_KEY__/${MOONBRIDGE_GATEWAY_API_KEY}/g" \
    -e "s/__DEEPSEEK_API_KEY__/${MOONBRIDGE_DEEPSEEK_API_KEY}/g" \
    -e "s/__QWEN_API_KEY__/${MOONBRIDGE_QWEN_API_KEY}/g" \
    "${1}" >"${2}"
' sh "${template_file}" "${runtime_config}"
unset MOONBRIDGE_GATEWAY_API_KEY MOONBRIDGE_DEEPSEEK_API_KEY MOONBRIDGE_QWEN_API_KEY

exec su-exec 65532:65532 /usr/local/bin/moonbridge -config "${runtime_config}"
