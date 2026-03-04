# taskmaster

A minimal, production-ready process supervisor written in Go.
It runs a command in a continuous loop and forwards OS signals to the child process.

## Overview

`taskmaster` is designed for long-running workers that are expected to restart automatically after a clean exit (code 0), while stopping immediately on failure (non-zero exit code) or on operator request (SIGTERM / SIGINT).

```
taskmaster php worker.php --queue=emails
taskmaster python consumer.py
taskmaster ./my-worker --concurrency=4
```

## How it works

```
start
  │
  ▼
┌─────────────────────────────┐
│  run child process           │
│                              │
│  exit 0  ──► wait RestartDelay ──► restart
│  exit N  ──► stop (error)
│  SIGTERM ──► forward to child ──► wait up to 5s ──► stop (nil)
│  SIGINT  ──► forward to child ──► wait up to 5s ──► stop (nil)
│  MaxRestarts reached ──────────────────────────────► stop (error)
└─────────────────────────────┘
```

- **Exit 0** — the process completed normally; taskmaster waits `RestartDelay` and starts it again.
- **Non-zero exit** — treated as a failure; taskmaster stops and exits with a non-zero code.
- **SIGTERM / SIGINT** — forwarded to the child process; taskmaster waits up to 5 seconds for a graceful shutdown, then exits with code 0.
- **MaxRestarts** — optional cap on the number of restarts; 0 means unlimited.

## Installation

### Download a pre-built binary

Grab the latest release from the [Releases](../../releases) page.

```bash
# Linux amd64
curl -Lo taskmaster https://github.com/SomeBlackMagic/taskmaster/releases/latest/download/taskmaster-linux-amd64
chmod +x taskmaster
sudo mv taskmaster /usr/local/bin/
```

Available targets: `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.exe`

### Build from source

Requires Go 1.24+.

```bash
git clone https://github.com/SomeBlackMagic/taskmaster.git
cd taskmaster
go build -o taskmaster ./cmd/taskmaster
```

Or with the Makefile:

```bash
make build        # produces dist/taskmaster
```

## Usage

```
taskmaster <command> [args...]
```

### Examples

```bash
# Run a PHP worker, restart on clean exit
taskmaster php artisan queue:work

# Run a Python consumer with arguments
taskmaster python consumer.py --topic=orders --group=service-a

# Run a shell script
taskmaster ./scripts/worker.sh

# Windows
taskmaster.exe node worker.js
```

### Exit codes

| Code | Meaning |
|------|---------|
| `0`  | Stopped by SIGTERM or SIGINT (graceful shutdown) |
| `1`  | Child process exited with a non-zero code, or failed to start |

## Configuration

Configuration is currently passed via command-line arguments and environment is inherited from the parent process. The restart behaviour is controlled by compile-time defaults:

| Parameter      | Default | Description |
|----------------|---------|-------------|
| `RestartDelay` | `1s`    | Pause between restarts after a clean exit (code 0) |
| `MaxRestarts`  | `0`     | Maximum number of restarts; `0` means unlimited |

## Logging

taskmaster writes structured JSON logs to stdout:

```json
{"time":"2024-01-15T10:00:00Z","level":"INFO","msg":"starting process","command":["php","worker.php"],"restarts":0}
{"time":"2024-01-15T10:00:05Z","level":"INFO","msg":"process exited with code 0, restarting","delay":"1s","restarts":1}
{"time":"2024-01-15T10:00:10Z","level":"ERROR","msg":"runner stopped with error","err":"process exited with code 1: ..."}
```

The child process inherits stdin, stdout and stderr from taskmaster, so its own output appears directly in the terminal or your log collector.

## Signal handling

| Signal    | Behaviour |
|-----------|-----------|
| `SIGTERM` | Forwarded to the child; taskmaster exits with code 0 after the child stops |
| `SIGINT`  | Same as SIGTERM |
| Others    | Can be forwarded programmatically via the `runner.Signal()` API |

The child process has up to **5 seconds** to shut down after receiving the signal. After that, it is forcibly killed.

## Development

```bash
make test       # go test -race ./...
make build      # compile taskmaster + testapp into dist/
make lint       # go vet ./...
```

### Project layout

```
cmd/taskmaster/        # entry point: arg parsing, signal setup, DI
internal/runner/       # core logic: Runner, Config, ProcessStarter interface
  runner.go            # restart loop, signal forwarding, panic recovery
  config.go            # Config struct with defaults and validation
  runner_test.go       # unit tests (100% coverage, race detector)
test/testapp/          # helper binary used by integration tests
```

### Running tests

```bash
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Current coverage: **100%** on `internal/runner`.

## CI / Releases

Every push runs vet, tests (with race detector), and a coverage report via GitHub Actions.

On a `v*` tag a release job builds binaries for all supported platforms and publishes them as a GitHub Release automatically.

```bash
git tag v1.2.3
git push origin v1.2.3
```

## License

MIT
