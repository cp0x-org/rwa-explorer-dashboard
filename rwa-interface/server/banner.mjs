// Startup banner of the SSR server, in the same frame as the Go services (rwa-backend/internal/banner): what the
// process is, where it listens, what it connects to and what it serves. Off with STARTUP_BANNER=false.
import os from 'node:os';

const MIN_WIDTH = 64;
const MAX_WIDTH = 108;
const KEY_WIDTH = 12;

const len = (s) => [...s].length;
const pad = (s, n) => s + ' '.repeat(Math.max(0, n - len(s)));

function wrap(text, width) {
  const out = [];
  for (const para of String(text).split('\n')) {
    const words = para.split(/\s+/).filter(Boolean);
    if (!words.length) {
      out.push('');
      continue;
    }
    let cur = words[0];
    for (const w of words.slice(1)) {
      if (len(cur) + 1 + len(w) > width) {
        out.push(cur);
        cur = `  ${w}`;
      } else cur += ` ${w}`;
    }
    out.push(cur);
  }
  return out;
}

/** sections: [{ title, rows: [[key, value], …] }] */
export function renderBanner(title, subtitle, sections) {
  const lines = [{ text: title }, ...(subtitle ? wrap(subtitle, MAX_WIDTH).map((text) => ({ text })) : [])];
  for (const s of sections) {
    if (!s.rows.length) continue;
    lines.push({ rule: s.title });
    for (const [key, value] of s.rows) {
      wrap(value, MAX_WIDTH - KEY_WIDTH - 2).forEach((v, i) => lines.push({ text: `${pad(i === 0 ? key : '', KEY_WIDTH)}  ${v}` }));
    }
  }
  const width = Math.max(MIN_WIDTH, ...lines.map((l) => (l.rule ? len(l.rule) + 4 : len(l.text))));
  const out = [`╭${'─'.repeat(width + 2)}╮`];
  for (const l of lines) {
    if (l.rule) {
      const head = `├─ ${l.rule} `;
      out.push(`${head}${'─'.repeat(width + 3 - len(head))}┤`);
    } else out.push(`│ ${pad(l.text, width)} │`);
  }
  out.push(`╰${'─'.repeat(width + 2)}╯`);
  return out.join('\n');
}

export function printBanner({ port, apiUrl, siteUrl, apiPublicUrl, refreshMs, retryMs, templateVersion }) {
  if (process.env.STARTUP_BANNER === 'false') return;
  const version = process.env.APP_VERSION || 'dev';
  const sections = [
    {
      title: 'service',
      rows: [
        ['version', `${version} (node ${process.version}, ${process.platform}/${process.arch}, template ${templateVersion})`],
        ['host', `${os.hostname()}, pid ${process.pid}`],
        ['started', `${new Date().toISOString().replace('T', ' ').replace(/\.\d+Z$/, ' UTC')}`],
        ['logs', 'plain text to stdout/stderr']
      ]
    },
    {
      title: 'listens',
      rows: [
        ['http', `:${port} (PORT)`],
        ['public', siteUrl ? `${siteUrl} (SITE_URL: canonical links, sitemap.xml, robots.txt)` : 'not set (SITE_URL): links use the build default'],
        ['public api', apiPublicUrl ? `${apiPublicUrl} (API_PUBLIC_URL: named in llms.txt, JSON-LD and <head>)` : 'not set: the site\'s own /api/'],
        ['pages', 'every page rendered on the server, English and Chinese; /sitemap.xml, /robots.txt, /llms.txt'],
        ['static', 'dist/: hashed /assets/* cached for a year, the rest for an hour'],
        ['health', '/healthz: 503 until the first export is loaded']
      ]
    },
    {
      title: 'connects to',
      rows: [
        ['api', `${apiUrl} (API_URL)`],
        ['export', `GET /api/v1/export every ${refreshMs / 1000} s (If-None-Match: 304 if unchanged); every ${retryMs / 1000} s until the first`],
        ['proxy', '/api/* and /logos/* forwarded to the API as they are (one origin for the browser)'],
        ['database', 'none: the site reads only the API']
      ]
    }
  ];
  console.log(renderBanner('RWA Explorer · web', 'SSR site: renders every page from the API export', sections));
}
