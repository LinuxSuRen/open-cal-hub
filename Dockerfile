# syntax=docker/dockerfile:1

# ---------- 阶段一：前端构建（node:22 + corepack pnpm） ----------
FROM node:22-alpine AS frontend
WORKDIR /src
# 仓库未固定 packageManager 字段，此处激活 pnpm 与维护者本地一致的大版本
RUN corepack enable && corepack prepare pnpm@12 --activate

# 先拷贝清单以命中依赖缓存
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml /src/frontend/
RUN pnpm --dir frontend install --frozen-lockfile

# 只拷贝构建所需文件（保持构建上下文最小化，宿主机 node_modules 不会进入镜像）
COPY frontend/tsconfig.json frontend/tsconfig.app.json frontend/tsconfig.node.json \
     frontend/vite.config.ts frontend/index.html /src/frontend/
COPY frontend/src /src/frontend/src
RUN pnpm --dir frontend run build
# 产物：/src/frontend/dist

# ---------- 阶段二：后端构建（golang:1.27，CGO_ENABLED=0） ----------
FROM golang:1.27-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ENV CGO_ENABLED=0
WORKDIR /src

COPY backend/go.mod backend/go.sum /src/
RUN go mod download

COPY backend /src/
# 前端产物覆盖到 embed 目录，再编译单一二进制
COPY --from=frontend /src/frontend/dist /src/web/dist

RUN GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X github.com/linuxsuren/open-cal-hub/backend/internal/version.Version=${VERSION}" \
    -o /out/calhub ./cmd/calhub

# ---------- 阶段三：运行镜像（alpine，非 root） ----------
FROM alpine:3.24
ARG VERSION=dev
LABEL org.opencontainers.image.title="Open Cal Hub" \
      org.opencontainers.image.description="自托管的开源日历与预约平台（单二进制 + SQLite）" \
      org.opencontainers.image.source="https://github.com/linuxsuren/open-cal-hub" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"

RUN apk add --no-cache ca-certificates curl tzdata \
    && mkdir -p /data \
    && chown 10001:10001 /data

# 运行用户：非 root（uid/gid 10001，无需宿主机 passwd 条目）
USER 10001:10001

ENV CALHUB_SERVER_ADDRESS=":8080" \
    CALHUB_SERVER_DATA_DIR="/data"

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

COPY --from=backend /out/calhub /usr/local/bin/calhub

ENTRYPOINT ["/usr/local/bin/calhub"]
