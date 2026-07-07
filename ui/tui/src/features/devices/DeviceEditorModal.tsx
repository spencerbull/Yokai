import type { ReactNode } from "react"

import type { DeviceEditorForm, DeviceFormField } from "../../contracts/devices"
import { useTheme } from "../../theme/context"

const AUTH_METHODS = [
  { id: "agent", label: "SSH Agent" },
  { id: "key", label: "Key File" },
  { id: "password", label: "Password" },
] as const

type DeviceEditorModalProps = {
  field: DeviceFormField
  form: DeviceEditorForm
  onChange: (field: DeviceFormField, value: string) => void
}

export function DeviceEditorModal(props: DeviceEditorModalProps) {
  const theme = useTheme()

  return (
    <box
      position="absolute"
      left={0}
      top={0}
      width="100%"
      height="100%"
      justifyContent="center"
      alignItems="center"
    >
      <box
        width={72}
        border
        borderStyle="double"
        borderColor={theme.colors.borderStrong}
        backgroundColor={theme.colors.panel}
        padding={1}
        flexDirection="column"
        gap={1}
      >
        <text fg={theme.colors.text}>
          <strong>{props.form.mode === "create" ? "Add Device" : "Edit Device"}</strong>
        </text>
        <text fg={theme.colors.textSubtle}>Connection type: {props.form.connectionType}</text>
        <text fg={theme.colors.textSubtle}>Leave Agent Token blank to bootstrap the remote Yokai agent automatically.</text>
        <text fg={theme.colors.textSubtle}>{authMethodHint(props.form.authMethod)}</text>

        <Field label="Label" active={props.field === "label"}>
          <input value={props.form.label} onInput={(value) => props.onChange("label", value)} focused={props.field === "label"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="darkporgs" />
        </Field>
        <Field label="Host / DNS / IP" active={props.field === "host"}>
          <input value={props.form.host} onInput={(value) => props.onChange("host", value)} focused={props.field === "host"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="gpu-box.tailnet.ts.net" />
        </Field>
        <Field label="SSH User" active={props.field === "sshUser"}>
          <input value={props.form.sshUser} onInput={(value) => props.onChange("sshUser", value)} focused={props.field === "sshUser"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="root" />
        </Field>
        <Field label="Auth Method" active={props.field === "authMethod"}>
          <box flexDirection="row" gap={1}>
            {AUTH_METHODS.map((method) => {
              const selected = props.form.authMethod === method.id
              const active = props.field === "authMethod" && selected
              return (
                <box
                  key={method.id}
                  border
                  borderStyle={selected ? "double" : "single"}
                  borderColor={selected ? theme.colors.borderStrong : theme.colors.border}
                  backgroundColor={theme.colors.panelMuted}
                  paddingX={1}
                  onMouseDown={() => props.onChange("authMethod", method.id)}
                >
                  <text fg={selected ? theme.colors.accent : active ? theme.colors.text : theme.colors.textMuted}>{selected ? `▸ ${method.label}` : method.label}</text>
                </box>
              )
            })}
          </box>
        </Field>

        {props.form.authMethod === "key" ? (
          <>
            <Field label="SSH Key" active={props.field === "sshKey"}>
              <input value={props.form.sshKey} onInput={(value) => props.onChange("sshKey", value)} focused={props.field === "sshKey"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="~/.ssh/id_ed25519" />
            </Field>
            <Field label="Key Passphrase" active={props.field === "sshKeyPassphrase"}>
              <input value={props.form.sshKeyPassphrase} onInput={(value) => props.onChange("sshKeyPassphrase", value)} focused={props.field === "sshKeyPassphrase"} password width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="leave blank if key is not encrypted" />
            </Field>
          </>
        ) : null}

        {props.form.authMethod === "password" ? (
          <Field label="SSH Password" active={props.field === "sshPassword"}>
            <input value={props.form.sshPassword} onInput={(value) => props.onChange("sshPassword", value)} focused={props.field === "sshPassword"} password width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="remote account password" />
          </Field>
        ) : null}

        <box flexDirection="row" gap={1}>
          <Field label="SSH Port" active={props.field === "sshPort"}>
            <input value={props.form.sshPort} onInput={(value) => props.onChange("sshPort", value)} focused={props.field === "sshPort"} width={10} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="22" />
          </Field>
          <Field label="Agent Port" active={props.field === "agentPort"}>
            <input value={props.form.agentPort} onInput={(value) => props.onChange("agentPort", value)} focused={props.field === "agentPort"} width={10} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="7474" />
          </Field>
        </box>
        <Field label="Agent Token" active={props.field === "agentToken"}>
          <input value={props.form.agentToken} onInput={(value) => props.onChange("agentToken", value)} focused={props.field === "agentToken"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="optional for already bootstrapped devices" />
        </Field>
        <Field label="Tags" active={props.field === "tagsText"}>
          <input value={props.form.tagsText} onInput={(value) => props.onChange("tagsText", value)} focused={props.field === "tagsText"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panelMuted} cursorColor={theme.colors.accent} placeholder="tag:ai-gpu, lab" />
        </Field>

        <text fg={theme.colors.textSubtle}>Tab cycles fields. Enter saves. Esc cancels.</text>
      </box>
    </box>
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

function authMethodHint(authMethod: DeviceEditorForm["authMethod"]) {
  switch (authMethod) {
    case "key":
      return "Key File uses the specified private key and optional passphrase during bootstrap."
    case "password":
      return "Password auth sends the remote account password only for bootstrap; it is not stored in config."
    default:
      return "SSH Agent uses your loaded SSH_AUTH_SOCK credentials during bootstrap."
  }
}
