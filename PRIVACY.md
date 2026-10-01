# Privacy Policy

## Overview

`sa` is designed to be privacy-first. This document explains exactly what
data `sa` collects, stores, and transmits.

## Data Collected

**sa collects no data.**

There is no telemetry, no analytics, no crash reporting, and no usage tracking.

## Data Stored Locally

`sa` stores only what is necessary to manage your profiles:

| Data | Location | Required |
|---|---|---|
| Profile name | `sa.json` | Yes |
| Email hint (if provided) | `sa.json` | No — you choose |
| Profile creation timestamp | `sa.json` | Yes |
| Profile last-used timestamp | `sa.json` | Yes |
| Current profile name | `sa.json` | Yes |

Configuration file location:

- macOS: `~/Library/Application Support/sa/sa.json`
- Linux: `$XDG_CONFIG_HOME/sa/sa.json` (default: `~/.config/sa/sa.json`)
- Windows: `%APPDATA%\sa\sa.json`

No data is stored outside this directory.

## What is NOT Stored

- Google passwords
- OAuth tokens (access, refresh, or ID tokens)
- API keys
- Browser cookies
- Browsing history
- Conversation history
- Files or code from your projects
- Any data from agy sessions

## Network Requests

**sa makes no network requests.**

The following commands operate entirely offline:

- `sa list`
- `sa current`
- `sa add`
- `sa remove`
- `sa rename`
- `sa doctor`
- `sa <profile>`

When `sa <profile>` launches agy, the resulting agy session may make network
requests to Google's servers. This is agy's normal operation and is not
controlled by or attributable to `sa`.

## Account Information

`sa` stores an optional email hint that you provide manually with:

```sh
sa add work --email work@example.com
```

This email is stored only in the local config file. It is:
- Never sent anywhere
- Not verified or used for authentication
- Not required
- Deletable by removing the profile

`sa` does not read your Google account email from agy or from OAuth tokens.

## Telemetry

There is no telemetry. If telemetry is ever added in a future version, it
will:
- Be opt-in by default
- Be fully documented
- Never collect credentials or sensitive data
- Have a clear opt-out mechanism

## Third-Party Dependencies

`sa` has **zero external dependencies**. It uses only the Go standard library.
There are no third-party libraries that could collect data.

## Data Deletion

To delete all sa data:

```sh
# macOS
rm -rf ~/Library/Application\ Support/sa/

# Linux
rm -rf ${XDG_CONFIG_HOME:-~/.config}/sa/

# Windows (PowerShell)
Remove-Item -Recurse "$env:APPDATA\sa"
```

This removes all profiles and configuration. agy's own data (authentication,
conversations) is stored separately and is not affected.

## Contact

If you have privacy questions or concerns, please open an issue on the GitHub
repository.
