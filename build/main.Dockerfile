FROM golang:1.25.0-alpine AS builder

COPY .. /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
WORKDIR /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
RUN go clean --modcache
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o ./.bin ./cmd/main/main.go


# ✅ Заменяем scratch на alpine
FROM alpine:3.19 AS runner

# ✅ Устанавливаем ffmpeg и ca-certificates
RUN apk add --no-cache ffmpeg ca-certificates tzdata

WORKDIR /dddance-back/

COPY --from=builder /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/.bin .

COPY .env .

# ✅ Создаём папку для временных файлов
RUN mkdir -p /dddance-back/tmp

ENV TZ="Europe/Moscow"

EXPOSE 5458

ENTRYPOINT ["./.bin"]