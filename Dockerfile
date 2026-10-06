FROM golang:1.25.1-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o lys ./cmd/lys

FROM alpine:latest
WORKDIR /app

RUN mkdir -p /app/lys

COPY --from=builder /app/lys /lys

EXPOSE 2400
ENV NOTIFICATION_PORT=2400
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:2400/health || exit 1
CMD ["/lys"]