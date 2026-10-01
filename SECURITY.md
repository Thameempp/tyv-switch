# Security Policy

## Security Model

`sa` is a local profile manager for the Antigravity CLI. Its security model
is based on strict separation of concerns:

**sa is responsible for:**
- Managing a local profile registry (names, metadata, timestamps)
- Validating user input (profile names, commands)
- Finding the agy executable safely
- Launching agy as a replacement process

**sa is NOT responsible for:**
- Authentication (handled exclusively by agy)
- Credential storage (handled by agy and the OS)
- Network communication (sa makes no network requests)
- OAuth token management

## What sa Can Access

- Its own configuration directory:
  - macOS: `~/Library/Application Support/sa/`
  - Linux: `$XDG_CONFIG_HOME/sa/` (default `~/.config/sa/`)
  - Windows: `%APPDATA%\sa\`
- The agy executable (read + execute, not write)
- Environment variables (to pass to the child process)
- The user's PATH (to locate agy)

## What sa Does Not Access

- `~/.gemini/oauth_creds.json` — never read, never written
- `~/.gemini/google_accounts.json` — never read, never written
- Any browser cookies or sessions
- Any OS keychain entries
- Any credential files
- Any network endpoints

## Credential Handling

`sa` does not handle credentials. Period.

- No password prompts
- No OAuth token extraction
- No token copying or caching
- No credential file manipulation
- No credential logging

## Supported Authentication Mechanism

All authentication is delegated to the official Antigravity CLI (`agy`).
`sa` launches `agy` and steps out of the way. agy manages its own OAuth
flow, token refresh, and credential storage.

## File Permissions

- Configuration directory: `0700` (owner only)
- Configuration file: `0600` (owner read/write only)
- Profile data directories: `0700`

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
1. `SA_AGY_PATH` environment variable (if set)
2. `exec.LookPath("agy")` (PATH search)
3. Known installation directories

The found path is validated (must exist, must be a file, must be executable)
before any `exec` call.

## Process Launching

On Unix: `syscall.Exec()` (execve) — replaces the current process atomically.
On Windows: child process with inherited stdout/stderr and exit-code forwarding.

Neither method uses shell interpolation. Arguments are passed directly as
a string slice, preventing command injection.

## Debug Mode

`SA_DEBUG=1` enables debug logging. Debug output:
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

1. **Multi-Google-account switching**: `sa` cannot perform instant credential
   switching between different Google accounts because agy does not expose a
   stable, documented API for this. Users must manually log in/out of agy
   for different Google accounts.

2. **No OS keychain integration for sa's own data**: `sa` stores only
   non-sensitive metadata (profile names, timestamps). No OS keychain
   integration is needed because no secrets are stored.

3. **Trust in agy binary**: `sa` trusts the agy binary found on PATH or at
   `SA_AGY_PATH`. If a malicious binary named `agy` is placed on the system,
   `sa` may launch it. Mitigated by: PATH integrity is the user's
   responsibility; `SA_AGY_PATH` provides an explicit override.
