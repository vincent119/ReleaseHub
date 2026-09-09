.DEFAULT_GOAL := help

export GOCACHE := $(CURDIR)/.cache/go-build
export GOSUMDB := sum.golang.org

DEV_API_ADDRESS ?= :7580
DEV_API_PROXY_TARGET ?= http://127.0.0.1:7580
DEV_WEB_PORT ?= 5173
DEV_WEB_URL ?= http://localhost:$(DEV_WEB_PORT)/
SERVER_CONFIG ?= ./configs/config.yaml
PNPM ?= $(shell if command -v pnpm >/dev/null 2>&1; then echo pnpm; elif command -v corepack >/dev/null 2>&1; then echo "corepack pnpm"; fi)

.PHONY: help check-pnpm check-dev-tools dev dev-api dev-web generate generate-server generate-web generate-check lint-openapi lint-go-structure fmt test lint build verify-docs verify-deployments verify-deployment-security verify

help: ## 顯示可用命令
	@awk 'BEGIN {FS = ":.*##"; printf "ReleaseHub commands:\n"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check-pnpm:
	@if [ -z "$(strip $(PNPM))" ]; then \
		echo "找不到 pnpm 或 corepack；請先安裝 pnpm 11.19.0。" >&2; \
		exit 1; \
	fi; \
	$(PNPM) --version >/dev/null || { \
		echo "pnpm 無法執行；請執行 corepack enable 或安裝 pnpm 11.19.0。" >&2; \
		exit 1; \
	}

check-dev-tools: check-pnpm
	@command -v openssl >/dev/null 2>&1 || { echo "找不到 openssl，無法產生本機 Session key。" >&2; exit 1; }

dev: check-dev-tools ## 同時啟動本機 API 與 Web
	$(MAKE) --no-print-directory -j2 dev-api dev-web

dev-api: ## 啟動本機 API 並套用安全的開發環境覆寫
	cd Server && \
		RELEASEHUB_SESSION_ENCRYPTION_KEY="$${RELEASEHUB_SESSION_ENCRYPTION_KEY:-$$(openssl rand -hex 32)}" \
		RELEASEHUB_SESSION_COOKIE_SECURE=false \
		RELEASEHUB_OIDC_REDIRECT_URL= \
		RELEASEHUB_OIDC_WEB_REDIRECT_URL="$(DEV_WEB_URL)" \
		go run ./cmd/releasehub api --config "$(SERVER_CONFIG)" --api-address "$(DEV_API_ADDRESS)"

dev-web: check-pnpm ## 啟動 Vite 並將 /api 代理至本機 API
	cd Web && VITE_API_PROXY_TARGET="$(DEV_API_PROXY_TARGET)" $(PNPM) dev --port "$(DEV_WEB_PORT)" --strictPort

generate: generate-server generate-web ## 重新產生所有 OpenAPI 程式碼

generate-server: ## 產生 Go OpenAPI boundary
	cd Server && go generate ./...

generate-web: check-pnpm ## 產生 TypeScript API client
	cd Web && $(PNPM) generate:api

generate-check: ## 確認重新產生前後內容一致
	sh tools/check-generated.sh

lint-openapi: check-pnpm ## 驗證 OpenAPI 文件
	cd Web && $(PNPM) lint:api

lint-go-structure: ## 驗證 HTTP、composition root 與 Deployment context 的 Go 結構門檻
	cd Server && go run ./tools/codequality -root ./internal/transport/httpserver
	cd Server && go run ./tools/codequality -root ./internal/bootstrap
	cd Server && go run ./tools/codequality -root ./internal/deployment/domain
	cd Server && go run ./tools/codequality -root ./internal/deployment/application
	cd Server && go run ./tools/codequality -root ./internal/deployment/infrastructure

fmt: check-pnpm ## 格式化 Go 與 Web 程式碼
	cd Server && gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	cd Web && $(PNPM) format

test: check-pnpm ## 執行 Server 與 Web 測試
	cd Server && go test ./...
	cd Web && $(PNPM) test

lint: lint-openapi lint-go-structure ## 執行靜態檢查
	cd Server && go vet ./...
	cd Web && $(PNPM) lint
	cd Web && $(PNPM) format:check

build: check-pnpm ## 建置 Server commands 與 Web
	cd Server && go build -o bin/releasehub ./cmd/releasehub
	cd Web && $(PNPM) build

verify-docs: ## 確認雙語文件具有相同相對路徑
	sh tools/check-doc-pairs.sh

verify-deployments: ## 驗證 Helm 與 Kustomize 可 render
	helm lint Deployments/helm/releasehub
	helm template releasehub Deployments/helm/releasehub --namespace releasehub >/dev/null
	kustomize build Deployments/kustomize/base >/dev/null
	sh tools/check-deployment-equivalence.sh

verify-deployment-security: ## 驗證 Kubernetes schema 與部署安全設定
	sh tools/check-deployment-security.sh

verify: generate-check lint test build verify-docs verify-deployments verify-deployment-security ## 執行 repository 整合驗證
