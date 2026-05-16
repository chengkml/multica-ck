# --- Build stage ---
FROM golang:1.26-alpine AS builder

ARG ALPINE_REPO=https://dl-cdn.alpinelinux.org/alpine
ARG ALPINE_REPO_FALLBACKS="https://mirrors.aliyun.com/alpine https://mirrors.tuna.tsinghua.edu.cn/alpine http://mirrors.aliyun.com/alpine http://mirrors.tuna.tsinghua.edu.cn/alpine"
RUN set -eux; \
    alpine_ver="$(cut -d. -f1,2 /etc/alpine-release)"; \
    for repo in "${ALPINE_REPO}" ${ALPINE_REPO_FALLBACKS}; do \
      printf '%s\n%s\n' \
        "${repo}/v${alpine_ver}/main" \
        "${repo}/v${alpine_ver}/community" > /etc/apk/repositories; \
      if apk add --no-cache git; then \
        echo "Installed git via ${repo}"; \
        exit 0; \
      fi; \
      echo "apk add git failed via ${repo}, trying next mirror..."; \
    done; \
    echo "apk add git failed for all configured Alpine mirrors"; \
    exit 1

WORKDIR /src

# Allow module proxy overrides for constrained networks. Default stays upstream.
ARG GO_MODULE_PROXY=https://proxy.golang.org,direct
ARG GO_MODULE_PROXY_FALLBACK=https://goproxy.cn,direct
ARG GO_MODULE_SUMDB=sum.golang.org
ENV GOPROXY=${GO_MODULE_PROXY}
ENV GOSUMDB=${GO_MODULE_SUMDB}

# Cache dependencies
COPY server/go.mod server/go.sum ./server/
RUN cd server && \
    go mod download || \
    (echo "Primary GOPROXY failed, retrying with fallback GOPROXY=${GO_MODULE_PROXY_FALLBACK}" && \
      GOPROXY="${GO_MODULE_PROXY_FALLBACK}" go mod download)

# Copy server source
COPY server/ ./server/

# Build binaries
ARG VERSION=dev
ARG COMMIT=unknown
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o bin/server ./cmd/server
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" -o bin/multica ./cmd/multica
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/migrate ./cmd/migrate

# --- Runtime stage ---
FROM alpine:3.21

ARG ALPINE_REPO=https://dl-cdn.alpinelinux.org/alpine
ARG ALPINE_REPO_FALLBACKS="https://mirrors.aliyun.com/alpine https://mirrors.tuna.tsinghua.edu.cn/alpine http://mirrors.aliyun.com/alpine http://mirrors.tuna.tsinghua.edu.cn/alpine"
RUN set -eux; \
    alpine_ver="$(cut -d. -f1,2 /etc/alpine-release)"; \
    for repo in "${ALPINE_REPO}" ${ALPINE_REPO_FALLBACKS}; do \
      printf '%s\n%s\n' \
        "${repo}/v${alpine_ver}/main" \
        "${repo}/v${alpine_ver}/community" > /etc/apk/repositories; \
      if apk add --no-cache ca-certificates tzdata; then \
        echo "Installed runtime packages via ${repo}"; \
        exit 0; \
      fi; \
      echo "apk add runtime packages failed via ${repo}, trying next mirror..."; \
    done; \
    echo "apk add runtime packages failed for all configured Alpine mirrors"; \
    exit 1

WORKDIR /app

COPY --from=builder /src/server/bin/server .
COPY --from=builder /src/server/bin/multica .
COPY --from=builder /src/server/bin/migrate .
COPY server/migrations/ ./migrations/
COPY docker/entrypoint.sh .
RUN sed -i 's/\r$//' entrypoint.sh && chmod +x entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["./entrypoint.sh"]
