import type { SSHConfigHost } from "../../contracts/devices"
import { useTheme } from "../../theme/context"

type SSHImportModalProps = {
  error?: string
  hosts: SSHConfigHost[]
  loading: boolean
  selectedIndex: number
}

export function SSHImportModal(props: SSHImportModalProps) {
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
          <strong>Import From SSH Config</strong>
        </text>
        <text fg={theme.colors.textSubtle}>Up/Down choose a host. Enter imports it into the editor form. Esc cancels.</text>

        {props.loading ? <text fg={theme.colors.textMuted}>Loading ~/.ssh/config hosts from the daemon...</text> : null}
        {props.error ? <text fg={theme.colors.danger}>{props.error}</text> : null}

        {!props.loading && !props.error && props.hosts.length === 0 ? (
          <text fg={theme.colors.textSubtle}>No SSH config hosts were found.</text>
        ) : null}

        {props.hosts.map((host, index) => {
          const selected = index === props.selectedIndex
          const detail = `${host.user || "root"}@${host.host}:${host.port}`
          const suffix = host.identity_file_encrypted ? " encrypted-key" : host.identity_file ? " key" : ""

          return (
            <text key={`${host.alias}-${host.host}`} fg={selected ? theme.colors.selectionText : theme.colors.textMuted} bg={selected ? theme.colors.selectionBackground : theme.colors.panel}>
              {selected ? "▌" : " "} {truncate(host.alias, 18)} <span fg={selected ? theme.colors.selectionText : theme.colors.textSubtle}>{truncate(detail, 30)}{suffix}</span>
            </text>
          )
        })}
      </box>
    </box>
  )
}

function truncate(value: string, width: number) {
  if (value.length <= width) {
    return value
  }
  if (width <= 1) {
    return value.slice(0, width)
  }
  return `${value.slice(0, width - 1)}…`
}
