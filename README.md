# xdl

[![Go Version](https://img.shields.io/badge/go-1.21%2B-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/status-alpha-orange.svg)](#project-status)

A precision media-extraction tool for X (Twitter).  
It pulls images and videos from a single profile and stores them in a clean, deterministic folder structure.  
No tracking, no telemetry, no external API — everything runs locally.

---

## Features

- Extracts all images and videos from any public X profile  
- Local-only execution (no backend, no cloud, no external servers)  
- Uses your own session cookies — **nothing is stored by xdl**  
- Deterministic output layout  
- Full debug mode with timestamped logs  
- Supports high-bitrate video variants (best-quality selection)  
- Robust retry logic with per-file timeouts  
- Safe downloads using temp files (Windows-friendly)

---

## Requirements

- Go 1.21+  
- Valid X/Twitter session cookies (exported via browser extension)

---

## Installation

```bash
git clone https://github.com/ghostlawless/xdl
cd xdl
go build -o xdl ./cmd/xdl
```

This produces a self-contained binary named `xdl`.

---

## Usage

### Basic usage

```bash
./xdl -c cookies.json <username>
```

Example:

```bash
./xdl -c cookies.json lawlessmedusax
```

### Debug mode

```bash
./xdl -d -c cookies.json lawlessmedusax
```

### Quiet mode

```bash
./xdl -q -c cookies.json lawlessmedusax
```

Flags:

- `-c cookies.json` — cookie file exported from your browser (for example: Cookie-Editor 1.13.0 on Google Chrome)
- `-d` — debug mode (logs to `logs/run_<id>/`)
- `-q` — quiet mode (minimal console output)

---

## Cookie file format

xdl does **not** store or generate cookies.  
The user provides them using a browser extension that exports cookies in the standard Chrome/Firefox JSON format, like:

```json
[
  {
    "domain": ".x.com",
    "name": "auth_token",
    "value": "..."
  },
  {
    "domain": ".x.com",
    "name": "ct0",
    "value": "..."
  },
  {
    "domain": ".x.com",
    "name": "guest_id",
    "value": "..."
  }
]
```

xdl automatically extracts the required values (`auth_token`, `ct0`, `guest_id`) and applies them **in memory only**.

The cookie file is never modified or saved back.

---

## Output layout

Downloaded media is stored under:

```text
xDownloads/
  xDownload - <username>@<run_id>/
    images/
    videos/
    gifs/
    others/
```

Each run creates a new isolated folder.

Filenames are sanitized to be filesystem-safe and are derived from the original media URL.

---

## essentials.json

The file `essentials.json` contains the static wiring needed for xdl to talk to X’s internal GraphQL endpoints:

- GraphQL operations used by xdl (`UserByScreenName`, `UserMedia`)
- The public web bearer token used by the X web client
- Default headers (auth type, active user, user-agent, language, etc.)
- Feature flags required by the current X web client
- Runtime parameters (timeouts, retries)
- Empty cookie slots (filled at runtime via `-c cookies.json`)

This file MUST NOT contain personal cookies or private tokens.  
Cookies are always injected at runtime via the `-c` flag.

---

## Safety & ethics

xdl is a local utility designed for lawful archiving and personal use.

- No telemetry  
- No analytics  
- No remote calls besides X itself  
- No token or cookie persistence by xdl

You are responsible for respecting:

- Copyright and neighboring rights  
- Platform terms of service  
- Local laws and regulations

---

## License

This project is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).  
See the `LICENSE` file for details.

---

## Contribution

Pull requests are welcome.

For non-trivial changes:

1. Open an issue describing the motivation and approach.
2. Keep the style minimal and focused on local, single-profile scraping.
3. Avoid adding third-party dependencies unless strictly necessary.

---

## Roadmap

Planned / potential improvements:

- Automatic GraphQL endpoint discovery  
- Automatic features introspection  
- Optional resume mode for large profiles  
- Optional per-profile caching layer  
- Interactive TUI mode  
- Basic API server mode for local-only automation

---

## Project status

Early but functional.

The core flow (lookup user → collect media → download assets) is stable enough for daily use, but internals may still change.
