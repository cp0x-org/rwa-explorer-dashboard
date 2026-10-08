---
name: verify
description: Build, launch, and drive this app (React + Vite + MUI boilerplate) to verify changes at the UI surface.
---

# Verify: pi-euler-interface

## Build & launch

```bash
pnpm install
pnpm lint          # eslint + prettier (must be clean)
pnpm build         # tsc && vite build
pnpm start         # dev server on http://localhost:3000
```

## Drive (headless browser)

No Playwright in the repo. Use `playwright-core` from a scratch dir with the cached
headless shell at
`~/Library/Caches/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-mac-arm64/chrome-headless-shell`
as `executablePath`.

## Flows worth driving

- `/assets` — basket table (data from `public/data/rwa.json`); row click → `/assets/:ticker`
- `/assets/NVDA` — versions table; row click → `/tokens/:cmcId`
- `/tokens/42461` — deployments, breakdowns, positions; position row click → detail dialog (`[role="dialog"]`)
- `/issuers` → row click → `/issuers/:issuerId`
- `/opportunities?underlying=NVDA` — filters + pagination
- Error state: route `**/data/rwa.json` to 404 → "Data unavailable"

## Gotchas

- MUI `Typography variant="h2"` often renders as `span` (component prop) — don't select by tag.
- Page headers/tabs live in `MainLayout`; nav tabs are hardcoded in `src/layout/MainLayout/index.tsx`.
- No external requests are expected: all data comes from `public/data/rwa.json`.
