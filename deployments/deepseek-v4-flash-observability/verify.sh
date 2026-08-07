#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel)"
readonly PROMETHEUS_URL="http://100.126.187.30:9090"
readonly GRAFANA_URL="http://100.126.187.30:3001"

wait_for_url() {
  local url="$1"
  local attempts="${2:-36}"
  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if curl -fs --max-time 4 "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  echo "Timed out waiting for ${url}" >&2
  return 1
}

prom_query() {
  local query="$1"
  curl -sSG --max-time 10 \
    --data-urlencode "query=${query}" \
    "${PROMETHEUS_URL}/api/v1/query"
}

wait_for_url "${PROMETHEUS_URL}/-/ready"
wait_for_url "${GRAFANA_URL}/api/health"

targets_json=""
targets_ready=false
for ((attempt = 1; attempt <= 12; attempt++)); do
  if targets_json="$(curl -fsS --max-time 10 "${PROMETHEUS_URL}/api/v1/targets")" &&
    jq -e '
      .status == "success" and
      ([.data.activeTargets[] | select(.health != "up")] | length) == 0 and
      ([.data.activeTargets[] | "\(.labels.job)/\(.labels.host)"] | sort) ==
        ([
          "deepseek-vllm/kyber",
          "coleman-moonbridge/coleman",
          "coleman-qwen-vllm/coleman",
          "coleman-yokai-agent/coleman",
          "node/beskar",
          "node/coleman",
          "node/kyber",
          "prometheus/beskar",
          "yokai-agent-beskar/beskar",
          "yokai-agent-kyber/kyber"
        ] | sort)
    ' <<<"${targets_json}" >/dev/null; then
    targets_ready=true
    break
  fi
  if ((attempt < 12)); then
    sleep 5
  fi
done

if [[ "${targets_ready}" != "true" ]]; then
  echo "Prometheus did not reach the exact ten-target healthy inventory." >&2
  jq '.data.activeTargets[]? | {scrapePool,scrapeUrl,job:.labels.job,host:.labels.host,health,lastError}' \
    <<<"${targets_json}" >&2 || true
  exit 1
fi

require_series() {
  local query="$1"
  local minimum="$2"
  local description="$3"
  local result observed attempt
  for ((attempt = 1; attempt <= 12; attempt++)); do
    result="$(prom_query "${query}")"
    if ! jq -e '.status == "success"' <<<"${result}" >/dev/null; then
      echo "Representative query failed for ${description}: ${query}" >&2
      exit 1
    fi
    observed="$(jq '.data.result | length' <<<"${result}")"
    if ((observed >= minimum)); then
      return 0
    fi
    if ((attempt < 12)); then
      sleep 5
    fi
  done
  echo "Expected at least ${minimum} live series for ${description}; observed ${observed}: ${query}" >&2
  exit 1
}

# Each dashboard section has at least one raw or recorded metric whose live
# presence is mandatory. Individual range/rate queries may legitimately be
# empty before a request arrives, so those are syntax-checked separately.
representative_checks=(
  'vllm:prompt_tokens_total{cluster="deepseek-v4-flash"}|1|logical input token counters'
  'vllm:generation_tokens_total{cluster="deepseek-v4-flash"}|1|generated token counters'
  'vllm:time_to_first_token_seconds_bucket{cluster="deepseek-v4-flash"}|1|latency histograms'
  'vllm:prefix_cache_queries_total{cluster="deepseek-v4-flash"}|1|cache metrics'
  'vllm:spec_decode_num_draft_tokens_total{cluster="deepseek-v4-flash"}|1|speculative decoding metrics'
  'deepseek:host_memory_used_ratio{cluster="deepseek-v4-flash"}|2|recorded host memory ratios'
  'node_pressure_memory_waiting_seconds_total{cluster="deepseek-v4-flash"}|2|memory pressure metrics'
  'yokai_gpu_utilization{cluster="deepseek-v4-flash"}|2|GPU utilization metrics'
  'yokai_gpu_temperature_celsius{cluster="deepseek-v4-flash"}|2|GPU temperature metrics'
  'yokai_gpu_power_draw_watts{cluster="deepseek-v4-flash"}|2|GPU power metrics'
  'count by (host) (node_network_receive_bytes_total{cluster="deepseek-v4-flash"})|2|network metrics on both hosts'
  'count by (host) (node_disk_read_bytes_total{cluster="deepseek-v4-flash"})|2|disk metrics on both hosts'
  'node_uname_info{cluster="coleman-vision-gateway",host="coleman"}|1|Coleman host metrics'
  'yokai_gpu_utilization{cluster="coleman-vision-gateway",host="coleman"}|1|Coleman GPU utilization'
  'yokai_gpu_power_draw_watts{cluster="coleman-vision-gateway",host="coleman"}|1|Coleman GPU power'
  'vllm:num_requests_running{cluster="coleman-vision-gateway",host="coleman"}|1|Qwen3-VL scheduler metrics'
  'probe_success{cluster="coleman-vision-gateway",host="coleman"}|1|Moon Bridge auth-boundary probe'
)
for check in "${representative_checks[@]}"; do
  IFS='|' read -r query minimum description <<<"${check}"
  require_series "${query}" "${minimum}" "${description}"
done

for query in \
  'sum(up{cluster="coleman-vision-gateway"})' \
  'sum(probe_success{cluster="coleman-vision-gateway",job="coleman-moonbridge"})'; do
  result="$(prom_query "${query}")"
  observed="$(jq -r '.data.result[0].value[1] // "0" | tonumber' <<<"${result}")"
  expected=4
  [[ "${query}" == *probe_success* ]] && expected=1
  if [[ "${observed}" != "${expected}" ]]; then
    echo "Coleman readiness failed for ${query}; observed ${observed}, expected ${expected}." >&2
    exit 1
  fi
done

for query in \
  'count(node_uname_info{cluster="deepseek-v4-flash"})' \
  'count(yokai_gpu_utilization{cluster="deepseek-v4-flash"})' \
  'count(yokai_gpu_power_draw_watts{cluster="deepseek-v4-flash"} > 0)' \
  'count((time() - timestamp(yokai_gpu_power_draw_watts{cluster="deepseek-v4-flash"})) < 20)'; do
  result="$(prom_query "${query}")"
  observed="$(jq -r '.data.result[0].value[1] // "0" | tonumber' <<<"${result}")"
  if [[ "${observed}" != "2" ]]; then
    echo "Expected two hosts for ${query}; observed ${observed}." >&2
    exit 1
  fi
done

queries_json="$(cd "${REPO_ROOT}" && go run ./deployments/deepseek-v4-flash-observability/cmd/dashboard -queries -compact)"
query_failures=0
while IFS= read -r query; do
  query="${query//\$__rate_interval/1m}"
  query="${query//\$__range_s/3600}"
  query="${query//\$__range/1h}"
  query="${query//\$electricity_rate/0.1615}"
  query="${query//\$model/.+}"
  query="${query//\$host/.+}"
  result="$(prom_query "${query}")"
  if ! jq -e '.status == "success"' <<<"${result}" >/dev/null; then
    echo "PromQL validation failed: ${query}" >&2
    jq '.errorType,.error' <<<"${result}" >&2
    query_failures=$((query_failures + 1))
  fi
done < <(jq -r '.[]' <<<"${queries_json}")
if ((query_failures > 0)); then
  exit 1
fi

dashboard_json="$(curl -fsS --max-time 10 "${GRAFANA_URL}/api/dashboards/uid/deepseek-v4-flash")"
if ! jq -e '
    .dashboard.uid == "deepseek-v4-flash" and
    (.dashboard.panels | length >= 96) and
    ([.dashboard.panels[] | select(.title == "07 · Cost compare · local vs comparable APIs")] | length) == 1 and
    ([.dashboard.panels[] | select(.title == "09 · Coleman vision gateway")] | length) == 1 and
    ([.dashboard.templating.list[] | select(.name == "electricity_rate" and .query == "0.1615")] | length) == 1
  ' <<<"${dashboard_json}" >/dev/null; then
  echo "Grafana dashboard gate failed; expected UID, >=96 panels, Cost Compare, Coleman row, and electricity_rate=0.1615." >&2
  jq '{uid:.dashboard.uid,panels:(.dashboard.panels|length),cost_rows:([.dashboard.panels[]|select(.title=="07 · Cost compare · local vs comparable APIs")]|length),electricity_rate:([.dashboard.templating.list[]|select(.name=="electricity_rate")|.query])}' \
    <<<"${dashboard_json}" >&2
  exit 1
fi

printf 'healthy_targets=%s dashboard_panels=%s prompt_tokens=%s generated_tokens=%s\n' \
  "$(jq '.data.activeTargets | length' <<<"${targets_json}")" \
  "$(jq '.dashboard.panels | length' <<<"${dashboard_json}")" \
  "$(prom_query 'sum(vllm:prompt_tokens_total{cluster="deepseek-v4-flash"})' | jq -r '.data.result[0].value[1]')" \
  "$(prom_query 'sum(vllm:generation_tokens_total{cluster="deepseek-v4-flash"})' | jq -r '.data.result[0].value[1]')"
