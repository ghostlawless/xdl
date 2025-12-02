# xdl – X (Twitter) Media Downloader & Scraper (CLI)
Keywords: twitter media downloader, x scraper, twitter image downloader, twitter video downloader, cli, golang.

`xdl` is a simple, fast, **local** tool that downloads **all images and videos** from any public X (Twitter) profile.  
Everything runs directly on your machine.

---

## ⭐ Key Features

- Download **all media** (images + videos) from public profiles  
- Works with **the same endpoints used by the X web client**  
- Also works on private profiles **you follow**  
- 100% **local**  
- Cross-platform: Windows, Linux, macOS  
- Fast CLI workflow with minimal configuration  

---

## 🔍 Configuration

### Cookies (required)

Use the **Cookie-Editor** browser extension while logged into `https://x.com`.

1. Open Cookie-Editor  
2. Click **Export → Export as JSON**  
3. Save the result to:

```
config/cookies.json
```

---

## 🛠️ Installation

Requires **Go 1.21+**

```bash
#Clone **one** of the repositories:
git clone https://github.com/ghostlawless/xdl.git # For github (Primary)
git clone https://gitlab.com/medusax/xdl # For gitlab (Mirror)

#Enter the project directory:
cd xdl

#Build
go build -o xdl ./cmd/xdl       # Linux / macOS
go build -o xdl.exe ./cmd/xdl   # Windows
```

---

## 🚀 Usage

```bash
xdl -c cookies.json USERNAME
```

Example:

```bash
xdl -c cookies.json lawlessmedusax
```

---

## 📁 Output Structure

```
exports/
  USERNAME/
    images/
    videos/
logs/
debug/
debug_raw/
```

---

## 📘 Project Structure

```
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

## 🔐 Privacy

- No telemetry  
- No analytics  
- No external services  
- All scraping happens locally

---

## ⚖️ Legal

For educational and personal use.  
You are responsible for complying with X’s Terms of Service and local laws.

---

## 📜 License

AGPL-3.0  
Fork, study, modify, contribute.

---

### xdl — practical, searchable, local-first media downloader.
