# Kinlyze

**Analyze the kin behind your code.**

Map knowledge concentration risk in any Git repository. Find your bus factor before someone quits and takes it with them.

```
  KINLYZE  ·  Analyze the kin behind your code  ·  kinlyze.com

  SCAN SUMMARY
  ────────────────────────────────────────────────────────
  Repository      my-api          Files analyzed   267
  Branch          main            Modules found     64
  Total commits   1,842           Developers        12
  History window  365 days        Alerts             4

  🔴  3  critical    🟠  5  high    🟡  8  medium    🟢  48  healthy

  KNOWLEDGE HEAT MAP
  ────────────────────────────────────────────────────────
  MODULE                  BF  ↕   FILES  PRIMARY OWNER       OWNS
  🔴 src/payments          1  →    4     Sarah K          ████████ 94%   today
  🔴 src/auth              1  ↓    3     Sarah K          ███████░ 81%   3d ago
  🟠 src/api               2  →    12    Marcus R         █████░░░ 57%   today
  🟢 src/utils             4  ↑    8     Various          ██░░░░░░ 22%   today

  RISK ALERTS
  ────────────────────────────────────────────────────────
  ✖  CRITICAL   Bus factor 1 — Sarah K is a single point of failure
                2 modules with no backup: `src/payments`, `src/auth`.
                → Schedule knowledge transfer sessions.
```

---

## Installation

### macOS — Homebrew (recommended)

```bash
brew tap kinlyze/tap
brew install kinlyze
```

Upgrade:
```bash
brew upgrade kinlyze
```

---

### macOS / Linux — Install script (one line)

```bash
curl -sSL https://kinlyze.com/install.sh | sh
```

This downloads the correct binary for your OS and architecture, verifies the checksum, and installs to `/usr/local/bin`.

Custom install directory:
```bash
KINLYZE_INSTALL_DIR=~/.local/bin curl -sSL https://kinlyze.com/install.sh | sh
```

---

### Windows — Scoop

```powershell
scoop bucket add kinlyze https://github.com/kinlyze/scoop-bucket
scoop install kinlyze
```

Upgrade:
```powershell
scoop update kinlyze
```

---

### Windows — Direct download

1. Go to [github.com/talhakhalidmtk/kinlyze-library/releases/latest](https://github.com/talhakhalidmtk/kinlyze-library/releases/latest)
2. Download `kinlyze_VERSION_windows_amd64.zip`
3. Extract `kinlyze.exe`
4. Move to a directory on your `PATH` (e.g. `C:\Windows\System32\` or `C:\Users\YourName\bin\`)
5. Open a new terminal and run `kinlyze --help`

---

### Linux — Direct binary

```bash
# amd64 (most servers and desktops)
curl -sSLO https://github.com/talhakhalidmtk/kinlyze-library/releases/latest/download/kinlyze_VERSION_linux_amd64.tar.gz
tar -xzf kinlyze_VERSION_linux_amd64.tar.gz
sudo mv kinlyze /usr/local/bin/

# arm64 (Raspberry Pi, AWS Graviton, etc.)
curl -sSLO https://github.com/talhakhalidmtk/kinlyze-library/releases/latest/download/kinlyze_VERSION_linux_arm64.tar.gz
tar -xzf kinlyze_VERSION_linux_arm64.tar.gz
sudo mv kinlyze /usr/local/bin/
```

Replace `VERSION` with the latest version number from [releases](https://github.com/talhakhalidmtk/kinlyze-library/releases).

---

### Go — Build from source

Requires Go 1.21+.

```bash
go install github.com/talhakhalidmtk/kinlyze-library@latest
```

Or clone and build:

```bash
git clone https://github.com/talhakhalidmtk/kinlyze-library
cd kinlyze
make build
sudo make install
```

---

### Python — pip (alternative)

If you prefer Python, the same analysis is available as a pip package:

```bash
pip install kinlyze
kinlyze --repo /path/to/repo
```

---

## Requirements

| Requirement | Version |
|-------------|---------|
| Git         | 2.0+    |
| OS          | macOS, Linux, Windows |
| Arch        | amd64, arm64 |

No runtime required. The Go binary is self-contained.

---

## Usage

```bash
# Analyze current directory (must be inside a git repo)
kinlyze

# Analyze a specific repo
kinlyze --repo /path/to/your/repo

# Last 6 months only
kinlyze --days 180

# Last 2 years
kinlyze --days 730

# Show only top 20 riskiest modules
kinlyze --top 20

# Filter noise — ignore files touched fewer than 5 times
kinlyze --min-commits 5

# Plain text output (no color — great for CI or piping to a file)
kinlyze --no-color

# Save report to a file
kinlyze --no-color > report.txt

# Raw JSON output (for scripting, dashboards, CI)
kinlyze --json

# JSON + jq for scripting
kinlyze --json | jq '.summary'
kinlyze --json | jq '.modules[] | select(.risk_level == "critical")'
kinlyze --json | jq '.developers[0]'

# Print version
kinlyze version
```

---

## All flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--repo` | `-r` | `.` | Path to git repository |
| `--days` | `-d` | `365` | Days of history to analyze |
| `--top` | `-t` | `0` (all) | Show only top N riskiest modules |
| `--min-commits` | — | `2` | Minimum commits for a file to be included |
| `--no-color` | — | false | Disable ANSI color output |
| `--json` | — | false | Output raw JSON |

---

## What it shows

### Feature 1 — Knowledge Heat Map

Every module in your codebase ranked by knowledge concentration risk:

```
🔴 src/payments   BF:1  Sarah K     ████████  94%  today
🟠 src/auth       BF:2  Marcus R    ██████░░  71%  3d ago
🟢 src/utils      BF:4  Various     ██░░░░░░  22%  today
```

- **BF** = Bus Factor (1 = one person leaving breaks this module)
- **Trend** ↓ ↑ → = getting worse / better / stable vs 90 days ago
- **OWNS** = percentage of knowledge held by primary owner

### Feature 2 — Bus Factor Analysis

Detailed drilldown per risky module, grouped by primary owner:

```
🔴  Sarah K  ·  2 modules at risk  ·  1 sole owner
   ›  src/payments    BF 1  ████████████████████  94%
   ›  src/auth        BF 1  ████████████░░░░░░░░  81%  + Marcus R (19%)
```

### Feature 3 — Developer Profiles

Departure impact for every engineer:

```
DEVELOPER        KNOWLEDGE     IF THEY LEAVE  INACTIVE  MODULES
Sarah K          ████████ 31%  CRITICAL       0d        payments  auth
Marcus R         ███░░░░░ 14%  MEDIUM         3d        api
```

The **CRITICAL** tag means if Sarah leaves tomorrow, 31% of the total codebase knowledge leaves with her.

### Feature 4 — Risk Alerts

Grouped, actionable alerts — never one-per-module noise:

```
✖  CRITICAL   Bus factor 1 — Sarah K is a single point of failure
              2 modules with no backup: `src/payments`, `src/auth`.
              → Schedule knowledge transfer sessions.

▲  HIGH       High dependency — Sarah K holds 31% of knowledge
              Primary owner of 2 modules.
              → Create a knowledge transfer plan.
```

---

## How the scoring works

```
ownership_score = (commit_score  × 0.40)
               + (exclusivity   × 0.40)
               + (criticality   × 0.20)
```

| Signal | Weight | Meaning |
|--------|--------|---------|
| **Commit score** | 40% | Recency-weighted authorship. Last 30 days = 3×, 181–365 days = 0.5× |
| **Exclusivity** | 40% | What % of this file's commits does this developer own? |
| **Criticality** | 20% | Files in payments/auth/core paths score higher |

**Bus factor** = minimum developers whose removal causes ≥50% knowledge loss in a module.

**Trend** = comparison of bus factors between the most recent 90-day window and the prior 90-day window.

---

## Privacy and security

- **No source code read** — only Git metadata: author emails, dates, file paths, commit counts
- **No network requests** — fully offline, nothing sent anywhere
- **No diffs or content** — `--diff-filter=AM` only tells us which files were touched, not how
- **Local only** — runs entirely on your machine

---

## CI integration

Add to your CI pipeline to catch risk increases automatically:

```yaml
# .github/workflows/kinlyze.yml
name: Knowledge Risk Check

on:
  schedule:
    - cron: "0 9 * * 1"   # every Monday at 9am

jobs:
  kinlyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # full history needed

      - name: Install kinlyze
        run: curl -sSL https://kinlyze.com/install.sh | sh

      - name: Run analysis
        run: |
          kinlyze --json > kinlyze-report.json
          # Fail CI if critical bus factor found
          CRITICAL=$(cat kinlyze-report.json | jq '.summary.critical')
          echo "Critical modules: $CRITICAL"
          if [ "$CRITICAL" -gt 0 ]; then
            echo "⚠ Critical bus factor risk detected."
            kinlyze --no-color
          fi

      - name: Upload report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: kinlyze-report
          path: kinlyze-report.json
```

---

## JSON output schema

```json
{
  "repo_root":      "/path/to/repo",
  "repo_info": {
    "name":         "my-api",
    "branch":       "main",
    "total_commits": 1842
  },
  "since_days":     365,
  "files_analyzed": 267,
  "total_modules":  64,
  "summary": {
    "critical": 3,
    "high":     5,
    "medium":   8,
    "low":     48
  },
  "modules": [
    {
      "module":        "src/payments",
      "bus_factor":    1,
      "risk_level":    "critical",
      "primary_owner": "Sarah K",
      "primary_pct":   94.0,
      "trend":         "stable",
      "file_count":    4,
      "commit_count":  87,
      "contributors": [
        { "name": "Sarah K",  "email": "sarah@co.com", "pct": 94.0 },
        { "name": "Marcus R", "email": "marcus@co.com","pct":  6.0 }
      ]
    }
  ],
  "developers": [
    {
      "name":          "Sarah K",
      "knowledge_pct": 31.0,
      "risk":          "critical",
      "days_inactive": 0,
      "owned_modules": ["src/payments", "src/auth"],
      "sole_modules":  ["src/auth"]
    }
  ],
  "alerts": [
    {
      "severity": "critical",
      "type":     "bus_factor_1",
      "title":    "Bus factor 1 — Sarah K is a single point of failure",
      "detail":   "2 modules with no backup: `src/payments`, `src/auth`.",
      "action":   "Schedule knowledge transfer sessions."
    }
  ]
}
```

---

## Development

### Prerequisites

- Go 1.21+
- [GoReleaser](https://goreleaser.com) (for releases)
- [golangci-lint](https://golangci-lint.run) (for linting)

### Build and run

```bash
git clone https://github.com/talhakhalidmtk/kinlyze-library
cd kinlyze

# Build for current platform
make build

# Run against a repo
./kinlyze --repo /path/to/any/git/repo

# Run tests
make test

# Build for all platforms
make cross

# Install locally
make install
```

### Project structure

```
kinlyze/
├── main.go                      Entry point
├── cmd/
│   └── root.go                  Cobra CLI — all flags and validation
├── internal/
│   ├── git/
│   │   └── git.go               Git history reader (single bulk log call)
│   ├── scoring/
│   │   └── scoring.go           Ownership algorithm, bus factor, alerts
│   └── renderer/
│       └── renderer.go          ANSI terminal output
├── scripts/
│   └── install.sh               Universal install script
├── Formula/
│   └── kinlyze.rb               Homebrew formula
├── .goreleaser.yaml             Cross-platform release config
├── .github/workflows/
│   ├── ci.yml                   Tests on every push
│   └── release.yml              Build + publish on git tag
└── Makefile                     Developer shortcuts
```

### Releasing a new version

```bash
# 1. Update version in go.mod if needed
# 2. Tag the release
make release TAG=v0.2.0

# GitHub Actions will:
#   - Run tests on all platforms
#   - Build binaries for darwin/linux/windows × amd64/arm64
#   - Create a GitHub Release with all artifacts
#   - Update the Homebrew tap automatically
#   - Update the Scoop bucket automatically
```

---

## License

MIT © [Kinlyze](https://kinlyze.com)

---

## Links

- Website: [kinlyze.com](https://kinlyze.com)
- Wishlist: [kinlyze.com/#waitlist](https://kinlyze.com/#waitlist)
- Issues: [github.com/talhakhalidmtk/kinlyze-library/issues](https://github.com/talhakhalidmtk/kinlyze-library/issues)
- Python package: `pip install kinlyze`
