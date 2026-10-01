# tyv

Fast, privacy-first Antigravity account/profile manager.

## What it does

`tyv` lets you create named profiles and quickly switch between them when
launching the Antigravity CLI (`agy`). Each profile has its own isolated agy
login, so you can keep several Google accounts signed in side by side. Running
`tyv` with no arguments shows each account's remaining Gemini and Claude quota
and the Google account (full email) it is signed into.

```
tyv add personal
tyv add work --email work@example.com

tyv personal       # launch agy as "personal"
tyv work           # launch agy as "work"

tyv list
tyv current
tyv edit work company   # rename a profile
tyv remove company
tyv doctor
```

## Why it exists

The Antigravity CLI (agy) does not provide an official multi-account switching
mechanism. `tyv` fills this gap by giving each profile its own isolated agy
home directory (and therefore its own Google login) plus a fast, minimal
profile registry. All authentication remains under agy's full control; tyv
never reads or stores credentials.

## Features

- Named profiles (personal, work, university, …), each with its own agy login
- Interactive selector (`tyv`) showing live remaining Gemini / Claude quota and each account's email
- Near-instant profile switching (local operation, no network)
- Guided first-time sign-in with `tyv add`
- Optional email hint per profile for human reference
- Atomic config writes — no partial-write corruption
- Restrictive file permissions (0600 config, 0700 directories)
- Full cross-platform support: macOS, Linux, Windows
- No telemetry, no analytics; tyv itself makes no network requests
- No credential handling — authentication stays with agy
- Zero external dependencies (Go standard library only)

## Installation

Install to `~/.local/bin` (no sudo needed). This directory is already in
your PATH on most systems.

### Using make (recommended)

```sh
git clone https://github.com/Thameempp/tyv-switch
cd tyv-switch
make install
```

### Using the install script

```sh
git clone https://github.com/Thameempp/tyv-switch
cd tyv-switch
bash install.sh
```

### Manually (requires Go 1.27+)

```sh
git clone https://github.com/Thameempp/tyv-switch
cd tyv-switch
go build -ldflags="-s -w" -o tyv ./cmd/tyv/
mkdir -p ~/.local/bin
cp tyv ~/.local/bin/tyv
```

### Verify your PATH

`~/.local/bin` should be early in your PATH. Check with:

```sh
which tyv    # should show ~/.local/bin/tyv
tyv version
```

If `~/.local/bin` is not in your PATH, add it:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## Quick Start

```sh
# Create profiles
tyv add personal
tyv add work --email work@example.com

# Switch and launch agy
tyv personal
tyv work

# Manage profiles
tyv list
tyv current
tyv edit work company
tyv remove company

# Diagnostics
tyv doctor
```

## Commands

| Command | Description |
|---|---|
| `tyv` | Interactive account selector with usage (↑/↓ move, Enter switch, **r** refresh usage, Esc/q exit, Ctrl+C abort). Prints help when not run in a terminal or when no profiles exist |
| `tyv <profile>` | Switch to profile and launch Antigravity |
| `tyv add <name>` | Create a profile, then (if you agree) launch agy once so you can sign in with Google. tyv never sees credentials |
| `tyv add <name> --email <addr>` | Create a profile with an email hint |
| `tyv list` | List all profiles |
| `tyv current` | Show the active profile |
| `tyv remove <name>` | Remove a profile **and its agy login** (the profile's private home) |
| `tyv edit <old> <new>` | Rename a profile (`rename` and `mv` also work). Keeps the profile's login |
| `tyv doctor` | Run diagnostics |
| `tyv version` | Show version |
| `tyv help` | Show usage |

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

- agy uses Google OAuth (access token + refresh token). It stores them in the
  OS keychain (macOS keychain, desktop keyring on Linux) and falls back to
  `~/.gemini/oauth_creds.json` when the keychain is unavailable.
- `~/.gemini/google_accounts.json` tracks the active account email in file
  mode; with keychain storage agy only logs who signed in
  (`~/.gemini/antigravity-cli/log/`).
- agy has **no official** `--account`, `--profile`, `--login`, or `--switch`
  flags.
- The binary contains an internal `--app_data_dir` flag (relative path only)
  with no stable, documented API contract for authentication isolation.
- `GEMINI_CLI_APP_DATA_DIR` is **ignored** by agy, but agy resolves its data
  from `$HOME`, so a per-profile `HOME` gives a fully separate login.
- `agy --print /usage --output-format json` reports model quota
  (`remaining_fraction`, `reset_time`) for the signed-in account.

### Approach used by tyv

`tyv` does **not** read or write authentication files, credentials, or OAuth
tokens. (The one file it reads is agy's account index, only to show the signed-in
email; see [Usage display](#usage-display).) Instead:

1. `tyv` keeps a local profile registry (`~/Library/Application Support/tyv/tyv.json`
   on macOS).
2. Each profile gets its own home directory, `profiles/<name>/home/`. It holds
   private copies of the places agy keeps its login (`~/.gemini`, and on macOS
   `~/Library/Keychains`; on Linux `~/.local/share/keyrings`) plus symlinks to
   the rest of your real home, so git, ssh and other tools keep working.
   Without the private keychain directory agy found your existing login through
   the macOS keychain and every profile silently became the same account.
   On macOS each profile also gets its own keychain file
   (`profiles/<name>/home/Library/Keychains/login.keychain-db`, created with
   `/usr/bin/security`), because agy stores its login in the keychain and
   shows a "Keychain Not Found" dialog if none exists. That keychain uses a
   fixed password and is protected only by the `0700` directory; it exists for
   isolation, not secrecy. On Linux tyv hides the D-Bus session bus from agy so
   it uses its private file storage instead of the shared desktop keyring.
3. `tyv <profile>` runs agy with `HOME` pointing at that directory (and
   `TYV_ACTIVE_PROFILE` set), via `execve(2)` on Unix or a child process on
   Windows. agy does all authentication itself.
4. `tyv add <name>` offers to launch agy once so you can sign in; each profile
   signs in separately. Existing profiles must sign in again the first time
   after upgrading, and settings in the real `~/.gemini` (settings, history,
   MCP config) are not shared with profiles. Because the keychain is private
   to each profile, tools run inside a profile (for example `git` with the
   macOS keychain credential helper) will not see your login keychain.
5. `tyv remove <name>` deletes the profile directory, including that profile's
   agy login. Symlinks are removed, never followed, so your real home is
   untouched.

### Usage display

```
AI Account Usage

  Account         Gemini            Claude  Email
  ───────────────────────────────────────────────────────────
❯ personal  100% (6d 23h 59m)    98% (3h 10m)  me@gmail.com
  work          11% (4h 12m)  77% (6d 1h 44m)  work@company.com
  university               —                —  —

  ↑/↓ Navigate   Enter Select   R Refresh   Esc Exit
```

tyv finds the email in two places, in this order: the `active` field of
`<profile home>/.gemini/google_accounts.json` (agy's account index; written
only when agy stores its login in a file), and otherwise the sign-in line
(`authenticated successfully as …`) in agy's most recent logs for that profile.
agy keeps its login in the macOS keychain, so on macOS the log line is the
source. Tokens and the keychain are never read. The detected email is saved to
the profile (the same field as the `--email` hint) so it survives log rotation.
If nothing is found, tyv shows the `--email` hint, or `—`. A profile that is not signed in
resolves to `—` within about a second.

Plain `tyv` runs `agy --print /usage --output-format json` per profile (in
parallel, in the background) and shows the lowest of the 5-hour and weekly
remaining quota for Gemini and for Claude/GPT models. The bracket after each
percentage is the time until that limiting quota resets, in
plain units: `6d 14h 5m`, `4h 12m` or `35m`.

Press **r** to fetch fresh numbers again without leaving the picker: all rows go
back to the spinner and update as each lookup finishes (`r` is ignored while a
refresh is already running).

The current profile (the one you last switched to) is shown with its name and
email in blue, and `tyv list` marks it the same way.

Percentages are coloured by how much quota is left: **green** at 50% or more,
**yellow** from 20% to 49%, **red** below 20%. Set `NO_COLOR=1` (or use a
`TERM=dumb` terminal) to turn colours off. Figures show an animated
spinner while loading and `—` if a profile is not signed in or agy cannot be
queried. **Every run fetches fresh numbers**: all rows start on the spinner and
fill in as each lookup finishes (about 10 seconds per profile, all in parallel),
so you never see stale figures that suddenly change. If a lookup fails, the
last successful reading is shown instead (kept in `usage-cache.json`:
percentages only, no credentials). Switching with `tyv <profile>` never queries
usage.

## Security

- `tyv` never asks for your Google password.
- `tyv` never stores OAuth tokens, access tokens, or refresh tokens.
- `tyv` never reads or copies credential files, tokens or the keychain. The only things it reads are the signed-in email (see [Usage display](#usage-display)) and the account names/quota agy reports.
- `tyv` never makes network requests itself (agy does, for sign-in and usage).
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
- tyv's own data: profile names, optional email hints, timestamps, and the
  cached usage percentages.
- Each profile's agy login and data are written by agy inside that profile's
  directory under tyv's config directory.

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
their target OS. In particular, the Windows console handling of the
interactive selector is untested.

## Architecture

```
tyv
├── cmd/tyv/main.go              Command dispatcher
├── internal/
│   ├── config/config.go        Profile registry, atomic config writes
│   ├── doctor/doctor.go        Diagnostic command
│   ├── exitcode/exitcode.go    Consistent exit codes
│   ├── launcher/launcher.go    agy launching, per-profile HOME isolation
│   ├── providers/antigravity/  Quota lookup via `agy /usage` (quota, parser, cache)
│   ├── selector/               Interactive picker (+ per-OS raw terminal mode)
│   ├── platform/
│   │   ├── platform.go         Interface definition
│   │   ├── darwin.go           macOS implementation
│   │   ├── linux.go            Linux implementation
│   │   └── windows.go          Windows implementation
│   └── validation/validation.go Profile name validation
└── tests/                      Integration tests
```

Platform-specific code is isolated behind the `Platform` interface and
build-tagged files (`platform/*`, `selector/term_*.go`). The only runtime OS
checks are in `launcher` (Windows cannot symlink the home directory without
elevated rights, so it only redirects `USERPROFILE`/`HOME`).

## Troubleshooting

**tyv: agy executable not found**
Install the Antigravity CLI from https://antigravity.google/download
or set `TYV_AGY_PATH=/path/to/agy`.

**tyv: profile "x" does not exist**
Run `tyv list` to see available profiles. Create with `tyv add x`.

**tyv: configuration file appears to be corrupted**
Run `tyv doctor` for diagnostics. If needed:
```sh
rm ~/Library/Application\ Support/tyv/tyv.json   # macOS
```

**macOS shows "Keychain Not Found … antigravity" during sign-in**
Choose **Cancel**, never "Reset To Defaults". This only happens for profiles
created before tyv gave each profile its own keychain; update tyv, then
`tyv remove <name>` and `tyv add <name>` again.

**A profile's Email column shows `—`**
The profile is not signed in, or agy has not logged a sign-in for it yet. Run
`tyv <name>`, sign in, and run `tyv` again. You can set a fallback with
`tyv add <name> --email <addr>` when creating the profile.

**A profile shows `—` in the selector**
That profile is not signed in yet (run `tyv <name>` and sign in), or agy could
not be queried (check `tyv doctor`, then try `agy --print /usage` manually).

**Enable debug output**
```sh
TYV_DEBUG=1 tyv personal
```
Debug output never prints credentials.

## Development

```sh
# Build
go build ./cmd/tyv/

# Test (with race detector)
go test -race ./...

# Vet and format
go vet ./...
gofmt -l .

# Cross-platform builds
GOOS=linux GOARCH=amd64 go build ./cmd/tyv/
GOOS=windows GOARCH=amd64 go build ./cmd/tyv/
```

## Testing

```sh
go test -v -race ./...
```

Tests cover:
- Profile CRUD (create, read, update, delete); removal never follows symlinks
- Per-profile `HOME` isolation of the launch environment
- Selector navigation, wrap-around, exit keys and live updates
- `/usage` parsing and the usage cache
- Atomic config writes
- File permission verification
- Corruption detection
- Path traversal rejection
- Shell metacharacter rejection
- Reserved name rejection
- Concurrent access (race detector)

## Building Releases

```sh
make cross-build   # all platforms into dist/
make checksums     # SHA-256 sums for dist/
```

Pushing a `v*` tag runs the CI release job, which publishes the binaries and
`SHA256SUMS`.

## Environment Variables

| Variable | Purpose |
|---|---|
| `TYV_AGY_PATH` | Absolute path to the agy executable |
| `TYV_DEBUG=1` | Debug output (never prints credentials) |
| `TYV_INSTALL_DIR` | Install directory for `install.sh` (default `~/.local/bin`) |
| `TYV_ACTIVE_PROFILE` | Set by tyv for agy: the active profile name |
| `NO_COLOR` | Any non-empty value turns off colours in the selector |

## Contributing

1. All security-relevant changes require a threat model review.
2. No external dependencies without documented justification.
3. All new features require tests.
4. Run `go vet ./...` and `go test -race ./...` before submitting.

## License

MIT
