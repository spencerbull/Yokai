import { DAEMON_URL } from "../config"
import type { BootstrapDeviceResponse, DeviceDeleteResult, DeviceRequest, DeviceTestResult, SSHConfigHostsResponse, TailscalePeersResponse, TailscaleStatus } from "../contracts/devices"
import type { DevicesResponse, LogTarget, MetricsResponse } from "../contracts/fleet"
import type { DeployHistory, HFSettings, SettingsDocument, SettingsPatch } from "../contracts/settings"
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

export async function getDeployHistory() {
  return daemonRequest<DeployHistory>("/history/deploy")
}

export async function putDeployHistory(history: DeployHistory) {
  return daemonRequest<DeployHistory>("/history/deploy", {
    method: "PUT",
    body: JSON.stringify(history),
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

export async function deleteDevice(deviceId: string) {
  return daemonRequest<DeviceDeleteResult>(`/devices/${encodeURIComponent(deviceId)}`, {
    method: "DELETE",
  })
}

export async function bootstrapDevice(device: DeviceRequest) {
  return daemonRequest<BootstrapDeviceResponse>("/bootstrap/device", {
    method: "POST",
    body: JSON.stringify(device),
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
