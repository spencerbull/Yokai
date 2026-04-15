import { useEffect, useMemo, useState } from "react"

import type { AppRouteId } from "../../app/routes"
import { useTheme } from "../../theme/context"

type RouteCard = {
  id: AppRouteId
  label: string
}

type LandingRouteProps = {
  onActivate: (routeId: AppRouteId) => void
  onSelectIndex: (index: number) => void
  routes: readonly RouteCard[]
  selectedIndex: number
  terminalHeight: number
  terminalWidth: number
}

const YOKAI_ART = [
  "██╗   ██╗ ██████╗ ██╗  ██╗ █████╗ ██╗",
  "╚██╗ ██╔╝██╔═══██╗██║ ██╔╝██╔══██╗██║",
  " ╚████╔╝ ██║   ██║█████╔╝ ███████║██║",
  "  ╚██╔╝  ██║   ██║██╔═██╗ ██╔══██║██║",
  "   ██║   ╚██████╔╝██║  ██╗██║  ██║██║",
  "   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝",
]

export function LandingRoute(props: LandingRouteProps) {
  const theme = useTheme()
  const [phase, setPhase] = useState(0)
  const bgWidth = Math.max(20, props.terminalWidth - 2)
  const bgHeight = Math.max(12, props.terminalHeight - 2)

  useEffect(() => {
    const timer = setInterval(() => {
      setPhase((current) => current + 1)
    }, 85)
    return () => clearInterval(timer)
  }, [])

  const background = useMemo(() => renderBackground(bgWidth, bgHeight, phase, theme), [bgHeight, bgWidth, phase, theme])

  return (
    <box width="100%" height="100%" backgroundColor={theme.colors.background}>
      <box position="absolute" left={0} top={0} width="100%" height="100%" flexDirection="column">
        {background.map((line, index) => (
          <text key={index} fg={line.color}>
            {line.text}
          </text>
        ))}
      </box>

      <box position="absolute" left={0} top={0} width="100%" height="100%" justifyContent="center" alignItems="center">
        <box flexDirection="column" alignItems="center" gap={1}>
          <box flexDirection="column" bottom={1} alignItems="center">
            {YOKAI_ART.map((line, index) => (
              <text key={index} fg={index === 0 ? theme.colors.text : theme.colors.accent}>
                {line}
              </text>
            ))}
            <box top={1} bottom={1}>
              <text fg={theme.colors.text}>Deploy, monitor, and manage AI inference devices from a single terminal.</text>
            </box> 
          </box>

          <box
            border
            borderStyle="rounded"
            borderColor={theme.colors.borderStrong}
            backgroundColor={theme.colors.panel}
            paddingX={2}
            paddingY={1}
            flexDirection="column"
            gap={0}
            width={32}
          >
            <text fg={theme.colors.accent}>
              <strong>Choose a destination</strong>
            </text>
            {props.routes.map((route, index) => {
              const selected = index === props.selectedIndex
              return (
                <box
                  key={route.id}
                  focusable
                  backgroundColor={theme.colors.panel}
                  onMouseDown={() => props.onActivate(route.id)}
                >
                  <text fg={selected ? theme.colors.accent : theme.colors.textMuted}>
                    {selected ? "▌ " : "  "}{index + 1}. {route.label}
                  </text>
                </box>
              )
            })}
          </box>
        </box>
      </box>

      <box position="absolute" left={0} bottom={1} width="100%" justifyContent="center" alignItems="center">
        <text fg={theme.colors.textSubtle}>↑↓ select  ·  Enter open  ·  1-4 jump  ·  Esc Esc quit</text>
      </box>
    </box>
  )
}

function renderBackground(width: number, height: number, phase: number, theme: ReturnType<typeof useTheme>) {
  const lines: Array<{ color: string; text: string }> = []
  const waveColor = mixHex(theme.colors.panelMuted, theme.colors.accent, 0.42)
  const brightWave = mixHex(theme.colors.text, theme.colors.accent, 0.7)
  const dimWave = mixHex(theme.colors.background, theme.colors.accent, 0.22)
  const waveChars = ["·", "•", "◦", "◌", "◍", "◉", "◎"]

  for (let y = 0; y < height; y += 1) {
    let text = ""
    let intensity = 0
    const density = y / Math.max(1, height - 1)
    const topLift = (1 - density) * 0.22
    const bottomBoost = Math.pow(density, 1.45) * 0.045

    for (let x = 0; x < width; x += 1) {
      const jitter = noise(x + phase * 5, y * 3 + phase)
      const diagonal = x * 0.17 + y * 0.31 - phase * 0.18
      const ridgeA = Math.sin(diagonal)
      const ridgeB = Math.sin((x * 0.06) - (y * 0.16) + phase * 0.11)
      const ridgeC = Math.cos((x * 0.028) + (y * 0.11) - phase * 0.06)
      const ridgeD = Math.sin((x * 0.014) + (y * 0.045) + phase * 0.035)
      const combined = (ridgeA * 0.42) + (ridgeB * 0.24) + (ridgeC * 0.2) + (ridgeD * 0.14)
      const normalized = (combined + 1.55) / 3.1

      const coarseA = Math.sin((x * 0.018) + (y * 0.058) - phase * 0.03)
      const coarseB = Math.cos((x * 0.011) - (y * 0.031) + phase * 0.02)
      const baseField = ((coarseA * 0.5) + (coarseB * 0.5) + 1) / 2

      const threshold = 0.58 - density * 0.08 - topLift - bottomBoost * 0.18 + (jitter - 0.5) * 0.045
      const band = normalized - threshold
      const shaped = Math.max(0, Math.min(1, band * (1.55 + density * 0.68) + topLift * 0.95 + bottomBoost * 0.2 + baseField * 0.16))

      if (shaped <= 0) {
        if (band > -0.045 && jitter < 0.3 + topLift * 0.8 + bottomBoost * 0.12) {
          text += "·"
          intensity = Math.max(intensity, 0.12 + bottomBoost * 0.08)
        } else {
          text += " "
        }
        continue
      }

      const emphasized = Math.max(0, Math.min(1, shaped + baseField * 0.08 + bottomBoost * 0.04 + (jitter - 0.5) * 0.08))
      const index = Math.max(0, Math.min(waveChars.length - 1, Math.floor(emphasized * waveChars.length)))
      intensity = Math.max(intensity, shaped)
      text += waveChars[index]
    }

    lines.push({
      color: intensity > 0.68
        ? brightWave
        : intensity > 0.3
          ? waveColor
          : dimWave,
      text,
    })
  }

  return lines
}

function noise(x: number, y: number) {
  const value = Math.sin(x * 12.9898 + y * 78.233) * 43758.5453
  return value - Math.floor(value)
}

function mixHex(baseHex: string, mixHexValue: string, ratio: number) {
  const base = parseHex(baseHex)
  const mix = parseHex(mixHexValue)
  if (!base || !mix) {
    return mixHexValue
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
