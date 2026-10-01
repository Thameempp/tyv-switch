# Security Policy

## Security Model

`tyv` is a local profile manager for the Antigravity CLI. Its security model
is based on strict separation of concerns:

**tyv is responsible for:**
- Managing a local profile registry (names, metadata, timestamps)
- Validating user input (profile names, commands)
- Finding the agy executable safely
- Giving each profile an isolated home directory for agy (`HOME` override)
- Launching agy as a replacement process, or as a child process for sign-in
  (`tyv add`) and quota lookups (the `tyv` selector)

**tyv is NOT responsible for:**
- Authentication (handled exclusively by agy)
- Credential storage (handled by agy, inside each profile's private home)
- Network communication (tyv makes no network requests; agy does)
- OAuth token management

## What tyv Can Access

- Its own configuration directory:
  - macOS: `~/Library/Application Support/tyv/`
  - Linux: `$XDG_CONFIG_HOME/tyv/` (default `~/.config/tyv/`)
  - Windows: `%APPDATA%\tyv\`
- The agy executable (read + execute, not write)
- The names of the entries in your real home directory (to mirror them into
  each profile home as symlinks); it does not read their contents
- Environment variables (to pass to the child process)
- The user's PATH (to locate agy)

## What tyv Does Not Access

- `~/.gemini/oauth_creds.json` — never read, never written (the real
  `~/.gemini` is deliberately **not** mirrored into profile homes)
- `<profile>/home/.gemini/*` — written by agy; tyv never writes it and only
  deletes it when you run `tyv remove`. The single exception is that tyv reads
  the `active` field of `google_accounts.json` and, when that is absent, the
  sign-in line in agy's logs (an email address, no tokens) to show which
  account a profile is signed into; the value is validated and never logged. `oauth_creds.json` and other credential files are never opened.
- Any browser cookies or sessions
- Any OS keychain entries
- Any credential files
- Any network endpoints

## Credential Handling

`tyv` does not handle credentials. Period.

- No password prompts
- No OAuth token extraction
- No token copying or caching
- No credential file manipulation
- No credential logging

## Supported Authentication Mechanism

All authentication is delegated to the official Antigravity CLI (`agy`).
`tyv` launches `agy` and steps out of the way. agy manages its own OAuth
flow, token refresh, and credential storage.

## File Permissions

- Configuration directory: `0700` (owner only)
- Configuration file: `0600` (owner read/write only)
- Profile data directories and profile homes: `0700`
- `usage-cache.json`: `0600` (percentages only)
- Per-profile keychain (macOS): created and unlocked with `/usr/bin/security`
  using a fixed password. It protects nothing beyond the `0700` directory; its
  purpose is to keep each profile's agy login out of your login keychain. tyv
  never reads it and never touches your real keychain or keychain preferences.

Atomic write pattern used for all config updates:
1. Write to a temporary file
2. Sync to disk
3. Atomic `rename()` to final path

This prevents partial-write corruption even on system crash.

## Input Validation

All profile names are validated before any filesystem operation:

- Must match `^[a-z0-9][a-z0-9\-_]{0,62}$`
- No path separators (`/`, `\`)
- No path traversal (`..`)
- No control characters
- No shell metacharacters
- No reserved command names

## Executable Discovery

The agy executable is found via:
1. `TYV_AGY_PATH` environment variable (if set)
2. `exec.LookPath("agy")` (PATH search)
3. Known installation directories

The found path is validated (must exist, must be a file, must be executable)
before any `exec` call.

## Process Launching

On Unix: `syscall.Exec()` (execve) — replaces the current process atomically.
On Windows: child process with inherited stdout/stderr and exit-code forwarding.

`tyv add` (sign-in) and the selector (`agy --print /usage --output-format
json`, a fixed argument list, 40 s timeout) run agy as a child process.

No method uses shell interpolation. Arguments are passed directly as a string
slice, preventing command injection. The environment passed to agy is the
current environment with `HOME` (and `USERPROFILE` on Windows) and
`TYV_ACTIVE_PROFILE` replaced; no secrets are injected.

## Profile Isolation

Each profile runs agy with `HOME=<config dir>/profiles/<name>/home`. That
directory contains private, real copies of the places agy stores its login
(`.gemini`, `Library/Keychains` on macOS, `.local/share/keyrings` on Linux)
plus symlinks to every other entry in the real home so tools launched by agy
(git, ssh, …) keep working. The keychain directory must be private because
agy also finds logins through the user keychain; mirroring it made every
profile sign in as the same account.
Consequences:

- Profiles cannot see each other's logins.
- Anything running inside a profile has the same access to your real home
  (except `~/.gemini`) as before, through the symlinks.
- `tyv remove` uses `os.RemoveAll`, which removes symlinks without following
  them (covered by a test).

## Debug Mode

`TYV_DEBUG=1` enables debug logging. Debug output:
- Logs profile name
- Logs executable path
- Logs "launching process"
- NEVER logs tokens, secrets, environment variable values, or credentials

## Vulnerability Reporting

To report a security vulnerability, please email the maintainer privately
(see contact in `go.mod` or repository settings) rather than opening a
public issue.

Please include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment

We will respond within 72 hours and aim to release a fix within 14 days for
critical issues.

## Responsible Disclosure

We follow responsible disclosure principles:
1. Report privately first
2. Maintainer confirms and assesses impact
3. Fix is developed
4. Coordinated public disclosure after fix is released

## Known Limitations

1. **Account isolation relies on agy's use of `$HOME`**: agy has no documented
   multi-account support. Isolation works because agy resolves its data
   directory from `$HOME`; a future agy version could change this. Each
   profile must sign in once (`tyv add` / `tyv <name>`).

2. **No OS keychain integration for tyv's own data**: `tyv` stores only
   non-sensitive metadata (profile names, timestamps, cached usage
   percentages). agy's own tokens live in the profile homes (`0700` directory
   tree under tyv's config directory), protected by file permissions only.

3. **Trust in agy binary**: `tyv` trusts the agy binary found on PATH or at
   `TYV_AGY_PATH`. If a malicious binary named `agy` is placed on the system,
   `tyv` may launch it. Mitigated by: PATH integrity is the user's
   responsibility; `TYV_AGY_PATH` provides an explicit override.
