# remo

Single-VPS PaaS. `git push` deploys JS apps to a V8 runtime. No Kubernetes, no Docker per app, no build step.

```
remo push  →  git archive  →  extract  →  symlink swap  →  nano-rs reload
```

Apps live at `https://{owner}-{app}.{domain}`. One nano-rs process routes all apps by `Host` header.

---

## Architecture

```
laptop
  remo push  (git push ssh://yourdomain.tld/myapp)
    │
    HOST sshd :22 → forced command: remo git-hook --user alice
      ├── verify: alice owns myapp?
      ├── git-receive-pack /var/lib/remo/git/myapp.git
      └── post-receive: extract → symlink → nano-rs reload

VPS
  nginx :443      →  nano-rs :8080  (app traffic, Host-header routing)
  yourdomain.tld  →  remo API :7070 (CLI traffic)
```

---

## Get started

### 1. Install the CLI

```bash
git clone https://github.com/gleicon/remo && cd remo
make install          # cargo build --release + copy to /usr/local/bin/remo
```

### 2. Connect to a server

```bash
remo setup
# prompts: server URL · admin or user · master/user token · username · SSH key
```

If you're setting up the server itself, see [docs/SETUP_GUIDE.md](docs/SETUP_GUIDE.md).

### 3. Deploy your first app

```bash
remo apps create myapp
cd myapp
remo push
# live at https://alice-myapp.yourdomain.tld
```

---

## App templates

`remo apps create` scaffolds a starter file, git repo, and remote.

### JS (default) — ES module fetch handler

```bash
remo apps create myapi
```

```javascript
export default {
  fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === "/") {
      return new Response("hello from myapi", { status: 200 });
    }
    return new Response("Not Found", { status: 404 });
  },
};
```

### HTML — styled HTML page served from a JS worker

```bash
remo apps create mysite --html
```

Creates `index.js` with an HTML template literal. Edit the HTML in the file and `remo push`.

### WASM — WebAssembly module wrapped in a JS worker

```bash
remo apps create mycalc --wasm
```

Ships with a working inline wasm module (`add(a, b)`). Hit `/add?a=2&b=3` immediately.
To use your own: compile with `wasm-pack`, base64-encode the `.wasm`, replace `WASM_B64` in `index.js`.

The `addEventListener("fetch", ...)` Service Worker pattern is also supported in all templates.

---

## Day-to-day

```bash
remo push                      # deploy (infers app from git remote)
remo apps list                 # list your apps
remo logs                      # tail logs
remo env set KEY=value         # set env var
remo env list                  # list env vars
remo env unset KEY             # remove env var
remo apps delete myapp         # delete app
```

Commands that take an app name infer it from the git remote when run inside the app directory.

---

## Invite a user

```bash
remo users invite alice        # prints a link + one-liner command
```

Alice runs `remo setup --invite <link-or-token>` on her laptop. See [docs/SETUP_GUIDE.md §6](docs/SETUP_GUIDE.md#6-invite-another-user-alice).

---

## Contributing

```bash
make test                       # run tests
make release VERSION=v0.5.0     # bump version, tag, push → CI builds linux/amd64 binary
make deploy                     # rsync to VPS, rebuild container, update host binary
```

VPS connection via `.make.env` (gitignored): `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`, `VPS_DIR`.

API reference: [docs/DESIGN.md](docs/DESIGN.md#control-plane-api).
