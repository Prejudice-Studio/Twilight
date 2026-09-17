# ============================================================================
# Twilight WebUI — Next.js standalone Docker build
# ============================================================================
# Build:
#   docker build -t twilight-webui -f webui/Dockerfile webui/
#
# Run:
#   docker run -e NEXT_PUBLIC_API_URL=https://api.example.com twilight-webui
#
# The backend API base URL MUST be set via NEXT_PUBLIC_API_URL at build time
# (Next.js bakes it into the JS bundle). If you need runtime-dynamic API URLs,
# set NEXT_PUBLIC_API_URL to empty and configure the Next.js rewrite proxy
# via BACKEND_URL environment variable at runtime.
# ============================================================================

# ---- Stage 1: Build ----
FROM node:22-bookworm-slim AS build

WORKDIR /app

# Next.js bakes NEXT_PUBLIC_* into the browser bundle, so this must be present at
# build time. Leave it empty for a same-origin deployment: the browser then calls
# its own /api/* and next.config.mjs rewrites that to BACKEND_URL at runtime.
ARG NEXT_PUBLIC_API_URL=
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

# Enable pnpm
RUN corepack enable && corepack prepare pnpm@9.15.0 --activate

# Install dependencies
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy source and build
COPY . .
RUN pnpm build

# ---- Stage 2: Runtime ----
FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r nextjs -g 1001 && useradd -r -g nextjs -u 1001 -m nextjs

WORKDIR /app

# Copy standalone output (Next.js output: 'standalone')
COPY --from=build --chown=nextjs:nextjs /app/.next/standalone ./

# Static assets must be copied alongside the standalone output
COPY --from=build --chown=nextjs:nextjs /app/.next/static ./.next/static

# Public assets (favicon, robots.txt, etc.)
COPY --from=build --chown=nextjs:nextjs /app/public ./public

USER nextjs

ENV NODE_ENV=production
ENV PORT=3000

EXPOSE 3000

CMD ["node", "server.js"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD node -e "fetch('http://localhost:3000').then(r => process.exit(r.ok ? 0 : 1)).catch(() => process.exit(1))"

STOPSIGNAL SIGTERM
