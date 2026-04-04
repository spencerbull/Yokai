import type { DeviceEditorForm, DeviceFormField } from "../../contracts/devices"
import { useTheme } from "../../theme/context"

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

        <Field label="Label" active={props.field === "label"}>
          <input value={props.form.label} onInput={(value) => props.onChange("label", value)} focused={props.field === "label"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="darkporgs" />
        </Field>
        <Field label="Host / IP" active={props.field === "host"}>
          <input value={props.form.host} onInput={(value) => props.onChange("host", value)} focused={props.field === "host"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="100.64.0.2" />
        </Field>
        <Field label="SSH User" active={props.field === "sshUser"}>
          <input value={props.form.sshUser} onInput={(value) => props.onChange("sshUser", value)} focused={props.field === "sshUser"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="root" />
        </Field>
        <Field label="SSH Key" active={props.field === "sshKey"}>
          <input value={props.form.sshKey} onInput={(value) => props.onChange("sshKey", value)} focused={props.field === "sshKey"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="~/.ssh/id_ed25519" />
        </Field>
        <box flexDirection="row" gap={1}>
          <Field label="SSH Port" active={props.field === "sshPort"}>
            <input value={props.form.sshPort} onInput={(value) => props.onChange("sshPort", value)} focused={props.field === "sshPort"} width={10} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="22" />
          </Field>
          <Field label="Agent Port" active={props.field === "agentPort"}>
            <input value={props.form.agentPort} onInput={(value) => props.onChange("agentPort", value)} focused={props.field === "agentPort"} width={10} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="7474" />
          </Field>
        </box>
        <Field label="Agent Token" active={props.field === "agentToken"}>
          <input value={props.form.agentToken} onInput={(value) => props.onChange("agentToken", value)} focused={props.field === "agentToken"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="optional for already bootstrapped devices" />
        </Field>
        <Field label="Tags" active={props.field === "tagsText"}>
          <input value={props.form.tagsText} onInput={(value) => props.onChange("tagsText", value)} focused={props.field === "tagsText"} width={46} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.selectionBackground} cursorColor={theme.colors.selectionText} placeholder="tag:ai-gpu, lab" />
        </Field>

        <text fg={theme.colors.textSubtle}>Tab cycles fields. Enter saves. Esc cancels.</text>
      </box>
    </box>
  )
}

function Field(props: { active: boolean; children: ReactNode; label: string }) {
  const theme = useTheme()

  return (
    <box flexDirection="column">
      <text fg={props.active ? theme.colors.borderStrong : theme.colors.textSubtle}>{props.label}</text>
      {props.children}
    </box>
  )
}
