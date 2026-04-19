# Install

Daimon ships as a single binary with zero runtime dependencies. Pick the
install path that fits your environment.

## Option A — One-liner (recommended)

Detects your OS and architecture, downloads the latest release, and installs
to `/usr/local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/mmmarxdr/daimon/main/install.sh | sh
```

## Option B — Download a release manually

Grab the binary for your platform from [Releases](https://github.com/mmmarxdr/daimon/releases).
Release archives include the web frontend.

```bash
# Linux (amd64)
tar -xzf daimon_*_linux_amd64.tar.gz
chmod +x daimon
sudo mv daimon /usr/local/bin/
```

## Option C — Build from source

```bash
git clone https://github.com/mmmarxdr/daimon.git
cd daimon

# TUI-only (no Node.js needed)
make build

# With web frontend (downloads pre-built assets, no Node.js needed)
make build-full

# Binary lands at bin/daimon
```

## Option D — `go install`

```bash
go install github.com/mmmarxdr/daimon/cmd/daimon@latest
```

> Note: `go install` builds without the web frontend. The TUI and API
> still work. To add the frontend, see
> [docs/WEB_DASHBOARD.md](WEB_DASHBOARD.md).

## First run

```bash
daimon web
```

On first run with no config, the browser-based setup wizard launches
automatically. It walks you through provider, API key and model, validates
the key with a real API call, and writes `~/.daimon/config.yaml`.

Alternative TUI wizard:

```bash
daimon --setup
```

Manual config: see [docs/CONFIG.md](CONFIG.md).
