import type { MetricSeries } from "../../contracts/fleet"
import { useTheme } from "../../theme/context"

type MetricVizProps = {
  compact?: boolean
  height?: number
  history: number[]
  label: string
  value: number
  width?: number
}

type VizTone = "default" | "success" | "warning" | "danger"

export function MetricViz(props: MetricVizProps) {
  const theme = useTheme()
  const tone = toneForPercent(props.value)
  const color = colorForTone(theme, tone)
  const width = Math.max(18, props.width ?? 30)
  const barWidth = Math.max(8, Math.min(18, Math.floor(width * 0.34)))
  const graphWidth = Math.max(10, width - barWidth - 7)
  const graphHeight = Math.max(2, props.height ?? (props.compact ? 2 : 3))
  const graphRows = dotGraph(props.history, graphWidth, graphHeight)

  return (
    <box flexDirection="column" gap={0}>
      <text fg={theme.colors.textMuted}>
        <span fg={theme.colors.textSubtle}>{props.label}</span> <span fg={color}>{props.value.toFixed(0)}%</span>
      </text>
      <text fg={theme.colors.textMuted}>
        <span fg={color}>{progressBar(props.value, barWidth)}</span>
        <span fg={theme.colors.textSubtle}> {metricScale(props.value)}</span>
      </text>
      {graphRows.map((row, index) => (
        <text key={`${props.label}-${index}`} fg={color}>
          {row}
        </text>
      ))}
    </box>
  )
}

export function compactSparkline(history: number[], width: number) {
  return dotGraph(history, width, 2)[1] ?? ""
}

function dotGraph(values: number[], width: number, height: number) {
  if (width <= 0 || height <= 0) {
    return []
  }

  const samples = resample(values, width)
  const matrix = Array.from({ length: height }, () => Array.from({ length: width }, () => " "))

  for (let column = 0; column < width; column += 1) {
    const level = Math.max(0, Math.min(height, Math.round((samples[column] / 100) * height)))
    for (let row = 0; row < height; row += 1) {
      const fromBottom = height - row
      if (fromBottom <= level) {
        matrix[row][column] = fromBottom === level ? "•" : "·"
      }
    }
  }

  return matrix.map((row) => row.join(""))
}

function progressBar(value: number, width: number) {
  const clamped = Math.max(0, Math.min(100, value))
  const filled = Math.round((clamped / 100) * width)
  return `${"█".repeat(filled)}${"░".repeat(Math.max(0, width - filled))}`
}

function metricScale(value: number) {
  return `${Math.round(value).toString().padStart(3, " ")}%`
}

function toneForPercent(value: number): VizTone {
  if (value >= 80) {
    return "danger"
  }
  if (value >= 55) {
    return "warning"
  }
  if (value > 0) {
    return "success"
  }
  return "default"
}

function colorForTone(theme: ReturnType<typeof useTheme>, tone: VizTone) {
  switch (tone) {
    case "success":
      return theme.colors.success
    case "warning":
      return theme.colors.warning
    case "danger":
      return theme.colors.danger
    default:
      return theme.colors.textMuted
  }
}

export function seriesForDevice(history: Record<string, MetricSeries>, deviceId: string) {
  return history[deviceId] ?? { cpu: [], ram: [], gpu: [], vram: [] }
}

function resample(values: number[], width: number) {
  if (values.length === 0) {
    return Array.from({ length: width }, () => 0)
  }

  if (values.length <= width) {
    return [...values, ...Array.from({ length: width - values.length }, () => values[values.length - 1] ?? 0)]
  }

  const bucketSize = values.length / width
  return Array.from({ length: width }, (_, index) => {
    const start = Math.floor(index * bucketSize)
    const end = Math.max(start + 1, Math.floor((index + 1) * bucketSize))
    const bucket = values.slice(start, end)
    if (bucket.length === 0) {
      return values[values.length - 1] ?? 0
    }
    return bucket.reduce((sum, value) => sum + value, 0) / bucket.length
  })
}
