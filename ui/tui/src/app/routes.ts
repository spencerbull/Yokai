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
    status: "Planned",
    summary: "Workload selection, model search, config review, and progress.",
  },
  {
    id: "settings",
    label: "Settings",
    status: "Planned",
    summary: "Integrations, Hugging Face token, and deploy defaults.",
  },
] as const

export type AppRouteId = (typeof APP_ROUTES)[number]["id"]
