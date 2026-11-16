# syntax=docker/dockerfile:1

#----------------------------------
# Builder stage (runs on the build host, cross-compiles to the target platform)
#----------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS builder

WORKDIR /src

# REQUIRED for multi-arch builds
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=dev

# Download dependencies first so this layer is cached between code changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# templates/ and static/ are embedded into the binary via go:embed.
COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w -X main.appVersion=${VERSION}" -o /out/gofipe .

#----------------------------------
# Runtime stage
#----------------------------------
FROM alpine:3.24.4

ARG VERSION=dev

# About image
LABEL maintainer="Aécio Pires" \
      date_create="11/16/2026" \
      version="4.1.0" \
      description="A web application that provides a user-friendly interface to search the Brazilian FIPE vehicle price table." \
      license="GPL-3.0" \
      org.opencontainers.image.source="https://github.com/aeciopires/gofipe" \
      org.opencontainers.image.licenses="GPL-3.0"

# ca-certificates is required to call the FIPE API over HTTPS.
# Versions are not pinned on purpose: Alpine only keeps the latest patch in its repos.
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 gofipe \
    && adduser -S -D -H -u 10001 -G gofipe gofipe

WORKDIR /app

# Port the application listens on (read by the app from $PORT)
ENV PORT=8080

COPY --from=builder /out/gofipe /app/gofipe

USER 10001:10001

EXPOSE ${PORT}

# Shell form is required so ${PORT} is expanded at runtime.
# hadolint ignore=DL3025
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/health" || exit 1

ENTRYPOINT ["/app/gofipe"]
