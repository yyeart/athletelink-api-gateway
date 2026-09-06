FROM golang:1.27.1-alpine3.24 AS builder
WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/gateway \
    ./cmd/gateway

FROM alpine:3.24

RUN apk add --no-cache ca-certificates \
    && addgroup -S gateway \
    && adduser -S -D -H -G gateway gateway

COPY --from=builder --chown=gateway:gateway \
    /out/gateway /usr/local/bin/gateway

USER gateway
EXPOSE 8080
ENTRYPOINT [ "/usr/local/bin/gateway" ]