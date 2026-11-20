// Cache of the API data the server renders from: the current export (with its ETag); the status on demand.
// The export is refreshed every REFRESH_MS with If-None-Match (an unchanged build costs a 304); until the first success
// it is retried every RETRY_MS. After that a failing API changes nothing: the cached export keeps being served.

const REFRESH_MS = 60_000;
const RETRY_MS = 10_000;
const TIMEOUT_MS = 15_000;
const STATUS_TTL_MS = 60_000;

export function createDataStore({ apiUrl, log = console }) {
  const state = { export: null, etag: null, buildId: null, loadedAt: null, status: null, statusAt: 0 };
  let timer;
  let stopped = false;

  async function get(path, headers = {}) {
    return fetch(`${apiUrl}${path}`, { headers: { accept: 'application/json', ...headers }, signal: AbortSignal.timeout(TIMEOUT_MS) });
  }

  async function refreshExport() {
    const res = await get('/api/v1/export', state.etag ? { 'if-none-match': state.etag } : {});
    if (res.status === 304) return;
    if (!res.ok) throw new Error(`GET /api/v1/export: HTTP ${res.status}`);
    const data = await res.json();
    if (typeof data.build_id !== 'number') throw new Error('GET /api/v1/export: no build_id in the document');
    state.export = data;
    state.etag = res.headers.get('etag');
    state.loadedAt = Date.now();
    if (data.build_id !== state.buildId) log.log(`export: build ${data.build_id} (run ${data.run_id})`);
    state.buildId = data.build_id;
  }

  async function tick() {
    try {
      await refreshExport();
    } catch (err) {
      log.warn(`export refresh failed${state.export ? ' (serving the cached build ' + state.buildId + ')' : ''}: ${err.message}`);
    }
    if (!stopped) timer = setTimeout(tick, state.export ? REFRESH_MS : RETRY_MS);
  }

  return {
    get export() {
      return state.export;
    },
    get buildId() {
      return state.buildId;
    },
    // /api/v1/status, fetched when asked for and cached for STATUS_TTL_MS; the last known value when the API fails
    async getStatus() {
      if (Date.now() - state.statusAt < STATUS_TTL_MS) return state.status;
      try {
        const res = await get('/api/v1/status');
        if (res.ok) {
          state.status = await res.json();
          state.statusAt = Date.now();
        }
      } catch {
        // keep the previous value
      }
      return state.status;
    },
    start() {
      return tick();
    },
    stop() {
      stopped = true;
      clearTimeout(timer);
    }
  };
}
