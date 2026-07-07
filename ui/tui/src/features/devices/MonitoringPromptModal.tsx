import { useTheme } from "../../theme/context"

export function MonitoringPromptModal() {
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
          <strong>Deploy monitoring stack?</strong>
        </text>
        <text fg={theme.colors.textMuted}>
          Yokai can also install Prometheus, Grafana, and Node Exporter during bootstrap.
        </text>
        <text fg={theme.colors.textSubtle}>
          If the target has NVIDIA GPUs, GPU monitoring will be included too.
        </text>
        <text fg={theme.colors.textSubtle}>
          <span fg={theme.colors.textMuted}>Y</span> install monitoring  <span fg={theme.colors.textMuted}>N</span> skip monitoring  <span fg={theme.colors.textMuted}>Esc</span> go back
        </text>
      </box>
    </box>
  )
}
