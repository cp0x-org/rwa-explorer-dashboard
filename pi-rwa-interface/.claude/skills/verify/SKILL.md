---
name: verify
description: Build, launch, and drive pi-rwa-interface (RWA Explorer, React 19 + Vite + MUI 7) to verify changes at the UI surface.
---

# Verify: pi-rwa-interface

## Build & launch

```bash
pnpm install
pnpm lint          # eslint (must be clean)
pnpm test          # vitest: dictionary parity, locale paths, page metadata, JSON-LD
pnpm build         # tsc && vite build && SSR bundle && prerender of every page in both languages into dist/
pnpm start         # dev server on http://localhost:3000 (reads public/data/rwa.json on every reload)
pnpm serve         # serves dist/ like production (clean URLs, real 404s) on http://localhost:4173
```

## Drive (Playwright)

Playwright is in the repo: `playwright.config.ts`, specs in `e2e/` (`rendering`, `seo`, `i18n`, `interaction`, `a11y`),
shared helpers and sample ids in `e2e/helpers.ts` (`ROUTES`, `SAMPLE`). Tests run against `dist/`, so `pnpm build` first.

```bash
npx playwright install chromium          # once
pnpm test:e2e                            # all specs, desktop + mobile projects
pnpm test:e2e -- e2e/interaction.spec.ts # one spec
pnpm test:e2e -- --headed                # watch it
```

The config starts `scripts/serve-dist.mjs` on port 4321 by itself. For an ad-hoc check write a temporary spec in
`e2e/` and delete it afterwards, do not hand-roll `playwright-core` scripts.

## Flows worth driving

- `/assets` — basket table (data from `public/data/rwa.json`); row link → `/assets/:ticker`
- `/assets/NVDA` — tokenized assets table; row link → `/tokens/:cmcId`
- `/tokenized` — all tokenized assets, sort + filters
- `/tokens/<id>` — deployments, breakdowns, positions; position row → detail dialog (`[role="dialog"]`, `OpportunityDialog`)
- `/issuers` → row link → `/issuers/:issuerId`
- `/opportunities?underlying=NVDA` — filters + pagination
- `/summary` — charts with accessible legend and text summary
- `/zh/...` — every page in Simplified Chinese; header switcher `EN | 中文` links to the same page
- Error state: route `**/data/rwa.json` to 404 → "Data unavailable"
- Unknown path or id → `dist/404.html` with HTTP 404, "Page not found" in the language of the URL

## Gotchas

- Prerendered pages must match the first browser render: no `useMediaQuery` for layout, use `hooks/useHydrated` and
  responsive `sx`. A hydration mismatch shows up as a console error in the e2e run.
- MUI `Typography variant="h2"` often renders as `span` (component prop) — don't select by tag.
- Nav tabs are defined in `src/layout/MainLayout/index.tsx`; header links in `src/layout/MainLayout/Header/MenuItems.tsx`
  and `MobileMenu.tsx`.
- Rows that open a page use a real link (`RowLink`); select by role `link`, not by clicking the row.
- All UI text goes through `useT()`; `src/i18n/messages/en.ts` and `zh.ts` must have the same keys.
- No external requests are expected: all data comes from `public/data/rwa.json` and `public/logos/`.
