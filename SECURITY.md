# Security

## Reporting

Please do not open a public issue for a vulnerability. Contact the maintainers privately via
Telegram [@cp0xdotcom](https://t.me/cp0xdotcom) or a GitHub security advisory on this repository. We will confirm
receipt and keep you informed about the fix.

## Scope

- **The site is read-only.** It has no wallet connection, no transactions, and never calls external APIs from the
  browser. It serves prerendered static files and one JSON file.
- **API keys** (CoinMarketCap, Portals) are used only by the data pipeline, read from `rwa-data-pipeline/.env` or the
  environment, and are git-ignored. They must never be written to snapshots, raw responses, run reports, exports or
  logs. If you find a key in the repository history or in generated data, report it as a vulnerability.
- **Dependencies.** Both projects pin their dependencies (`pnpm-lock.yaml`, `uv.lock`). Pull requests that add
  dependencies are reviewed for supply-chain risk.
