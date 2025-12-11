# xdl – X (Twitter) Media Downloader & Scraper (CLI)  

`xdl` is a simple, fast, **local** tool that downloads **images and videos** from any public X (Twitter) profile. Everything runs directly on your machine.

---

## ⭐ Key Features

- Download **media** (images + videos) from public profiles  
- Works with **the same endpoints used by the X web client**  
- Also works on private profiles **you follow**  
- 100% **local**  
- Cross-platform: Windows, Linux, macOS  
- Fast CLI workflow with minimal configuration

---

## Configuration

### Cookies (required)

Use the **Cookie-Editor** browser extension while logged into `https://x.com`.

1. Open Cookie-Editor  
2. Click **Export → Export as JSON**  
3. Save the result to:

```text
config/cookies.json
```

---

## Installation

Requires **Go 1.21+**

```bash
# Clone one of the repositories:
git clone https://github.com/ghostlawless/xdl.git  # GitHub (primary)
# or
git clone https://gitlab.com/medusax/xdl           # GitLab (mirror)

# Enter the project directory:
cd xdl

# Build
go build ./cmd/xdl       # Linux / macOS
go build ./cmd/xdl   # Windows
```

---

## Usage

```bash or powershell (using .exe)
xdl USERNAME
```

Example:

```bash
xdl lawlessmedusax
xdl.exe google
```

---

## Output Structure

```text
debug
logs
  /run_id
*xDownloads*
  /username_run
    /images
    /videos
```

---

## Project Structure

```text
cmd/xdl          → CLI entrypoint  
config/          → essentials  
internal/  
  scraper/       → media discovery  
  downloader/    → file downloading  
  runtime/       → timing & behavior  
  httpx/         → HTTP helpers  
  app/           → orchestration  
  utils/         → small helpers  
LICENSE  
README.md  
```

---

## Privacy

- No telemetry  
- No analytics  
- No external services  
- All scraping happens locally

---

## ⚖️ Legal

For educational and personal use.  
You are responsible for complying with X’s Terms of Service and local laws.

---

## Media Limitations on X

`xdl` downloads **all media that your logged-in X session can see** in the **Media** tab of a profile.

X applies internal timeline limits — both in the UI and in the underlying GraphQL endpoints.  
This means that, for many profiles, **only a portion of the full historical media** is exposed through the official web client. After a certain depth, the backend simply **stops returning new pages**, even if the profile contains older posts.

**Note on HQ mode:** `xdl` now always runs in HQ (high quality) mode, prioritizing the best available media variants over raw speed. As a result, downloads may feel slower, since the tool performs extra checks and uses more cautious, human-like request pacing and batching to stay friendly to the underlying platform.

`xdl` mirrors this exact behavior:

- It fetches **every media item** delivered by X’s `UserMedia` timeline  
- When X stops supplying new pages, `xdl` reaches the **end of the visible media history**  
- No hidden or older content exists for the tool to retrieve via the normal web interface

This is **not** a bug in `xdl` — it’s a structural limitation of the X web client API.

If X’s UI does not load more media when you scroll to the bottom of the **Media** tab,  
`xdl` will not receive more media either.

---

## License

AGPL-3.0  
Fork, study, modify, contribute.

---

### xdl — practical, searchable, local-first media downloader
