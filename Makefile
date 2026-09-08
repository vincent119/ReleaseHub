.DEFAULT_GOAL := help

export GOCACHE := $(CURDIR)/.cache/go-build
export GOSUMDB := sum.golang.org

.PHONY: help generate generate-server generate-web generate-check lint-openapi lint-go-structure fmt test lint build verify-docs verify-deployments verify-deployment-security verify

help: ## 顯示可用命令
	@awk 'BEGIN {FS = ":.*##"; printf "ReleaseHub commands:\n"} /^[a-zA-Z_-]+:.*?##/ {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

generate: generate-server generate-web ## 重新產生所有 OpenAPI 程式碼

generate-server: ## 產生 Go OpenAPI boundary
	cd Server && go generate ./...

generate-web: ## 產生 TypeScript API client
	cd Web && pnpm generate:api

generate-check: ## 確認重新產生前後內容一致
	sh tools/check-generated.sh

lint-openapi: ## 驗證 OpenAPI 文件
	cd Web && pnpm lint:api

lint-go-structure: ## 驗證 HTTP、composition root 與 Deployment context 的 Go 結構門檻
	cd Server && go run ./tools/codequality -root ./internal/transport/httpserver
	cd Server && go run ./tools/codequality -root ./internal/bootstrap
	cd Server && go run ./tools/codequality -root ./internal/deployment/domain
	cd Server && go run ./tools/codequality -root ./internal/deployment/application
	cd Server && go run ./tools/codequality -root ./internal/deployment/infrastructure

fmt: ## 格式化 Go 與 Web 程式碼
	cd Server && gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	cd Web && pnpm format

test: ## 執行 Server 與 Web 測試
	cd Server && go test ./...
	cd Web && pnpm test

lint: lint-openapi lint-go-structure ## 執行靜態檢查
	cd Server && go vet ./...
	cd Web && pnpm lint
	cd Web && pnpm format:check

build: ## 建置 Server commands 與 Web
	cd Server && go build -o bin/releasehub ./cmd/releasehub
	cd Web && pnpm build

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
