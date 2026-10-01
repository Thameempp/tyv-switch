# sa

Fast, privacy-first Antigravity account/profile manager.

## What it does

`sa` lets you create named profiles and quickly switch between them when
launching the Antigravity CLI (`agy`). Each profile is a named label that
tracks which agy instance you want to use.

```
sa add personal
sa add work --email work@example.com

sa personal       # launch agy as "personal"
sa work           # launch agy as "work"

sa list
sa current
sa rename work company
sa remove company
sa doctor
```

## Why it exists

The Antigravity CLI (agy) does not provide an official multi-account switching
mechanism. `sa` fills this gap by providing a fast, minimal profile registry
backed by local configuration only. All authentication remains under agy's
full control.

## Features

- Named profiles (personal, work, university, …)
- Near-instant profile switching (local operation, no network)
- Optional email hint per profile for human reference
- Atomic config writes — no partial-write corruption
- Restrictive file permissions (0600 config, 0700 directories)
- Full cross-platform support: macOS, Linux, Windows
- No telemetry, no analytics, no network requests
- No credential storage — authentication stays with agy

## Installation

### From GitHub Releases (recommended)

1. Download the binary for your platform from the Releases page.
2. Verify the checksum.
3. Move to a directory in your PATH:

```sh
# macOS (Apple Silicon)
curl -L https://github.com/thameem/sa/releases/latest/download/sa-darwin-arm64 -o sa
chmod +x sa
mv sa /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/thameem/sa/releases/latest/download/sa-darwin-amd64 -o sa
chmod +x sa
mv sa /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/thameem/sa/releases/latest/download/sa-linux-amd64 -o sa
chmod +x sa
mv sa ~/.local/bin/
```

### From source (requires Go 1.27+)

```sh
git clone https://github.com/thameem/sa
cd sa
go build -o sa ./cmd/sa/
```

## Quick Start

```sh
# Create profiles
sa add personal
sa add work --email work@example.com

# Switch and launch agy
sa personal
sa work

# Manage profiles
sa list
sa current
sa rename work company
sa remove company

# Diagnostics
sa doctor
```

## Commands

| Command | Description |
|---|---|
| `sa <profile>` | Switch to profile and launch Antigravity |
| `sa add <name>` | Create a new profile |
| `sa add <name> --email <addr>` | Create a profile with an email hint |
| `sa list` | List all profiles |
| `sa current` | Show the active profile |
| `sa remove <name>` | Remove a profile |
| `sa rename <old> <new>` | Rename a profile |
| `sa doctor` | Run diagnostics |
| `sa version` | Show version |
| `sa help` | Show usage |

## Profile Names

Valid names:

- lowercase letters, digits, hyphens, underscores
- 1–63 characters
- must start with a letter or digit

Examples: `personal`, `work`, `my-work`, `uni-2024`

Rejected: uppercase, path separators, shell metacharacters, reserved command names.

## How Profile Switching Works

### Authentication research findings

The Antigravity CLI (agy v1.2.14) was inspected to understand its
authentication architecture:

- agy uses Google OAuth (access token + refresh token) stored in
  `~/.gemini/oauth_creds.json`.
- `~/.gemini/google_accounts.json` tracks the active account email.
- agy has **no official** `--account`, `--profile`, `--login`, or `--switch`
  flags.
- The binary contains an internal `--app_data_dir` flag (relative path only)
  with no stable, documented API contract for authentication isolation.

### Safe approach used by sa

`sa` does **not** manipulate authentication files, credentials, or OAuth
tokens. Instead:

1. `sa` maintains a local profile registry (`~/Library/Application Support/sa/sa.json`
   on macOS).
2. Each profile gets a dedicated data directory within sa's app directory.
3. When you run `sa personal`, sa looks up the profile, sets `SA_ACTIVE_PROFILE`
   in the environment, then executes `agy` via `execve(2)` (Unix) or a child
   process (Windows).
4. agy starts using its own authentication — whichever Google account is logged
   in.

### Multi-Google-account workflows

If you need to use two different Google accounts:

1. Log in to the first account in agy normally.
2. Use `sa personal` for that account.
3. To use a second Google account, log out of agy and log in with the second
   account, then use `sa work`.

This is the only safe, supported mechanism given agy's current architecture.
If agy adds official multi-account support in future versions, `sa` will adopt
it.

## Security

- `sa` never asks for your Google password.
- `sa` never stores OAuth tokens, access tokens, or refresh tokens.
- `sa` never reads or copies credential files.
- `sa` never makes network requests.
- Configuration directory: 0700 permissions.
- Configuration file: 0600 permissions.
- All authentication is handled exclusively by agy.

See [SECURITY.md](SECURITY.md) and [THREAT_MODEL.md](THREAT_MODEL.md) for
a full security analysis.

## Privacy

- No telemetry.
- No analytics.
- No remote server.
- No user profiling.
- Only data stored: your profile names, optional email hints, and timestamps.

See [PRIVACY.md](PRIVACY.md).

## Supported Platforms

| Platform | Architecture | Status |
|---|---|---|
| macOS | Apple Silicon (arm64) | Built and tested |
| macOS | Intel (amd64) | Built (cross-compiled) |
| Linux | x86_64 (amd64) | Built (cross-compiled) |
| Linux | ARM64 | Built (cross-compiled) |
| Windows | x86_64 (amd64) | Built (cross-compiled) |
| Windows | ARM64 | Built (cross-compiled) |

Note: "built" means the binary compiled successfully for the target. "tested"
means executed on that OS. Cross-compiled builds have not been executed on
their target OS.

## Architecture

```
sa
├── cmd/sa/main.go              Command dispatcher
├── internal/
│   ├── config/config.go        Profile registry, atomic config writes
│   ├── doctor/doctor.go        Diagnostic command
│   ├── exitcode/exitcode.go    Consistent exit codes
│   ├── launcher/launcher.go    AGY process launching
│   ├── platform/
│   │   ├── platform.go         Interface definition
│   │   ├── darwin.go           macOS implementation
│   │   ├── linux.go            Linux implementation
│   │   └── windows.go          Windows implementation
│   └── validation/validation.go Profile name validation
└── tests/                      Integration tests
```

All platform-specific code is isolated behind the `Platform` interface.
Business logic contains no `runtime.GOOS` checks.

## Troubleshooting

**sa: agy executable not found**
Install the Antigravity CLI from https://antigravity.google/download
or set `SA_AGY_PATH=/path/to/agy`.

**sa: profile "x" does not exist**
Run `sa list` to see available profiles. Create with `sa add x`.

**sa: configuration file appears to be corrupted**
Run `sa doctor` for diagnostics. If needed:
```sh
rm ~/Library/Application\ Support/sa/sa.json   # macOS
```

**Enable debug output**
```sh
SA_DEBUG=1 sa personal
```
Debug output never prints credentials.

## Development

```sh
# Build
go build ./cmd/sa/

# Test (with race detector)
go test -race ./...

# Vet and format
go vet ./...
gofmt -l .

# Cross-platform builds
GOOS=linux GOARCH=amd64 go build ./cmd/sa/
GOOS=windows GOARCH=amd64 go build ./cmd/sa/
```

## Testing

```sh
go test -v -race ./...
```

Tests cover:
- Profile CRUD (create, read, update, delete)
- Atomic config writes
- File permission verification
- Corruption detection
- Path traversal rejection
- Shell metacharacter rejection
- Reserved name rejection
- Concurrent access (race detector)

## Building Releases

```sh
./scripts/build-release.sh   # if present, otherwise see Makefile
```

Or manually:
```sh
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/sa-darwin-arm64 ./cmd/sa/
```

## Contributing

1. All security-relevant changes require a threat model review.
2. No external dependencies without documented justification.
3. All new features require tests.
4. Run `go vet ./...` and `go test -race ./...` before submitting.

## License

MIT
