# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-bookworm AS builder

WORKDIR /src

# CGO + C
RUN apt-get update && apt-get install -y --no-install-recommends build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
# -p 1 trades build time for lower peak memory
RUN go build -p 1 -tags sqlite_fts5 -o /out/twork ./cmd/twork

FROM debian:bookworm-slim

# ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/twork /app/twork

# point database.sqlite and telegram.session
VOLUME ["/app/data"]

ENTRYPOINT ["/app/twork"]
CMD ["--config", "/app/config.yaml"]