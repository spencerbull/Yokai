# DeepSeek V4 Flash observability

## Goal

Run one durable Prometheus and Grafana observability hub on Beskar for the
two-node DeepSeek-V4-Flash-0731 vLLM deployment on Beskar and Kyber.

## Done criteria

- Prometheus reports healthy scrapes for vLLM, both Yokai agents, and both node
  exporters without exposing bearer tokens in source control or logs.
- Grafana provisions a sectioned dashboard covering model activity, token
  volume and throughput, latency, cache/speculative decoding, unified-memory
  pressure, CPU, network, disk, temperature, and power.
- Hourly token volume and process-lifetime token counters are clearly labeled;
  retained-history totals never claim to predate telemetry collection.
- The existing DeepSeek containers remain running throughout deployment.
- Static/config tests, live PromQL checks, and browser-level dashboard QA pass.
- Cost Compare shows selected-range local GPU electricity beside current
  standard uncached API equivalents for a source-backed intelligence peer band.
- Power tracking includes GPU energy, estimated electricity cost, telemetry
  coverage, a configurable Texas statewide fallback for North Houston, and visible
  GPU-domain versus wall-power limitations.

## Streams

- Branch/worktree: `deepseek-observability-dashboard` in this checkout.
- Files in scope: `deployments/deepseek-v4-flash-observability/**`, this ledger,
  and narrowly related documentation/tests if required.
- Files to avoid: model runtime files on Beskar/Kyber, existing user secrets,
  unrelated Yokai product code, and existing Prometheus/Grafana data volumes.

## Allowed actions

- Add version-controlled monitoring configuration and dashboard assets.
- Start or reconcile monitoring/exporter services on Beskar and Kyber.
- Copy existing API/agent credentials into root-readable monitoring secret
  files without printing or committing them.
- Reload/recreate monitoring containers while preserving their named volumes.

## Forbidden actions

- Restart or recreate either DeepSeek vLLM container.
- Deploy production, rotate credentials, expose new public ingress, or delete
  Prometheus/Grafana data volumes.
- Alter unrelated containers or user work.

## Verification gates

- [x] JSON/YAML/Compose/Prometheus configuration validation.
- [x] Focused repository tests and dashboard query linting.
- [x] Independent code/config review and fixes.
- [x] Both model containers retain their original start timestamps/IDs.
- [x] All intended Prometheus targets are healthy and queries return live data.
- [x] Grafana datasource/dashboard provisioning and real UI rendering verified.

## Open questions and checkpoints

- DCGM's current image is wrong-architecture and GB10 uses unified memory;
  prefer Yokai's `nvidia-smi` telemetry unless a compatible DCGM path proves
  materially better.
- Native vLLM metrics have model/engine labels, not user/session identifiers;
  do not invent per-user or per-session panels.
- "Lifetime" is vLLM-process lifetime unless a separately labeled retained
  history total is available after Prometheus begins scraping.

## Completed evidence

- [x] Live topology mapped: Kyber is rank 0/API, Beskar is rank 1/headless.
- [x] Current vLLM metric families, labels, counters, and histograms sampled.
- [x] Existing Beskar stack audited; at discovery time vLLM and Kyber were not
  scraped, and the obsolete DCGM target was down.
- [x] Prometheus 3.11.3 validates the configuration and all 16 rules; all 81
  generated dashboard expressions parse against the live Prometheus engine.
- [x] Full `go test ./...`, `go vet ./...`, Bash syntax, both Compose configs,
  generated-dashboard checks, and `git diff --check` pass. `shellcheck` was not
  run because the local mise shim has no configured shellcheck version.
- [x] Independent review found and fixed target-presence blind spots, dropped
  alert labels, topology-label collisions, directory-swap recreation risk, and
  overly permissive live-data checks; final re-review found no remaining issue.
- [x] Live deployment reports six healthy scrape targets, two node hosts, two
  Yokai GPU hosts, 69 provisioned panels, and representative data in every
  dashboard section.
- [x] The deployer compared both DeepSeek container IDs/start timestamps before
  and after monitoring activation; neither model worker was recreated.
- [x] Grafana 13 API and full-page Chromium render verified the seven-section
  dashboard. Idle/request-history panels are truthfully empty until new samples
  and requests arrive; host, GPU, and platform panels render live values.
- [x] Cost Compare and power-cost extension passed static tests, all 94 live
  PromQL checks, preserved both model-container identities, provisioned 85
  panels with six healthy scrape targets, and rendered without browser errors,
  failed requests, or missing data in the new section.
