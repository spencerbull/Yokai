import type { DeviceRecord } from "./fleet"

export type SSHConfigHost = {
  alias: string
  host: string
  user?: string
  port: number
  identity_file?: string
  identity_file_encrypted: boolean
}

export type SSHConfigHostsResponse = {
  hosts: SSHConfigHost[]
}

export type TailscaleStatus = {
  installed: boolean
  running: boolean
  needs_login: boolean
  backend_state?: string
  self?: {
    hostname?: string
    dns_name?: string
    ip?: string
  }
  error?: string
  install_instructions?: string
  tag_help?: string
}

export type TailscalePeer = {
  hostname: string
  dns_name?: string
  ip: string
  ips?: string[]
  os?: string
  online: boolean
  tags?: string[]
  highlighted_tags?: string[]
  other_tags?: string[]
  recommended: boolean
}

export type TailscalePeersResponse = {
  peers: TailscalePeer[]
}

export type DeviceRequest = {
  id?: string
  label?: string
  host: string
  ssh_user?: string
  ssh_key?: string
  ssh_port?: number
  connection_type?: string
  agent_port?: number
  agent_token?: string
  gpu_type?: string
  tags?: string[]
}

export type BootstrapDeviceRequest = DeviceRequest & {
  install_monitoring?: boolean
  ssh_key_passphrase?: string
  ssh_password?: string
}

export type DeviceTestResult = {
  device_id: string
  ssh_ok: boolean
  agent_ok: boolean
  version?: string
  message: string
}

export type DeviceUpgradeResult = {
  device_id: string
  ok: boolean
  message: string
}

export type BulkDeviceTestResponse = {
  results: DeviceTestResult[]
}

export type BulkDeviceUpgradeResponse = {
  results: DeviceUpgradeResult[]
}

export type DeviceDeleteResult = {
  removed_device_id: string
  removed_services: number
  cleanup_requested: boolean
}

export type BootstrapDeviceResponse = {
  device: DeviceRecord
  preflight?: {
    KernelOS: string
    OS: string
    Arch: string
    DockerInstalled: boolean
    DockerVersion: string
    GPUDetected: boolean
    GPUName: string
    GPUVRAMMb: number
    NvidiaToolkitInstalled: boolean
    NvidiaRuntimeAvailable: boolean
    DiskFreeGB: number
  }
  agent_token: string
  install_monitoring: boolean
  monitoring_installed: boolean
  message: string
}

export type DeviceEditorForm = {
  mode: "create" | "edit"
  connectionType: string
  authMethod: "agent" | "key" | "password"
  id?: string
  label: string
  host: string
  sshUser: string
  sshKey: string
  sshKeyPassphrase: string
  sshPassword: string
  sshPort: string
  agentPort: string
  agentToken: string
  tagsText: string
}

export type DeviceFormField =
  | "label"
  | "host"
  | "sshUser"
  | "authMethod"
  | "sshKey"
  | "sshKeyPassphrase"
  | "sshPassword"
  | "sshPort"
  | "agentPort"
  | "agentToken"
  | "tagsText"

export function editorFormFromDevice(device: DeviceRecord): DeviceEditorForm {
  return {
    mode: "edit",
    connectionType: device.connection_type || "manual",
    authMethod: device.ssh_key ? "key" : "agent",
    id: device.id,
    label: device.label || "",
    host: device.host,
    sshUser: device.ssh_user || "",
    sshKey: device.ssh_key || "",
    sshKeyPassphrase: "",
    sshPassword: "",
    sshPort: `${device.ssh_port ?? 22}`,
    agentPort: `${device.agent_port ?? 7474}`,
    agentToken: device.agent_token || "",
    tagsText: (device.tags || []).join(", "),
  }
}

export function emptyDeviceEditorForm(): DeviceEditorForm {
  return {
    mode: "create",
    connectionType: "manual",
    authMethod: "agent",
    label: "",
    host: "",
    sshUser: "",
    sshKey: "",
    sshKeyPassphrase: "",
    sshPassword: "",
    sshPort: "22",
    agentPort: "7474",
    agentToken: "",
    tagsText: "",
  }
}

export function editorFormFromSSHHost(host: SSHConfigHost): DeviceEditorForm {
  return {
    mode: "create",
    connectionType: "ssh-config",
    authMethod: host.identity_file ? "key" : "agent",
    label: host.alias,
    host: host.host,
    sshUser: host.user || "",
    sshKey: host.identity_file || "",
    sshKeyPassphrase: "",
    sshPassword: "",
    sshPort: `${host.port || 22}`,
    agentPort: "7474",
    agentToken: "",
    tagsText: "",
  }
}

export function editorFormFromTailscalePeer(peer: TailscalePeer): DeviceEditorForm {
  return {
    mode: "create",
    connectionType: "tailscale",
    authMethod: "agent",
    label: peer.hostname,
    host: peer.dns_name || peer.ip,
    sshUser: "",
    sshKey: "",
    sshKeyPassphrase: "",
    sshPassword: "",
    sshPort: "22",
    agentPort: "7474",
    agentToken: "",
    tagsText: (peer.tags || []).join(", "),
  }
}
