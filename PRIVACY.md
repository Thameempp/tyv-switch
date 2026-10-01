# Privacy Policy

## Overview

`tyv` is designed to be privacy-first. This document explains exactly what
data `tyv` collects, stores, and transmits.

## Data Collected

**tyv collects no data.**

There is no telemetry, no analytics, no crash reporting, and no usage tracking.

## Data Stored Locally

`tyv` stores only what is necessary to manage your profiles:

| Data | Location | Required |
|---|---|---|
| Profile name | `tyv.json` | Yes |
| Email hint (if provided) | `tyv.json` | No — you choose |
| Profile creation timestamp | `tyv.json` | Yes |
| Profile last-used timestamp | `tyv.json` | Yes |
| Current profile name | `tyv.json` | Yes |
| Cached usage percentages (Gemini / Claude) and fetch time | `usage-cache.json` | Yes (last successful reading, used only as a fallback when a fresh lookup fails) |

Configuration file location:

- macOS: `~/Library/Application Support/tyv/tyv.json`
- Linux: `$XDG_CONFIG_HOME/tyv/tyv.json` (default: `~/.config/tyv/tyv.json`)
- Windows: `%APPDATA%\tyv\tyv.json`

No tyv data is stored outside this directory.

Each profile also has an isolated home directory,
`<config dir>/profiles/<name>/home/`. **agy** (not tyv) writes that profile's
Google login, conversations and settings into its private `.gemini` folder
there. tyv never reads those files, with two narrow exceptions used to display
which Google account a profile is signed into: the `active` (email) field of
agy's `google_accounts.json`, and the single "authenticated successfully as
<email>" line in agy's logs. Neither holds tokens. The email is shown on your
own terminal and stored in `tyv.json` as the profile's email. The rest of that directory is symlinks to
your real home directory.

## What is NOT Stored

- Google passwords
- OAuth tokens (access, refresh, or ID tokens) — tyv does not store or read
  them; agy keeps them in the profile's private home (see above)
- API keys
- Browser cookies
- Browsing history
- Conversation history
- Files or code from your projects
- Any data from agy sessions

## Network Requests

**tyv makes no network requests itself.**

The following commands operate entirely offline:

- `tyv list`
- `tyv current`
- `tyv remove`
- `tyv rename` / `tyv edit`
- `tyv doctor`

`tyv add` (when you agree to sign in) and `tyv <profile>` launch agy, which
talks to Google for sign-in and normal operation.

Running plain `tyv` (the account selector) executes
`agy --print /usage --output-format json` for each profile to show remaining
quota. agy contacts Google to answer; tyv only receives the resulting
percentages and caches them locally (`usage-cache.json`, no credentials).

When `tyv <profile>` launches agy, the resulting agy session may make network
requests to Google's servers. This is agy's normal operation and is not
controlled by or attributable to `tyv`.

## Account Information

`tyv` stores an optional email hint that you provide manually with:

```sh
tyv add work --email work@example.com
```

This email is stored only in the local config file. It is:
- Never sent anywhere
- Not verified or used for authentication
- Not required
- Deletable by removing the profile

`tyv` does not read your Google account email from agy or from OAuth tokens.

## Telemetry

There is no telemetry. If telemetry is ever added in a future version, it
will:
- Be opt-in by default
- Be fully documented
- Never collect credentials or sensitive data
- Have a clear opt-out mechanism

## Third-Party Dependencies

`tyv` has **zero external dependencies**. It uses only the Go standard library.
There are no third-party libraries that could collect data.

## Data Deletion

To delete all tyv data:

```sh
# macOS
rm -rf ~/Library/Application\ Support/tyv/

# Linux
rm -rf ${XDG_CONFIG_HOME:-~/.config}/tyv/

# Windows (PowerShell)
Remove-Item -Recurse "$env:APPDATA\tyv"
```

This removes all profiles and configuration, **including each profile's agy
login and conversations**, which live inside the profile directories. Your
real `~/.gemini` (the login agy uses outside tyv) is not affected. To remove a
single profile and its login, use `tyv remove <name>`.

## Contact

If you have privacy questions or concerns, please open an issue on the GitHub
repository.
