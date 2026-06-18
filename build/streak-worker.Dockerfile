FROM golang:1.25.0-alpine AS builder

COPY .. /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
WORKDIR /github.com/Elizaveta-Makeeva/2026_1_DavaiDavaiDeploy_Backend/
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
RUN go clean --modcache
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o /streak-worker ./cmd/streak-worker/

FROM alpine:3.19 AS runner
RUN apk add --no-cache ca-certificates tzdata

ENV TZ="Europe/Moscow"

COPY --from=builder /streak-worker /streak-worker

ENTRYPOINT ["/streak-worker"]
