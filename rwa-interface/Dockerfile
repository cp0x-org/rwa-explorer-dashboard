FROM node:22-alpine AS builder

WORKDIR /app

# pnpm version comes from "packageManager" in package.json
RUN corepack enable

COPY pnpm-lock.yaml package.json ./
RUN pnpm install --frozen-lockfile

COPY . .

# Client bundle (dist/) and server bundle (dist-ssr/). The data comes from the API at runtime, not from the image:
# public/data and public/logos are copied into dist/ by Vite but never served from it, so they are dropped.
RUN pnpm run build && rm -rf dist/data dist/logos

FROM node:22-alpine

WORKDIR /app

# server/ is plain Node (no dependencies): the app bundle in dist-ssr/ already contains everything it renders with
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/dist-ssr ./dist-ssr
COPY server ./server
COPY package.json ./

# API_URL: where the server reads the export from; SITE_URL: public address (canonical links, sitemap.xml)
ENV NODE_ENV=production \
    PORT=4173 \
    API_URL=http://rwa-api:8080 \
    SITE_URL=https://rwa.cp0x.com

EXPOSE 4173

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/healthz" || exit 1

USER node

CMD ["node", "server/index.mjs"]
