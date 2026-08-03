.PHONY: build test lint release update-sha deploy logs status health

# ── VPS SSH config ────────────────────────────────────────────────────────────
VPS_USER    ?= ubuntu
VPS_HOST    ?= REDACTED_VPS_IP
VPS_SSH_KEY ?= ~/.ssh/id_rsa_mgc_saas_apps
VPS_DIR     ?= /home/ubuntu/remo
SSH          = ssh -i $(VPS_SSH_KEY) -o StrictHostKeyChecking=no $(VPS_USER)@$(VPS_HOST)

# ── Local ─────────────────────────────────────────────────────────────────────

build:
	cargo build --release

test:
	cargo test

lint:
	cargo clippy -- -D warnings
	cargo fmt --check

# ── Release ───────────────────────────────────────────────────────────────────
# GitHub Actions (.github/workflows/release.yml) builds the linux/amd64 binary
# and publishes remo-linux-amd64 + remo-linux-amd64.sha256 on every tag push.

# Tag and push to trigger the CI release build.
release:
	@test -n "$(VERSION)" || (echo "Usage: make release VERSION=v0.2.1"; exit 1)
	git tag $(VERSION)
	git push origin $(VERSION)
	@echo ""
	@echo "  Release $(VERSION) queued."
	@echo "  Monitor: https://github.com/gleicon/remo/actions"
	@echo ""
	@echo "  Once binary is uploaded (~2 min), run:"
	@echo "    make update-sha VERSION=$(VERSION)"

# Fetch SHA256 from the GitHub release and patch docker-compose.yml.
# Run after CI finishes building the binary.
update-sha:
	@test -n "$(VERSION)" || (echo "Usage: make update-sha VERSION=v0.2.1"; exit 1)
	$(eval SHA256 := $(shell curl -sL \
	  https://github.com/gleicon/remo/releases/download/$(VERSION)/remo-linux-amd64.sha256 \
	  | awk '{print $$1}'))
	@test -n "$(SHA256)" || (echo "SHA256 not found — is the CI release done? Check https://github.com/gleicon/remo/releases"; exit 1)
	perl -i -pe 's/REMO_VERSION: .*/REMO_VERSION: $(VERSION)/g' docker-compose.yml
	perl -i -pe 's/REMO_SHA256: .*/REMO_SHA256: $(SHA256)/g' docker-compose.yml
	@echo ""
	@echo "  docker-compose.yml updated:"
	@echo "    REMO_VERSION: $(VERSION)"
	@echo "    REMO_SHA256:  $(SHA256)"
	@echo ""
	@echo "  Commit and deploy:"
	@echo "    git add docker-compose.yml && git commit -m 'chore: bump remo to $(VERSION)'"
	@echo "    make deploy"

# ── VPS deployment ────────────────────────────────────────────────────────────
# Syncs repo to VPS, rebuilds containers from the pinned binary release,
# and restarts. nano-rs is untouched unless its Dockerfile changed.

deploy:
	rsync -avz --delete \
	  --exclude=target/ --exclude=.git/ --exclude='*.db' \
	  -e "ssh -i $(VPS_SSH_KEY) -o StrictHostKeyChecking=no" \
	  . $(VPS_USER)@$(VPS_HOST):$(VPS_DIR)/
	$(SSH) "cd $(VPS_DIR) && sudo docker compose build --pull remo remo-sshd && sudo docker compose up -d"

# ── Observability ─────────────────────────────────────────────────────────────

logs:
	$(SSH) "cd $(VPS_DIR) && sudo docker compose logs -f remo"

status:
	$(SSH) "sudo docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"

health:
	@curl -sf https://cloud.remoapps.site/health | jq .
