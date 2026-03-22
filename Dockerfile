# TunnelBypass — multi-stage build (Go static binary, Alpine runtime, non-root, /data)
#
# Build: golang:1.24-bookworm + GOTOOLCHAIN=auto (go.mod may require newer Go than the image tag).
# Runtime: alpine:3.20, uid 65532, ca-certificates, WORKDIR /data.
# No EXPOSE: map whatever ports your transport uses in docker run / compose.
#
# Interactive default: docker run -it ...  → wizard / main menu (no args).
# Headless transport: docker run ... run ssh   or   docker compose with command: ["run","ssh"]

FROM golang:1.24-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /tunnelbypass ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
	&& adduser -D -H -u 65532 nonroot \
	&& mkdir -p /data \
	&& chown nonroot:nonroot /data

COPY docker/entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

COPY --from=build /tunnelbypass /usr/local/bin/tunnelbypass

USER nonroot
WORKDIR /data

ENV TB_LOG=json
# Fixed data root for portable layout (wizard + run); satisfies run layout checks with TB_PORTABLE=1
ENV TB_DATA_DIR=/data TB_PORTABLE=1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD []
