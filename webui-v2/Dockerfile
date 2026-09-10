# Twilight V2 SvelteKit SSR image

FROM node:22-bookworm-slim AS build

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@10.14.0 --activate

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .
RUN pnpm build

FROM node:22-bookworm-slim

RUN groupadd --system --gid 1001 twilight-webui \
    && useradd --system --uid 1001 --gid twilight-webui --create-home twilight-webui

WORKDIR /app
COPY --from=build --chown=twilight-webui:twilight-webui /app/build ./build

USER twilight-webui

ENV NODE_ENV=production
ENV HOST=0.0.0.0
ENV PORT=3000
ENV ORIGIN=http://localhost:3000
ENV BACKEND_URL=http://twilight:5000

EXPOSE 3000

CMD ["node", "build"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD node -e "fetch('http://localhost:3000').then(r => process.exit(r.ok ? 0 : 1)).catch(() => process.exit(1))"

STOPSIGNAL SIGTERM
