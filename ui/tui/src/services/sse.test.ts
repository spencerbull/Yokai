import { describe, expect, test } from "bun:test"

import { parseSSEChunks } from "./sse"

describe("parseSSEChunks", () => {
  test("collects data lines and preserves incomplete trailing chunks", () => {
    const first = parseSSEChunks("", "data: first\n\ndata: sec")
    expect(first.messages).toEqual(["first"])
    expect(first.buffer).toBe("data: sec")

    const second = parseSSEChunks(first.buffer, "ond\n\n:data ignored\n")
    expect(second.messages).toEqual(["second"])
    expect(second.buffer).toBe("")
  })
})
