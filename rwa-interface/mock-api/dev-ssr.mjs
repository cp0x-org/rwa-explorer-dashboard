// `pnpm mock:ssr`: the MOCK API (:8080, frozen 1.x snapshot public/data/rwa.json) and the SSR server (:4173) on it, in one
// terminal; Ctrl-C stops both. Not the backend: for real data run `rwa api` and `pnpm dev:ssr` (see docs/run-locally.md).
// Run `pnpm build` first (the script `mock:ssr` does).
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const run = (file, env = {}) => spawn(process.execPath, [file], { cwd: root, stdio: 'inherit', env: { ...process.env, ...env } });

console.log('\n  MOCK API: the site shows the frozen 1.x snapshot (public/data/rwa.json), not the backend.\n  For real data: run `go run ./cmd/rwa api` in rwa-backend/ and `pnpm dev:ssr` here (docs/run-locally.md).\n');

const children = [
  run('mock-api/server.mjs'),
  run('server/index.mjs', { NODE_ENV: 'production', API_URL: 'http://127.0.0.1:8080', PORT: process.env.PORT ?? '4173' })
];
const stop = () => children.forEach((c) => c.kill());
children.forEach((c) => c.on('exit', stop));
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, stop);
