import { useEffect, useRef, useState } from "react"

import type { LogTarget } from "../../contracts/fleet"
import { streamLogs } from "../../services/daemon-client"

type LogsState = {
  status: "closed" | "connecting" | "open" | "error"
  target: LogTarget | null
  lines: string[]
  follow: boolean
  offset: number
  error?: string
}

const MAX_LOG_LINES = 800

export function useLogsStream(target: LogTarget | null, viewportLines: number, active: boolean) {
  const viewportRef = useRef(viewportLines)
  viewportRef.current = viewportLines

  const [state, setState] = useState<LogsState>({
    status: "closed",
    target: null,
    lines: [],
    follow: true,
    offset: 0,
  })

  useEffect(() => {
    if (!active || !target) {
      setState((current) => ({
        ...current,
        status: "closed",
        target: null,
        lines: [],
        offset: 0,
        follow: true,
        error: undefined,
      }))
      return
    }

    const controller = new AbortController()
    setState({
      status: "connecting",
      target,
      lines: [],
      follow: true,
      offset: 0,
    })

    void streamLogs(target, controller.signal, (line) => {
      setState((current) => appendLogLine(current, line, viewportRef.current))
    })
      .then(() => {
        setState((current) => {
          if (current.target?.containerId !== target.containerId) {
            return current
          }

          if (current.status === "error") {
            return current
          }

          return {
            ...current,
            status: current.lines.length > 0 ? current.status : "closed",
          }
        })
      })
      .catch((cause: unknown) => {
        if (controller.signal.aborted) {
          return
        }

        setState((current) => ({
          ...current,
          status: "error",
          error: cause instanceof Error ? cause.message : "failed to stream logs",
        }))
      })

    return () => {
      controller.abort()
    }
  }, [active, target?.containerId, target?.deviceId])

  return {
    ...state,
    close() {
      setState({
        status: "closed",
        target: null,
        lines: [],
        follow: true,
        offset: 0,
      })
    },
    pageDown() {
      setState((current) => pageLogs(current, viewportRef.current, viewportRef.current))
    },
    pageUp() {
      setState((current) => pageLogs(current, -viewportRef.current, viewportRef.current))
    },
    scrollEnd() {
      setState((current) => ({
        ...current,
        follow: true,
        offset: maxOffset(current.lines.length, viewportRef.current),
      }))
    },
    scrollHome() {
      setState((current) => ({
        ...current,
        follow: false,
        offset: 0,
      }))
    },
    toggleFollow() {
      setState((current) => ({
        ...current,
        follow: !current.follow,
        offset: !current.follow ? current.offset : maxOffset(current.lines.length, viewportRef.current),
      }))
    },
  }
}

function appendLogLine(state: LogsState, line: string, viewportLines: number): LogsState {
  const parsedError = parseDaemonLogError(line)
  if (parsedError) {
    return {
      ...state,
      status: "error",
      error: parsedError,
    }
  }

  const nextLines = [...state.lines, line]
  const overflow = Math.max(0, nextLines.length - MAX_LOG_LINES)
  const trimmedLines = overflow > 0 ? nextLines.slice(overflow) : nextLines
  const nextOffset = state.follow
    ? maxOffset(trimmedLines.length, viewportLines)
    : Math.max(0, state.offset - overflow)

  return {
    ...state,
    status: "open",
    lines: trimmedLines,
    offset: nextOffset,
    error: undefined,
  }
}

function pageLogs(state: LogsState, delta: number, viewportLines: number): LogsState {
  const nextOffset = clampOffset(state.offset + delta, state.lines.length, viewportLines)
  const atEnd = nextOffset >= maxOffset(state.lines.length, viewportLines)

  return {
    ...state,
    follow: state.follow ? atEnd : false,
    offset: nextOffset,
  }
}

function clampOffset(offset: number, lineCount: number, viewportLines: number) {
  const max = maxOffset(lineCount, viewportLines)
  if (offset < 0) {
    return 0
  }
  if (offset > max) {
    return max
  }
  return offset
}

function maxOffset(lineCount: number, viewportLines: number) {
  return Math.max(0, lineCount - Math.max(1, viewportLines))
}

function parseDaemonLogError(line: string) {
  if (!line.startsWith("{")) {
    return null
  }

  try {
    const parsed = JSON.parse(line) as { error?: string }
    if (typeof parsed.error === "string" && parsed.error !== "") {
      return parsed.error
    }
  } catch {
    return null
  }

  return null
}

export type LogsStreamState = ReturnType<typeof useLogsStream>
