#!/usr/bin/env bash
set -Eeuo pipefail

readonly COLEMAN="dell@coleman.trout-rooster.ts.net"
readonly GATEWAY_BASE="http://coleman.trout-rooster.ts.net:38440/v1"
readonly VISION_BASE="http://coleman.trout-rooster.ts.net:8001/v1"
readonly DIRECT_BASE="http://kyber.trout-rooster.ts.net:8000/v1"
readonly API_KEY_FILE="/home/sbull/.config/deepseek-spark-vision.key"
readonly DIRECT_API_KEY_FILE="/home/sbull/.config/deepseek-spark.key"
readonly MODEL="deepseek-v4-flash-0731-vision"
readonly DIRECT_MODEL="deepseek-v4-flash-0731"

stage_dir="$(mktemp -d)"
cleanup() {
  chmod -R u+rwX "${stage_dir}" 2>/dev/null || true
  find "${stage_dir}" -mindepth 1 -delete 2>/dev/null || true
  rmdir "${stage_dir}" 2>/dev/null || true
}
trap cleanup EXIT

api_key="$(tr -d '\r\n' <"${API_KEY_FILE}")"
direct_api_key="$(tr -d '\r\n' <"${DIRECT_API_KEY_FILE}")"
chmod 0700 "${stage_dir}"
printf 'header = "Authorization: Bearer %s"\n' "${api_key}" >"${stage_dir}/curl.conf"
printf 'header = "Authorization: Bearer %s"\n' "${direct_api_key}" >"${stage_dir}/direct-curl.conf"
chmod 0600 "${stage_dir}/curl.conf" "${stage_dir}/direct-curl.conf"
unset api_key direct_api_key

auth_curl() {
  curl --silent --show-error --fail-with-body --config "${stage_dir}/curl.conf" "$@"
}

direct_curl() {
  curl --silent --show-error --fail-with-body --config "${stage_dir}/direct-curl.conf" "$@"
}

echo "Checking authentication boundaries and model catalog..."
[[ "$(curl -sS -o /dev/null -w '%{http_code}' "${GATEWAY_BASE}/models")" == "401" ]]
auth_curl "${GATEWAY_BASE}/models" >"${stage_dir}/models.json"
jq -e --arg model "${MODEL}" '
  (.models // .data // []) |
  any((.slug // .id) == $model)
' "${stage_dir}/models.json" >/dev/null

echo "Checking text Responses and the direct rollback endpoint..."
jq -n --arg model "${MODEL}" '{model:$model,input:"Reply with exactly: VISION_GATEWAY_TEXT_OK",max_output_tokens:64}' \
  >"${stage_dir}/text-request.json"
auth_curl -H 'Content-Type: application/json' --data-binary "@${stage_dir}/text-request.json" \
  "${GATEWAY_BASE}/responses" >"${stage_dir}/text-response.json"
jq -er '.output_text // ([.output[]?.content[]? | select(.type == "output_text" or .type == "text") | .text] | join("\n"))' \
  "${stage_dir}/text-response.json" | grep -F 'VISION_GATEWAY_TEXT_OK' >/dev/null
direct_curl "${DIRECT_BASE}/models" >/dev/null
jq -n --arg model "${DIRECT_MODEL}" '{model:$model,input:"Reply with exactly: DIRECT_ROLLBACK_OK",max_output_tokens:64}' \
  >"${stage_dir}/direct-request.json"
direct_curl -H 'Content-Type: application/json' --data-binary "@${stage_dir}/direct-request.json" \
  "${DIRECT_BASE}/responses" >"${stage_dir}/direct-response.json"
jq -er '.output_text // ([.output[]?.content[]? | select(.type == "output_text" or .type == "text") | .text] | join("\n"))' \
  "${stage_dir}/direct-response.json" | grep -F 'DIRECT_ROLLBACK_OK' >/dev/null

echo "Checking real Responses streaming..."
jq -n --arg model "${MODEL}" '{model:$model,input:"Count from one to five, one number per line.",stream:true,max_output_tokens:128}' \
  >"${stage_dir}/stream-request.json"
auth_curl -N -H 'Content-Type: application/json' --data-binary "@${stage_dir}/stream-request.json" \
  "${GATEWAY_BASE}/responses" >"${stage_dir}/stream.sse"
grep -F 'response.output_text.delta' "${stage_dir}/stream.sse" >/dev/null
grep -F 'response.completed' "${stage_dir}/stream.sse" >/dev/null

echo "Creating a deterministic OCR and color test image..."
magick -size 960x540 canvas:white \
  -fill '#d71920' -draw 'rectangle 80,80 880,300' \
  -fill black -font /usr/share/fonts/liberation/LiberationSans-Bold.ttf -pointsize 140 -gravity south -annotate +0+35 'HELLO' \
  "${stage_dir}/vision-test.png"
base64 -w0 "${stage_dir}/vision-test.png" >"${stage_dir}/vision-test.b64"
jq -n \
  --arg model "${MODEL}" \
  --rawfile image "${stage_dir}/vision-test.b64" \
  '{
    model:$model,
    input:[{
      role:"user",
      content:[
        {type:"input_text",text:"Inspect the attached image. Transcribe the word printed below the rectangle, then name the rectangle color. Reply exactly as TEXT=<transcription>; COLOR=<color>."},
        {type:"input_image",image_url:("data:image/png;base64," + $image),detail:"high"}
      ]
    }],
    max_output_tokens:512
  }' >"${stage_dir}/image-request.json"

echo "Checking the DeepSeek-to-Qwen visual tool loop..."
auth_curl -H 'Content-Type: application/json' --data-binary "@${stage_dir}/image-request.json" \
  "${GATEWAY_BASE}/responses" >"${stage_dir}/image-response.json"
image_answer="$(jq -er '.output_text // ([.output[]?.content[]? | select(.type == "output_text" or .type == "text") | .text] | join("\n"))' "${stage_dir}/image-response.json")"
grep -Eiq 'HELLO' <<<"${image_answer}"
grep -Eiq 'red' <<<"${image_answer}"

echo "Checking mixed visual and client-owned tool handling..."
jq -n \
  --arg model "${MODEL}" \
  --rawfile image "${stage_dir}/vision-test.b64" \
  '{
    model:$model,
    input:[{
      role:"user",
      content:[
        {type:"input_text",text:"Inspect the image first. Then call report_image exactly once with the transcribed text and rectangle color. Do not answer in prose."},
        {type:"input_image",image_url:("data:image/png;base64," + $image),detail:"high"}
      ]
    }],
    tools:[{
      type:"function",
      name:"report_image",
      description:"Report visual evidence after inspecting the supplied image.",
      parameters:{
        type:"object",
        properties:{
          text:{type:"string"},
          color:{type:"string"}
        },
        required:["text","color"],
        additionalProperties:false
      },
      strict:true
    }],
    tool_choice:"required",
    max_output_tokens:512
  }' >"${stage_dir}/tool-request.json"
auth_curl -H 'Content-Type: application/json' --data-binary "@${stage_dir}/tool-request.json" \
  "${GATEWAY_BASE}/responses" >"${stage_dir}/tool-response.json"
tool_arguments="$(jq -er '[.output[]? | select(.type == "function_call" and .name == "report_image")][0].arguments' "${stage_dir}/tool-response.json")"
jq -e '
  (.text | test("HELLO"; "i")) and
  (.color | test("red"; "i"))
' <<<"${tool_arguments}" >/dev/null

echo "Checking vision metrics, service bindings, and preserved Coleman workload..."
curl -fsS "http://coleman.trout-rooster.ts.net:8001/metrics" | grep -F 'vllm:' >/dev/null
tailscale ssh "${COLEMAN}" \
  'systemctl --user is-active --quiet qwen36-27b-mtp.service; docker compose -f /home/dell/.local/share/deepseek-vision-gateway/docker-compose.yml ps --status running --quiet | grep -q .; ss -ltn | grep -q "100.104.98.40:38440"; ss -ltn | grep -q "100.104.98.40:8001"; docker logs deepseek-vision-moonbridge 2>&1 | grep -q "Visual tool executed"'

echo "Vision gateway verification passed."
