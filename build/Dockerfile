# -------- Global ARGs (must be before any FROM) --------
# Service selection: server, web or mcp-server
ARG SERVICE=server

# -------- Frontend Builder: Node.js for web assets --------
FROM --platform=$BUILDPLATFORM node:26.11.1-alpine@sha256:143494b1da2945f061539253adc65e4f1569ddf07da2d384c022c791a9d90a4a AS frontend-builder

# Re-declare ARG to use it in this stage
ARG SERVICE

WORKDIR /frontend

# Copy frontend package files and conditionally build
COPY web/package*.json ./

# Install dependencies and build only if SERVICE=web
RUN if [ "$SERVICE" = "web" ]; then \
    npm ci --no-audit --no-fund --ignore-scripts; \
fi

# Copy frontend source
COPY web/ ./

# Build frontend only if SERVICE=web
RUN if [ "$SERVICE" = "web" ]; then \
    npm run build; \
else \
    echo "Skipping frontend build for SERVICE=$SERVICE"; \
    mkdir -p dist; \
fi

# -------- Backend Builder: pinned Go toolchain, no network after mod download --------
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS backend-builder

# Re-declare ARG to use it in this stage
ARG SERVICE

# Fails fast on unknown instructions or errors
SHELL ["/bin/ash", "-eo", "pipefail", "-c"]

# Install minimal build deps only in the builder
RUN apk add --no-cache git build-base

WORKDIR /build

# Copy mod files first for better cache and to freeze deps
COPY go.mod go.sum ./

# Download modules with network, then turn network off for the rest of the build stage
RUN go mod download && go mod verify

# Copy the rest of the source (after deps are cached)
COPY . .

# Build metadata
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

# Compile a static binary with reproducibility and hardening flags
# Notes:
#  - CGO=0 gives a static binary suitable for distroless/static
#  - -trimpath removes local paths
#  - -buildvcs=false prevents embedding VCS info automatically (we pass our own)
#  - -ldflags strip symbols, set version fields; -extldflags '-static' ensures static link
ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -mod=readonly -trimpath -buildvcs=false \
    -ldflags="-w -s -extldflags '-static' \
      -X 'github.com/DefensePoint/keycloak-monitoring/internal/version.Version=${VERSION}' \
      -X 'github.com/DefensePoint/keycloak-monitoring/internal/version.GitCommit=${VCS_REF}' \
      -X 'github.com/DefensePoint/keycloak-monitoring/internal/version.BuildDate=${BUILD_DATE}'" \
    -o /build/kmt-${SERVICE} ./cmd/${SERVICE}

# -------- Runtime: Conditional base image selection --------
# For server: use debian-slim with Chrome for PDF generation
# For web: use distroless for minimal size

# Server runtime with Chrome
FROM debian:13.7-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS runtime-server

# Install Chrome and dependencies for PDF generation
RUN apt-get update && apt-get install -y \
    chromium \
    chromium-sandbox \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mv /usr/lib/chromium/chrome_crashpad_handler /usr/lib/chromium/chrome_crashpad_handler.real \
    && printf '#!/bin/sh\nexec /usr/lib/chromium/chrome_crashpad_handler.real --database=/tmp "$@"\n' > /usr/lib/chromium/chrome_crashpad_handler \
    && chmod +x /usr/lib/chromium/chrome_crashpad_handler

# Create nonroot user and group
RUN groupadd -g 65532 nonroot && \
    useradd -u 65532 -g nonroot -s /bin/false -d /app nonroot

WORKDIR /app

# Copy the server binary
COPY --chown=nonroot:nonroot --chmod=0555 --from=backend-builder /build/kmt-server /app/kmt

# Copy report assets (logo, cover, footer)
COPY --chown=nonroot:nonroot assets /app/assets

# Copy HTML templates for reports
COPY --chown=nonroot:nonroot internal/reports/templates /app/internal/reports/templates

# GODEBUG improves memory release behavior
ENV GODEBUG=madvdontneed=1

# Chrome environment variables
ENV CHROME_BIN=/usr/bin/chromium
ENV CHROME_PATH=/usr/lib/chromium/

STOPSIGNAL SIGTERM
USER nonroot:nonroot

EXPOSE 7888

ENTRYPOINT ["/app/kmt"]
CMD ["-config", "/app/config.yaml"]

# Web runtime with distroless
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime-web

WORKDIR /app

# Copy the web binary
COPY --chown=nonroot:nonroot --chmod=0555 --from=backend-builder /build/kmt-web /app/kmt

# Copy frontend assets
COPY --chown=nonroot:nonroot --chmod=0555 --from=frontend-builder /frontend/dist /app/web/dist

ENV GODEBUG=madvdontneed=1

STOPSIGNAL SIGTERM
USER nonroot:nonroot

WORKDIR /app

EXPOSE 7880

ENTRYPOINT ["/app/kmt"]
CMD ["-config", "/app/config.yaml"]

# MCP server runtime with distroless
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime-mcp-server

WORKDIR /app

# Copy the MCP server binary
COPY --chown=nonroot:nonroot --chmod=0555 --from=backend-builder /build/kmt-mcp-server /app/kmt

ENV GODEBUG=madvdontneed=1

STOPSIGNAL SIGTERM
USER nonroot:nonroot

EXPOSE 7889

ENTRYPOINT ["/app/kmt"]
CMD ["-config", "/app/config.yaml"]

# Final stage: select appropriate runtime based on SERVICE arg
# Re-use the global SERVICE ARG
ARG SERVICE
FROM runtime-${SERVICE} AS runtime
