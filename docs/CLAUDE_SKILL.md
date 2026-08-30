# remo Claude Code skill

`/remo-deploy` is a Claude Code skill that pushes your app, waits for the deploy to land, smoke tests the live URL, auto-rollbacks on 5xx, and then tails live stats until you stop it.

## Install

```bash
mkdir -p ~/.claude/skills/remo-deploy
cp skills/remo-deploy/SKILL.md ~/.claude/skills/remo-deploy/SKILL.md
```

Or without cloning the repo:

```bash
mkdir -p ~/.claude/skills/remo-deploy
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/skills/remo-deploy/SKILL.md \
  -o ~/.claude/skills/remo-deploy/SKILL.md
```

## Prerequisites

- [Claude Code](https://claude.ai/code) installed
- `remo` CLI installed and configured (`remo login` already run)
- Inside a directory with a `remo` git remote (`git remote add remo git@<server>:<appname>`)

## Usage

```bash
# from inside your app directory
/remo-deploy

# override app name (if outside the repo)
/remo-deploy myapp
```

## What it does

1. Runs `remo push`
2. Polls `remo deployments` until `pending → success` (or `failed`)
3. Smoke tests the live URL — auto-rollbacks on 5xx
4. Tails `remo logs --follow` until you Ctrl+C
