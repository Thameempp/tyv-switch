# Threat Model

## Overview

This document describes the threat model for `tyv`, the Antigravity account
profile manager. For each threat, we describe the attack, impact, mitigation,
and remaining limitations.

---

## 1. Malicious Local Process

**Attack**: Another process on the same machine reads tyv's configuration file.

**Impact**: An attacker learns the profile names and email hints the user
configured. tyv's own files hold no credentials. (agy's per-profile logins
live under the same directory tree; see threat 14.)

**Mitigation**: Configuration directory is `0700` (owner-only). Config file
is `0600` (owner read/write only). On Unix, other processes cannot read the
file without privilege escalation.

**Remaining limitation**: If the attacker has root/admin access, they can
read any file on the system. tyv cannot protect against root-level attackers.

---

## 2. Compromised User Account

**Attack**: An attacker gains access to the user's account (e.g., by stealing
their shell session).

**Impact**: The attacker can run `tyv` and launch agy sessions. They can read
profile metadata, and read the per-profile agy logins (see threat 14).

**Mitigation**: tyv itself stores no credentials, and the directory tree is
owner-only (`0700`).

**Remaining limitation**: If the attacker has user-level access, they can
interact with agy directly. This is a limitation of the underlying OS security
model, not tyv.

---

## 3. Malicious Profile Name

**Attack**: A user attempts to create a profile with a name like
`../../etc/passwd`, `profile; rm -rf /`, or `$(malicious-command)`.

**Impact**: Could lead to path traversal, command injection, or filesystem
damage if tyv naively used the profile name in file paths or shell commands.

**Mitigation**: All profile names are validated with a strict allowlist regex
before any filesystem operation. Names must match `^[a-z0-9][a-z0-9\-_]{0,62}$`.
Path separators and shell metacharacters are explicitly rejected. tyv does not
use shell invocation for process launching.

**Remaining limitation**: None identified for this vector.

---

## 4. Path Traversal

**Attack**: A malicious profile name containing `../` components is used to
read or write files outside the tyv configuration directory.

**Impact**: Could read sensitive files or overwrite system files.

**Mitigation**: Profile names are validated to reject `/`, `\`, and `..`
before any path construction. Profile data directories are constructed as
`filepath.Join(appDataDir, "profiles", validatedName)`. `filepath.Join`
resolves paths but the pre-validated name cannot produce traversal sequences.

**Remaining limitation**: None identified.

---

## 5. Command Injection

**Attack**: A malicious argument is passed to agy via shell interpolation.

**Impact**: Arbitrary command execution.

**Mitigation**: tyv uses `syscall.Exec()` / `exec.Command()` everywhere, which
accept argument arrays and do not invoke a shell.
No shell interpolation occurs. Arguments are passed directly.

**Remaining limitation**: None for tyv's own argument passing. If agy itself
has argument-processing vulnerabilities, those are outside tyv's scope.

---

## 6. Credential Leakage

**Attack**: tyv accidentally logs or prints OAuth tokens or passwords.

**Impact**: Credential exposure in terminal output or log files.

**Mitigation**: tyv never reads credential files. tyv never stores credentials.
tyv's debug output (`TYV_DEBUG=1`) logs only: profile name, executable path,
and "launching process". No environment variable values are logged. The usage
lookup keeps only percentages; agy's raw `/usage` output is parsed and
discarded. The only account data tyv extracts from agy's files is the signed-in
email address (account index or one log line), which is validated before use.

**Remaining limitation**: None identified.

---

## 7. Log Leakage

**Attack**: tyv writes sensitive data to log files that are readable by other
processes.

**Impact**: Credential or secret exposure via log files.

**Mitigation**: tyv does not write log files. All output goes to stdout/stderr
only. Debug output is ephemeral and never contains secrets.

**Remaining limitation**: Terminal history may capture tyv command invocations
(which contain only profile names, not secrets).

---

## 8. Configuration File Tampering

**Attack**: An attacker modifies `tyv.json` to change profile names or inject
malicious data.

**Impact**: tyv loads tampered configuration and may behave unexpectedly. Since
profile names are validated at read time (implicitly via existence checks) and
the data is non-executable JSON, the impact is limited to confusion, not RCE.

**Mitigation**: Config directory is `0700`. Config is validated on load (JSON
unmarshal). If the file is corrupted, tyv reports an actionable error rather
than silently proceeding.

**Remaining limitation**: If an attacker has user-level access, they can modify
the config. The impact is that they can change which profile name maps to which
data directory, not that they gain credentials.

---

## 9. Symlink Attacks

**Attack**: An attacker creates a symlink at the config path pointing to a
sensitive file. tyv follows the symlink and reads or writes it.

**Impact**: Reading a sensitive file (low: tyv only reads its own JSON).
Overwriting a sensitive file (higher: if tyv writes to a symlinked path).

**Mitigation**: tyv writes via atomic temp file + rename, which does not follow
existing symlinks at the destination. The rename atomically replaces the
destination. Config directory is `0700`, so an attacker must have user-level
access to plant a symlink there.

**Remaining limitation**: If the attacker has user-level access, symlink
attacks within the user's own directory are possible. This is a general OS
security limitation. No credential data is at risk because tyv doesn't read
credentials.

---

## 10. Unauthorized Profile Access

**Attack**: User A reads User B's tyv profiles on a shared machine.

**Impact**: Learns profile names and email hints of another user.

**Mitigation**: Config directory is `0700`. Other users cannot read or list
the directory. Standard Unix permission model applies.

**Remaining limitation**: Root can always read all files.

---

## 11. Accidental Credential Exposure

**Attack**: A user accidentally runs `TYV_DEBUG=1 tyv personal | tee log.txt`,
producing a log file with sensitive data.

**Impact**: If tyv logged credentials, the log file would expose them.

**Mitigation**: tyv never logs credentials. Debug output contains only:
profile name, agy path, "launching process". Even with debug mode enabled
and output redirected, no credentials are exposed.

**Remaining limitation**: None identified.

---

## 12. Compromised Dependency

**Attack**: A malicious dependency is introduced into tyv's supply chain.

**Impact**: Malicious code executes with tyv's permissions.

**Mitigation**: tyv has **zero external Go dependencies**. Only the Go standard
library is used. The Go standard library is part of the Go distribution and
subject to Google's security process.

**Remaining limitation**: Compromise of the Go toolchain itself would affect tyv.
This is mitigated by using official Go releases and verifying checksums.

---

## 13. Malicious Release Binary

**Attack**: An attacker distributes a malicious binary named `tyv`.

**Impact**: The binary executes with user permissions and could steal
credentials or perform harmful actions.

**Mitigation**: GitHub Releases should include SHA-256 checksums. Users should
verify checksums before installing. Signed releases provide stronger guarantees.

**Remaining limitation**: If users download from unofficial sources without
verifying checksums, they are at risk. This is a general distribution trust
problem, not specific to tyv.

---

## 14. Profile Logins Stored Under tyv's Directory

**Attack**: A local process or backup tool reads
`<config dir>/profiles/<name>/home/.gemini/`, where agy keeps that profile's
OAuth login.

**Impact**: Possible theft of a Google account session for that profile.

**Mitigation**: tyv creates the directory tree with `0700`, so other users
cannot traverse it; agy creates its credential files with its own restrictive
modes. tyv never reads, copies or logs these files.

On macOS agy also stores tokens in the profile's own keychain file, created
with a fixed, non-secret password, so it protects against nothing except
mix-ups between accounts.

**Remaining limitation**: Same-user processes and unencrypted backups of the
config directory can read them. Exclude the `profiles/` directory from backups
and sync tools. tyv offers no encryption at rest.

---

## 15. Home-Directory Mirroring

**Attack**: Symlinks from a profile home into the real home are abused, e.g.
`tyv remove` follows them and deletes real files, or a loop causes endless
traversal.

**Impact**: Data loss in the real home directory.

**Mitigation**: Removal uses `os.RemoveAll`, which unlinks symlinks without
following them; a test verifies files behind a symlink survive `tyv remove`.
The real `~/.gemini`, `~/Library/Keychains` and `~/.local/share/keyrings` are
never mirrored, so profiles cannot reach the default login through their own
home.

**Remaining limitation**: On macOS the mirrored `Library` entry contains tyv's
own config directory, so a recursive scan of a profile home (e.g. `find ~`)
may loop until the OS symlink limit. Tools that follow symlinks blindly should
not be pointed at profile homes.

---

## 16. Untrusted agy Output

**Attack**: A malicious or buggy `agy` returns crafted output to the usage
lookup.

**Impact**: Wrong numbers displayed, or resource exhaustion.

**Mitigation**: Output is parsed as JSON into a fixed structure; only numeric
fractions are used, clamped to 0–100%. Lookups have a 40 s timeout and are
killed when the selector exits. The agy path is the same one used for
launching (threat 13 applies).

**Remaining limitation**: tyv trusts whichever `agy` binary it finds.

---

## Out of Scope

The following are explicitly out of scope for this threat model:

- Vulnerabilities in agy itself
- Vulnerabilities in Google's OAuth infrastructure
- OS-level kernel vulnerabilities
- Physical access attacks
- Compromise of the user's Google account
