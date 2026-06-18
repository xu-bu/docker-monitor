# ====================================================================
#  Docker Service Monitor — multi-stage build
#  Stage 1: Go backend binary
#  Stage 2: Next.js static export
#  Stage 3: Runtime image
# ====================================================================

# ---- Stage 1: Go binary -------------------------------------------
FROM golang:1.23-alpine AS go-builder

# Clear any proxy env vars that leak from the host
ENV HTTP_PROXY="" HTTPS_PROXY="" http_proxy="" https_proxy="" \
    GOPROXY=https://proxy.golang.org,direct \
    GONOSUMCHECK='*' \
    GONOSUMDB='*'

WORKDIR /build

# Cache dependencies first
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 go build -o /monitor .

# ---- Stage 2: Next.js static export ---------------------------------
FROM node:22-alpine AS frontend-builder

# Clear proxys that leak from Docker daemon config
ENV HTTP_PROXY="" HTTPS_PROXY="" http_proxy="" https_proxy="" \
    npm_config_proxy="" npm_config_https_proxy=""

WORKDIR /build

COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci

COPY frontend/ .
RUN npm run build   # outputs to /build/out/

# ---- Stage 3: Runtime ----------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tini wget

WORKDIR /app

COPY --from=go-builder   /monitor           .
COPY --from=frontend-builder /build/out     ./public

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./monitor"]
