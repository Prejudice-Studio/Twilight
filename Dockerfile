# ============================================================================
# Twilight Go Backend — Multi-stage Docker build
# ============================================================================
# Usage:
#   docker build -t twilight-backend .
#   docker run -v ./config.toml:/app/config.toml twilight-backend
#
# Run modes:
#   all       — API + scheduler + bot (single process, recommended for Docker)
#   api       — API only
#   scheduler — scheduler only
#   bot       — Telegram bot only
# ============================================================================

# ---- Stage 1: Build Go binary ----
FROM golang:1.25-bookworm AS build

WORKDIR /src

# Cache Go module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o bin/twilight ./cmd/twilight

# ---- Stage 2: Minimal runtime ----
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        tzdata \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r twilight -g 1001 && useradd -r -g twilight -u 1001 -m twilight

WORKDIR /app
COPY --from=build --chown=twilight:twilight /src/bin/twilight .

# Runtime directories
RUN mkdir -p db db/backups uploads && chown -R twilight:twilight .

USER twilight

# Default: run all services in one process
ENTRYPOINT ["./twilight"]
CMD ["all", "--host", "0.0.0.0", "--port", "5000", "--config", "config.toml"]

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ./twilight version || exit 1

STOPSIGNAL SIGTERM
