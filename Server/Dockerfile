FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/releasehub ./cmd/releasehub

FROM cgr.dev/chainguard/static:latest@sha256:fc42af33e643f20d08baf03cddcc1c02ba57052dfb02edb0d79f3fc47bcea8d9

WORKDIR /app
COPY --from=build --chown=65532:65532 /out/releasehub /app/releasehub

USER 65532:65532
ENTRYPOINT ["/app/releasehub"]
CMD ["api", "--config", "/app/configs/config.yaml"]
