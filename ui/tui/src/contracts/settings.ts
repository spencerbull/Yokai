export type HFSettings = {
  configured: boolean
  source: "none" | "env" | "config"
  username?: string
}

export type Preferences = {
  theme: string
  default_vllm_image: string
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
  }
}

export type SettingsPatch = {
  preferences?: Partial<Preferences>
}
