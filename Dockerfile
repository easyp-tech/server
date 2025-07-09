FROM golang:1.22-alpine AS builder

RUN apk update && apk add --no-cache \
    ca-certificates \
    git \
    build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /easyp-server ./cmd/easyp

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /easyp-server /easyp-server

COPY --from=builder /app/local.config.yml /local.config.yml

ENTRYPOINT ["/easyp-server"]
CMD ["-cfg", "/local.config.yml"]