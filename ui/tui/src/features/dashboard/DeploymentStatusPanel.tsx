import { useState } from "react"

import type { DeploymentRecord } from "../../contracts/deploy"
import { startDeployment } from "../../services/daemon-client"
import { useTheme } from "../../theme/context"

export function DeploymentStatusPanel(props: { deployments: DeploymentRecord[] }) {
  const theme = useTheme()
  const [apiKeys, setAPIKeys] = useState<Record<string, string>>({})
  const [pendingID, setPendingID] = useState("")
  const [notice, setNotice] = useState("")
  if (props.deployments.length === 0) return null
  return (
    <box border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} paddingX={1} flexDirection="column">
      <text fg={theme.colors.text}><strong>Coordinated Deployments</strong></text>
      {props.deployments.slice(0, 4).map((deployment) => {
        const members = deployment.members ?? []
        const roles = members.map((member) => `${member.role}:${member.status}`).join(" · ") || "members pending"
        const stateColor = deployment.state === "rollback_failed" ? theme.colors.danger : deployment.state === "running" ? theme.colors.success : theme.colors.textMuted
		return (
		  <box key={deployment.id} flexDirection="column">
			<text fg={stateColor}>{deployment.id} · {deployment.state} · g{deployment.generation} · {roles}</text>
			{deployment.state === "stopped" ? (
			  <box flexDirection="row" gap={1}>
				<input value={apiKeys[deployment.id] ?? ""} onInput={(value) => setAPIKeys((current) => ({ ...current, [deployment.id]: value }))} password width={36} backgroundColor={theme.colors.panelMuted} textColor={theme.colors.text} focusedTextColor={theme.colors.text} cursorColor={theme.colors.accent} placeholder="original launch API key" />
				<box border borderStyle="single" borderColor={theme.colors.borderStrong} paddingX={1} onMouseDown={() => void startStopped(deployment.id)}>
				  <text fg={theme.colors.accent}>{pendingID === deployment.id ? "Starting..." : "Start group"}</text>
				</box>
			  </box>
			) : null}
		  </box>
		)
      })}
	  {notice ? <text fg={notice.startsWith("Started") ? theme.colors.success : theme.colors.danger}>{notice}</text> : null}
	  <text fg={theme.colors.textSubtle}>Start requires the original launch API key retained by rank 0; it cannot rotate the key. A different key fails readiness and returns the group to stopped. Authenticated live metrics are unavailable without a request key.</text>
    </box>
  )

  async function startStopped(id: string) {
	const apiKey = apiKeys[id] ?? ""
	if (!apiKey || pendingID) {
	  setNotice(apiKey ? "Another group action is already running" : "The original launch API key is required to start a stopped group")
	  return
	}
	setPendingID(id)
	setNotice("")
	try {
	  await startDeployment(id, apiKey)
	  setNotice(`Started ${id}`)
	} catch (cause) {
	  setNotice(cause instanceof Error ? cause.message : "group start failed")
	} finally {
	  setAPIKeys((current) => ({ ...current, [id]: "" }))
	  setPendingID("")
	}
  }
}
