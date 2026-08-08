# VPS connection — override via environment or .make.env (gitignored).
# Example: VPS_HOST=1.2.3.4 VPS_SSH_KEY=~/.ssh/id_rsa make deploy
-include .make.env
VPS_USER    ?= ubuntu
VPS_HOST    ?=
VPS_SSH_KEY ?= ~/.ssh/id_rsa
VPS_DIR     ?= /home/ubuntu/remo
SSH          = ssh -i $(VPS_SSH_KEY) -o StrictHostKeyChecking=no $(VPS_USER)@$(VPS_HOST)

.PHONY: build test lint check install release deploy logs status health

build:
	cargo build --release

test:
	cargo test

lint:
	cargo clippy -- -D warnings
	cargo fmt --check

check: lint test

# Install binary locally (macOS or any host with cargo).
install: build
	cp target/release/remo /usr/local/bin/remo
	@remo --version

# Bump version in Cargo.toml, commit, tag, push — triggers CI (linux/amd64 release binary).
# Usage: make release VERSION=v0.4.0
release:
	@test -n "$(VERSION)" || (echo "Usage: make release VERSION=v0.4.0"; exit 1)
	@BARE=$$(echo "$(VERSION)" | sed 's/^v//'); \
	  perl -i -pe 's/^version = .*/version = "'"$$BARE"'"/' Cargo.toml
	git add Cargo.toml
	git commit -m "chore: bump version to $(VERSION)"
	git tag $(VERSION)
	git push origin main
	git push origin $(VERSION)
	@echo "$(VERSION) pushed — CI building linux/amd64 at https://github.com/gleicon/remo/actions"

# Sync source to VPS, rebuild container from source, copy host binary out of container.
# The host binary is needed for the git-hook forced command (runs outside Docker).
deploy:
	rsync -avz --delete \
	  --exclude=target/ --exclude=.git/ --exclude='*.db' --exclude='.env' \
	  -e "ssh -i $(VPS_SSH_KEY) -o StrictHostKeyChecking=no" \
	  . $(VPS_USER)@$(VPS_HOST):$(VPS_DIR)/
	$(SSH) "cd $(VPS_DIR) && sudo docker compose build --no-cache nano-rs remo && sudo docker compose up -d"
	$(SSH) "cd $(VPS_DIR) && sudo docker compose cp remo:/usr/local/bin/remo /usr/local/bin/remo && remo --version"

logs:
	$(SSH) "cd $(VPS_DIR) && sudo docker compose logs -f remo"

status:
	$(SSH) "sudo docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"

health:
	$(SSH) "curl -sf http://127.0.0.1:7070/health | jq . || echo no /health endpoint"
