# Makefile for remo development and release.
# remo users manage their apps via the remo CLI, not make.

.PHONY: build test lint tag tag-push bump deploy logs status health

VPS_USER    ?= ubuntu
VPS_HOST    ?= REDACTED_VPS_IP
VPS_SSH_KEY ?= ~/.ssh/id_rsa_mgc_saas_apps
VPS_DIR     ?= /home/ubuntu/remo
SSH          = ssh -i $(VPS_SSH_KEY) -o StrictHostKeyChecking=no $(VPS_USER)@$(VPS_HOST)

build:
	cargo build --release

test:
	cargo test

lint:
	cargo clippy -- -D warnings
	cargo fmt --check

# Tag the current commit. Does not push.
tag:
	@test -n "$(VERSION)" || (echo "Usage: make tag VERSION=v1.0.0"; exit 1)
	git tag $(VERSION)

# Push the tag to origin. Triggers .github/workflows/release.yml.
# CI builds a static linux/amd64 binary and publishes it as a GitHub release.
tag-push:
	@test -n "$(VERSION)" || (echo "Usage: make tag-push VERSION=v1.0.0"; exit 1)
	git push origin $(VERSION)

# Fetch SHA256 from the published release and update docker-compose.yml.
# Run after CI finishes (check https://github.com/gleicon/remo/actions).
bump:
	@test -n "$(VERSION)" || (echo "Usage: make bump VERSION=v1.0.0"; exit 1)
	$(eval SHA256 := $(shell curl -sL \
	  https://github.com/gleicon/remo/releases/download/$(VERSION)/remo-linux-amd64.sha256 \
	  | awk '{print $$1}'))
	@test -n "$(SHA256)" || (echo "SHA256 not found — CI release not done yet?"; exit 1)
	perl -i -pe 's/REMO_VERSION: .*/REMO_VERSION: $(VERSION)/g' docker-compose.yml
	perl -i -pe 's/REMO_SHA256: .*/REMO_SHA256: $(SHA256)/g' docker-compose.yml
	@echo "Bumped to $(VERSION) (sha256=$(SHA256))"

# Sync repo to VPS, rebuild remo container, and install remo binary on the host.
# The host binary is used by the git user's forced command for git-push deploys.
deploy:
	rsync -avz --delete \
	  --exclude=target/ --exclude=.git/ --exclude='*.db' --exclude='.env' \
	  -e "ssh -i $(VPS_SSH_KEY) -o StrictHostKeyChecking=no" \
	  . $(VPS_USER)@$(VPS_HOST):$(VPS_DIR)/
	$(SSH) "cd $(VPS_DIR) && sudo docker compose build --pull remo && sudo docker compose up -d"
	$(SSH) "VER=\$$(grep 'REMO_VERSION:' $(VPS_DIR)/docker-compose.yml | head -1 | awk '{print \$$2}') && \
	  sudo wget -qO /usr/local/bin/remo https://github.com/gleicon/remo/releases/download/\$$VER/remo-linux-amd64 && \
	  sudo chmod +x /usr/local/bin/remo && echo host binary updated to \$$VER"

logs:
	$(SSH) "cd $(VPS_DIR) && sudo docker compose logs -f remo"

status:
	$(SSH) "sudo docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"

health:
	@curl -sf https://$(VPS_HOST)/health | jq .
