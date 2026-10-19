# i18n (EN / 简体中文), accessibility, technical SEO and prerendering

Plan and record of the work on `pi-rwa-interface`. Written before the implementation, updated with what was done.

## 1. Audit (confirmed from the repository and production)

### Architecture

- React 19 + Vite 6 + MUI 7, React Router 7 (data routers), `react-intl` already a dependency (Berry template, unused for
  app text). Package manager pnpm. No test framework, no CI config in the repo.
- Data: one static file `public/data/rwa.json` (3.5 MB; 125 underlyings, 647 tokens, 10 issuers, 2 059 positions)
  written by `../rwa-data-pipeline`. The browser never calls external APIs.
- Rendering: **build-time prerender (SSG)**, not CSR-only. `pnpm build` runs `tsc`, `vite build`, `vite build --ssr
  src/entry-server.tsx`, then `scripts/prerender.mjs` renders every route with `renderToString` + emotion critical CSS
  into `dist/<path>/index.html` (230 pages). `src/index.tsx` loads the JSON and hydrates the page when it was rendered
  for the same pathname (no query string), otherwise renders anew in place.
- Serving: Docker image with `serve dist` (static files, no SPA fallback). Unknown paths → `dist/404.html` with HTTP 404
  (confirmed on production: `/tokens/999999999` and `/nonexistent` return 404; `/zh/assets` returns 404 today).
- SEO today: per-page `<title>`, description, OG title/description, canonical (`src/utils/pageMeta.ts`), `sitemap.xml`
  (230 URLs, `lastmod` = newest data collection), `robots.txt`. Missing: hreflang, `og:url`, `og:locale`, Twitter cards,
  JSON-LD, localized URLs, `<html lang>` per page.

### Accessibility issues confirmed in the prerendered DOM (`dist/assets/NVDA/index.html`)

1. No `h1` on any page. Page titles are `h3`; stat values are `h3` and their labels `h6` (metric cards), so the heading
   outline is `h3 8 / h6 Tokenized assets / h3 7 / …`.
2. `<footer>` is nested inside `<main>` (contentinfo landmark not top level).
3. Desktop section navigation is a MUI `Tabs` (`role="tablist"` / `role="tab"`) made of links, without a `nav`
   landmark; it is not a tab panel pattern. The mobile grid is a proper `<nav>` with `aria-current`.
4. MUI `Tooltip` used for hints sets `aria-label` on its child, so the accessible name of a sort button or a stat label
   becomes the hint text instead of the visible label (e.g. "Distinct positions across tokenized assets" instead of
   "Eligible positions"). Hints are not reachable by keyboard (non-focusable spans, chips).
5. Sortable headers have no `aria-sort`. Tables have `aria-label` but no caption.
6. Position rows open a dialog only through `onClick` on the row: no keyboard/no-JS way to open it.
7. `Select` filters have no `labelId`: the combobox has no accessible name.
8. Donut chart: focusable `<circle tabindex="0">` elements without accessible names; `StackedBarList` segments carry
   their values only in hover tooltips.
9. Footer logo link and social links have no accessible name (SVG only); header logo link is named `theme-logo`;
   mobile menu close button has no name; copy-address icon is a clickable SVG, not a button.
10. "How the rating works" (methodology) lives only in a tooltip on a non-focusable span: not in the page text, not
    reachable by keyboard.
11. `<html lang="en">` is fixed in the template.

### Internationalization scope

All application-owned text in `src/views/rwa/**`, `src/layout/MainLayout/**`, `src/utils/rwa.ts` (labels of statuses,
categories, flags, reasons), `src/utils/pageMeta.ts`, `src/components/CopyableAddress.tsx`, `index.html` (noscript,
meta). Roughly 330 strings. Not translated: tickers, symbols, addresses, ISINs, protocol/network names, issuer names,
URLs, ids.

### Risks

- Every internal link and `navigate()` call must carry the locale prefix; a missed one silently drops the user to
  English. Mitigated by a `LocaleLink` wrapper and a `useLocaleNavigate` hook, plus an e2e test that no link on a
  Chinese page points to an English route.
- Hydration mismatch if the locale or messages differ between prerender and first client render. Mitigated: locale
  comes from the URL only; messages are loaded before `hydrateRoot`.
- Double the prerendered pages (460) and sitemap entries: build time ~2×, `dist/` ~190 MB. Acceptable for `serve`.
- Changing headings/landmarks changes selectors used by `DataTableContainer` (`header.MuiAppBar-positionFixed`):
  keep the AppBar as the fixed header.

## 2. Decisions

- **Keep the SSG prerender** (least disruptive, already works on production, no server runtime). Extend it to the
  `/zh/...` routes, `<html lang>`, hreflang, JSON-LD, sitemap alternates, a real prerendered 404 page.
- **Locale in the URL**: `/...` = English (unchanged), `/zh/...` = Simplified Chinese (`zh-CN`). Route tree is declared
  once and mounted twice (`/` and `/zh`). No redirects based on browser language. The switcher is a plain link to the
  equivalent page (query string preserved), so a language change is a full navigation to the prerendered page of
  that language: `<html lang>`, messages and metadata always match.
- **Translation system**: `react-intl` (already installed) with TypeScript dictionaries `src/i18n/messages/en.ts`
  (source of truth, `MessageId = keyof typeof en`) and `zh.ts: Record<MessageId, string>` so a missing or extra Chinese
  key is a compile error. `useT()` returns a typed `t(id, values)`; ICU plurals/interpolation via react-intl; a unit
  test checks placeholder parity. English fallback at runtime through `defaultMessage`. Chinese messages are
  code-split and loaded before hydration.
- **Formatting**: `useFormat()` binds `Intl.NumberFormat`, `Intl.DateTimeFormat`, `Intl.RelativeTimeFormat` to the
  page locale. Calculations and ratings stay untouched.
- **Tests**: `vitest` for unit tests (dictionaries, `pageMeta`, locale paths, SEO helpers) and `@playwright/test` +
  `@axe-core/playwright` for e2e against the built `dist/` served statically (a11y, i18n, SEO, no-JS rendering,
  404). Both are the standard Vite/React choices; nothing else is added.

## 3. Files

| Area | Files |
| --- | --- |
| i18n core | `src/i18n/{index.ts, locale.ts, messages/en.ts, messages/zh.ts, LocaleProvider.tsx, LocaleLink.tsx, useFormat.ts}` |
| Routing | `src/routes/MainRoutes.tsx`, `src/routes/index.tsx`, `src/index.tsx`, `src/entry-server.tsx` |
| Layout | `src/layout/MainLayout/{index.tsx, Header/*, Footer.tsx}`, new `Header/LanguageSwitch.tsx` |
| Pages | `src/views/rwa/*.tsx`, `src/views/rwa/components/*.tsx`, `src/utils/rwa.ts` (labels → message ids) |
| SEO | `src/utils/pageMeta.ts`, new `src/utils/seo.ts` (JSON-LD, alternates), `scripts/prerender.mjs`, `index.html` |
| Tests | `vitest.config.ts`, `src/**/*.test.ts`, `playwright.config.ts`, `e2e/*.spec.ts` |
| Docs | this file, `README.md` |

## 4. Accessibility fixes

- One `h1` per page (page title / entity name), sections `h2`, chart cards `h3`; stat values and labels are not
  headings (`component="p"` / `span` with the same visual variant).
- Landmarks: `<header>` (AppBar), `<nav aria-label>` for sections (desktop and mobile, same label; the inactive one
  is `display: none` so it is not in the accessibility tree), `<main>`, `<footer>` outside `<main>`, breadcrumb `nav`.
- Tooltips that explain (hints) use `describeChild`; the visible label stays the name. Hints also exist as visually
  hidden text so screen readers get them without hover.
- `aria-sort` on sortable `<th>`; `<caption>` (visually hidden) on every data table.
- Position name becomes a button that opens the details dialog (keyboard), rows keep their click handler.
- `Select` + `InputLabel` linked with `labelId`; language switcher is a `nav` of links with `aria-current`,
  `hreflang` and `lang`.
- Charts: SVG `aria-hidden`, no focusable SVG nodes; the legend is the accessible, keyboard-reachable equivalent
  (links); stacked bars carry a visually hidden text of their segments; every chart has a text summary or table.
- Named footer/header/logo links, close/copy buttons; methodology as a disclosure button with the text in the page.

## 5. SEO

- Per page and locale: `<title>`, description, canonical (self, per language), `og:url`, `og:locale`,
  `og:locale:alternate`, Twitter card, `hreflang` en / zh-CN / x-default, `<html lang>`.
- JSON-LD in the prerendered HTML: `WebSite` + `Organization` on every page, `BreadcrumbList` on detail pages,
  `Dataset` + `ItemList` on listing pages (data from the same export that renders the page).
- `sitemap.xml` with both languages and `xhtml:link` alternates; `lastmod` = newest data collection (real data).
- `404.html` becomes a prerendered "not found" page with `noindex` and links to both languages.

## 6. Test plan

- Unit: dictionary parity (keys, ICU placeholders, no tickers translated), `pageMeta` titles unique for representative
  routes, locale path helpers.
- E2E (built `dist/`): axe on the main pages in both languages (no critical/serious violations), `<html lang>`,
  canonical/hreflang pairs, JSON-LD validity, Chinese text present in the raw HTML of `/zh/...`, no English route
  links on Chinese pages, language switcher keeps the route, keyboard navigation (section nav, sort, dialog),
  JS-disabled rendering, HTTP 404 for unknown ids, hydration without console errors.

## 7. Rollback

All changes are in the frontend repository; the data pipeline and `rwa.json` are untouched. Reverting the commit
restores the previous build; the Docker/serve deployment needs no change (static files only).

## 8. What was done (record, 2026-10-19)

### Rendering

- SSG prerender kept and extended: `pnpm build` renders 787 pages × 2 languages = 1 574 HTML files in ~17 s
  (`scripts/prerender.mjs`, `src/entry-server.tsx`). `/` and `/zh` carry the assets page. `dist/` grows from 96 MB
  to ~260 MB (same static `serve` deployment, no infrastructure change).
- `dist/404.html` is now a prerendered "Page not found" page (`noindex`, header, navigation, footer); the static server
  returns it with HTTP 404 for unknown ids and paths (`scripts/serve-dist.mjs` mirrors the Dockerfile's `serve`).
- Hydration: `src/index.tsx` loads the data and the dictionary of the URL's language before `hydrateRoot`; the e2e
  suite asserts zero console errors on every page type in both languages.

### Internationalization

- `src/i18n/`: typed dictionaries (`messages/en.ts`, `messages/zh.ts`, 389 ids), `LocaleProvider` (react-intl, mounted
  per locale in the route tree), `LocaleLink` / `useLocaleNavigate`, `useT`, `useFormat` (Intl number / percent /
  currency / date / relative time). Chinese dictionary is a lazy chunk (34 KB, 9.7 KB gzip).
- Routes: one page tree mounted under `/` and `/zh` (`src/routes/`). Language switcher `EN | 中文` in the header
  (top right, `nav` of plain links with `hreflang`, `lang`, `aria-current`; keeps path and query; full navigation to the
  prerendered page of the other language).
- Everything application-owned is translated: navigation, headings, intros, stat cards, column headers and hints,
  filters, pager, legends, methodology, dialog, chips, flags, exclusion reasons, not-found pages, `noscript`, metadata.
  Tickers, symbols, addresses, protocol / network / issuer names are untouched (unit test).

### Accessibility (confirmed fixes)

- One `h1` per page; sections `h2`; chart cards `h3`; stat values and labels are a `<dl>`; legend is a `<dl>`.
- Landmarks: `header`, `nav` (sections, site menu, language, breadcrumb, pagination), `main` (with skip link), `footer`
  outside `main`. Desktop section strip is a `nav` of links (no tab roles); the phone grid and the strip are both in the
  markup but only one is displayed, so one navigation per viewport in the accessibility tree.
- Tables: visually hidden `<caption>`, `th scope="row"` on name cells, `aria-sort` on sortable headers; hints no longer
  replace accessible names (`describeChild` + hidden text). Position names are buttons that open the details dialog
  (`aria-labelledby`, Escape closes). `Select` + `InputLabel` linked by `labelId`. Named logo / social / close / copy
  buttons; copy icon is a real button.
- Charts: SVG `aria-hidden`, no focusable SVG nodes; donut legend rows are links; stacked rows are buttons with
  `aria-pressed` and a hidden text of their segments; every chart has a hidden text summary; AA contrast on avatar
  fallbacks and network chips (outlined instead of faded).
- Methodology is a disclosure button with the text in the page (also for crawlers), not a hover-only tooltip.
- Results: axe-core (wcag2a/aa, wcag21, wcag22aa, best-practice) reports no critical or serious violations on 16 pages
  × 2 viewports; Lighthouse accessibility 100 on `/assets` and `/zh/assets/NVDA`.

### SEO

- Per page and language: unique `<title>` / description, self-referencing canonical, `hreflang` en / zh-CN / x-default,
  `og:url`, `og:locale`, `og:locale:alternate`, Twitter card, `<html lang>`, translated keywords.
- JSON-LD graph in the prerendered HTML: `Organization`, `WebSite`, `WebPage` everywhere; `Dataset` + `ItemList` (up to
  100 rows, same order as the table) on listing pages; `BreadcrumbList` on detail pages. Nothing invented (no ratings,
  offers, prices).
- `sitemap.xml`: 1 574 URLs with `xhtml:link` alternates, `lastmod` = newest data collection, no query variants, no
  not-found page. `robots.txt` unchanged. Lighthouse SEO 100.
- Short introductory paragraphs on the five listing pages (both languages) with the research terms; methodology text
  unchanged.

### Tests

- `pnpm test` (vitest, 15 tests): dictionary key / ICU-argument parity, every message compiles in its locale, CJK
  presence, no tickers in messages, locale path helpers, unique titles and descriptions for 130 representative routes
  in both languages, not-found detection, canonical / hreflang, JSON-LD shape.
- `pnpm test:e2e` (Playwright, 74 tests on desktop Chrome + Pixel 7): raw HTML of every route type in both languages,
  no-JS rendering, HTTP 404s, root pages, sitemap and robots, metadata and structured data, language switcher (route and
  query kept, no browser-language redirect), client-side navigation stays Chinese, hydration without console errors,
  sorting / filters / paging / dialog, chart keyboard use, methodology disclosure, axe on 16 pages, document structure,
  keyboard navigation, control names, mobile menu.

### Deployment

- No change to `Dockerfile`, `docker-compose.yml` or `serve.json`: the output is still static files served by `serve`
  without SPA fallback. The image is larger (dist ~260 MB). `VITE_SITE_URL` must stay set in `.env.production`
  (canonical, hreflang, sitemap).
- Build time ~35 s (prerender ~17 s). New dev dependencies: `vitest`, `@playwright/test`, `@axe-core/playwright`
  (no runtime dependencies added; react-intl was already there). CI running `pnpm test:e2e` needs
  `npx playwright install chromium`.

### Known limits / not done

- Lighthouse performance stays ~45 (lab, mobile throttling) as before: the client downloads the whole 3.5 MB
  `rwa.json` on every page and the main bundle is 974 KB (303 KB gzip; +1 % from this work). Splitting the data per
  page and compressing responses at the server would be the next step; out of scope here.
- CLS ~0.2 comes from the existing "2d ago" / data-age switch after hydration and from table re-layout; not addressed.
- `404.html` is a single file, so the no-JS "not found" page is English (with the language switcher); with JavaScript
  the page is shown in the language of the URL.
- The `ErrorBoundary` route element from the template is still unused.
