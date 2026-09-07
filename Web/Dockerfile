FROM node:22.22-alpine AS build

WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM ghcr.io/nginx/nginx-unprivileged:1.31.3-alpine3.24

USER root
RUN apk upgrade --no-cache

COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY --from=build --chown=101:101 /src/dist /usr/share/nginx/html

USER 101:101
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/healthz"]
