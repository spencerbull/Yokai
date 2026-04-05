import type { ReactNode } from "react"

import { useTheme } from "../../theme/context"
import type { SettingsController } from "./useSettingsController"

type SettingsRouteProps = {
  controller: SettingsController
}

export function SettingsRoute(props: SettingsRouteProps) {
  const theme = useTheme()
  const settings = props.controller.settings
  const endpointLines = props.controller.endpoints.slice(0, 4)

  return (
    <box flexDirection="column" gap={1} flexGrow={1}>
      {props.controller.notice ? <Banner color={noticeColor(theme, props.controller.notice.level)}>{props.controller.notice.message}</Banner> : null}
      {props.controller.pendingAction ? <Banner color={theme.colors.accent}>Running {props.controller.pendingAction}...</Banner> : null}

      <box flexDirection="row" gap={1}>
        <Card title="Theme">
          <ThemeChoice
            active={theme.preference === "auto"}
            description={theme.source === "omarchy" ? `Following Omarchy ${theme.themeName || "theme"}` : "Follow terminal/Omarchy automatically"}
            keys="A"
            label="Auto"
            onSelect={() => theme.setPreference("auto")}
          />
          <ThemeChoice
            active={theme.preference === "dark"}
            description="Use Yokai's built-in dark theme"
            keys="D"
            label="Dark"
            onSelect={() => theme.setPreference("dark")}
          />
        </Card>

        <Card title="Hugging Face">
          <Line label="Status" value={settings.hf.configured ? `configured (${settings.hf.source})` : "not configured"} />
          <Line label="User" value={settings.hf.username || "unknown"} />
          <ActionText keys="H" onSelect={props.controller.openHFEditor}>Edit HF token</ActionText>
        </Card>
      </box>

      <box flexDirection="row" gap={1}>
        <Card title="Deploy Defaults">
          <Line label="VLLM" value={settings.preferences.default_vllm_image} marquee />
          <Line label="Llama" value={settings.preferences.default_llama_image} marquee />
          <Line label="Comfy" value={settings.preferences.default_comfyui_image} marquee />
          <ActionText keys="P" onSelect={props.controller.openDefaultsEditor}>Edit defaults</ActionText>
        </Card>

        <Card title="Deploy History">
          <Line label="Images" value={settings.history.images.slice(0, 2).join(" · ") || "none"} marquee />
          <Line label="Models" value={settings.history.models.slice(0, 2).join(" · ") || "none"} marquee />
          <ActionText keys="R" onSelect={props.controller.refresh}>Refresh settings</ActionText>
        </Card>
      </box>

      <Card title="AI Integrations">
        <box flexDirection="row" gap={1}>
          <ToolToggle
            active={props.controller.selectedTools.vscode}
            configured={settings.integrations.vscode.configured}
            keys="V"
            label="VS Code"
            onSelect={() => props.controller.toggleTool("vscode")}
          />
          <ToolToggle
            active={props.controller.selectedTools.opencode}
            configured={settings.integrations.opencode.configured}
            keys="O"
            label="OpenCode"
            onSelect={() => props.controller.toggleTool("opencode")}
          />
          <ToolToggle
            active={props.controller.selectedTools.openclaw}
            configured={settings.integrations.openclaw.configured}
            keys="L"
            label="OpenClaw"
            onSelect={() => props.controller.toggleTool("openclaw")}
          />
          <ToolToggle
            active={props.controller.selectedTools.claudecode}
            configured={settings.integrations.claudecode.configured}
            keys="K"
            label="Claude Code"
            onSelect={() => props.controller.toggleTool("claudecode")}
          />
          <ToolToggle
            active={props.controller.selectedTools.codex}
            configured={settings.integrations.codex.configured}
            keys="X"
            label="Codex"
            onSelect={() => props.controller.toggleTool("codex")}
          />
        </box>

        {settings.integrations.claudecode.note ? <text fg={theme.colors.warning}>Claude Code: {settings.integrations.claudecode.note}</text> : null}

        <text fg={theme.colors.textSubtle}>OpenAI-compatible endpoints discovered: {props.controller.endpoints.length}</text>
        {endpointLines.map((endpoint) => (
          <text key={endpoint.service_id} fg={theme.colors.textMuted}>
            • {endpoint.display_name} <span fg={theme.colors.textSubtle}>{endpoint.base_url}</span>
          </text>
        ))}
        {props.controller.endpoints.length > endpointLines.length ? (
          <text fg={theme.colors.textSubtle}>Showing {endpointLines.length} of {props.controller.endpoints.length}</text>
        ) : null}

        <ActionText keys="C" onSelect={props.controller.runConfigureIntegrations}>Configure selected tools</ActionText>
      </Card>

      {props.controller.hfEditor ? <HFTokenModal controller={props.controller} /> : null}
      {props.controller.defaultsEditor ? <DefaultsModal controller={props.controller} /> : null}
    </box>
  )
}

function HFTokenModal(props: { controller: SettingsController }) {
  const theme = useTheme()
  const editor = props.controller.hfEditor
  if (!editor) {
    return null
  }

  return (
    <box position="absolute" left={0} top={0} width="100%" height="100%" justifyContent="center" alignItems="center">
      <box width={72} border borderStyle="double" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" gap={1}>
        <text fg={theme.colors.text}><strong>Save Hugging Face token</strong></text>
        <text fg={theme.colors.textSubtle}>Enter validates and saves. Esc closes.</text>
        <input
          value={editor.token}
          onInput={props.controller.setHFTokenValue}
          focused
          width={50}
          backgroundColor={theme.colors.panelMuted}
          textColor={theme.colors.text}
          focusedBackgroundColor={theme.colors.panelMuted}
          cursorColor={theme.colors.accent}
          placeholder="hf_..."
        />
      </box>
    </box>
  )
}

function DefaultsModal(props: { controller: SettingsController }) {
  const theme = useTheme()
  const editor = props.controller.defaultsEditor
  if (!editor) {
    return null
  }

  return (
    <box position="absolute" left={0} top={0} width="100%" height="100%" justifyContent="center" alignItems="center">
      <box width={76} border borderStyle="double" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" gap={1}>
        <text fg={theme.colors.text}><strong>Edit deploy defaults</strong></text>
        <text fg={theme.colors.textSubtle}>Tab cycles fields. Enter saves. Esc closes.</text>

        <Field label="Default VLLM image" active={editor.field === "vllm"}>
          <input value={editor.values.vllm} onInput={(value) => props.controller.saveDefaultsValue("vllm", value)} focused={editor.field === "vllm"} width={52} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="vllm/vllm-openai:latest" />
        </Field>
        <Field label="Default llama.cpp image" active={editor.field === "llama"}>
          <input value={editor.values.llama} onInput={(value) => props.controller.saveDefaultsValue("llama", value)} focused={editor.field === "llama"} width={52} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="ghcr.io/ggml-org/llama.cpp:server-cuda" />
        </Field>
        <Field label="Default ComfyUI image" active={editor.field === "comfy"}>
          <input value={editor.values.comfy} onInput={(value) => props.controller.saveDefaultsValue("comfy", value)} focused={editor.field === "comfy"} width={52} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="spencerbull/yokai-comfyui:latest" />
        </Field>
      </box>
    </box>
  )
}

function Card(props: { children: ReactNode; title: string }) {
  const theme = useTheme()

  return (
    <box flexGrow={1} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
      <text fg={theme.colors.text}><strong>{props.title}</strong></text>
      {props.children}
    </box>
  )
}

function ThemeChoice(props: { active: boolean; description: string; keys: string; label: string; onSelect: () => void }) {
  const theme = useTheme()
  return (
    <box border borderStyle={props.active ? "double" : "single"} borderColor={props.active ? theme.colors.borderStrong : theme.colors.border} backgroundColor={props.active ? theme.colors.selectionBackground : theme.colors.panel} padding={1} flexDirection="column" onMouseDown={props.onSelect}>
      <text fg={props.active ? theme.colors.selectionText : theme.colors.text}><strong>{props.label}</strong> <span fg={props.active ? theme.colors.selectionText : theme.colors.textSubtle}>[{props.keys}]</span></text>
      <text fg={props.active ? theme.colors.selectionText : theme.colors.textMuted}>{props.description}</text>
    </box>
  )
}

function ToolToggle(props: { active: boolean; configured: boolean; keys: string; label: string; onSelect: () => void }) {
  const theme = useTheme()
  return (
    <box border borderStyle={props.active ? "double" : "single"} borderColor={props.active ? theme.colors.borderStrong : theme.colors.border} backgroundColor={props.active ? theme.colors.selectionBackground : theme.colors.panel} padding={1} flexDirection="column" onMouseDown={props.onSelect}>
      <text fg={props.active ? theme.colors.selectionText : theme.colors.text}><strong>{props.label}</strong> <span fg={props.active ? theme.colors.selectionText : theme.colors.textSubtle}>[{props.keys}]</span></text>
      <text fg={props.active ? theme.colors.selectionText : theme.colors.textMuted}>{props.configured ? "configured" : "not configured"}</text>
    </box>
  )
}

function ActionText(props: { children: string; keys: string; onSelect: () => void }) {
  const theme = useTheme()
  return (
    <text fg={theme.colors.accent} onMouseDown={props.onSelect}>
      [{props.keys}] {props.children}
    </text>
  )
}

function Line(props: { label: string; marquee?: boolean; value: string }) {
  const theme = useTheme()
  return (
    <text fg={theme.colors.textMuted}>
      <span fg={theme.colors.textSubtle}>{props.label}:</span> {props.value}
    </text>
  )
}

function Field(props: { active: boolean; children: ReactNode; label: string }) {
  const theme = useTheme()
  return (
    <box flexDirection="column" gap={0}>
      <text fg={props.active ? theme.colors.borderStrong : theme.colors.textSubtle}>{props.label}</text>
      <box border borderStyle={props.active ? "double" : "single"} borderColor={props.active ? theme.colors.borderStrong : theme.colors.border} backgroundColor={theme.colors.panelMuted} paddingX={1}>
        {props.children}
      </box>
    </box>
  )
}

function Banner(props: { children: string; color: string }) {
  const theme = useTheme()
  return (
    <box border borderStyle="single" borderColor={props.color} backgroundColor={theme.colors.panelMuted} paddingX={1}>
      <text fg={props.color}>{props.children}</text>
    </box>
  )
}

function noticeColor(theme: ReturnType<typeof useTheme>, level: "info" | "success" | "warning" | "error") {
  switch (level) {
    case "success":
      return theme.colors.success
    case "warning":
      return theme.colors.warning
    case "error":
      return theme.colors.danger
    default:
      return theme.colors.accent
  }
}
