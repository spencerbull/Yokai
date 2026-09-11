export type HFSettings = {
  configured: boolean
  source: "none" | "env" | "config"
  username?: string
}

export type Preferences = {
  theme: string
  default_vllm_image: string
  default_sglang_image: string
  default_llama_image: string
  default_comfyui_image: string
}

export type DeployHistory = {
  images: string[]
  models: string[]
}

export type IntegrationToolStatus = {
  available: boolean
  configured: boolean
  path?: string
}

export type SettingsDocument = {
  hf: HFSettings
  preferences: Preferences
  history: DeployHistory
  integrations: {
    vscode: IntegrationToolStatus
    opencode: IntegrationToolStatus
    openclaw: IntegrationToolStatus
    claudecode: IntegrationToolStatus
    codex: IntegrationToolStatus
  }
}

export type SettingsPatch = {
  preferences?: Partial<Preferences>
}

export type HFTokenValidation = {
  valid: boolean
  username?: string
}

export type OpenAIEndpoint = {
  service_id: string
  device_id: string
  device_label: string
  host: string
  port: number
  base_url: string
  service_type: string
  model_ids: string[]
  display_name: string
  reachable: boolean
}

export type IntegrationsConfigureRequest = {
  tools: string[]
}

export type IntegrationsConfigureResponse = {
  results: Array<{
    name: string
    ok: boolean
    err?: string
  }>
}

// Normalize a settings document fetched from the daemon so that history
// (images/models) is always an array. Some daemon versions marshal an absent
// history.json to JSON null for these fields, which would crash TUI
// render-time `.slice()` calls. Apply whenever storing a settings document.
export function normalizeSettingsDocument(settings: SettingsDocument): SettingsDocument {
  const history = settings.history ?? { images: [], models: [] }
  return {
    ...settings,
    history: {
      images: Array.isArray(history.images) ? history.images : [],
      models: Array.isArray(history.models) ? history.models : [],
    },
  }
}
