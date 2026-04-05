export function getShellFrameWidth(terminalWidth: number) {
  return Math.max(40, Math.min(Math.max(0, terminalWidth - 2), 156))
}

export function getShellContentWidth(terminalWidth: number) {
  return Math.max(20, getShellFrameWidth(terminalWidth) - 4)
}

export function getShellContentHeight(terminalHeight: number) {
  return Math.max(12, terminalHeight - 22)
}
