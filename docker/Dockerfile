# Stage 1 — Build the Go binary
FROM golang:1.22-bookworm AS builder

WORKDIR /app

# Copy dependency manifests first for Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build a statically-linked binary
# CGO_ENABLED=1 is required by go-sqlite3 (CGO driver)
RUN CGO_ENABLED=1 GOOS=linux go build -o /job-auto-apply ./cmd/main.go

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2 — Runtime image with Playwright + Chromium
# Use the official Playwright Go image which includes Chromium pre-installed
FROM mcr.microsoft.com/playwright/go:v0.4501.0-jammy

# Install SQLite runtime libraries needed by the CGO driver
RUN apt-get update && apt-get install -y --no-install-recommends \
    libsqlite3-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy compiled binary from builder
COPY --from=builder /job-auto-apply /usr/local/bin/job-auto-apply

# Copy configs and resume directories (populated by user via bind mount or COPY)
COPY configs/ ./configs/
COPY resume/   ./resume/

# Create data directory for SQLite database
RUN mkdir -p /app/data

# Playwright browser path used by playwright-go
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

# Entrypoint — supports all CLI flags passed via `docker run` or compose
ENTRYPOINT ["/usr/local/bin/job-auto-apply"]
CMD ["--headless=true"]
