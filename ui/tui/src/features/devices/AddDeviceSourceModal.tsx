import { useTheme } from "../../theme/context"

type AddDeviceSourceModalProps = {
  onSelect: (index: number) => void
  selectedIndex: number
}

export const ADD_DEVICE_SOURCES = [
  {
    title: "Manual",
    description: "Type the host, SSH details, and agent port yourself.",
  },
  {
    title: "SSH Config",
    description: "Import a host from ~/.ssh/config and review the connection details.",
  },
  {
    title: "Tailscale",
    description: "Browse online Tailscale peers with Yokai AI GPU tag highlighting.",
  },
] as const

export function AddDeviceSourceModal(props: AddDeviceSourceModalProps) {
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
          <strong>Add Device</strong>
        </text>
        <text fg={theme.colors.textSubtle}>Choose how you want to add the device. Up/Down move, Enter continues, Esc cancels.</text>

        {ADD_DEVICE_SOURCES.map((source, index) => {
          const selected = index === props.selectedIndex
          return (
            <box
              key={source.title}
              focusable
              border
              borderStyle={selected ? "double" : "single"}
              borderColor={selected ? theme.colors.borderStrong : theme.colors.border}
              backgroundColor={selected ? theme.colors.selectionBackground : theme.colors.panelMuted}
              padding={1}
              flexDirection="column"
              onMouseDown={() => props.onSelect(index)}
            >
              <text fg={selected ? theme.colors.selectionText : theme.colors.text}>
                <strong>{source.title}</strong>
              </text>
              <text fg={selected ? theme.colors.selectionText : theme.colors.textMuted}>{source.description}</text>
            </box>
          )
        })}
      </box>
    </box>
  )
}
