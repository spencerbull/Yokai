import { DAEMON_URL } from "../config"
import type { BootstrapDeviceRequest, BootstrapDeviceResponse, BulkDeviceTestResponse, BulkDeviceUpgradeResponse, DeviceDeleteResult, DeviceRequest, DeviceTestResult, DeviceUpgradeResult, SSHConfigHostsResponse, TailscalePeersResponse, TailscaleStatus } from "../contracts/devices"
import type { DeployBKC, DeployRequest, DeployResult, GGUFVariantsResponse, HFModel, VLLMMemoryEstimate, WorkloadType } from "../contracts/deploy"
import type { DevicesResponse, LogTarget, MetricsResponse } from "../contracts/fleet"
import type { DeployHistory, HFSettings, HFTokenValidation, IntegrationsConfigureRequest, IntegrationsConfigureResponse, OpenAIEndpoint, SettingsDocument, SettingsPatch } from "../contracts/settings"
import { readSSEStream } from "./sse"

type StatusResult = {
  status: string
}

type ServiceTestResult = {
  service_type: string
  message: string
  model?: string
  prompt_id?: string
}

type RemoveServiceResult = {
  status: string
  removed_services?: number
}

type ErrorEnvelope = {
  error?:
    | string
    | {
        code?: string
        message?: string
        details?: unknown
      }
  message?: string
}

export async function getSettings() {
  return daemonRequest<SettingsDocument>("/settings")
}

export async function getDevices() {
  return daemonRequest<DevicesResponse>("/devices")
}

export async function getSSHConfigHosts() {
  return daemonRequest<SSHConfigHostsResponse>("/discovery/ssh-config-hosts")
}

export async function getTailscaleStatus() {
  return daemonRequest<TailscaleStatus>("/discovery/tailscale/status")
}

export async function getTailscalePeers() {
  return daemonRequest<TailscalePeersResponse>("/discovery/tailscale/peers")
}

export async function getMetrics() {
  return daemonRequest<MetricsResponse>("/metrics")
}

export async function getHFModels(query: string, workload: WorkloadType) {
  const response = await daemonRequest<{ models: HFModel[] }>(`/hf/models?query=${encodeURIComponent(query)}&workload=${encodeURIComponent(workload)}`)
  return response.models ?? []
}

export async function getGGUFVariants(model: string) {
  try {
    const response = await daemonRequest<GGUFVariantsResponse>(`/hf/gguf-variants?model=${encodeURIComponent(model)}`)
    return response.variants ?? []
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : String(cause)
    // A plain 404 here means the daemon doesn't register `/hf/gguf-variants`
    // yet — i.e. the daemon process is running an older binary than the TUI.
    // Rewrite the error into something the user can act on rather than
    // surfacing the raw "daemon request failed: 404 Not Found".
    if (/\b404\b/.test(message) && !/HF API/i.test(message)) {
      throw new Error(
        "GGUF variant listing is not available on this daemon. Restart `yokai daemon` after upgrading yokai to enable it.",
      )
    }
    throw cause
  }
}

export async function getDeployBKC(workload: WorkloadType, model: string, deviceId?: string) {
  const params = new URLSearchParams({ workload, model })
  if (deviceId) {
    params.set("device_id", deviceId)
  }
  const response = await daemonRequest<{ config?: DeployBKC }>(`/deploy/bkc?${params.toString()}`)
  return response.config ?? null
}

export async function getVLLMMemoryEstimate(request: {
  context_length: number
  device_id: string
  model: string
  overhead_gb: string
  extra_args?: string
}) {
  return daemonRequest<VLLMMemoryEstimate>("/deploy/vllm-memory-estimate", {
    method: "POST",
    body: JSON.stringify(request),
  })
}

export async function patchSettings(patch: SettingsPatch) {
  return daemonRequest<SettingsDocument>("/settings", {
    method: "PATCH",
    body: JSON.stringify(patch),
  })
}

export async function putHFToken(token: string) {
  return daemonRequest<HFSettings>("/settings/hf-token", {
    method: "PUT",
    body: JSON.stringify({ token }),
  })
}

export async function validateHFToken(token: string) {
  return daemonRequest<HFTokenValidation>("/settings/hf-token/validate", {
    method: "POST",
    body: JSON.stringify({ token }),
  })
}

export async function getDeployHistory() {
  return daemonRequest<DeployHistory>("/history/deploy")
}

export async function putDeployHistory(history: DeployHistory) {
  return daemonRequest<DeployHistory>("/history/deploy", {
    method: "PUT",
    body: JSON.stringify(history),
  })
}

export async function getOpenAIEndpoints() {
  const response = await daemonRequest<{ endpoints: OpenAIEndpoint[] }>("/integrations/openai-endpoints")
  return response.endpoints ?? []
}

export async function configureIntegrations(request: IntegrationsConfigureRequest) {
  return daemonRequest<IntegrationsConfigureResponse>("/integrations/configure", {
    method: "POST",
    body: JSON.stringify(request),
  })
}

export async function streamLogs(
  target: LogTarget,
  signal: AbortSignal,
  onLine: (line: string) => void,
) {
  const response = await fetch(
    new URL(
      `/logs/${encodeURIComponent(target.deviceId)}/${encodeURIComponent(target.containerId)}`,
      ensureTrailingSlash(DAEMON_URL),
    ),
    {
      headers: {
        Accept: "text/event-stream",
      },
      signal,
    },
  )

  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }

  if (!response.body) {
    throw new Error("daemon log stream is unavailable")
  }

  await readSSEStream(response.body, onLine)
}

export async function stopContainer(deviceId: string, containerId: string) {
  return daemonRequest<StatusResult>(`/containers/${encodeURIComponent(deviceId)}/${encodeURIComponent(containerId)}/stop`, {
    method: "POST",
  })
}

export async function restartContainer(deviceId: string, containerId: string) {
  return daemonRequest<StatusResult>(`/containers/${encodeURIComponent(deviceId)}/${encodeURIComponent(containerId)}/restart`, {
    method: "POST",
  })
}

export async function testContainer(deviceId: string, containerId: string) {
  return daemonRequest<ServiceTestResult>(`/containers/${encodeURIComponent(deviceId)}/${encodeURIComponent(containerId)}/test`, {
    method: "POST",
    body: JSON.stringify({}),
  })
}

export async function removeContainer(deviceId: string, containerId: string) {
  return daemonRequest<RemoveServiceResult>(`/containers/${encodeURIComponent(deviceId)}/${encodeURIComponent(containerId)}/remove`, {
    method: "DELETE",
  })
}

export async function createDevice(device: DeviceRequest) {
  return daemonRequest<DevicesResponse["devices"][number]>("/devices", {
    method: "POST",
    body: JSON.stringify(device),
  })
}

export async function updateDevice(deviceId: string, device: DeviceRequest) {
  return daemonRequest<DevicesResponse["devices"][number]>(`/devices/${encodeURIComponent(deviceId)}`, {
    method: "PUT",
    body: JSON.stringify(device),
  })
}

export async function testDevice(deviceId: string) {
  return daemonRequest<DeviceTestResult>(`/devices/${encodeURIComponent(deviceId)}/test`, {
    method: "POST",
  })
}

export async function upgradeDevice(deviceId: string) {
  return daemonRequest<DeviceUpgradeResult>(`/devices/${encodeURIComponent(deviceId)}/upgrade`, {
    method: "POST",
  })
}

export async function testAllDevices() {
  return daemonRequest<BulkDeviceTestResponse>("/devices/test-all", {
    method: "POST",
  })
}

export async function upgradeAllDevices() {
  return daemonRequest<BulkDeviceUpgradeResponse>("/devices/upgrade-all", {
    method: "POST",
  })
}

export async function deleteDevice(deviceId: string) {
  return daemonRequest<DeviceDeleteResult>(`/devices/${encodeURIComponent(deviceId)}`, {
    method: "DELETE",
  })
}

export async function bootstrapDevice(device: BootstrapDeviceRequest) {
  return daemonRequest<BootstrapDeviceResponse>("/bootstrap/device", {
    method: "POST",
    body: JSON.stringify(device),
  })
}

export async function deployService(request: DeployRequest) {
  return daemonRequest<DeployResult>("/deploy", {
    method: "POST",
    body: JSON.stringify(request),
  })
}

async function daemonRequest<T>(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers)
  if (init.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json")
  }

  const response = await fetch(new URL(path, ensureTrailingSlash(DAEMON_URL)), {
    ...init,
    headers,
  })

  if (!response.ok) {
    throw new Error(await readErrorMessage(response))
  }

  return (await response.json()) as T
}

async function readErrorMessage(response: Response) {
  const fallback = `daemon request failed: ${response.status} ${response.statusText}`

  try {
    const payload = (await response.json()) as ErrorEnvelope
    if (typeof payload.error === "string" && payload.error) {
      return payload.error
    }
    if (payload.error && typeof payload.error === "object" && payload.error.message) {
      return payload.error.message
    }
    if (payload.message) {
      return payload.message
    }
  } catch {
    return fallback
  }

  return fallback
}

function ensureTrailingSlash(url: string) {
  return url.endsWith("/") ? url : `${url}/`
}
