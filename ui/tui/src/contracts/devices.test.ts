import { describe, expect, test } from "bun:test"

import { editorFormFromTailscalePeer, type TailscalePeer } from "./devices"

describe("editorFormFromTailscalePeer", () => {
  test("prefers the peer DNS name for the host", () => {
    const peer: TailscalePeer = {
      hostname: "gpu-box",
      dns_name: "gpu-box.tailnet.ts.net",
      ip: "100.64.0.2",
      online: true,
      recommended: true,
    }

    expect(editorFormFromTailscalePeer(peer).host).toBe("gpu-box.tailnet.ts.net")
  })

  test("falls back to the peer IP when DNS is missing", () => {
    const peer: TailscalePeer = {
      hostname: "gpu-box",
      ip: "100.64.0.2",
      online: true,
      recommended: true,
    }

    expect(editorFormFromTailscalePeer(peer).host).toBe("100.64.0.2")
  })
})
