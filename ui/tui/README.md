# Yokai OpenTUI Frontend

This package is the new terminal frontend for Yokai.

## Purpose

- replace the current Bubble Tea client
- talk only to the local Yokai daemon over REST and SSE
- keep backend behavior reusable for future frontends

## Current Status

This is the initial scaffold for the migration.

Planned first implementation slice:

- shell frame
- dashboard layout
- logs viewer
- daemon client and polling hooks

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
