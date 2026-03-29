FROM golang:1.25.0-alpine AS builder

COPY .. /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
WORKDIR /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
RUN go clean --modcache
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o ./.bin ./cmd/main/main.go


FROM scratch AS runner

WORKDIR /dddance-back/

COPY --from=builder /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/.bin .

COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /

COPY .env .
ENV TZ="Europe/Moscow"
ENV ZONEINFO=/zoneinfo.zip


EXPOSE 5458

ENTRYPOINT ["./.bin"]
