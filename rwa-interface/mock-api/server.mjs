// Mock of the RWA Explorer 2.0 API (docs/v2/03-api.md) for dev and e2e: no dependencies, Node 22.
// Serves public/data/rwa.json as build 1 plus synthetic status, runs and history.
//   node mock-api/server.mjs [--port 8080] [--fail] [--delay-ms 0]
// --fail: every /api/* answers 503 (error-state tests). The mock is read-only and keeps no state.
// Mock-only extras (not in the API contract): `history/...?points=N` keeps the last N points (0 = an empty series), for the
// one-point and no-point states of the charts; the history clock is the export's own (see CLOCK below), so ranges such as
// `from=<30 days ago>` keep selecting the mock's points however old the export file gets.
import { createHash } from 'node:crypto';
import { createReadStream, readFileSync, statSync } from 'node:fs';
import { createServer } from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { COLUMNS, metricType } from './metrics.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const publicDir = path.join(root, 'public');

const args = process.argv.slice(2);
const opt = (name, fallback) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 && args[i + 1] !== undefined && !args[i + 1].startsWith('--') ? args[i + 1] : fallback;
};
const port = Number(opt('port', process.env.MOCK_API_PORT ?? 8080));
const fail = args.includes('--fail');
const delayMs = Number(opt('delay-ms', 0));

const BUILD_ID = 1;
const RUN_ID = 1;
const rwa = JSON.parse(readFileSync(path.join(publicDir, 'data', 'rwa.json'), 'utf8'));
const exportBody = JSON.stringify({ ...rwa, build_id: BUILD_ID, run_id: RUN_ID });
const exportSha = createHash('sha256').update(exportBody).digest('hex');

// ---- status and runs -------------------------------------------------------------------------------------------------
const started = Date.parse(rwa.generated_at);
const iso = (ms) => new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z');

const okRows = (seconds, count = 3) =>
  rwa.underlyings.slice(0, count).map((u) => ({ underlying_id: u.id, ticker: u.ticker, status: 'ok', error: null, duration_seconds: seconds }));
const firstValues = (list, pick, n) => [...new Set(list.map(pick))].slice(0, n);
// like compute/changes.go: signed eligible-position deltas per network and protocol, reclassified CMC ids (first_build only on the first build)
const detailsOf = () => ({
  eligible_by_network: Object.fromEntries(firstValues(rwa.opportunities, (o) => o.network, 3).map((n, i) => [n, 5 - 4 * i])),
  eligible_by_platform: Object.fromEntries(firstValues(rwa.opportunities, (o) => o.platform, 3).map((p, i) => [p, 4 - 3 * i])),
  reclassified: rwa.tokens.slice(2, 3).map((t) => t.id)
});
const namedIssuer = rwa.issuers.find((i) => i.name) ?? rwa.issuers[0];

// `detail` is what /runs/{id} adds to the list item
const RUNS = [
  {
    id: 3,
    run_key: 'mock-partial',
    mode: 'refresh',
    trigger: 'schedule',
    status: 'partial',
    started_at: iso(started + 2 * 86400_000),
    finished_at: iso(started + 2 * 86400_000 + 540_000),
    duration_seconds: 540,
    selected_count: 125,
    completed_count: 124,
    failed_count: 1,
    cmc_requests: 40,
    portals_requests: 910,
    cmc_credits: 60,
    warnings_count: 2,
    error: null,
    build_id: null,
    changes: null,
    detail: {
      warnings: ['Portals search for IREN returned 502 after 3 retries', 'CMC rate limit hit once, backed off 30s'],
      underlyings: [
        ...okRows(4.2),
        { underlying_id: 'US46267X1081', ticker: 'IREN', status: 'failed', error: 'portals: HTTP 502', duration_seconds: 12.5 }
      ],
      changes: null
    }
  },
  {
    id: 2,
    run_key: 'mock-refresh',
    mode: 'refresh',
    trigger: 'schedule',
    status: 'succeeded',
    started_at: iso(started + 86400_000),
    finished_at: iso(started + 86400_000 + 600_000),
    duration_seconds: 600,
    selected_count: 125,
    completed_count: 125,
    failed_count: 0,
    cmc_requests: 41,
    portals_requests: 930,
    cmc_credits: 62,
    warnings_count: 0,
    error: null,
    build_id: BUILD_ID,
    changes: { new_tokens_count: 2, removed_tokens_count: 0, new_positions_count: 14, gone_positions_count: 6, eligible_delta: 8 },
    detail: {
      warnings: [],
      underlyings: [
        ...okRows(4.0, 7),
        { underlying_id: rwa.underlyings[7].id, ticker: rwa.underlyings[7].ticker, status: 'skipped', error: 'not due yet', duration_seconds: 0 }
      ],
      changes: {
        new_tokens: rwa.tokens.slice(0, 2).map((t) => t.id),
        removed_tokens: [999999],
        new_issuers: [namedIssuer.id],
        new_positions_count: 14,
        gone_positions_count: 6,
        eligible_delta: 8,
        details: detailsOf()
      }
    }
  },
  {
    id: 4,
    run_key: 'mock-rebuild',
    mode: 'rebuild',
    trigger: 'manual',
    status: 'succeeded',
    started_at: iso(started + 3 * 86400_000),
    finished_at: iso(started + 3 * 86400_000 + 20_000),
    duration_seconds: 20,
    selected_count: 125,
    completed_count: 125,
    failed_count: 0,
    cmc_requests: 0,
    portals_requests: 0,
    cmc_credits: 0,
    warnings_count: 0,
    error: null,
    build_id: BUILD_ID,
    changes: null,
    detail: { warnings: [], underlyings: [], changes: null }
  },
  {
    id: 1,
    run_key: 'import-1x',
    mode: 'import',
    trigger: 'import',
    status: 'succeeded',
    started_at: iso(started),
    finished_at: iso(started + 5_000),
    duration_seconds: 5,
    selected_count: 125,
    completed_count: 125,
    failed_count: 0,
    cmc_requests: 0,
    portals_requests: 0,
    cmc_credits: 0,
    warnings_count: 0,
    error: null,
    build_id: BUILD_ID,
    changes: null,
    detail: { warnings: [], underlyings: [], changes: null }
  }
];

RUNS.sort((a, b) => b.id - a.id); // newest first, like the API

function run(id, withDetail) {
  const found = RUNS.find((r) => r.id === id);
  if (!found) return null;
  const { detail, ...item } = found;
  return withDetail ? { ...item, warnings: detail.warnings, underlyings: detail.underlyings, changes: detail.changes } : item;
}

const lastRun = ({ id, run_key, mode, trigger, status, started_at, finished_at, completed_count, failed_count, error }) => ({
  id, run_key, mode, trigger, status, started_at, finished_at, completed_count, failed_count, error
});

// the current build is the export of run 1 (the 1.x import); the later runs are listed for the updates page
const status = () => ({
  current_build: {
    id: BUILD_ID,
    run_id: RUN_ID,
    generated_at: rwa.generated_at,
    score_version: rwa.score_version,
    export_sha256: exportSha,
    schema_version: rwa.schema_version
  },
  last_run: lastRun(run(3, false)),
  next_run_at: iso(Date.now() + 6 * 3600_000),
  schedule: { cron: '0 */6 * * *', interval: null },
  collector_version: 'mock-0.1.0',
  api_version: 'v1'
});

// ---- history ---------------------------------------------------------------------------------------------------------
const num = (v) => (v === null || v === undefined || v === '' ? null : Number(v));
const sum = (list, f) => list.reduce((a, x) => a + (num(f(x)) ?? 0), 0);
const field = (name) => (e) => e[name];

// current value of a metric on an export row (top level, `counts` or `yield` of a token); null when the export has none
const current = (row, name) => {
  const v = row[name] ?? row.counts?.[name] ?? row.yield?.[name];
  if (v !== undefined) return num(v);
  if (name === 'deployments_count' && row.deployments) return row.deployments.length;
  if (name === 'indexed_deployments_count' && row.deployments) return row.deployments.filter((d) => d.status === 'indexed').length;
  return null;
};
const eligibleLiquidity = () => sum(rwa.opportunities.filter((o) => o.eligibility === 'eligible'), field('liquidity_usd'));
const summaryRow = {
  ...rwa.summary,
  tokenized_market_cap_usd: sum(rwa.underlyings, field('tokenized_market_cap_usd')),
  tokenized_volume_24h_usd: sum(rwa.underlyings, field('tokenized_volume_24h_usd')),
  eligible_liquidity_usd: eligibleLiquidity()
};

const ENTITIES = {
  summary: { find: () => summaryRow },
  underlyings: { find: (id) => rwa.underlyings.find((u) => u.id === id || u.ticker === id) },
  tokens: { find: (id) => rwa.tokens.find((t) => String(t.id) === id) },
  issuers: { find: (id) => rwa.issuers.find((i) => i.id === id) },
  opportunities: { find: (id) => rwa.opportunities.find((o) => o.key === id) }
};
for (const [entity, def] of Object.entries(ENTITIES)) def.metrics = COLUMNS[entity];

// deterministic PRNG (mulberry32) seeded by a string
function rng(seed) {
  let h = 1779033703 ^ seed.length;
  for (let i = 0; i < seed.length; i++) h = (Math.imul(h ^ seed.charCodeAt(i), 3432918353) << 13) | (h >>> 19);
  let a = h >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const DAYS = 10;
const DAY_MS = 86400_000;
const LIMIT_DEFAULT = 1000;
const LIMIT_MAX = 5000;
const bad = (message, extra = {}) => ({ status: 400, body: { error: { code: 'bad_request', message, ...extra } } });

function history(entity, id, query) {
  const def = ENTITIES[entity];
  const allowed = def.metrics;
  const metrics = (query.get('metrics') ?? '').split(',').filter(Boolean);
  if (!metrics.length) return bad('metrics is required', { allowed });
  const unknown = metrics.find((m) => !allowed.includes(m));
  if (unknown) return { status: 400, body: { error: { code: 'unknown_metric', message: `unknown metric: ${unknown}`, metric: unknown, allowed } } };
  const row = def.find(id);
  if (!row) return { status: 404, body: { error: { code: 'not_found', message: `${entity} ${id} not found` } } };

  const step = query.get('step') ?? 'run';
  if (!['run', 'day', 'week'].includes(step)) return bad('step must be run, day or week');
  // CLOCK: "now" of the mock history is the day after the export, so a range counted from the real today still holds its points
  const clockShift = Date.now() - (Math.floor(Date.parse(rwa.generated_at) / DAY_MS) * DAY_MS + DAY_MS * 1.5);
  const from = (query.get('from') ? Date.parse(query.get('from')) : Date.now() - 365 * DAY_MS) - clockShift; // default: a year back, like the API
  const to = query.get('to') ? Date.parse(query.get('to')) - clockShift : Infinity;
  if (Number.isNaN(from) || Number.isNaN(to)) return bad('from and to must be dates');
  const limitRaw = query.get('limit') === null ? LIMIT_DEFAULT : Number(query.get('limit'));
  if (!Number.isInteger(limitRaw) || limitRaw < 1) return bad('limit must be a positive integer');
  const limit = Math.min(limitRaw, LIMIT_MAX);

  // series[0] is the current value (today), series[d] the value d days earlier: a ±5% walk backwards from today
  const series = Object.fromEntries(
    metrics.map((m) => {
      const base = current(row, m);
      const random = rng(`${entity}/${id}/${m}`);
      const walk = [base];
      for (let d = 1; d < DAYS; d++) walk.push(base === null ? null : walk[d - 1] * (1 + (random() - 0.5) * 0.1));
      return [m, walk];
    })
  );
  const end = Math.floor(Date.parse(rwa.generated_at) / DAY_MS) * DAY_MS + DAY_MS / 2;
  const points = [];
  for (let d = DAYS - 1; d >= 0; d -= step === 'week' ? 7 : 1) {
    const observed = end - d * DAY_MS;
    if (observed < from || observed > to) continue;
    const values = {};
    for (const m of metrics) {
      const v = series[m][d];
      const type = metricType(m);
      // money and APY are decimal strings, counts and scores numbers (like the export and the API)
      values[m] = v === null ? null : type === 'money' ? v.toFixed(2) : type === 'percent' ? v.toFixed(4) : type === 'count' ? Math.round(v) : Math.round(v * 100) / 100;
    }
    points.push({ run_id: RUN_ID + (DAYS - 1 - d), observed_at: iso(observed), values });
  }
  const keep = query.get('points') === null ? points.length : Number(query.get('points'));
  const limited = keep > 0 ? points.slice(-keep) : keep === 0 ? [] : points;
  return {
    status: 200,
    body: { entity, id: entity === 'summary' ? null : id, metrics, step, points: limited.slice(-limit), truncated: limited.length > limit }
  };
}

// ---- http ------------------------------------------------------------------------------------------------------------
const types = { '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg', '.svg': 'image/svg+xml', '.webp': 'image/webp' };
const CORS = {
  'access-control-allow-origin': '*',
  'access-control-allow-headers': 'if-none-match, content-type',
  'access-control-expose-headers': 'etag'
};

const json = (res, status, body, headers = {}) => {
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', ...CORS, ...headers });
  res.end(typeof body === 'string' ? body : JSON.stringify(body));
};
const err = (res, status, code, message) => json(res, status, { error: { code, message } });

async function handle(req, res) {
  if (delayMs) await new Promise((r) => setTimeout(r, delayMs));
  const url = new URL(req.url, 'http://mock.local');
  const p = url.pathname.replace(/\/+$/, '') || '/';
  if (req.method === 'OPTIONS') {
    res.writeHead(204, { ...CORS, 'access-control-allow-methods': 'GET, HEAD, OPTIONS' });
    return res.end();
  }
  if (req.method !== 'GET' && req.method !== 'HEAD') return err(res, 405, 'method_not_allowed', 'only GET and HEAD are supported');

  if (p.startsWith('/logos/')) {
    const logos = path.join(publicDir, 'logos');
    const file = path.join(publicDir, decodeURIComponent(p));
    const info = file.startsWith(logos + path.sep) ? statSync(file, { throwIfNoEntry: false }) : undefined;
    if (!info?.isFile()) return err(res, 404, 'not_found', 'logo not found');
    const etag = `"${info.size}-${Math.round(info.mtimeMs)}"`;
    const headers = { etag, 'cache-control': 'public, max-age=86400', ...CORS };
    if (req.headers['if-none-match'] === etag) {
      res.writeHead(304, headers);
      return res.end();
    }
    res.writeHead(200, { 'content-type': types[path.extname(file)] ?? 'application/octet-stream', 'content-length': info.size, ...headers });
    return req.method === 'HEAD' ? res.end() : createReadStream(file).pipe(res);
  }
  if (!p.startsWith('/api/v1/')) return err(res, 404, 'not_found', 'not found');
  if (fail) return err(res, 503, 'unavailable', 'mock API started with --fail');
  const route = p.slice('/api/v1/'.length);

  if (route === 'export') {
    const build = url.searchParams.get('build');
    if (build !== null) {
      if (build !== String(BUILD_ID)) return err(res, 404, 'not_found', `build ${build} not found`);
      return json(res, 200, exportBody, { etag: `"${exportSha}"`, 'cache-control': 'public, max-age=31536000, immutable' });
    }
    const headers = { etag: `"${exportSha}"`, 'cache-control': 'public, max-age=60, stale-while-revalidate=600' };
    if (req.headers['if-none-match'] === `"${exportSha}"`) {
      res.writeHead(304, { ...headers, ...CORS });
      return res.end();
    }
    return json(res, 200, exportBody, headers);
  }
  if (route === 'status') return json(res, 200, status());
  if (route === 'runs') {
    const limit = Math.max(1, Number(url.searchParams.get('limit') ?? 50));
    const after = url.searchParams.get('cursor') ? Number(url.searchParams.get('cursor')) : Infinity; // cursor = last id seen
    const rows = RUNS.filter((r) => r.id < after).map((r) => run(r.id, false));
    return json(res, 200, { items: rows.slice(0, limit), next_cursor: rows.length > limit ? String(rows[limit - 1].id) : null });
  }
  const runMatch = route.match(/^runs\/(\d+)$/);
  if (runMatch) {
    const r = run(Number(runMatch[1]), true);
    return r ? json(res, 200, r) : err(res, 404, 'not_found', `run ${runMatch[1]} not found`);
  }
  if (route === 'history/metrics') {
    const entities = Object.fromEntries(
      Object.entries(ENTITIES).map(([e, d]) => [e, d.metrics.map((name) => ({ name, type: metricType(name) }))])
    );
    return json(res, 200, { entities });
  }
  const hist = route.match(/^history\/(summary|underlyings|tokens|issuers|opportunities)(?:\/(.+))?$/);
  if (hist) {
    const [, entity, id] = hist;
    if ((entity === 'summary') === (id !== undefined)) return err(res, 404, 'not_found', 'not found');
    const r = history(entity, id === undefined ? null : decodeURIComponent(id), url.searchParams);
    return json(res, r.status, r.body);
  }
  return err(res, 404, 'not_found', 'not found');
}

createServer((req, res) => {
  handle(req, res).catch((e) => {
    // a malformed percent escape is the caller's mistake; anything else is a bug of the mock
    if (e instanceof URIError) return err(res, 400, 'bad_request', 'malformed URL');
    console.error(`${req.method} ${req.url}: ${e.stack ?? e}`);
    if (!res.headersSent) err(res, 500, 'internal', 'internal error');
  });
}).listen(port, () => console.log(`mock API on http://localhost:${port}${fail ? ' (--fail: every /api/* answers 503)' : ''}`));
