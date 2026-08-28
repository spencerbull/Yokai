import { describe, expect, test } from "bun:test"

import type { DeploymentRecord } from "../../contracts/deploy"
import { mergeDeploymentPoll } from "./useFleetSnapshot"

const prior = [{ id: "dep-1", state: "running" }] as DeploymentRecord[]

describe("deployment polling", () => {
  test("retains prior deployment data and surfaces unavailability", () => {
    expect(mergeDeploymentPoll(prior, { error: "daemon unavailable" })).toEqual({
      deployments: prior,
      error: "deployment status unavailable: daemon unavailable",
    })
  })

  test("replaces prior data only after a successful response", () => {
    expect(mergeDeploymentPoll(prior, { deployments: [] })).toEqual({ deployments: [], error: undefined })
  })
})
