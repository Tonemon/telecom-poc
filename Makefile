COMPOSE_4G := docker compose -f deploy/4g/docker-compose.yml
COMPOSE_MULTI := docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml
# Everything except the radio (enb) and the device (ue): the EPC core + provisioner.
CORE_4G := mongo provisioner nrf scp hss pcrf sgwc sgwu upf smf mme

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

.PHONY: 4g-auto-down
4g-auto-down: ## Stop and remove the whole 4G stack + volumes
	$(COMPOSE_4G) --profile "*" down -v --remove-orphans

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

.PHONY: 4g-auto
4g-auto: ## Full automated deployment: bring up everything, provision, attach, ping the internet
	$(COMPOSE_4G) up -d --build
	./deploy/4g/scripts/provision.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 4 8.8.8.8

# --- Split deployment (mirrors the manual validation in docs/4G.md §5.3) ----------
# 4g-infra brings up the operator's network (core + eNB) and provisions a subscriber,
# but starts NO UE. 4g-device then brings up the UE and attaches it. These are meant
# to sit ALONGSIDE the by-hand steps in §5.3, not replace them.

.PHONY: 4g-infra
4g-infra: ## Bring up the telco network (EPC core + eNB) only — no subscribers, no UE
	@echo "==> [1/2] Building & starting the EPC core + provisioner (no radio yet):"
	@echo "          $(CORE_4G)"
	$(COMPOSE_4G) up -d --build $(CORE_4G)
	@echo "==> [2/2] Starting the eNodeB (enb) — it S1-Setups to the MME..."
	$(COMPOSE_4G) up -d enb
	@echo ""
	@echo "Network is ready: full EPC core + eNB are up. No subscribers provisioned yet."
	@echo ""
	@echo "Use the telcoctl client to create & manage subscriptions. Quick demo (adds 3):"
	@echo "    make 4g-demo-subscribers"
	@echo ""
	@echo "Or add the soft-UE's own SIM by hand, so 'make 4g-device' can attach:"
	@echo "    $(COMPOSE_4G) exec -e TELCOCTL_TOKEN=dev-operator-token provisioner \\"
	@echo "      telcoctl add --imsi 999700000000001 --ki 465B5CE8B199B49FAA5F0A2EE238A6BC \\"
	@echo "      --opc E8ED289DEBA952E4283B54E88E6183CA --apn internet --reason NEW_ACTIVATION"
	@echo ""
	@echo "Or from your host: 'make telcoctl', then point it at the published API, e.g.:"
	@echo "    TELCOCTL_SERVER=http://127.0.0.1:8080 TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl list"

.PHONY: 4g-demo-subscribers
4g-demo-subscribers: ## Register 3 demo subscribers via telcoctl (run after 'make 4g-infra')
	./deploy/4g/scripts/demo-subscribers.sh

.PHONY: 4g-infra-down
4g-infra-down: ## Tear down the whole stack + volumes (infra is the foundation; takes the UE too)
	@echo "==> Tearing down the ENTIRE 4G stack (core + eNB + UE + capture sidecars) and volumes..."
	$(COMPOSE_4G) --profile "*" down -v --remove-orphans
	@echo "Stack down."

.PHONY: 4g-device
4g-device: ## Bring up the UE against a running infra, wait for attach, ping the internet
	@echo "    (needs subscriber 999700000000001 provisioned — 'make 4g-demo-subscribers' if not)"
	@echo "==> [1/2] Starting the UE (ue) — camps on the cell and runs NAS attach..."
	$(COMPOSE_4G) up -d ue
	./deploy/4g/scripts/wait-for-attach.sh
	@echo "==> [2/2] Data-plane check: ping 8.8.8.8 from the UE via tun_srsue..."
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 4 8.8.8.8
	@echo "Device attached and data plane verified."

.PHONY: 4g-device-down
4g-device-down: ## Stop and remove ONLY the UE container; leave the infra running
	@echo "==> Stopping and removing the UE container (ue). Infra (core + eNB) stays up."
	$(COMPOSE_4G) rm -sf ue
	@echo "UE removed. Bring it back with 'make 4g-device'."

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

# --- Multi-UE / multi-cell (ZMQ broker) — see docs/superpowers plan 2026-07-31 ------
# A cell = eNB + GNU Radio broker + its UEs. Brokers start LAST. Demo topology:
# 2 cells, 3 UEs (ue1+ue2 on enb-a, ue3 on enb-b).

.PHONY: 4g-multi
4g-multi: ## Bring up 2 cells + 3 UEs through the ZMQ broker (multi-UE, multi-eNB)
	@echo "==> [1/4] EPC core + provisioner..."
	$(COMPOSE_MULTI) up -d --build mongo provisioner nrf scp hss pcrf sgwc sgwu upf smf mme
	@echo "==> [2/4] Provisioning the 3 fixed subscribers..."
	./deploy/4g/scripts/provision-multi.sh
	@echo "==> [3/4] Starting eNBs (enb-a, enb-b) and UEs (ue1, ue2, ue3)..."
	$(COMPOSE_MULTI) up -d enb-a enb-b ue1 ue2 ue3
	@echo "==> [4/4] Starting brokers LAST (broker-a, broker-b)..."
	sleep 5
	$(COMPOSE_MULTI) up -d --no-deps broker-a broker-b
	@echo "Up. UEs attach in a few seconds — check with: make status-4g"

.PHONY: 4g-multi-down
4g-multi-down: ## Tear down the multi-UE topology + volumes
	$(COMPOSE_MULTI) --profile "*" down -v --remove-orphans

.PHONY: test-4g-multi
test-4g-multi: ## Acceptance: 3 UEs across 2 cells attach + ping; 2 share one eNB
	./deploy/4g/scripts/test-multi.sh

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
