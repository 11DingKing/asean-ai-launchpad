FROM --platform=$BUILDPLATFORM golang:1.24.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/launchpad ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates wget && addgroup -S launchpad && adduser -S -G launchpad launchpad
WORKDIR /app
COPY --from=build /out/launchpad /usr/local/bin/launchpad
RUN mkdir -p /data && chown launchpad:launchpad /data
USER launchpad
ENV HTTP_ADDR=:8080 DATABASE_PATH=/data/launchpad.db
EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=2s --start-period=2s --retries=15 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/launchpad"]
