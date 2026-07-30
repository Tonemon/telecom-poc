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

.PHONY: attach-4g
attach-4g: ## Start eNB + UE and follow their logs
	$(COMPOSE_4G) up -d enb ue
	$(COMPOSE_4G) logs -f enb ue

.PHONY: provisioner-build
provisioner-build: ## Build the provisioner image
	docker build -t telecom-poc/provisioner:local tools/provisioner

.PHONY: telcoctl
telcoctl: ## Build the telcoctl host binary to ./bin/telcoctl
	docker build -t telecom-poc/provisioner:local tools/provisioner
	@mkdir -p bin
	docker create --name telcoctl-extract telecom-poc/provisioner:local >/dev/null
	docker cp telcoctl-extract:/usr/local/bin/telcoctl bin/telcoctl
	docker rm telcoctl-extract >/dev/null
	@echo "built ./bin/telcoctl"

.PHONY: test-4g
test-4g: ## Acceptance: provision via telcoctl, attach, and ping the internet via the UPF
	$(COMPOSE_4G) up -d --build
	./deploy/4g/scripts/provision.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 4 8.8.8.8

.PHONY: test-provisioner-lifecycle
test-provisioner-lifecycle: ## Prove suspend blocks attach and resume restores it
	$(COMPOSE_4G) up -d --build
	./deploy/4g/scripts/provision.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 2 8.8.8.8
	./deploy/4g/scripts/assert-attach-rejected.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 2 8.8.8.8
	@echo "lifecycle test passed: suspend blocked attach, resume restored it"

.PHONY: capture-4g
capture-4g: ## Start the 4G stack WITH packet capture (pcaps -> deploy/4g/pcap/)
	$(COMPOSE_4G) --profile capture up -d --build
	./deploy/4g/scripts/add-subscriber.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 4 8.8.8.8
	@echo "Captures being written to deploy/4g/pcap/ — run 'make capture-stop-4g' to flush & stop."

.PHONY: capture-stop-4g
capture-stop-4g: ## Stop capture sidecars and list resulting pcap files
	$(COMPOSE_4G) --profile capture stop pcap-mme pcap-smf pcap-sgwu pcap-upf
	$(COMPOSE_4G) stop enb ue
	@ls -lh deploy/4g/pcap/*.pcap 2>/dev/null || echo "no pcaps found"
