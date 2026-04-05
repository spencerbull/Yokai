import { useEffect, useState } from "react"

import type { SettingsDocument } from "../../contracts/settings"
import { configureIntegrations, getOpenAIEndpoints, getSettings, patchSettings, putHFToken, validateHFToken } from "../../services/daemon-client"
import type { ThemeState } from "../../theme/types"

type DefaultsEditor = {
  field: "vllm" | "llama" | "comfy"
  values: {
    comfy: string
    llama: string
    vllm: string
  }
}

type HFEditor = {
  token: string
}

type SettingsNotice = {
  level: "info" | "success" | "warning" | "error"
  message: string
}

type KeyLike = {
  name: string
  shift?: boolean
}

const DEFAULT_SETTINGS: SettingsDocument = {
  hf: { configured: false, source: "none" },
  preferences: {
    theme: "auto",
    default_vllm_image: "",
    default_llama_image: "",
    default_comfyui_image: "",
  },
  history: { images: [], models: [] },
  integrations: {
    vscode: { available: false, configured: false },
    opencode: { available: false, configured: false },
    openclaw: { available: false, configured: false },
    claudecode: { available: false, configured: false },
    codex: { available: false, configured: false },
  },
}

export function useSettingsController(active: boolean, theme: ThemeState) {
  const [settings, setSettings] = useState<SettingsDocument>(DEFAULT_SETTINGS)
  const [endpoints, setEndpoints] = useState<Awaited<ReturnType<typeof getOpenAIEndpoints>>>([])
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading")
  const [notice, setNotice] = useState<SettingsNotice | null>(null)
  const [pendingAction, setPendingAction] = useState<string | null>(null)
  const [hfEditor, setHFEditor] = useState<HFEditor | null>(null)
  const [defaultsEditor, setDefaultsEditor] = useState<DefaultsEditor | null>(null)
  const [selectedTools, setSelectedTools] = useState({ claudecode: false, codex: true, openclaw: true, opencode: true, vscode: true })

  useEffect(() => {
    if (!active) {
      return
    }
    let cancelled = false

    const load = async () => {
      try {
        const [settingsDoc, discoveredEndpoints] = await Promise.all([getSettings(), getOpenAIEndpoints().catch(() => [])])
        if (cancelled) {
          return
        }
        setSettings(settingsDoc)
        setEndpoints(discoveredEndpoints)
        setStatus("ready")
      } catch (cause) {
        if (cancelled) {
          return
        }
        setStatus("error")
        setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to load settings" })
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [active])

  useEffect(() => {
    if (!notice) {
      return
    }
    const timeout = setTimeout(() => {
      setNotice((current) => (current === notice ? null : current))
    }, 4500)
    return () => clearTimeout(timeout)
  }, [notice])

  return {
    defaultsEditor,
    endpoints,
    hfEditor,
    locksGlobalNav: Boolean(hfEditor || defaultsEditor),
    notice,
    openDefaultsEditor() {
      setDefaultsEditor({
        field: "vllm",
        values: {
          comfy: settings.preferences.default_comfyui_image,
          llama: settings.preferences.default_llama_image,
          vllm: settings.preferences.default_vllm_image,
        },
      })
    },
    openHFEditor() {
      setHFEditor({ token: "" })
    },
    pendingAction,
    runConfigureIntegrations: () => void runConfigureIntegrations(),
    selectedTools,
    settings,
    status,
    handleKey(key: KeyLike) {
      if (hfEditor) {
        switch (key.name) {
          case "escape":
            setHFEditor(null)
            return true
          case "return":
          case "enter":
            void saveHFToken()
            return true
          default:
            return false
        }
      }

      if (defaultsEditor) {
        switch (key.name) {
          case "tab":
            setDefaultsEditor((current) => (current ? { ...current, field: nextDefaultsField(current.field, key.shift ? -1 : 1) } : current))
            return true
          case "escape":
            setDefaultsEditor(null)
            return true
          case "return":
          case "enter":
            void saveDefaults()
            return true
          default:
            return false
        }
      }

      switch (key.name) {
        case "a":
          theme.setPreference("auto")
          return true
        case "d":
          theme.setPreference("dark")
          return true
        case "h":
          setHFEditor({ token: "" })
          return true
        case "p":
          setDefaultsEditor({
            field: "vllm",
            values: {
              comfy: settings.preferences.default_comfyui_image,
              llama: settings.preferences.default_llama_image,
              vllm: settings.preferences.default_vllm_image,
            },
          })
          return true
        case "v":
          toggleTool("vscode")
          return true
        case "o":
          toggleTool("opencode")
          return true
        case "l":
          toggleTool("openclaw")
          return true
        case "k":
          toggleTool("claudecode")
          return true
        case "x":
          toggleTool("codex")
          return true
        case "c":
          void runConfigureIntegrations()
          return true
        case "r":
          void refresh()
          return true
        default:
          return false
      }
    },
    refresh: () => void refresh(),
    saveDefaultsValue(field: DefaultsEditor["field"], value: string) {
      setDefaultsEditor((current) =>
        current
          ? {
              ...current,
              values: {
                ...current.values,
                [field]: value,
              },
            }
          : current,
      )
    },
    setHFTokenValue(value: string) {
      setHFEditor((current) => (current ? { token: value } : current))
    },
    toggleTool,
  }

  function toggleTool(tool: keyof typeof selectedTools) {
    setSelectedTools((current) => ({
      ...current,
      [tool]: !current[tool],
    }))
  }

  async function refresh() {
    setPendingAction("refreshing settings")
    try {
      const [settingsDoc, discoveredEndpoints] = await Promise.all([getSettings(), getOpenAIEndpoints().catch(() => [])])
      setSettings(settingsDoc)
      setEndpoints(discoveredEndpoints)
      setStatus("ready")
    } catch (cause) {
      setStatus("error")
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to refresh settings" })
    } finally {
      setPendingAction(null)
    }
  }

  async function saveHFToken() {
    if (!hfEditor || pendingAction) {
      return
    }
    const token = hfEditor.token.trim()
    if (token === "") {
      setNotice({ level: "error", message: "HF token is required" })
      return
    }

    setPendingAction("validating HF token")
    try {
      const validation = await validateHFToken(token)
      if (!validation.valid) {
        throw new Error("token validation failed")
      }
      await putHFToken(token)
      setHFEditor(null)
      await refresh()
      setNotice({ level: "success", message: `Saved Hugging Face token for ${validation.username || "account"}` })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to save HF token" })
    } finally {
      setPendingAction(null)
    }
  }

  async function saveDefaults() {
    if (!defaultsEditor || pendingAction) {
      return
    }

    setPendingAction("saving defaults")
    try {
      const next = await patchSettings({
        preferences: {
          default_comfyui_image: defaultsEditor.values.comfy.trim(),
          default_llama_image: defaultsEditor.values.llama.trim(),
          default_vllm_image: defaultsEditor.values.vllm.trim(),
        },
      })
      setSettings(next)
      setDefaultsEditor(null)
      setNotice({ level: "success", message: "Saved deploy defaults" })
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to save defaults" })
    } finally {
      setPendingAction(null)
    }
  }

  async function runConfigureIntegrations() {
    if (pendingAction) {
      return
    }
    const tools = Object.entries(selectedTools)
      .filter(([, selected]) => selected)
      .map(([tool]) => tool)
    if (tools.length === 0) {
      setNotice({ level: "warning", message: "Select at least one integration tool" })
      return
    }

    setPendingAction("configuring integrations")
    try {
      const response = await configureIntegrations({ tools })
      await refresh()
      const failed = response.results.filter((result) => !result.ok)
      if (failed.length > 0) {
        setNotice({ level: "warning", message: failed.map((result) => `${result.name}: ${result.err}`).join(" | ") })
      } else {
        setNotice({ level: "success", message: `Configured ${response.results.length} tool(s)` })
      }
    } catch (cause) {
      setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to configure integrations" })
    } finally {
      setPendingAction(null)
    }
  }
}

function nextDefaultsField(field: DefaultsEditor["field"], delta: number) {
  const fields: DefaultsEditor["field"][] = ["vllm", "llama", "comfy"]
  const index = fields.findIndex((entry) => entry === field)
  return fields[(index + delta + fields.length) % fields.length]
}

export type SettingsController = ReturnType<typeof useSettingsController>
