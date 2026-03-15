# AI Job Auto Apply Bot

A fully local, zero-cost CLI tool that automatically discovers jobs on [Naukri.com](https://www.naukri.com), evaluates them with a local LLM (Ollama/Llama3), and applies on your behalf using Playwright browser automation.

---

## Project Structure

```
job-auto-apply/
├── cmd/main.go                    # CLI entrypoint
├── internal/
│   ├── config/loader.go           # Profile YAML loader + validation
│   ├── db/sqlite.go               # SQLite persistence
│   ├── scraper/naukri_scraper.go  # Playwright-based job discovery
│   ├── ai/matcher.go              # Ollama LLM matcher
│   ├── form/form_filler.go        # Generic HTML form filler
│   └── apply/apply_worker.go      # End-to-end application pipeline
├── configs/profile.yaml           # Your candidate profile
├── resume/                        # Drop your resume.pdf here
├── docker/Dockerfile              # Multi-stage build
├── docker-compose.yml             # Bot + Ollama sidecar
├── .env.example                   # Credential template
└── data/                          # SQLite DB (auto-created)
```

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | ≥ 1.22 | Build the binary |
| GCC / MinGW | any | CGO required for go-sqlite3 |
| Docker + Compose | any | Containerised run |
| Ollama | latest | Local LLM runtime |

To install Ollama: https://ollama.com/download

---

## Quick Start (Local)

### 1. Configure credentials
```powershell
Copy-Item .env.example .env
# Edit .env — fill NAUKRI_EMAIL and NAUKRI_PASSWORD
notepad .env
```

### 2. Edit your profile
```powershell
notepad configs\profile.yaml
# Set your name, skills, roles, years_of_experience, etc.
```

### 3. Add your resume
```
resume\resume.pdf   ← place your resume here
```

### 4. Pull Ollama model
```powershell
ollama pull llama3
```

### 5. Install Go dependencies
```powershell
go mod tidy
```

### 6. Run (dry-run first!)
```powershell
# Dry run — scrapes and scores jobs but does NOT submit
go run cmd/main.go --dry-run --limit 5 --verbose

# Live run — actually applies
go run cmd/main.go --limit 20
```

---

## Quick Start (Docker)

```powershell
# Copy and fill credentials
Copy-Item .env.example .env

# Build and start (Ollama auto-downloads llama3 on first run ~4 GB)
docker compose up --build

# Dry run via Docker
docker compose run --rm bot --dry-run --limit 5
```

---

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `configs/profile.yaml` | Path to profile file |
| `--dry-run` | `false` | Fill forms, skip submit |
| `--limit N` | `0` (unlimited) | Max jobs to process |
| `--threshold N` | profile value | Override match score threshold |
| `--headless` | `false` | Hide browser window |
| `--verbose` | `false` | Debug-level logs |

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NAUKRI_EMAIL` | ✅ | — | Your Naukri login email |
| `NAUKRI_PASSWORD` | ✅ | — | Your Naukri password |
| `OLLAMA_BASE_URL` | ❌ | `http://localhost:11434` | Ollama API endpoint |
| `OLLAMA_MODEL` | ❌ | `llama3` | Model tag to use |
| `DB_PATH` | ❌ | `data/jobs.db` | SQLite database path |

---

## Database

Jobs are stored in `data/jobs.db` (SQLite). Each job URL is unique — the bot never applies to the same job twice.

| Column | Type | Description |
|--------|------|-------------|
| `title` | TEXT | Job title |
| `company` | TEXT | Company name |
| `url` | TEXT UNIQUE | Job page URL |
| `description` | TEXT | Full JD text |
| `match_score` | REAL | AI score (0–100) |
| `status` | TEXT | `pending` / `applied` / `skipped` / `failed` |
| `applied_at` | DATETIME | Timestamp when applied |

Query with any SQLite viewer or:
```powershell
sqlite3 data/jobs.db "SELECT title, company, match_score, status FROM jobs ORDER BY applied_at DESC LIMIT 20;"
```

---

## Running Unit Tests

```powershell
go test ./internal/ai/... ./internal/form/...
```

No Ollama or browser required — tests cover the AI prompt builder and form label mapper.

---

## How It Works

```
Load profile.yaml
    ↓
Login to Naukri (Playwright / Chromium)
    ↓
Paginate search results for each expected role
    ↓
For each job URL:
    - Dedup check (SQLite)
    - Extract job description
    - Score with local LLM (Ollama)
    ↓
Score ≥ threshold?  →  Fill form + Upload resume + Submit
Score <  threshold? →  Mark skipped
    ↓
Log run summary
```

---

## Adding Support for Other Job Platforms

The scraper interface is intentionally simple. To add LinkedIn or Indeed:

1. Create `internal/scraper/linkedin_scraper.go`
2. Implement `Login`, `SearchJobs`, `ExtractJobDescription`
3. Wire it into `cmd/main.go` via a `--platform` flag

The `FormFiller` and `ApplyWorker` are platform-agnostic and reuse as-is.

---

## Security

- **Credentials** are **never** stored in code or config YAML — only in `.env`
- `.env` is in `.gitignore` — never commit it
- The SQLite database contains scraped JDs — treat it as private data

---

## License

MIT — open source, free to use and modify.
