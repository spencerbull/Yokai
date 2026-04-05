import { startTransition, useEffect, useMemo, useState } from "react"

import type {
  DeviceEditorForm,
  DeviceFormField,
  DeviceRequest,
  SSHConfigHost,
  TailscalePeer,
  TailscaleStatus,
} from "../../contracts/devices"
import {
  editorFormFromDevice,
  editorFormFromSSHHost,
  editorFormFromTailscalePeer,
  emptyDeviceEditorForm,
} from "../../contracts/devices"
import type { DeviceRecord } from "../../contracts/fleet"
import {
  bootstrapDevice,
  createDevice,
  deleteDevice,
  getDevices,
  getSSHConfigHosts,
  getTailscalePeers,
  getTailscaleStatus,
  testAllDevices,
  testDevice,
  updateDevice,
  upgradeAllDevices,
  upgradeDevice,
} from "../../services/daemon-client"
import { useFleetSnapshot } from "../dashboard/useFleetSnapshot"
import { visibleTailscalePeers } from "./tailscale-search"

type DevicesNotice = {
  level: "info" | "success" | "warning" | "error"
  message: string
}

type DeviceEditorState = {
  field: DeviceFormField
  form: DeviceEditorForm
}

type SSHImportState = {
  error?: string
  hosts: SSHConfigHost[]
  loading: boolean
  selectedIndex: number
}

type TailscaleImportState = {
  error?: string
  peers: TailscalePeer[]
  query: string
  selectedIndex: number
  showTagHelp: boolean
  status: TailscaleStatus | null
}

type AddSourceState = {
  selectedIndex: number
}

type MonitoringPromptState = {
  form: DeviceEditorForm
}

type KeyLike = {
  name: string
  shift?: boolean
}

const FORM_FIELDS: DeviceFormField[] = [
  "label",
  "host",
  "sshUser",
  "authMethod",
  "sshKey",
  "sshKeyPassphrase",
  "sshPassword",
  "sshPort",
  "agentPort",
  "agentToken",
  "tagsText",
]

export function useDevicesController(active: boolean) {
  const fleet = useFleetSnapshot(active)
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading")
  const [addSource, setAddSource] = useState<AddSourceState | null>(null)
  const [devices, setDevices] = useState<DeviceRecord[]>([])
  const [error, setError] = useState<string>()
  const [selectedDeviceId, setSelectedDeviceId] = useState<string | null>(null)
  const [notice, setNotice] = useState<DevicesNotice | null>(null)
  const [pendingAction, setPendingAction] = useState<string | null>(null)
  const [viewMode, setViewMode] = useState<"list" | "detail">("list")
  const [editor, setEditor] = useState<DeviceEditorState | null>(null)
  const [importer, setImporter] = useState<SSHImportState | null>(null)
  const [monitoringPrompt, setMonitoringPrompt] = useState<MonitoringPromptState | null>(null)
  const [tailscaleImporter, setTailscaleImporter] = useState<TailscaleImportState | null>(null)
  const [deleteCandidateId, setDeleteCandidateId] = useState<string | null>(null)

  useEffect(() => {
    if (!active) {
      return
    }

    let cancelled = false

    const refresh = async () => {
      try {
        const response = await getDevices()
        if (cancelled) {
          return
        }

        startTransition(() => {
          setDevices(response.devices)
          setError(undefined)
          setStatus("ready")
        })
      } catch (cause) {
        if (cancelled) {
          return
        }

        setError(cause instanceof Error ? cause.message : "failed to load devices")
        setStatus("error")
      }
    }

    void refresh()
    const interval = setInterval(() => {
      void refresh()
    }, 2500)

    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [active])

  useEffect(() => {
    if (!notice) {
      return
    }

    const timeout = setTimeout(() => {
      setNotice((current) => (current === notice ? null : current))
    }, 4500)

    return () => {
      clearTimeout(timeout)
    }
  }, [notice])

  useEffect(() => {
    if (devices.length === 0) {
      setSelectedDeviceId(null)
      return
    }

    if (!selectedDeviceId || !devices.some((device) => device.id === selectedDeviceId)) {
      setSelectedDeviceId(devices[0].id)
    }
  }, [devices, selectedDeviceId])

  const selectedDevice = devices.find((device) => device.id === selectedDeviceId) ?? null
  const selectedFleetDevice = fleet.snapshot.devices.find((device) => device.id === selectedDeviceId) ?? null
  const deleteCandidate = devices.find((device) => device.id === deleteCandidateId) ?? null
  const visiblePeers = useMemo(
    () => (tailscaleImporter ? visibleTailscalePeers(tailscaleImporter.peers, tailscaleImporter.query) : []),
    [tailscaleImporter],
  )

  useEffect(() => {
    if (!tailscaleImporter) {
      return
    }

    if (visiblePeers.length === 0 && tailscaleImporter.selectedIndex !== 0) {
      setTailscaleImporter((current) => (current ? { ...current, selectedIndex: 0 } : current))
      return
    }

    if (tailscaleImporter.selectedIndex >= visiblePeers.length) {
      setTailscaleImporter((current) =>
        current ? { ...current, selectedIndex: Math.max(0, visiblePeers.length - 1) } : current,
      )
    }
  }, [tailscaleImporter, visiblePeers])

  return {
    addSource,
    deleteCandidate,
    devices,
    editor,
    error,
    fleetHistory: fleet.history,
    fleetSnapshot: fleet.snapshot,
    hasOverlayOpen: Boolean(addSource || importer || tailscaleImporter || editor || deleteCandidate || monitoringPrompt),
    importer,
    monitoringPrompt,
    notice,
    openAddWizard() {
      setAddSource({ selectedIndex: 0 })
    },
    pendingAction,
    selectedDevice,
    selectedFleetDevice,
    selectedServices: fleet.snapshot.services.filter((service) => service.deviceId === selectedDeviceId),
    selectDevice(deviceId: string) {
      setSelectedDeviceId(deviceId)
    },
    openDeviceView(deviceId?: string) {
      const nextId = deviceId ?? selectedDeviceId
      if (!nextId) {
        return false
      }
      setSelectedDeviceId(nextId)
      setViewMode("detail")
      return true
    },
    closeDeviceView() {
      setViewMode("list")
    },
    openGrafana: () => void openGrafana(),
    status,
    tailscaleImporter,
    visiblePeers,
    viewMode,
    handleKey(key: KeyLike) {
      if (deleteCandidate) {
        switch (key.name) {
          case "y":
          case "return":
          case "enter":
            void confirmDelete()
            return true
          case "n":
          case "escape":
            setDeleteCandidateId(null)
            return true
          default:
            return true
        }
      }

      if (monitoringPrompt) {
        switch (key.name) {
          case "y":
            void submitBootstrapWithMonitoring(true)
            return true
          case "n":
          case "return":
          case "enter":
            void submitBootstrapWithMonitoring(false)
            return true
          case "escape":
            setMonitoringPrompt(null)
            setEditor({ field: "agentToken", form: monitoringPrompt.form })
            return true
          default:
            return true
        }
      }

      if (addSource) {
        switch (key.name) {
          case "escape":
            setAddSource(null)
            return true
          case "up":
          case "k":
            setAddSource((current) => (current ? { selectedIndex: Math.max(0, current.selectedIndex - 1) } : current))
            return true
          case "down":
          case "j":
            setAddSource((current) => (current ? { selectedIndex: Math.min(2, current.selectedIndex + 1) } : current))
            return true
          case "return":
          case "enter":
            void chooseAddSource(addSource.selectedIndex)
            return true
          default:
            return true
        }
      }

      if (tailscaleImporter) {
        switch (key.name) {
          case "escape":
            setTailscaleImporter(null)
            setAddSource({ selectedIndex: 2 })
            return true
          case "up":
          case "k":
            setTailscaleImporter((current) =>
              current ? { ...current, selectedIndex: Math.max(0, current.selectedIndex - 1) } : current,
            )
            return true
          case "down":
          case "j":
            setTailscaleImporter((current) =>
              current
                ? { ...current, selectedIndex: Math.min(Math.max(0, visiblePeers.length - 1), current.selectedIndex + 1) }
                : current,
            )
            return true
          case "h":
            setTailscaleImporter((current) =>
              current ? { ...current, showTagHelp: !current.showTagHelp } : current,
            )
            return true
          case "r":
            void openTailscaleImport(true)
            return true
          case "return":
          case "enter":
            if (visiblePeers.length > 0) {
              const peer = visiblePeers[tailscaleImporter.selectedIndex]
              setTailscaleImporter(null)
              setEditor({ field: "label", form: editorFormFromTailscalePeer(peer) })
            }
            return true
          default:
            return false
        }
      }

      if (importer) {
        switch (key.name) {
          case "escape":
            setImporter(null)
            setAddSource({ selectedIndex: 1 })
            return true
          case "up":
          case "k":
            setImporter((current) => (current ? { ...current, selectedIndex: Math.max(0, current.selectedIndex - 1) } : current))
            return true
          case "down":
          case "j":
            setImporter((current) =>
              current
                ? {
                    ...current,
                    selectedIndex: Math.min(Math.max(0, current.hosts.length - 1), current.selectedIndex + 1),
                  }
                : current,
            )
            return true
          case "r":
            void openSSHImport(true)
            return true
          case "return":
          case "enter":
            if (importer.hosts.length > 0) {
              const host = importer.hosts[importer.selectedIndex]
              setImporter(null)
              setEditor({ field: "label", form: editorFormFromSSHHost(host) })
            }
            return true
          default:
            return true
        }
      }

      if (editor) {
        switch (key.name) {
          case "tab":
            moveEditorField(key.shift ? -1 : 1)
            return true
          case "escape":
            if (editor.form.mode === "create") {
              setEditor(null)
              setAddSource({ selectedIndex: sourceIndexForConnectionType(editor.form.connectionType) })
              return true
            }
            setEditor(null)
            return true
          case "return":
          case "enter":
            void saveEditor()
            return true
          default:
            return false
        }
      }

      if (viewMode === "detail") {
        switch (key.name) {
          case "escape":
            setViewMode("list")
            return true
          case "up":
          case "k":
            moveSelection(-1)
            return true
          case "down":
          case "j":
            moveSelection(1)
            return true
          case "T":
            void runBulkTest()
            return true
          case "t":
            if (key.shift) {
              void runBulkTest()
            } else {
              void runTest()
            }
            return true
          case "U":
            void runBulkUpgrade()
            return true
          case "u":
            if (key.shift) {
              void runBulkUpgrade()
            } else {
              void runUpgrade()
            }
            return true
          case "x":
            if (selectedDevice) {
              setDeleteCandidateId(selectedDevice.id)
              return true
            }
            return false
          case "g":
            void openGrafana()
            return true
          default:
            return false
        }
      }

      switch (key.name) {
        case "up":
        case "k":
          moveSelection(-1)
          return true
        case "down":
        case "j":
          moveSelection(1)
          return true
        case "return":
        case "enter":
          return Boolean(selectedDevice) && (setViewMode("detail"), true)
        case "a":
          setAddSource({ selectedIndex: 0 })
          return true
        case "e":
          if (selectedDevice) {
            setEditor({ field: "label", form: editorFormFromDevice(selectedDevice) })
            return true
          }
          return false
        case "T":
          void runBulkTest()
          return true
        case "t":
          if (key.shift) {
            void runBulkTest()
          } else {
            void runTest()
          }
          return true
        case "U":
          void runBulkUpgrade()
          return true
        case "u":
          if (key.shift) {
            void runBulkUpgrade()
          } else {
            void runUpgrade()
          }
          return true
        case "x":
          if (selectedDevice) {
            setDeleteCandidateId(selectedDevice.id)
            return true
          }
          return false
        case "r":
          void refreshDevices()
          return true
        default:
          return false
      }
    },
    setEditorValue(field: DeviceFormField, value: string) {
      setEditor((current) => {
        if (!current) {
          return current
        }

        const nextForm = {
          ...current.form,
          [field]: value,
        }

        if (field === "authMethod") {
          const nextMethod = value as DeviceEditorForm["authMethod"]
          nextForm.authMethod = nextMethod
          if (nextMethod !== "key") {
            nextForm.sshKeyPassphrase = ""
          }
          if (nextMethod !== "password") {
            nextForm.sshPassword = ""
          }
          if (nextMethod === "agent") {
            nextForm.sshKey = ""
          }

          return {
            field: nextMethod === "key" ? "sshKey" : nextMethod === "password" ? "sshPassword" : "sshPort",
            form: nextForm,
          }
        }

        return {
          ...current,
          form: nextForm,
        }
      })
    },
    setTailscaleQuery(value: string) {
      setTailscaleImporter((current) =>
        current
          ? {
              ...current,
              query: value,
              selectedIndex: 0,
            }
          : current,
      )
    },
    selectAddSource(index: number) {
      void chooseAddSource(index)
    },
  }

  function moveSelection(delta: number) {
    if (devices.length === 0) {
      return
    }

    const currentIndex = selectedDevice ? devices.findIndex((device) => device.id === selectedDevice.id) : 0
    const nextIndex = (Math.max(0, currentIndex) + delta + devices.length) % devices.length
    setSelectedDeviceId(devices[nextIndex].id)
  }

  function moveEditorField(delta: number) {
    setEditor((current) => {
      if (!current) {
        return current
      }
      const fields = visibleFormFields(current.form)
      const currentIndex = fields.findIndex((field) => field === current.field)
      const nextIndex = (currentIndex + delta + fields.length) % fields.length
      return {
        ...current,
        field: fields[nextIndex],
      }
    })
  }

  async function chooseAddSource(index: number) {
    switch (index) {
      case 1:
        await openSSHImport(false)
        return
      case 2:
        await openTailscaleImport(false)
        return
      default:
        setAddSource(null)
        setEditor({ field: "label", form: emptyDeviceEditorForm() })
    }
  }

  async function openSSHImport(reload: boolean) {
    if (pendingAction === "discovering hosts" && reload) {
      return
    }

    setPendingAction("discovering hosts")
    setAddSource(null)
    if (!reload) {
      setImporter({ hosts: [], loading: true, selectedIndex: 0 })
    } else {
      setImporter((current) => ({ hosts: current?.hosts ?? [], loading: true, selectedIndex: current?.selectedIndex ?? 0 }))
    }

    try {
      const response = await getSSHConfigHosts()
      setImporter({ hosts: response.hosts, loading: false, selectedIndex: 0 })
      if (response.hosts.length === 0) {
        setNotice({ level: "warning", message: "No SSH config hosts were found." })
      }
    } catch (cause) {
      setImporter({ hosts: [], loading: false, selectedIndex: 0, error: cause instanceof Error ? cause.message : "failed to load ssh config hosts" })
    } finally {
      setPendingAction(null)
    }
  }

  async function openTailscaleImport(reload: boolean) {
    if (pendingAction === "discovering tailscale peers" && reload) {
      return
    }

    setPendingAction("discovering tailscale peers")
    setAddSource(null)
    if (!reload) {
      setTailscaleImporter({ peers: [], query: "", selectedIndex: 0, showTagHelp: false, status: null })
    }

    try {
      const status = await getTailscaleStatus()
      if (!status.installed || !status.running) {
        setTailscaleImporter({
          peers: [],
          query: "",
          selectedIndex: 0,
          showTagHelp: false,
          status,
          error: status.error,
        })
        return
      }

      const peers = await getTailscalePeers()
      setTailscaleImporter({
        peers: peers.peers,
        query: reload && tailscaleImporter ? tailscaleImporter.query : "",
        selectedIndex: 0,
        showTagHelp: reload && tailscaleImporter ? tailscaleImporter.showTagHelp : false,
        status,
      })
      if (peers.peers.length === 0) {
        setNotice({ level: "warning", message: "No online Tailscale peers were found." })
      }
    } catch (cause) {
      setTailscaleImporter({
        peers: [],
        query: "",
        selectedIndex: 0,
        showTagHelp: false,
        status: null,
        error: cause instanceof Error ? cause.message : "failed to load tailscale peers",
      })
    } finally {
      setPendingAction(null)
    }
  }

  async function refreshDevices() {
    try {
      const response = await getDevices()
      setDevices(response.devices)
      setError(undefined)
      setStatus("ready")
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "failed to load devices")
      setStatus("error")
    }
  }

  async function saveEditor() {
    if (!editor || pendingAction) {
      return
    }

    const validation = validateForm(editor.form)
    if (validation) {
      setNotice({ level: "error", message: validation })
      return
    }

    setNotice(null)

    const request = requestFromForm(editor.form)

    try {
      if (editor.form.mode === "create" && (!request.agent_token || request.agent_token.trim() === "")) {
        setMonitoringPrompt({ form: editor.form })
        setEditor(null)
        return
      }

      setPendingAction(editor.form.mode === "create" ? "creating device" : "saving device")

      const saved =
        editor.form.mode === "create"
          ? await createDevice(request)
          : await updateDevice(editor.form.id!, request)

      setEditor(null)
      setAddSource(null)
      setSelectedDeviceId(saved.id)
      await refreshDevices()
      setNotice({
        level: "success",
        message:
          editor.form.mode === "create"
            ? `Added ${saved.label || saved.id}`
            : `Saved ${saved.label || saved.id}`,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to save device" })
    } finally {
      setPendingAction(null)
    }
  }

  async function runTest() {
    if (!selectedDevice || pendingAction) {
      return
    }

    setPendingAction("testing device")
    setNotice(null)

    try {
      const result = await testDevice(selectedDevice.id)
      await refreshDevices()
      setNotice({
        level: result.agent_ok ? "success" : result.ssh_ok ? "warning" : "error",
        message: result.message,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to test device" })
    } finally {
      setPendingAction(null)
    }
  }

  async function runUpgrade() {
    if (!selectedDevice || pendingAction) {
      return
    }

    setPendingAction("upgrading device")
    setNotice(null)

    try {
      const result = await upgradeDevice(selectedDevice.id)
      await refreshDevices()
      setNotice({
        level: result.ok ? "success" : "error",
        message: result.message,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to upgrade device" })
    } finally {
      setPendingAction(null)
    }
  }

  async function runBulkTest() {
    if (pendingAction) {
      return
    }

    setPendingAction("testing all devices")
    setNotice(null)

    try {
      const response = await testAllDevices()
      await refreshDevices()
      const online = response.results.filter((result) => result.agent_ok).length
      const sshOnly = response.results.filter((result) => result.ssh_ok && !result.agent_ok).length
      const failed = response.results.length - online - sshOnly
      setNotice({
        level: failed > 0 ? "warning" : "success",
        message: `Bulk test complete: ${online} online, ${sshOnly} SSH-only, ${failed} failed`,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to test all devices" })
    } finally {
      setPendingAction(null)
    }
  }

  async function runBulkUpgrade() {
    if (pendingAction) {
      return
    }

    setPendingAction("upgrading all devices")
    setNotice(null)

    try {
      const response = await upgradeAllDevices()
      await refreshDevices()
      const succeeded = response.results.filter((result) => result.ok).length
      const failed = response.results.length - succeeded
      setNotice({
        level: failed > 0 ? "warning" : "success",
        message: `Bulk upgrade complete: ${succeeded} succeeded, ${failed} failed`,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to upgrade all devices" })
    } finally {
      setPendingAction(null)
    }
  }

  async function openGrafana() {
    if (!selectedDevice) {
      return
    }

    const grafanaService = fleet.snapshot.services.find(
      (service) => service.deviceId === selectedDevice.id && (service.name.includes("grafana") || service.image.includes("grafana")),
    )
    if (!grafanaService || grafanaService.port <= 0) {
      setNotice({ level: "warning", message: "No Grafana service detected on this device." })
      return
    }

    const url = `http://${selectedDevice.host}:${grafanaService.port}`
    try {
      Bun.spawn(["xdg-open", url], {
        stdout: "ignore",
        stderr: "ignore",
      })
      setNotice({ level: "info", message: `Opening Grafana at ${url}` })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to open Grafana" })
    }
  }

  async function confirmDelete() {
    if (!deleteCandidate || pendingAction) {
      return
    }

    setPendingAction("removing device")
    setNotice(null)

    try {
      const result = await deleteDevice(deleteCandidate.id)
      setDeleteCandidateId(null)
      await refreshDevices()
      setNotice({
        level: "success",
        message:
          result.removed_services > 0
            ? `Removed ${deleteCandidate.label || deleteCandidate.id} and ${result.removed_services} service entr${result.removed_services === 1 ? "y" : "ies"}`
            : `Removed ${deleteCandidate.label || deleteCandidate.id}`,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to remove device" })
    } finally {
      setPendingAction(null)
    }
  }

  async function submitBootstrapWithMonitoring(installMonitoring: boolean) {
    if (!monitoringPrompt || pendingAction) {
      return
    }

    setPendingAction(installMonitoring ? "bootstrapping device + monitoring" : "bootstrapping device")
    setNotice(null)

    const form = monitoringPrompt.form
    const request = bootstrapRequestFromForm(form)

    try {
      const response = await bootstrapDevice({
        ...request,
        install_monitoring: installMonitoring,
      })

      setMonitoringPrompt(null)
      setAddSource(null)
      setSelectedDeviceId(response.device.id)
      await refreshDevices()
      setNotice({
        level: "success",
        message: response.message,
      })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to bootstrap device" })
      setMonitoringPrompt({ form })
    } finally {
      setPendingAction(null)
    }
  }
}

function validateForm(form: DeviceEditorForm) {
  if (form.host.trim() === "") {
    return "Host is required."
  }

  if (form.agentToken.trim() === "" && form.sshUser.trim() === "") {
    return "SSH user is required to bootstrap a new device."
  }

  if (form.agentToken.trim() === "") {
    if (form.authMethod === "key" && form.sshKey.trim() === "") {
      return "SSH key path is required for key-file auth."
    }
    if (form.authMethod === "password" && form.sshPassword.trim() === "") {
      return "SSH password is required for password auth."
    }
  }

  const sshPort = Number.parseInt(form.sshPort.trim(), 10)
  if (!Number.isFinite(sshPort) || sshPort <= 0 || sshPort > 65535) {
    return "SSH port must be between 1 and 65535."
  }

  const agentPort = Number.parseInt(form.agentPort.trim(), 10)
  if (!Number.isFinite(agentPort) || agentPort <= 0 || agentPort > 65535) {
    return "Agent port must be between 1 and 65535."
  }

  return null
}

function requestFromForm(form: DeviceEditorForm): DeviceRequest {
  const usingExistingAgent = form.agentToken.trim() !== ""

  return {
    label: form.label.trim(),
    host: form.host.trim(),
    ssh_user: form.sshUser.trim(),
    ssh_key: form.authMethod === "key" || usingExistingAgent ? form.sshKey.trim() : "",
    ssh_port: Number.parseInt(form.sshPort.trim(), 10),
    connection_type: form.connectionType,
    agent_port: Number.parseInt(form.agentPort.trim(), 10),
    agent_token: form.agentToken.trim(),
    tags: form.tagsText
      .split(",")
      .map((tag) => tag.trim())
      .filter(Boolean),
  }
}

function bootstrapRequestFromForm(form: DeviceEditorForm) {
  return {
    ...requestFromForm(form),
    ssh_key_passphrase: form.authMethod === "key" ? form.sshKeyPassphrase : "",
    ssh_password: form.authMethod === "password" ? form.sshPassword : "",
  }
}

export type DevicesController = ReturnType<typeof useDevicesController>

function visibleFormFields(form: DeviceEditorForm) {
  const fields: DeviceFormField[] = ["label", "host", "sshUser", "authMethod"]

  if (form.authMethod === "key") {
    fields.push("sshKey", "sshKeyPassphrase")
  }
  if (form.authMethod === "password") {
    fields.push("sshPassword")
  }

  fields.push("sshPort", "agentPort", "agentToken", "tagsText")
  return fields
}

function sourceIndexForConnectionType(connectionType: string) {
  switch (connectionType) {
    case "ssh-config":
      return 1
    case "tailscale":
      return 2
    default:
      return 0
  }
}
