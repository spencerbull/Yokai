# Yokai OpenTUI Frontend

This package is the terminal frontend for Yokai.

## Purpose

- talk only to the local Yokai daemon over REST and SSE
- keep backend behavior reusable for future frontends

## Current Status

This is the active Yokai frontend.

Implemented here:

- shell frame and route navigation
- dashboard, service detail, and logs views
- deploy wizard
- devices and settings routes
- daemon REST and SSE clients

## Development

```bash
bun install
bun run dev
```

Environment variables:

- `YOKAI_DAEMON_URL` defaults to `http://127.0.0.1:7473`

## Notes

- Runtime is Bun.
- Framework is OpenTUI React.
- Do not call `process.exit()` directly; use `renderer.destroy()`.
