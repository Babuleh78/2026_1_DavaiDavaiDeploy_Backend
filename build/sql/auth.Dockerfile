FROM golang:1.24.0-alpine AS builder

COPY .. /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
COPY .env .
WORKDIR /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
ENV GOPROXY=https://proxy.golang.org,direct
ENV GO111MODULE=on

ENV TZ="Europe/Moscow"
ENV ZONEINFO=/zoneinfo.zip

EXPOSE 5459
RUN go mod download
RUN go build -o ./.bin ./cmd/auth/main.go

ENTRYPOINT ["./.bin"]