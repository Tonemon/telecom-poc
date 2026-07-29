COMPOSE_4G := docker compose -f deploy/4g/docker-compose.yml

.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: submodules
submodules: ## Initialise and update git submodules
	git submodule update --init --recursive

.PHONY: up-4g
up-4g: ## Build and start the 4G stack
	$(COMPOSE_4G) up -d --build

.PHONY: down-4g
down-4g: ## Stop and remove the 4G stack
	$(COMPOSE_4G) down -v

.PHONY: status-4g
status-4g: ## Show 4G service health
	$(COMPOSE_4G) ps

.PHONY: logs-4g
logs-4g: ## Tail 4G stack logs
	$(COMPOSE_4G) logs -f
