COMPOSE_4G := docker compose -f deploy/4g/docker-compose.yml
COMPOSE_MULTI := docker compose -f deploy/4g/docker-compose.yml -f deploy/4g/docker-compose.multi.yml
# Everything except the radio (enb) and the device (ue): the EPC core + provisioner.
CORE_4G := mongo provisioner nrf scp hss pcrf sgwc sgwu upf smf mme

.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

.PHONY: submodules
submodules: ## Initialise and update git submodules
	git submodule update --init --recursive

# =====================================================================================
# DEPLOY — one shot
# =====================================================================================

.PHONY: 4g-auto
4g-auto: 4g-multi ## One-shot MULTI: 2 eNBs + 3 UEs, provision, attach, ping all 3 (= 4g-multi + ping)
	@for u in ue1 ue2 ue3; do ./deploy/4g/scripts/wait-for-attach-svc.sh $$u; done
	@for u in ue1 ue2 ue3; do \
	  echo "== ping 8.8.8.8 from $$u =="; \
	  $(COMPOSE_MULTI) exec -T $$u ping -I tun_srsue -c 4 8.8.8.8; \
	done

.PHONY: 4g-multi
4g-multi: ## One-shot MULTI (no ping): 2 cells + 3 UEs through the shared ZMQ air
	@echo "==> [1/4] Building the broker image + EPC core + provisioner..."
	$(COMPOSE_MULTI) build air
	$(COMPOSE_MULTI) up -d --build $(CORE_4G)
	@echo "==> [2/4] Provisioning the 3 fixed subscribers..."
	./deploy/4g/scripts/provision-multi.sh
	@echo "==> [3/4] Starting eNBs (enb-a, enb-b) and UEs (ue1, ue2, ue3)..."
	$(COMPOSE_MULTI) up -d enb-a enb-b ue1 ue2 ue3
	@echo "==> [4/4] Waiting for every eNB/UE radio to be ready, then starting the air LAST..."
	./deploy/4g/scripts/wait-radio-ready.sh enb-a enb-b ue1 ue2 ue3
	$(COMPOSE_MULTI) up -d --no-deps air
	@echo "Up. UEs attach within about a minute — check with: make status-4g"

.PHONY: 4g-single
4g-single: ## One-shot SINGLE: 1 eNB + 1 UE + 1 subscriber, attach + ping (the pre-multi behaviour)
	@echo "==> [1/3] Building & starting the single-cell stack (core + enb + ue)..."
	$(COMPOSE_4G) up -d --build
	@echo "==> [2/3] Provisioning subscriber 999700000000001 (matches ue.conf)..."
	./deploy/4g/scripts/provision.sh
	./deploy/4g/scripts/wait-for-attach.sh
	@echo "==> [3/3] Data-plane check: ping 8.8.8.8 from the UE via tun_srsue..."
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 4 8.8.8.8
	@echo "Single-cell deployment attached and verified."

# =====================================================================================
# DEPLOY — in stages (the README "bring it up in stages" way, on the MULTI topology)
#
#   make 4g-infra              # operator network: EPC core + both eNBs (no subs, no UEs)
#   make 4g-demo-subscribers   # provision the 3 fixed subscribers (keys match the UE configs)
#   make 4g-devices            # all 3 devices (ue1, ue2, ue3) + the air; attach + ping
#   make 4g-devices-down       # devices + air off again; core, eNBs and subscribers stay
#
# All UEs always run together: the air holds every eNB and UE in one sample lockstep, so a
# missing UE would stall the whole radio.
# =====================================================================================

.PHONY: 4g-infra
4g-infra: ## Bring up the operator network (EPC core + both eNBs) only — no subscribers, no UEs
	@echo "==> [1/3] Building the broker image + EPC core + provisioner (no radio yet):"
	@echo "          $(CORE_4G)"
	$(COMPOSE_MULTI) build air
	$(COMPOSE_MULTI) up -d --build $(CORE_4G)
	@echo "==> [2/3] Starting the eNodeBs (enb-a, enb-b) — they S1-Setup to the MME..."
	$(COMPOSE_MULTI) up -d enb-a enb-b
	@echo ""
	@echo "Network is ready: full EPC core + both eNBs are up. No subscribers provisioned yet."
	@echo ""
	@echo "Next — provision the 3 fixed demo subscribers:"
	@echo "    make 4g-demo-subscribers"
	@echo "Then bring up the devices (all three, with the air):"
	@echo "    make 4g-devices"
	@echo ""
	@echo "Or drive the operator API by hand from your host: 'make telcoctl', then e.g.:"
	@echo "    TELCOCTL_SERVER=http://127.0.0.1:8080 TELCOCTL_TOKEN=dev-operator-token ./bin/telcoctl list"

.PHONY: 4g-demo-subscribers
4g-demo-subscribers: ## Provision the 3 fixed demo subscribers via telcoctl (keys match the UE configs)
	./deploy/4g/scripts/provision-multi.sh

.PHONY: 4g-devices
4g-devices: ## Bring up all 3 devices (ue1, ue2, ue3) + the air on a running infra; attach + ping all
	@echo "    (needs infra up — 'make 4g-infra' — and subs — 'make 4g-demo-subscribers')"
	@echo "==> [1/4] Restarting the radio side fresh: eNBs, UEs and the air (core stays up)..."
	@# Position in the sample stream is time: a new air starts at sample 0, so the eNBs must
	@# start again with it, or their clocks no longer line up with the UEs'.
	$(COMPOSE_MULTI) rm -sf air ue1 ue2 ue3 enb-a enb-b
	$(COMPOSE_MULTI) up -d enb-a enb-b ue1 ue2 ue3
	@echo "==> [2/4] Waiting for every eNB/UE radio to be ready, then starting the air LAST..."
	./deploy/4g/scripts/wait-radio-ready.sh enb-a enb-b ue1 ue2 ue3
	$(COMPOSE_MULTI) up -d --no-deps air
	@echo "==> [3/4] Waiting for all 3 devices to attach..."
	@for u in ue1 ue2 ue3; do ./deploy/4g/scripts/wait-for-attach-svc.sh $$u; done
	@echo "==> [4/4] Data-plane check: ping 8.8.8.8 from each device via tun_srsue..."
	@for u in ue1 ue2 ue3; do \
	  echo "== $$u =="; $(COMPOSE_MULTI) exec -T $$u ping -I tun_srsue -c 3 8.8.8.8 || exit 1; \
	done
	@echo "All 3 devices attached and data plane verified."

# =====================================================================================
# TEARDOWN
# =====================================================================================

.PHONY: 4g-auto-down
4g-auto-down: ## Tear down the whole MULTI/staged stack + volumes (core + eNBs + UEs + air)
	$(COMPOSE_MULTI) --profile "*" down -v --remove-orphans

.PHONY: 4g-single-down
4g-single-down: ## Tear down the single-cell stack + volumes
	$(COMPOSE_4G) --profile "*" down -v --remove-orphans

.PHONY: 4g-devices-down
4g-devices-down: ## Stop and remove the 3 devices + the air; core, eNBs and subscribers stay up
	@echo "==> Stopping and removing ue1, ue2, ue3 and the air. Core, eNBs and subscribers stay."
	$(COMPOSE_MULTI) rm -sf air ue1 ue2 ue3
	@echo "Devices removed. Bring them back with 'make 4g-devices'."

# =====================================================================================
# TESTS
# =====================================================================================

.PHONY: test-4g-multi
test-4g-multi: ## Acceptance: 3 UEs across 2 cells attach + ping; 2 share one eNB
	./deploy/4g/scripts/test-multi.sh

.PHONY: test-provisioner-lifecycle
test-provisioner-lifecycle: ## Prove suspend/resume and set-plan behave like a real telco: live detach, blocked reattach, signaled-not-enforced plan change
	$(COMPOSE_4G) up -d --build
	./deploy/4g/scripts/provision.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 2 8.8.8.8
	./deploy/4g/scripts/assert-plan-change-no-live-effect.sh
	./deploy/4g/scripts/assert-live-detach.sh
	./deploy/4g/scripts/assert-attach-rejected.sh
	./deploy/4g/scripts/wait-for-attach.sh
	$(COMPOSE_4G) exec -T ue ping -I tun_srsue -c 2 8.8.8.8
	@echo "lifecycle test passed: plan change signaled but not enforced live, suspend detached the live session and blocked new attach, resume restored both"

# =====================================================================================
# UTILITIES
# =====================================================================================

.PHONY: up-4g
up-4g: ## Build and start the single-cell 4G stack
	$(COMPOSE_4G) up -d --build

.PHONY: status-4g
status-4g: ## Show 4G service health
	$(COMPOSE_MULTI) ps

.PHONY: logs-4g
logs-4g: ## Tail 4G stack logs
	$(COMPOSE_4G) logs -f

.PHONY: attach-4g
attach-4g: ## Start single eNB + UE and follow their logs
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

.PHONY: capture-4g
capture-4g: ## Start the single-cell stack WITH packet capture (pcaps -> deploy/4g/pcap/)
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
