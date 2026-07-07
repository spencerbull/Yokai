export type SseParseResult = {
  buffer: string
  messages: string[]
}

export function parseSSEChunks(buffer: string, chunk: string): SseParseResult {
  const normalized = `${buffer}${chunk}`.replace(/\r/g, "")
  const lines = normalized.split("\n")
  const nextBuffer = normalized.endsWith("\n") ? "" : lines.pop() ?? ""
  const messages: string[] = []

  for (const line of lines) {
    if (!line.startsWith("data:")) {
      continue
    }

    const message = line.slice(5).trimStart()
    if (message !== "") {
      messages.push(message)
    }
  }

  return {
    buffer: nextBuffer,
    messages,
  }
}

export async function readSSEStream(
  stream: ReadableStream<Uint8Array>,
  onMessage: (message: string) => void,
) {
  const reader = stream.getReader()
  const decoder = new TextDecoder()
  let buffer = ""

  try {
    while (true) {
      const result = await reader.read()
      if (result.done) {
        break
      }

      const parsed = parseSSEChunks(buffer, decoder.decode(result.value, { stream: true }))
      buffer = parsed.buffer
      for (const message of parsed.messages) {
        onMessage(message)
      }
    }

    const finalChunk = decoder.decode()
    const parsed = parseSSEChunks(buffer, `${finalChunk}\n`)
    for (const message of parsed.messages) {
      onMessage(message)
    }
  } finally {
    reader.releaseLock()
  }
}
