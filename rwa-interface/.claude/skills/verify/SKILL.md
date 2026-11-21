---
name: verify
description: Build, launch, and drive pi-rwa-interface (RWA Explorer, React 19 + Vite + MUI 7) to verify changes at the UI surface.
---

# Verify: pi-rwa-interface

## Build & launch

```bash
pnpm install
pnpm lint          # eslint (must be clean)
pnpm test          # vitest: dictionary parity, locale paths, page metadata, JSON-LD, API types/keys against mock fixtures
pnpm build         # tsc && vite build (dist/) && vite build --ssr (dist-ssr/); no prerender step any more
pnpm mock:api      # mock API on http://localhost:8080: public/data/rwa.json as build 1, status, runs, history (--fail, --delay-ms)
pnpm serve         # SSR server on http://localhost:4173 reading API_URL (default http://127.0.0.1:8080); needs pnpm build + the API
pnpm dev:ssr       # pnpm build + pnpm serve (real API); pnpm mock:ssr = the same on the mock
pnpm start         # dev server on http://localhost:3000, client render only, /api proxied to :8080 (or VITE_API_URL)
```

Quick SSR checks: `curl -sI localhost:4173/assets/NVDA` (200), `curl -s .../assets/NVDA | grep -o 'rwa-build[^>]*'`,
`curl -sI .../tokens/999999` (404), `curl -s .../sitemap.xml | head`, `curl -sI .../healthz` (200 once the export is cached).
Start the server against `pnpm mock:api -- --fail` to see the 503 "Data unavailable" page; start the good mock on the
same port afterwards and the server recovers within 10 s, without a restart.

## Drive (Playwright)

Playwright is in the repo: `playwright.config.ts`, specs in `e2e/` (`rendering`, `seo`, `i18n`, `interaction`, `a11y`,
`hydration`, `unavailable`, `updates`, `history`), shared helpers and sample ids in `e2e/helpers.ts` (`ROUTES`, `SAMPLE`). Tests run against
`dist/` and `dist-ssr/`, so `pnpm build` first.

```bash
npx playwright install chromium          # once
pnpm test:e2e                            # all specs, desktop + mobile projects
pnpm test:e2e -- e2e/interaction.spec.ts # one spec
pnpm test:e2e -- --headed                # watch it
```

The config starts, by itself, the mock API (port 8180) and `server/index.mjs` (4321) on it, plus a second pair (mock with
`--fail` on 8181, server on 4322) for `unavailable.spec.ts`. Ports: `E2E_PORT`, `E2E_API_PORT`, `E2E_DOWN_PORT`,
`E2E_DOWN_API_PORT`. For an ad-hoc check write a temporary spec in `e2e/` and delete it afterwards, do not hand-roll
`playwright-core` scripts.

## Flows worth driving

- `/assets` — basket table (data from the API, `public/data/rwa.json` via the mock); row link → `/assets/:ticker`
- `/assets/NVDA` — tokenized assets table; row link → `/tokens/:cmcId`
- `/tokenized` — all tokenized assets, sort + filters
- `/tokens/<id>` — deployments, breakdowns, positions; position row → detail dialog (`[role="dialog"]`, `OpportunityDialog`)
- `/issuers` → row link → `/issuers/:issuerId`
- `/opportunities?underlying=NVDA` — filters + pagination
- `/summary` — charts with accessible legend and text summary; trend sparkline cards and the history chart (range toggle, metric chips)
- `/updates` — runs table (client-only: `curl` shows the heading and a busy placeholder, no table); "Show rebuilds" switch;
  date link → `/updates/3` (warnings, per-asset table with the "only not ok" switch) and `/updates/2` (new tokens/issuers, details); both end in the "Journal" section (level toggle, step chips, `<table>` of events; server HTML has only its heading)
- `/updates/5` — the mock's live run: journal grows every 5 s ("Running… N events", Follow switch), `succeeded` after ~60 s of mock uptime; `GET <mock>/__mock/restart-live` restarts that clock (e2e does)
- `/updates/999` — 200 + `noindex` from the server, "Update #999 not found" in the browser; `/updates/abc` — 404
- History states: the mock's `?points=1` / `?points=0` (rewrite the request with `page.route`, see `e2e/history.spec.ts`) give "history starts on ..." and "No history yet."
- Footer: "Data collected ... · next collection ~... · Update history" (the last two parts after hydration)
- `/zh/...` — every page in Simplified Chinese; header switcher `EN | 中文` links to the same page
- Error state: route `/api/v1/export` to 503 in a spec (or start the server on a `--fail` mock) → "Data unavailable"
- Unknown path or id → server-rendered not-found page with HTTP 404 in the language of the URL

## Gotchas

- Server-rendered pages must match the first browser render: no `useMediaQuery` for layout, use `hooks/useHydrated` and
  responsive `sx`. A hydration mismatch shows up as a console error in the e2e run.
- MUI `Typography variant="h2"` often renders as `span` (component prop) — don't select by tag.
- Nav tabs are defined in `src/layout/MainLayout/index.tsx`; header links in `src/layout/MainLayout/Header/MenuItems.tsx`
  and `MobileMenu.tsx`.
- Rows that open a page use a real link (`RowLink`); select by role `link`, not by clicking the row.
- All UI text goes through `useT()`; `src/i18n/messages/en.ts` and `zh.ts` must have the same keys.
- No external requests are expected: the browser calls only `/api/v1/*` and `/logos/*` of our own origin (`grep` the bundle for `coinmarketcap`/`portals.fi` API hosts: only the outbound CMC page link may appear).
