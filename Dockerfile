FROM golang:1.25.10-alpine AS build
ARG TARGETOS="linux"
ARG TARGETARCH="amd64"

RUN apk add --no-cache build-base

WORKDIR /usr/local/build
COPY go.mod .
COPY go.sum .

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go mod download

COPY . .

# Build binary output
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /usr/local/bin/stake_indexer -ldflags '-s -w' .

FROM alpine:3.18.4
RUN apk add --no-cache tzdata && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone

RUN adduser -u 1000 -D sato -h /data
USER sato
WORKDIR /data/

COPY --chown=sato --from=build /usr/local/bin/stake_indexer /data/stake_indexer

ENTRYPOINT ["./stake_indexer"]
