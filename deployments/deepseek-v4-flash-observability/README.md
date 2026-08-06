# DeepSeek V4 Flash observability

This deployment turns Beskar into the single monitoring hub for the two-node
DeepSeek-V4-Flash-0731 vLLM engine running across Beskar and Kyber.

## What it monitors

- Native, authenticated vLLM metrics from the Kyber rank-0 API process. Those
  metrics describe the full distributed engine and are intentionally scraped
  once rather than duplicated per worker.
- `node_exporter` on Beskar and Kyber for CPU, memory, PSI, swap, disk, network,
  filesystem, load, faults, and OOM telemetry.
- The Yokai agent on each host for GB10 utilization, unified-memory fallback,
  temperature, and power.
- Prometheus self-health and recording/alert rules.

Grafana provisions one read-only dashboard with eight sections:

1. model pulse
2. tokens, requests, and throughput
3. latency and user experience
4. cache, scheduler, and speculative decoding
5. GB10 unified memory and host pressure
6. GPU, thermals, power, energy, and efficiency
7. cost compare: local GPU electricity versus comparable hosted APIs
8. network, storage, and platform reliability

## Metric definitions that matter

- **Logical input throughput** is the rate of
  `vllm:prompt_tokens_total`; it includes cache hits.
- **New prefill throughput** is the `local_compute` source of
  `vllm:prompt_tokens_by_source_total`; it represents prompt work actually
  computed on the devices.
- **Cached input throughput** is the `local_cache_hit` source. It must not be
  described as new prefill compute.
- **Decode throughput** is the rate of `vllm:generation_tokens_total`.
- **Process-lifetime tokens** are current native counter values. They include
  the current run's pre-scrape activity, but reset when vLLM restarts.
- Hourly and selected-range totals use reset-aware `increase()` and only cover
  history Prometheus actually observed. Prometheus retains up to two years or
  50 GB, whichever limit is reached first.
- GB10 uses unified memory. Host RAM and Yokai's GPU-memory fallback are two
  views of the same physical pool and are never added together.
- Energy and electricity-cost panels integrate NVIDIA-reported GPU-domain power
  at the five-second scrape interval. They do not measure whole-system wall
  power and exclude CPU/SoC, memory, PSU losses, networking, cooling, fixed
  utility fees, and hardware amortization.
- The electricity-rate variable defaults to `0.1615` USD/kWh, the latest EIA
  Texas statewide residential year-to-date average available when captured on
  August 6, 2026 (through May 2026). This is not a North Houston tariff; the
  variable portion of the actual electricity plan or bill is the better input.
- Cost Compare reprices the selected-range prompt/output mix at first-party
  standard uncached API list prices captured August 6, 2026. It is a
  token-for-token counterfactual rather than a claim that peers use the same
  tokens or achieve identical task outcomes. Local cost includes observed idle
  GPU-domain power but not the rest of either system. The peer band and prices are:
  DeepSeek V4 Flash 0731 max (AA 52; 0.14/0.28 USD per M input/output), Gemini
  3.6 Flash high (AA 52; 1.50/7.50), GPT-5.6 Terra high (AA 50; 2.00/12.00
  base tier), and Claude Sonnet 5 max (AA 55; introductory 2.00/10.00 through
  August 31, 2026). Refresh the pricing snapshot before September 1, 2026.

Native vLLM metrics contain `model_name` and `engine`, but no user or session
identity. The dashboard therefore stays aggregate/per-model. Per-user token
accounting requires bounded-cardinality instrumentation in the authentication
gateway; request IDs and API keys must never become Prometheus labels.

Current comparison sources are linked directly in the dashboard: Artificial
Analysis for the intelligence band; first-party DeepSeek, Google, OpenAI, and
Anthropic pricing; and EIA for the Texas electricity-rate fallback.

## Deployment

From the Yokai repository root:

```bash
./deployments/deepseek-v4-flash-observability/deploy.sh
```

The deployer:

- records both model-container identities and refuses to proceed unless both
  workers are running;
- generates and validates the Grafana dashboard;
- copies existing bearer credentials through a mode-0700 temporary directory
  without committing or printing them;
- validates Compose and Prometheus configuration before activation;
- starts only Kyber's node exporter and existing Yokai user service;
- backs up the previous monitoring directories before replacing configuration;
- recreates only monitoring containers and preserves named data volumes;
- verifies that neither DeepSeek container ID/start timestamp changed;
- parses every dashboard PromQL expression through live Prometheus and requires
  representative live series from every dashboard section and both hosts.

The dashboard is available to Tailnet peers at:

```text
http://beskar:3001/d/deepseek-v4-flash
```

Prometheus is available at `http://beskar:9090`. Both published services bind
only to Beskar's Tailscale address. Grafana anonymous access is Viewer-only;
basic login is disabled because the previous stack retained an unsafe default
administrator credential in its persistent database.

## Verification

Run the non-mutating verification at any time:

```bash
./deployments/deepseek-v4-flash-observability/verify.sh
```

It requires healthy Prometheus, vLLM, both Yokai agents, both node exporters,
valid dashboard queries, representative live data for every section and both
hosts, and the provisioned Grafana dashboard API.

## Rollback

Deployment moves the prior directories to timestamped siblings such as:

```text
/home/dell/.local/share/yokai/monitoring.backup-YYYYMMDDTHHMMSSZ
/home/dell/.local/share/yokai/fleet-exporters.backup-YYYYMMDDTHHMMSSZ
```

To roll back, stop the replacement Compose project, move the desired backup
back to its original path, and run `docker compose up -d --force-recreate`
there. Do not remove
the `monitoring_prometheus_data` or `monitoring_grafana_data` Docker volumes;
they contain retained history and Grafana state.

## Intentional omissions

- DCGM Exporter is omitted. The previously configured image was wrong-arch on
  arm64/GB10, continuously down, and does not provide a trustworthy unified-
  memory view in this deployment.
- Alert rules are evaluated and visible in the dashboard, but no Alertmanager
  notification destination is configured. Adding email, Slack, PagerDuty, or
  another external receiver requires an explicit delivery and credential
  decision.
