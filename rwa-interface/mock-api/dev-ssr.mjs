// `pnpm dev:ssr`: the mock API (:8080) and the SSR server (:4173) on it, in one terminal; Ctrl-C stops both.
// Run `pnpm build` first (the script `dev:ssr` does).
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const run = (file, env = {}) => spawn(process.execPath, [file], { cwd: root, stdio: 'inherit', env: { ...process.env, ...env } });

const children = [
  run('mock-api/server.mjs'),
  run('server/index.mjs', { NODE_ENV: 'production', API_URL: 'http://127.0.0.1:8080', PORT: process.env.PORT ?? '4173' })
];
const stop = () => children.forEach((c) => c.kill());
children.forEach((c) => c.on('exit', stop));
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, stop);
