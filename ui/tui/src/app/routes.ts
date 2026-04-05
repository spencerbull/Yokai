export const APP_ROUTES = [
  {
    id: "dashboard",
    label: "Dashboard",
    status: "Read-only slice",
    summary: "Fleet services, inspector details, and contextual live logs.",
  },
  {
    id: "devices",
    label: "Devices",
    status: "First management slice",
    summary: "Device inventory, tunnel status, add/edit, SSH-config and Tailscale import, and testing.",
  },
  {
    id: "deploy",
    label: "Deploy",
    status: "Wizard slice",
    summary: "Workload selection, daemon-backed model search, defaults/history, and deploy submission.",
  },
  {
    id: "settings",
    label: "Settings",
    status: "Daemon-backed slice",
    summary: "Integrations, Hugging Face token, deploy defaults, and endpoint discovery.",
  },
] as const

export type AppRouteId = (typeof APP_ROUTES)[number]["id"]
