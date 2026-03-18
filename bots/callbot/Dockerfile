# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libolm-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /callbot .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libolm3 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -r -s /bin/false callbot
WORKDIR /app
RUN mkdir /data && chown callbot:callbot /data

COPY --from=builder /callbot /app/callbot

USER callbot

ENV CALLBOT_DB_PATH=/data/callbot.db \
    CONFIG_PATH=/data/config.json

VOLUME ["/data"]

ENTRYPOINT ["/app/callbot"]
