import type { MetricSeries } from "../../contracts/fleet"
import { useTheme } from "../../theme/context"

type MetricVizProps = {
  compact?: boolean
  detail?: string
  height?: number
  history: number[]
  label: string
  value: number
  width?: number
}

type VizTone = "default" | "success" | "warning" | "danger"

export function MetricViz(props: MetricVizProps) {
  const theme = useTheme()
  const width = Math.max(props.compact ? 12 : 24, props.width ?? (props.compact ? 16 : 36))
  const barWidth = Math.max(10, width)
  const graphWidth = Math.max(10, width)
  const graphHeight = Math.max(2, props.height ?? (props.compact ? 3 : 5))
  const graphRows = brailleGraph(props.history, graphWidth, graphHeight)
  const lineColor = colorForTone(theme, toneForPercent(props.value))

  return (
    <box flexDirection="column" gap={0}>
      <text fg={theme.colors.textMuted}>
        <span fg={theme.colors.textSubtle}>{props.label}</span> <span fg={lineColor}>{props.value.toFixed(0)}%</span>{props.detail ? <span fg={theme.colors.textSubtle}>{` · ${props.detail}`}</span> : null}
      </text>
      <SegmentedBar value={props.value} width={barWidth} />
      {graphRows.map((row, index) => (
        <text key={`${props.label}-${index}`} fg={lineColor}>
          {row}
        </text>
      ))}
    </box>
  )
}

export function compactSparkline(history: number[], width: number) {
  return brailleGraph(history, width, 1)[0] ?? ""
}

function brailleGraph(values: number[], charWidth: number, charHeight: number) {
  if (charWidth <= 0 || charHeight <= 0) {
    return []
  }

  const pixelWidth = charWidth * 2
  const pixelHeight = charHeight * 4
  const samples = resample(values, pixelWidth)
  const grid = Array.from({ length: pixelHeight }, () => Array.from({ length: pixelWidth }, () => false))

  let previousX = 0
  let previousY = valueToPixelY(samples[0] ?? 0, pixelHeight)

  for (let x = 0; x < pixelWidth; x += 1) {
    const y = valueToPixelY(samples[x] ?? 0, pixelHeight)
    drawLine(grid, previousX, previousY, x, y)
    fillColumn(grid, x, y)
    previousX = x
    previousY = y
  }

  return gridToBrailleRows(grid, charWidth, charHeight)
}

function progressBar(value: number, width: number) {
  const clamped = Math.max(0, Math.min(100, value))
  const filled = Math.round((clamped / 100) * width)
  return `${"█".repeat(filled)}${"░".repeat(Math.max(0, width - filled))}`
}

function SegmentedBar(props: { value: number; width: number }) {
  const theme = useTheme()
  const total = Math.max(1, props.width)
  const filled = Math.round((Math.max(0, Math.min(100, props.value)) / 100) * total)
  const palette = segmentedPalette(theme)

  return (
    <text>
      {Array.from({ length: total }, (_, index) => {
        const active = index < filled
        const ratio = total <= 1 ? 1 : index / (total - 1)
        const color = active ? palette[colorIndexForRatio(ratio, palette.length)] : theme.colors.border
        return (
          <span key={index} fg={color}>
            ■
          </span>
        )
      })}
    </text>
  )
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

function segmentedPalette(theme: ReturnType<typeof useTheme>) {
  const low = mixHex(theme.colors.textMuted, theme.colors.warning, 0.28)
  const mid = mixHex(theme.colors.textMuted, theme.colors.warning, 0.58)
  const high = theme.colors.warning
  const hot = mixHex(theme.colors.warning, theme.colors.danger, 0.45)

  return [low, mid, high, hot, theme.colors.danger]
}

function colorIndexForRatio(ratio: number, paletteSize: number) {
  return Math.max(0, Math.min(paletteSize - 1, Math.floor(ratio * paletteSize)))
}

function mixHex(baseHex: string, mixHexValue: string, ratio: number) {
  const base = parseHex(baseHex)
  const mix = parseHex(mixHexValue)
  if (!base || !mix) {
    return baseHex
  }

  const safeRatio = Math.max(0, Math.min(1, ratio))
  const blended = base.map((channel, index) => {
    return Math.round(channel + (mix[index] - channel) * safeRatio)
  })

  return `#${blended.map((value) => value.toString(16).padStart(2, "0")).join("")}`
}

function parseHex(hex: string) {
  const normalized = hex.trim().replace(/^#/, "")
  if (!/^[0-9a-fA-F]{6}$/.test(normalized)) {
    return null
  }

  return [
    Number.parseInt(normalized.slice(0, 2), 16),
    Number.parseInt(normalized.slice(2, 4), 16),
    Number.parseInt(normalized.slice(4, 6), 16),
  ] as const
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

function valueToPixelY(value: number, pixelHeight: number) {
  const clamped = Math.max(0, Math.min(100, value))
  const normalized = clamped / 100
  return pixelHeight - 1 - Math.round(normalized * Math.max(0, pixelHeight - 1))
}

function drawLine(grid: boolean[][], startX: number, startY: number, endX: number, endY: number) {
  let x0 = startX
  let y0 = startY
  const dx = Math.abs(endX - x0)
  const sx = x0 < endX ? 1 : -1
  const dy = -Math.abs(endY - y0)
  const sy = y0 < endY ? 1 : -1
  let err = dx + dy

  while (true) {
    setGridPoint(grid, x0, y0)
    if (x0 === endX && y0 === endY) {
      break
    }
    const e2 = 2 * err
    if (e2 >= dy) {
      err += dy
      x0 += sx
    }
    if (e2 <= dx) {
      err += dx
      y0 += sy
    }
  }
}

function setGridPoint(grid: boolean[][], x: number, y: number) {
  if (y < 0 || y >= grid.length) {
    return
  }
  if (x < 0 || x >= grid[0].length) {
    return
  }

  grid[y][x] = true
}

function fillColumn(grid: boolean[][], x: number, topY: number) {
  const bottom = grid.length - 1
  const start = Math.max(0, Math.min(bottom, topY))

  for (let y = start; y <= bottom; y += 1) {
    setGridPoint(grid, x, y)
  }
}

function gridToBrailleRows(grid: boolean[][], charWidth: number, charHeight: number) {
  const rows: string[] = []

  for (let charY = 0; charY < charHeight; charY += 1) {
    let line = ""
    for (let charX = 0; charX < charWidth; charX += 1) {
      let mask = 0

      for (let py = 0; py < 4; py += 1) {
        for (let px = 0; px < 2; px += 1) {
          const absoluteY = charY * 4 + py
          const absoluteX = charX * 2 + px
          if (!grid[absoluteY]?.[absoluteX]) {
            continue
          }

          mask |= brailleBit(px, py)
        }
      }

      line += String.fromCharCode(0x2800 + mask)
    }

    rows.push(line)
  }

  return rows
}

function brailleBit(px: number, py: number) {
  if (px === 0) {
    switch (py) {
      case 0:
        return 0x01
      case 1:
        return 0x02
      case 2:
        return 0x04
      default:
        return 0x40
    }
  }

  switch (py) {
    case 0:
      return 0x08
    case 1:
      return 0x10
    case 2:
      return 0x20
    default:
      return 0x80
  }
}
