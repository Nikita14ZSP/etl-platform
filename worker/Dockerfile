# Собирать из корня репозитория: docker build -f worker/Dockerfile .
FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY worker/ worker/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./worker/cmd/worker

FROM alpine:3.20

# bash понадобится Bash-оператору
RUN apk add --no-cache bash ca-certificates \
    && adduser -D -u 10001 worker

COPY --from=build /out/ /usr/local/bin/

USER worker
ENTRYPOINT ["worker"]
