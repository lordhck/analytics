.PHONY: help build run down clean test
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'

build: ## Build the docker image
	@echo "[i] Docker image build started ..."
	@docker compose build

test: ## Run the go test suite in a throwaway container
	@echo "[i] Running tests ..."
	@docker run --rm -v "$(PWD)/server":/src -w /src golang:1.25-alpine sh -c "go mod tidy && go test ./..."

run: ## Build and start the stack (detached)
	@echo "[i] Starting stack ..."
	@docker compose up -d --build

down: ## Stop the stack
	@echo "[i] Stopping stack ..."
	@docker compose down

clean: ## Stop the stack, remove volumes/images and data
	@echo "[i] Cleaning up volumes, image, data ..."
	@docker compose down -v --remove-orphans
	@docker rmi analytics:latest 2>/dev/null || true
	@rm -rf data
