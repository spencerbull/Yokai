import type { TailscalePeer, TailscaleStatus } from "../../contracts/devices"
import { useTheme } from "../../theme/context"

type TailscaleImportModalProps = {
  error?: string
  peers: TailscalePeer[]
  query: string
  selectedIndex: number
  showTagHelp: boolean
  status: TailscaleStatus | null
  visiblePeers: TailscalePeer[]
  onQueryChange: (value: string) => void
}

export function TailscaleImportModal(props: TailscaleImportModalProps) {
  const theme = useTheme()
  const status = props.status

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
        width={84}
        border
        borderStyle="double"
        borderColor={theme.colors.borderStrong}
        backgroundColor={theme.colors.panel}
        padding={1}
        flexDirection="column"
        gap={1}
      >
        <text fg={theme.colors.text}>
          <strong>Import From Tailscale</strong>
        </text>

        {status ? <StatusLine status={status} /> : <text fg={theme.colors.textMuted}>Checking Tailscale status...</text>}
        {props.error ? <text fg={theme.colors.danger}>{props.error}</text> : null}

        {status && status.installed && status.running ? (
          <>
            <text fg={theme.colors.textSubtle}>Type to filter by hostname, IP, OS, or tag. Up/Down choose a peer. Enter imports it into the device form.</text>
            <input
              value={props.query}
              onInput={props.onQueryChange}
              focused
              width={58}
              backgroundColor={theme.colors.panelMuted}
              textColor={theme.colors.text}
              focusedBackgroundColor={theme.colors.selectionBackground}
              cursorColor={theme.colors.selectionText}
              placeholder="Filter hostname, IP, OS, or tag"
            />

            {props.visiblePeers.length === 0 ? (
              <text fg={theme.colors.textSubtle}>No online peers match the current filter.</text>
            ) : (
              props.visiblePeers.map((peer, index) => {
                const selected = index === props.selectedIndex
                return (
                  <box key={`${peer.hostname}-${peer.ip}`} flexDirection="column">
                    <text fg={selected ? theme.colors.selectionText : theme.colors.textMuted} bg={selected ? theme.colors.selectionBackground : theme.colors.panel}>
                      {selected ? "▌" : " "} {peer.recommended ? recommendedBadge() + " " : ""}{truncate(peer.hostname, 18)}
                    </text>
                    <text fg={selected ? theme.colors.selectionText : theme.colors.textSubtle} bg={selected ? theme.colors.selectionBackground : theme.colors.panel}>
                      {truncate(renderPeerMeta(peer), 74)}
                    </text>
                  </box>
                )
              })
            )}
          </>
        ) : null}

        {status && !status.installed && status.install_instructions ? (
          <InstructionBlock title="Install Tailscale" text={status.install_instructions} />
        ) : null}

        {status && status.tag_help && props.showTagHelp ? (
          <InstructionBlock title="Yokai Tailscale Tagging" text={status.tag_help} />
        ) : null}

        <text fg={theme.colors.textSubtle}>Keys: Up/Down navigate · Enter import · H tag help · R refresh · Esc cancel</text>
      </box>
    </box>
  )

  function recommendedBadge() {
    return "AI GPU"
  }
}

function StatusLine(props: { status: TailscaleStatus }) {
  const theme = useTheme()

  if (!props.status.installed) {
    return <text fg={theme.colors.warning}>Tailscale CLI is not installed on this machine.</text>
  }

  if (props.status.needs_login) {
    return <text fg={theme.colors.warning}>Tailscale needs login or machine auth. Run `sudo tailscale up` and retry.</text>
  }

  if (!props.status.running) {
    return <text fg={theme.colors.warning}>{props.status.error || `Tailscale is not connected (${props.status.backend_state || "unknown"}).`}</text>
  }

  const self = props.status.self
  const summary = self?.hostname || self?.dns_name || self?.ip || "tailnet"
  return <text fg={theme.colors.success}>Connected to Tailscale as {summary}.</text>
}

function InstructionBlock(props: { text: string; title: string }) {
  const theme = useTheme()

  return (
    <box border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column">
      <text fg={theme.colors.text}><strong>{props.title}</strong></text>
      {props.text.split("\n").map((line, index) => (
        <text key={`${props.title}-${index}`} fg={theme.colors.textMuted}>{line || " "}</text>
      ))}
    </box>
  )
}

function renderPeerMeta(peer: TailscalePeer) {
  const parts = [peer.ip, peer.os || "unknown"]
  if (peer.recommended) {
    parts.push("recommended for Yokai", "tag:ai-gpu")
  }
  if (peer.other_tags && peer.other_tags.length > 0) {
    parts.push(`tags: ${peer.other_tags.join(", ")}`)
  } else if (!peer.recommended) {
    parts.push("no tags")
  }

  return parts.join("  •  ")
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
