// /sitemap.xml and /robots.txt, generated from the current export and cached until the build id changes.
const escapeXml = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

export function createSitemap({ renderer }) {
  let cached = null; // { buildId, xml }

  return {
    // every crawler, search engines and AI agents alike, may read every page and the public API; only the admin API is
    // closed (nginx answers it with 404 anyway). /llms.txt is the map of the site for language models.
    robots: () =>
      renderer.site
        ? `# RWA Explorer by cp0x. A map of the site for language models: ${renderer.site}/llms.txt\n` +
          `User-agent: *\nAllow: /\nDisallow: /api/v1/admin/\n\nSitemap: ${renderer.site}/sitemap.xml\n`
        : null,

    // every page in both languages with its alternates; lastmod = newest data collection
    xml(data) {
      if (!renderer.site) return null;
      if (cached?.buildId === data.build_id) return cached.xml;
      const newest = data.summary.newest_fetched_at;
      const lastmod = newest ? new Date(newest).toISOString().slice(0, 10) : null;
      const urls = renderer.sitemapHeads(data).map(({ canonical, hreflang }) => {
        const links = hreflang.map(({ lang, href }) => `<xhtml:link rel="alternate" hreflang="${lang}" href="${escapeXml(href)}"/>`);
        return `  <url><loc>${escapeXml(canonical)}</loc>${lastmod ? `<lastmod>${lastmod}</lastmod>` : ''}${links.join('')}</url>`;
      });
      const xml =
        `<?xml version="1.0" encoding="UTF-8"?>\n` +
        `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">\n` +
        `${urls.join('\n')}\n</urlset>\n`;
      cached = { buildId: data.build_id, xml };
      return xml;
    }
  };
}
