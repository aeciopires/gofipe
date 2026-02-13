FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
# Builder stage

WORKDIR /app

# REQUIRED for multi-arch builds
ARG TARGETOS
ARG TARGETARCH

COPY . .

RUN go mod download

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -o /out/gofipe main.go

#----------------------------------    

FROM alpine:3.23.3

# About image
LABEL maintainer="Aécio Pires" \
      date_create="02/13/2026" \
      version="3.0.0" \
      description="A web application that provides a user-friendly interface to search the Brazilian FIPE vehicle price table." \
      licensce="GPL-3.0"

WORKDIR /app

# Set the default port if not provided during runtime
ENV PORT=8080

# Copy files to image
COPY --from=builder /out/gofipe /app/gofipe
COPY templates/ /app/templates
COPY static/ /app/static

# Exposing port of application
EXPOSE ${PORT}

CMD ["./gofipe"]

