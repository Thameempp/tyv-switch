# Threat Model

## Overview

This document describes the threat model for `sa`, the Antigravity account
profile manager. For each threat, we describe the attack, impact, mitigation,
and remaining limitations.

---

## 1. Malicious Local Process

**Attack**: Another process on the same machine reads sa's configuration file.

**Impact**: An attacker learns the profile names and email hints the user
configured. No credentials are exposed because sa does not store credentials.

**Mitigation**: Configuration directory is `0700` (owner-only). Config file
is `0600` (owner read/write only). On Unix, other processes cannot read the
file without privilege escalation.

**Remaining limitation**: If the attacker has root/admin access, they can
read any file on the system. sa cannot protect against root-level attackers.

---

## 2. Compromised User Account

**Attack**: An attacker gains access to the user's account (e.g., by stealing
their shell session).

**Impact**: The attacker can run `sa` and launch agy sessions. They can read
profile metadata. They cannot extract credentials from sa (sa has none).

**Mitigation**: sa does not store credentials, so compromising sa's data
directory does not give the attacker authentication material.

**Remaining limitation**: If the attacker has user-level access, they can
interact with agy directly. This is a limitation of the underlying OS security
model, not sa.

---

## 3. Malicious Profile Name

**Attack**: A user attempts to create a profile with a name like
`../../etc/passwd`, `profile; rm -rf /`, or `$(malicious-command)`.

**Impact**: Could lead to path traversal, command injection, or filesystem
damage if sa naively used the profile name in file paths or shell commands.

**Mitigation**: All profile names are validated with a strict allowlist regex
before any filesystem operation. Names must match `^[a-z0-9][a-z0-9\-_]{0,62}$`.
Path separators and shell metacharacters are explicitly rejected. sa does not
use shell invocation for process launching.

**Remaining limitation**: None identified for this vector.

---

## 4. Path Traversal

**Attack**: A malicious profile name containing `../` components is used to
read or write files outside the sa configuration directory.

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

**Mitigation**: sa uses `syscall.Exec()` on Unix and `exec.Command()` on
Windows, both of which accept argument arrays and do not invoke a shell.
No shell interpolation occurs. Arguments are passed directly.

**Remaining limitation**: None for sa's own argument passing. If agy itself
has argument-processing vulnerabilities, those are outside sa's scope.

---

## 6. Credential Leakage

**Attack**: sa accidentally logs or prints OAuth tokens or passwords.

**Impact**: Credential exposure in terminal output or log files.

**Mitigation**: sa never reads credential files. sa never stores credentials.
sa's debug output (`SA_DEBUG=1`) logs only: profile name, executable path,
and "launching process". No environment variable values are logged.

**Remaining limitation**: None identified.

---

## 7. Log Leakage

**Attack**: sa writes sensitive data to log files that are readable by other
processes.

**Impact**: Credential or secret exposure via log files.

**Mitigation**: sa does not write log files. All output goes to stdout/stderr
only. Debug output is ephemeral and never contains secrets.

**Remaining limitation**: Terminal history may capture sa command invocations
(which contain only profile names, not secrets).

---

## 8. Configuration File Tampering

**Attack**: An attacker modifies `sa.json` to change profile names or inject
malicious data.

**Impact**: sa loads tampered configuration and may behave unexpectedly. Since
profile names are validated at read time (implicitly via existence checks) and
the data is non-executable JSON, the impact is limited to confusion, not RCE.

**Mitigation**: Config directory is `0700`. Config is validated on load (JSON
unmarshal). If the file is corrupted, sa reports an actionable error rather
than silently proceeding.

**Remaining limitation**: If an attacker has user-level access, they can modify
the config. The impact is that they can change which profile name maps to which
data directory, not that they gain credentials.

---

## 9. Symlink Attacks

**Attack**: An attacker creates a symlink at the config path pointing to a
sensitive file. sa follows the symlink and reads or writes it.

**Impact**: Reading a sensitive file (low: sa only reads its own JSON).
Overwriting a sensitive file (higher: if sa writes to a symlinked path).

**Mitigation**: sa writes via atomic temp file + rename, which does not follow
existing symlinks at the destination. The rename atomically replaces the
destination. Config directory is `0700`, so an attacker must have user-level
access to plant a symlink there.

**Remaining limitation**: If the attacker has user-level access, symlink
attacks within the user's own directory are possible. This is a general OS
security limitation. No credential data is at risk because sa doesn't read
credentials.

---

## 10. Unauthorized Profile Access

**Attack**: User A reads User B's sa profiles on a shared machine.

**Impact**: Learns profile names and email hints of another user.

**Mitigation**: Config directory is `0700`. Other users cannot read or list
the directory. Standard Unix permission model applies.

**Remaining limitation**: Root can always read all files.

---

## 11. Accidental Credential Exposure

**Attack**: A user accidentally runs `SA_DEBUG=1 sa personal | tee log.txt`,
producing a log file with sensitive data.

**Impact**: If sa logged credentials, the log file would expose them.

**Mitigation**: sa never logs credentials. Debug output contains only:
profile name, agy path, "launching process". Even with debug mode enabled
and output redirected, no credentials are exposed.

**Remaining limitation**: None identified.

---

## 12. Compromised Dependency

**Attack**: A malicious dependency is introduced into sa's supply chain.

**Impact**: Malicious code executes with sa's permissions.

**Mitigation**: sa has **zero external Go dependencies**. Only the Go standard
library is used. The Go standard library is part of the Go distribution and
subject to Google's security process.

**Remaining limitation**: Compromise of the Go toolchain itself would affect sa.
This is mitigated by using official Go releases and verifying checksums.

---

## 13. Malicious Release Binary

**Attack**: An attacker distributes a malicious binary named `sa`.

**Impact**: The binary executes with user permissions and could steal
credentials or perform harmful actions.

**Mitigation**: GitHub Releases should include SHA-256 checksums. Users should
verify checksums before installing. Signed releases provide stronger guarantees.

**Remaining limitation**: If users download from unofficial sources without
verifying checksums, they are at risk. This is a general distribution trust
problem, not specific to sa.

---

## Out of Scope

The following are explicitly out of scope for this threat model:

- Vulnerabilities in agy itself
- Vulnerabilities in Google's OAuth infrastructure
- OS-level kernel vulnerabilities
- Physical access attacks
- Compromise of the user's Google account
