import { describe, expect, test } from "bun:test"

import type { TailscalePeer } from "../../contracts/devices"
import { visibleTailscalePeers } from "./tailscale-search"

describe("visibleTailscalePeers", () => {
  test("prefers recommended AI GPU peers when query is empty", () => {
    const peers: TailscalePeer[] = [
      {
        hostname: "worker-2",
        ip: "100.64.0.3",
        online: true,
        recommended: false,
      },
      {
        hostname: "gpu-box",
        ip: "100.64.0.2",
        online: true,
        recommended: true,
        tags: ["tag:ai-gpu"],
      },
    ]

    const visible = visibleTailscalePeers(peers, "")
    expect(visible[0].hostname).toBe("gpu-box")
  })

  test("matches hostname, ip, os, and tags", () => {
    const peers: TailscalePeer[] = [
      {
        hostname: "gpu-box",
        ip: "100.64.0.2",
        os: "linux",
        online: true,
        recommended: true,
        tags: ["tag:ai-gpu", "lab"],
      },
      {
        hostname: "macbook",
        ip: "100.64.0.4",
        os: "darwin",
        online: true,
        recommended: false,
      },
    ]

    expect(visibleTailscalePeers(peers, "ai-gpu")).toHaveLength(1)
    expect(visibleTailscalePeers(peers, "100.64.0.4")[0].hostname).toBe("macbook")
    expect(visibleTailscalePeers(peers, "lin")[0].hostname).toBe("gpu-box")
  })
})
