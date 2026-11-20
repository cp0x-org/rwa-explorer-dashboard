# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS builder

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# go.mod pins `toolchain go1.26.4`, the same as this image's Go, so nothing is downloaded at build time.
# Bump both together (the base tag and the toolchain line); with an older image Go would fetch the toolchain from the network.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/rwa ./cmd/rwa

FROM alpine:3.21.7

# the HEALTHCHECK uses busybox wget (already in alpine); tzdata for time zone names; ca-certificates for HTTPS to CMC/Portals
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 -s /sbin/nologin rwa

COPY --from=builder /out/rwa /usr/local/bin/rwa
# Defaults for ASSETS_PATH / SETTINGS_PATH; the stack overrides them by mounting its own files over /config/*.json
COPY config/ /config/

USER rwa
WORKDIR /

EXPOSE 8080

# Meaningful for `api`; the collector has no HTTP server and disables it in compose (healthcheck: disable: true).
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:8080/readyz" || exit 1

ENTRYPOINT ["rwa"]
CMD ["api"]
