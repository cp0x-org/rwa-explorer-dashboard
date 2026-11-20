# Security

## Reporting

Please do not open a public issue for a vulnerability. Contact the maintainers privately via
Telegram [@cp0xdotcom](https://t.me/cp0xdotcom) or a GitHub security advisory on this repository. We will confirm
receipt and keep you informed about the fix.

## Scope

- **The site is read-only.** It has no wallet connection, no transactions, and never calls external APIs from the
  browser. It renders pages on a Node server from one export document of our own API.
- **API keys** (CoinMarketCap, Portals) and the admin token live only on the server (the `.env` of the deployment
  stack) and in developers' local `.env` files, which are git-ignored. They are read only by the collector and the API
  and must never be written to snapshots, raw responses, run reports, exports or logs. If you find a key in the
  repository history or in generated data, report it as a vulnerability.
- **Dependencies.** The projects pin their dependencies (`go.sum`, `pnpm-lock.yaml`, `uv.lock`). Pull requests that add
  dependencies are reviewed for supply-chain risk.
