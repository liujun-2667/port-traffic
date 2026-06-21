FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git
ENV GOFLAGS=-mod=mod GONOSUMDB=* GONOSUMCHECK=* GOFLAGS=-mod=mod
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM golang:1.21-alpine
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY config /app/config
COPY migrations /app/migrations
ENV CONFIG_PATH=/app/config/port.yaml \
    DATABASE_URL="postgres://port:port@db:5432/porttraffic?sslmode=disable" \
    PORT=8080
EXPOSE 8080
CMD ["/app/server"]
