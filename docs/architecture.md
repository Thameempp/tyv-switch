# sa Architecture

## Overview

`sa` is a single Go binary with no external dependencies. It maintains a
local profile registry and delegates all authentication and AI functionality
to the Antigravity CLI (`agy`).

```
┌─────────────────────────────────────────────────────────┐
│                    sa binary                            │
│                                                         │
│  cmd/sa/main.go                                         │
│    Command dispatcher                                   │
│    ↓                                                    │
│  internal/validation/  ←─ Input validation (first)     │
│  internal/config/      ←─ Profile registry              │
│  internal/platform/    ←─ OS abstraction layer          │
│  internal/launcher/    ←─ Process launching             │
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
│  Manages ~/.gemini/oauth_creds.json                     │
│  Manages ~/.gemini/google_accounts.json                 │
└─────────────────────────────────────────────────────────┘
```

## Directory Structure

```
.
├── cmd/
│   └── sa/
│       └── main.go              # Entry point, command dispatcher
├── internal/
│   ├── config/
│   │   └── config.go            # Profile registry, atomic writes
│   ├── doctor/
│   │   └── doctor.go            # sa doctor command
│   ├── exitcode/
│   │   └── exitcode.go          # Standardized exit codes
│   ├── launcher/
│   │   └── launcher.go          # AGY process launching
│   ├── platform/
│   │   ├── platform.go          # Platform interface
│   │   ├── darwin.go            # macOS implementation (+build darwin)
│   │   ├── linux.go             # Linux implementation (+build linux)
│   │   └── windows.go           # Windows implementation (+build windows)
│   └── validation/
│       └── validation.go        # Profile name validation
├── tests/
│   ├── config_test.go           # Config package tests
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
- `~/.gemini/oauth_creds.json` — OAuth tokens (access, refresh, id)
- `~/.gemini/google_accounts.json` — Active account email + history
- `~/.gemini/antigravity-cli/` — CLI app data (conversations, settings)
- `~/Library/Application Support/Antigravity/` — Browser-based Electron app data

**Mechanisms evaluated:**

| Mechanism | Status | Notes |
|---|---|---|
| `--account <email>` flag | Not found | No such flag in agy |
| `--profile <name>` flag | Not found | No such flag in agy |
| `--login`/`--logout` flags | Not found | No such flags in agy |
| `GEMINI_DIR` env var | Not confirmed | Binary contains string but not verified |
| `GEMINI_HOME` env var | Not confirmed | Not found in binary |
| `--app_data_dir` flag | Found (internal) | Requires relative path, undocumented, unstable |
| Official profile support | Not present | No documented mechanism |

**Conclusion**: agy v1.2.14 has no officially supported, stable API for
multi-account isolation or profile switching. The only safe approach is to
manage labels locally and let agy handle its own authentication.

## Data Flow: Profile Switch

```
User: sa personal
        │
        ▼
main.go: parse args → cmd = "personal" (not a reserved command)
        │
        ▼
validation.ProfileName("personal") → OK
        │
        ▼
config.Manager.Load() → reads ~/Library/Application Support/sa/sa.json
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
buildEnv(profile, profileDataDir) → copies os.Environ() + SA_ACTIVE_PROFILE=personal
        │
        ▼
validateExecutablePath("/Users/thameem/.local/bin/agy") → OK
        │
        ▼
syscall.Exec("/Users/thameem/.local/bin/agy", [], env) → process replaced
        │
        ▼
agy runs with the user's current auth (sa is gone from the process table)
```

## Config File Format

Location: `<AppDataDir>/sa.json`

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
select the correct implementation at compile time. No runtime `GOOS` checks
in business logic.

## Atomic Config Writes

To prevent config corruption:

1. JSON is marshaled in-memory
2. A temp file is created in the same directory as `sa.json`
3. Data is written to the temp file
4. `Sync()` is called to flush to disk
5. File permissions are set to `0600`
6. `os.Rename()` atomically replaces `sa.json`

If the process crashes at any point before step 6, the original `sa.json`
is unchanged. If it crashes during step 6, the OS guarantees rename atomicity.

## Security-Critical Code Paths

| Path | Security property |
|---|---|
| validation.ProfileName() | Blocks injection before any I/O |
| config.Manager.save() | Atomic write, restrictive permissions |
| launcher.validateExecutablePath() | Null-byte check before exec |
| launcher.buildEnv() | Only adds SA_ACTIVE_PROFILE, no secret injection |
| platform.Launch() | No shell invocation (no sh -c) |
| doctor.Print() | Never prints credentials |

## Performance Design

`sa personal` hot path:

1. Parse single argument — O(1)
2. Validate name — regex match, O(n) where n ≤ 63
3. Load config — single JSON file read
4. Check profile exists — map lookup, O(1)
5. Touch last used — write (atomic)
6. Find agy — cached after first call; PATH lookup + stat
7. execve — OS replaces process, zero sa overhead after this point

Total sa overhead before agy starts: < 10ms in normal conditions.
