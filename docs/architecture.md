# tyv Architecture

## Overview

`tyv` is a single Go binary with no external dependencies. It maintains a
local profile registry, gives each profile an isolated agy home directory, and
delegates all authentication and AI functionality to the Antigravity CLI
(`agy`). It also shows each account's remaining quota via `agy /usage`.

```
┌─────────────────────────────────────────────────────────┐
│                    tyv binary                            │
│                                                         │
│  cmd/tyv/main.go                                         │
│    Command dispatcher                                   │
│    ↓                                                    │
│  internal/validation/  ←─ Input validation (first)     │
│  internal/config/      ←─ Profile registry              │
│  internal/platform/    ←─ OS abstraction layer          │
│  internal/launcher/    ←─ Launching + per-profile HOME  │
│  internal/selector/    ←─ Interactive picker (TTY)      │
│  internal/providers/   ←─ Quota lookup (agy /usage)     │
│  internal/doctor/      ←─ Diagnostics                  │
│  internal/exitcode/    ←─ Consistent exit codes         │
└─────────────────────┬───────────────────────────────────┘
                      │ syscall.Exec (Unix)
                      │ exec.Command (Windows)
                      ↓
┌─────────────────────────────────────────────────────────┐
│                   agy (Antigravity CLI)                 │
│                                                         │
│  Handles authentication, AI sessions, conversations     │
│  Manages $HOME/.gemini/* — with HOME set per profile,   │
│  each profile has its own login                         │
└─────────────────────────────────────────────────────────┘
```

## Directory Structure

```
.
├── cmd/
│   └── tyv/
│       └── main.go              # Entry point, command dispatcher
├── internal/
│   ├── config/
│   │   └── config.go            # Profile registry, atomic writes
│   ├── doctor/
│   │   └── doctor.go            # tyv doctor command
│   ├── exitcode/
│   │   └── exitcode.go          # Standardized exit codes
│   ├── launcher/
│   │   ├── launcher.go          # AGY launching, per-profile HOME
│   │   ├── keychain_darwin.go   # Per-profile macOS keychain via `security`
│   │   └── keychain_other.go    # No-op on other platforms
│   ├── providers/
│   │   └── antigravity/
│   │       ├── quota.go         # Runs `agy --print /usage --output-format json`
│   │       ├── parser.go        # JSON -> Gemini/Claude remaining %
│   │       ├── cache.go         # last-known usage fallback cache (percentages only)
│   │       └── account.go       # Signed-in email (agy account index, else sign-in log line)
│   ├── selector/
│   │   ├── selector.go          # Picker rendering + key handling
│   │   ├── term_unix.go         # Raw mode via stty (+build !windows)
│   │   └── term_windows.go      # Console mode via kernel32 (+build windows)
│   ├── platform/
│   │   ├── platform.go          # Platform interface
│   │   ├── darwin.go            # macOS implementation (+build darwin)
│   │   ├── linux.go             # Linux implementation (+build linux)
│   │   └── windows.go           # Windows implementation (+build windows)
│   └── validation/
│       └── validation.go        # Profile name validation
├── tests/
│   ├── config_test.go           # Config package tests
│   ├── launcher_test.go         # HOME isolation, safe removal
│   ├── selector_test.go         # Picker keys and live updates
│   ├── usage_parser_test.go     # /usage parsing, cache
│   └── validation_test.go       # Validation security tests
├── docs/
│   └── architecture.md          # This file
├── dist/                        # Cross-platform release binaries
├── go.mod
├── README.md
├── SECURITY.md
├── PRIVACY.md
└── THREAT_MODEL.md
```

## Authentication Research Findings

Before implementing account switching, the Antigravity CLI binary (agy v1.2.14)
was analyzed:

**Storage locations discovered:**
- OS keychain (item `gemini` / `antigravity`) — OAuth tokens when the keychain
  is available (macOS keychain, Linux desktop keyring)
- `~/.gemini/oauth_creds.json` — OAuth tokens, file-storage fallback
- `~/.gemini/google_accounts.json` — Active account email + history (file mode only)
- `~/.gemini/antigravity-cli/log/` — CLI logs, including the signed-in email
- `~/.gemini/antigravity-cli/` — CLI app data (conversations, settings)
- `~/Library/Application Support/Antigravity/` — Browser-based Electron app data

**Mechanisms evaluated:**

| Mechanism | Status | Notes |
|---|---|---|
| `--account <email>` flag | Not found | No such flag in agy |
| `--profile <name>` flag | Not found | No such flag in agy |
| `--login`/`--logout` flags | Not found | No such flags in agy |
| `GEMINI_CLI_APP_DATA_DIR` env var | Ignored | Tested: agy still used the default login |
| `HOME` env var | **Honoured** | A fresh `HOME` triggers a fresh login prompt — used for isolation |
| macOS keychain (`~/Library/Keychains`) | Used by agy | Must be private per profile; mirroring it leaked the default login into every profile |
| `agy --print /usage --output-format json` | Works | Quota per model family, 5 h and weekly buckets |
| `GEMINI_DIR` env var | Not confirmed | Binary contains string but not verified |
| `GEMINI_HOME` env var | Not confirmed | Not found in binary |
| `--app_data_dir` flag | Found (internal) | Requires relative path, undocumented, unstable |
| Official profile support | Not present | No documented mechanism |

**Conclusion**: agy v1.2.14 has no officially supported API for
multi-account isolation. tyv relies on agy resolving its data from `$HOME`:
each profile runs agy with its own `HOME` (a private `.gemini` plus symlinks to
the real home), so every profile keeps a separate login while agy still handles
all authentication itself.

## Data Flow: Profile Switch

```
User: tyv personal
        │
        ▼
main.go: parse args → cmd = "personal" (not a reserved command)
        │
        ▼
validation.ProfileName("personal") → OK
        │
        ▼
config.Manager.Load() → reads ~/Library/Application Support/tyv/tyv.json
        │
        ▼
config.Manager.ProfileExists("personal") → true
        │
        ▼
config.Manager.TouchLastUsed("personal") → updates timestamp, saves atomically
        │
        ▼
launcher.New(plat, mgr).LaunchForProfile("personal", [])
        │
        ▼
plat.FindAGY() → /Users/thameem/.local/bin/agy
        │
        ▼
buildEnv(profile, profileDataDir) → prepareHome() creates profiles/personal/home
        │   (private .gemini, Library/Keychains, .local/share/keyrings + symlinks to real home), then os.Environ() with
        │   HOME=<that dir> and TYV_ACTIVE_PROFILE=personal
        │
        ▼
validateExecutablePath("/Users/thameem/.local/bin/agy") → OK
        │
        ▼
syscall.Exec("/Users/thameem/.local/bin/agy", [], env) → process replaced
        │
        ▼
agy runs with the profile's own login (tyv is gone from the process table)
```

## Data Flow: Selector (`tyv` with no arguments)

```
tyv (TTY, profiles exist)
   │
   ├─ load config → every row starts on the spinner (no stale numbers shown)
   ├─ for each profile, in parallel:
   │     agy --print /usage --output-format json   (HOME = profile home)
   │       → parser: min(5h, weekly) per family → live row update
   │       (on failure: fall back to the last reading in usage-cache.json)
   ├─ raw terminal mode → picker loop (↑/↓, Enter, Esc/q, Ctrl+C)
   └─ Enter → same path as `tyv <profile>`; lookups still running are cancelled
```

## Data Flow: `tyv add <name>`

```
create profile + directory → (TTY only) "Open Google authentication in browser? [Y/n]"
   → Y: run agy as a child process with the profile's HOME; the user signs in
        inside agy (agy opens the browser, handles OAuth)
   → exit 0: ✓ configured   |   otherwise: ✗ failed (profile kept; retry with `tyv <name>`)
```

## Config File Format

Location: `<AppDataDir>/tyv.json`

```json
{
  "version": 1,
  "current": "personal",
  "profiles": {
    "personal": {
      "name": "personal",
      "email": "me@example.com",
      "created_at": "2026-10-01T20:55:00Z",
      "last_used_at": "2026-10-01T21:00:00Z"
    },
    "work": {
      "name": "work",
      "email": "work@company.com",
      "created_at": "2026-10-01T20:56:00Z"
    }
  }
}
```

No credentials, tokens, or secrets are ever stored in this file.

## Platform Abstraction

All OS-specific behavior is hidden behind `internal/platform/Platform`:

```go
type Platform interface {
    OS() string
    Arch() string
    AppDataDir() (string, error)
    FindAGY() (string, error)
    Launch(executable string, args []string, env []string) error
}
```

Build tags (`//go:build darwin`, `//go:build linux`, `//go:build windows`)
select the correct implementation at compile time. The only runtime `GOOS`
checks are in `launcher.buildEnv` / `prepareHome` (Windows skips the symlink
mirror and also sets `USERPROFILE`).

## Atomic Config Writes

To prevent config corruption:

1. JSON is marshaled in-memory
2. A temp file is created in the same directory as `tyv.json`
3. Data is written to the temp file
4. `Sync()` is called to flush to disk
5. File permissions are set to `0600`
6. `os.Rename()` atomically replaces `tyv.json`

If the process crashes at any point before step 6, the original `tyv.json`
is unchanged. If it crashes during step 6, the OS guarantees rename atomicity.

## Security-Critical Code Paths

| Path | Security property |
|---|---|
| validation.ProfileName() | Blocks injection before any I/O |
| config.Manager.save() | Atomic write, restrictive permissions |
| launcher.validateExecutablePath() | Null-byte check before exec |
| launcher.buildEnv() | Replaces HOME / TYV_ACTIVE_PROFILE only, no secret injection |
| launcher.prepareHome() | Never mirrors ~/.gemini or keychain directories into a profile |
| config.Manager.RemoveProfile() | RemoveAll does not follow symlinks (tested) |
| antigravity.Parse() | Fixed JSON structure, values clamped to 0–100 |
| platform.Launch() | No shell invocation (no sh -c) |
| doctor.Print() | Never prints credentials |

## Performance Design

`tyv personal` hot path:

1. Parse single argument — O(1)
2. Validate name — regex match, O(n) where n ≤ 63
3. Load config — single JSON file read
4. Check profile exists — map lookup, O(1)
5. Touch last used — write (atomic)
6. Find agy — cached after first call; PATH lookup + stat
7. Prepare profile home — one `ReadDir` of the real home plus one `Lstat` per
   entry (creates only missing symlinks)
8. execve — OS replaces process, zero tyv overhead after this point

Total tyv overhead before agy starts: typically a few milliseconds. No usage
lookup happens on this path (agy's `/usage` takes ~10 s, so it is only run for
the selector, in the background).
