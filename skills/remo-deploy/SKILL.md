---
name: remo-deploy
description: "Push, verify, and watch a remo app. Runs remo push, waits for deploy completion, smoke tests the live URL, auto-rollbacks on 5xx, then tails live stats until interrupted."
---

Push the current app to remo, verify it deployed correctly, and watch it live.

## Prerequisites

- `remo` CLI installed and `remo login` already run (`~/.remo/config.toml` must exist)
- Current directory must have a git remote named `remo` (`git remote get-url remo`)
- Run from the app's root directory

## Steps — execute in order, stop and report on any failure

### 1. Detect app name

```bash
git remote get-url remo
```

Parse the app name: last segment after `:` (for `git@host:appname`) or after the last `/` (for `ssh://host/appname`). If the remote doesn't exist, stop with: "no `remo` git remote found — run `git remote add remo git@<server>:<appname>` first."

### 2. Push

```bash
remo push
```

Capture stdout. On failure, stop and show the full output. On success, extract the URL from the `URL:` line in the output — you'll need it for the smoke test.

### 3. Wait for deploy to complete

Poll every 2 seconds (max 60s):

```bash
remo deployments <appname>
```

The first line is the most recent deploy. Wait until its status field is `success` or `failed`.

- If `failed`: show the deployment line and stop. Do not proceed to smoke test.
- If still `pending` after 60s: warn and stop.

### 4. Smoke test

```bash
curl -s -o /dev/null -w "%{http_code}" https://<hostname>/
```

Use the hostname from the `URL:` line captured in step 2.

- **2xx or 3xx**: print `✓ <hostname> responded <status>` and continue to step 5.
- **5xx**: immediately run `remo rollback <appname>`, print `✗ smoke test failed (<status>) — rolled back to previous deploy`, and stop.
- **Connection refused / timeout**: print the error. Do not auto-rollback (server may still be warming up). Stop.

### 5. Watch live stats

```bash
remo logs <appname> --follow
```

Run this and let it stream until the user interrupts (Ctrl+C). Before starting, print a summary line:

```
✓ <appname> deployed @ <sha>  →  https://<hostname>/
  watching live stats — Ctrl+C to stop
```

## Arguments

Optional positional argument overrides auto-detected app name:

```
/remo-deploy [appname]
```

Useful when invoked from outside the app's directory.
