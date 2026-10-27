FROM golang:1.27.3-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o lys ./cmd/lys

# Lighthouse builds this stage before deploying: a failure stops the deploy.
FROM builder AS test
RUN apk add --no-cache gcc musl-dev
RUN unformatted="$(gofmt -l .)" && [ -z "$unformatted" ] || { echo "These files need gofmt:"; echo "$unformatted"; exit 1; }
RUN go vet ./...
RUN CGO_ENABLED=1 go test -race -short -count=1 ./...

# Pinned on purpose; bump it deliberately (e.g. for security fixes).
FROM alpine:3.24
WORKDIR /app

COPY --from=builder /app/lys /lys

EXPOSE 2400
ENV NOTIFICATION_PORT=2400
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:2400/health || exit 1
CMD ["/lys"]
