FROM node:22-alpine AS builder

WORKDIR /app

# pnpm version comes from "packageManager" in package.json
RUN corepack enable

COPY pnpm-lock.yaml package.json ./
RUN pnpm install --frozen-lockfile

COPY . .

# Site data (public/data, public/logos) is baked into dist: rebuild the image after `rwa-pipeline sync`
RUN pnpm run build

FROM node:22-alpine

WORKDIR /app

RUN npm install -g serve@14.2.4

COPY --from=builder /app/dist ./dist

EXPOSE 4173

CMD ["serve", "-s", "dist", "-l", "4173"]
