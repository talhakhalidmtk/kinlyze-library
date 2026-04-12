# Kinlyze

![GitHub Stars](https://img.shields.io/github/stars/talhakhalidmtk/kinlyze-library?style=flat)
![GitHub Forks](https://img.shields.io/github/forks/talhakhalidmtk/kinlyze-library?style=flat)
![Total Downloads](https://img.shields.io/github/downloads/talhakhalidmtk/kinlyze-library/total?style=flat)
![Latest Release](https://img.shields.io/github/v/release/talhakhalidmtk/kinlyze-library?style=flat)

**Analyze the kin behind your code.**

Map knowledge concentration risk in any Git repository. Find your bus factor before someone quits and takes it with them.

## See it in action

We ran Kinlyze on [FastAPI](https://github.com/fastapi/fastapi) — 97,000+ stars,
used in production at Netflix, Uber, and Microsoft. Here's what came back.

<details>
<summary><strong>View full analysis — FastAPI</strong></summary>

### Scan Summary
```
  KINLYZE  ·  Analyze the kin behind your code  ·  kinlyze.com

  SCAN SUMMARY
  ────────────────────────────────────────────────────────────────────────────
  Repository      fastapi              Files analyzed      1,358
  Branch          master               Modules found         196
  Total commits   7,015                User flows              6
  Contributors      535                Alerts                 14
  Repo maturity   Team project         Scoring  significance-weighted

  🔴  193 critical    🟠  3 high    🟡  0 medium    🟢  0 healthy

  ├  93 runtime-critical  ·  103 in low-impact paths (examples, templates, docs)
```

### Key Insights
```
  KEY INSIGHTS  Repository-wide patterns
  ────────────────────────────────────────────────────────────────────────────

  ⚠  Monoculture — 94% of the repository is owned by Sebastián Ramírez
     184 of 196 modules have a single contributor. This creates extreme
     maintainer dependency risk — if this person becomes unavailable, the
     majority of the codebase has zero knowledgeable backup.
     → This is not a per-module problem — it is a structural dependency.

  ℹ  51% of critical modules are in low-impact paths
     100 of 193 critical modules are in examples, templates, docs, or
     generated code. These inflate the risk count but do not represent
     production operational risk. 93 critical modules affect runtime code.
```

### User Flow Risk
```
  USER FLOW RISK  6 flows detected  ·  does one person own an entire user journey?
  ────────────────────────────────────────────────────────────────────────────

       USER FLOW          BF    PRIMARY OWNER        OWNS            COVERAGE
  ────────────────────────────────────────────────────────────────────────────
  🔴  Authentication      1     Sebastián Ramírez   ██████████ 100%  1 person
  🔴  Middleware          1     Sebastián Ramírez   ██████████ 100%  1 person
  🔴  Testing             1     Sebastián Ramírez   █████████░  90%  2 people
  🔴  Infrastructure      1     Sebastián Ramírez   █████████░  87%  1 person
  🔴  API Layer           1     Sebastián Ramírez   ████████░░  83%  1 person
  🔴  Data Persistence    1     Sebastián Ramírez   ████████░░  77%  2 people

  Coverage = developers with >5% of flow knowledge
  BF = Bus Factor — minimum developers whose departure causes ≥50% knowledge loss
```

</details>

> **This is not a criticism of FastAPI.** Sebastián Ramírez has built something
> extraordinary, and concentrated ownership is structurally expected in open source.
>
> The point is that **your internal codebase has the same pattern**.
> You just don't know which developer it is yet.

```bash
# Run it on your own repo
kinlyze --repo /path/to/your/repo
```

---

## What makes Kinlyze different

**Context-aware analysis.** Kinlyze doesn't just count commits — it understands what kind of repository it's looking at and adjusts its signals accordingly.

- **Repo maturity detection** — A solo project with 20 commits gets friendly guidance ("expected single-owner pattern"), not fire alarms. A team project with 2000 commits gets the full critical treatment.
- **Impact classification** — Modules in `examples/`, `templates/`, `docs/`, and `registry/` are flagged as low-impact. A BF=1 in `src/payments/` is a real risk; a BF=1 in `examples/demo/` is noise.
- **Significance-weighted scoring** — A 200-line feature scores 10× more than a 1-line typo fix. Recency × change size × exclusivity × criticality.
- **User flow analysis** — Groups modules into end-to-end capabilities (authentication, payments, API) and detects when one person owns an entire feature journey.
- **Insights over noise** — Instead of 310 red alerts, Kinlyze tells you: "98% of the repository is owned by shadcn. This is a structural dependency, not a per-module problem."

---

## Installation

### macOS / Linux — Install script

```bash
curl -sSL https://kinlyze.com/install.sh | sh
```

### Windows
Download `kinlyze_VERSION_windows_amd64.zip` from
[releases](https://github.com/talhakhalidmtk/kinlyze-library/releases/latest)

See [full installation guide](#full-installation-options) at the bottom of this README.

---

## Quick start

```bash
# Full scan of current directory
kinlyze

# Scan a specific repo
kinlyze --repo /path/to/your/repo

# Just the executive summary (insights + alerts)
kinlyze insights

# Just the heat map
kinlyze heatmap --top 20

# JSON for CI pipelines
kinlyze --json | jq '.maturity'
```

---

## Commands

Kinlyze provides focused subcommands so you can get exactly the view you need.

| Command | What it shows |
|---------|---------------|
| `kinlyze` | Full scan — all sections |
| `kinlyze scan` | Same as above (explicit) |
| `kinlyze insights` | Key insights + risk alerts — the executive summary |
| `kinlyze heatmap` | Knowledge heat map — every module ranked by risk |
| `kinlyze busfactor` | Bus factor deep dive — at-risk modules grouped by owner |
| `kinlyze developers` | Developer profiles — departure impact per engineer |
| `kinlyze flows` | User flow risk — end-to-end feature ownership |
| `kinlyze version` | Print version |

All commands accept the same flags (`--repo`, `--days`, `--json`, etc.).

### Examples

```bash
# Executive summary for a specific repo
kinlyze insights --repo /path/to/repo

# Top 10 riskiest modules, last 6 months
kinlyze heatmap --top 10 --days 180

# Bus factor analysis aliased as 'bf'
kinlyze bf

# Developer profiles aliased as 'devs'
kinlyze devs --days 90

# User flow risk
kinlyze flows

# JSON output (works with any command)
kinlyze insights --json | jq '.insights'
kinlyze --json | jq '.maturity.stage'
kinlyze --json | jq '.summary.runtime_critical'

# CI pipeline — fail if runtime-critical modules exist
CRITICAL=$(kinlyze --json | jq '.summary.runtime_critical')
if [ "$CRITICAL" -gt 0 ]; then echo "⚠ Runtime risk detected"; fi
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
| `--no-bots` | — | true | Exclude bot/CI commits (dependabot, renovate, etc.) |
| `--exclude-emails` | — | — | Comma-separated email addresses to exclude |

---

## What it shows

### Key Insights — repository-wide patterns

The highest-signal view. Instead of listing every module, insights describe structural patterns:

```
  ℹ  Solo project detected — Early-stage solo project (8 commits, 1 contributor)
     All risk signals below are structurally expected for a single-developer repository.
     Bus factor = 1 everywhere is normal at this stage.
     → As the project scales, consider adding redundancy to the highest-traffic modules.
```

```
  ⚠  Monoculture — 98% of the repository is owned by shadcn
     310 of 312 modules have a single contributor. This creates extreme
     maintainer dependency risk.
     → This is not a per-module problem — it is a structural dependency.
```

```
  ℹ  97% of critical modules are in low-impact paths
     305 of 310 critical modules are in examples, templates, docs. Only 5 affect runtime code.
```

### Knowledge Heat Map

Every module ranked by knowledge concentration risk. Low-impact modules (examples, templates, docs) are dimmed to reduce noise:

```
  🔴 src/payments          BF:1  Sarah K     ████████  94%   today
  🟠 src/api               BF:2  Marcus R    ██████░░  57%   today
  ○  examples/demo         BF:1  Sarah K     ██████░░  100%  30d ago    ← dimmed
  🟢 src/utils             BF:4  Various     ██░░░░░░  22%   today
```

### Bus Factor Analysis

At-risk modules grouped by primary owner:

```
  🔴  Sarah K  ·  3 modules at risk  ·  2 sole owner
     ›  src/payments    BF 1  ████████████████████  94%
     ›  src/auth        BF 1  ████████████░░░░░░░░  81%  + Marcus R (19%)
```

### Developer Profiles

Departure impact for every engineer:

```
  DEVELOPER        KNOWLEDGE     IF THEY LEAVE  MODULES
  Sarah K          ████████ 31%  CRITICAL       payments  auth
  Marcus R         ███░░░░░ 14%  MEDIUM         api
```

### User Flow Risk

Does one person own an entire user journey?

```
  🔴  Authentication   BF 1   Sarah K   91%   1 person   auth · session · token
  🟠  API Layer        BF 2   Marcus R  57%   3 people   api · routes · handlers
```

### Risk Alerts

Grouped, actionable alerts with maturity-aware severity:

**Team repos** get urgent language:
```
  ✖  CRITICAL   Bus factor 1 — Sarah K is a single point of failure
```

**Solo repos** get planning guidance:
```
  ●  MEDIUM     Single-owner pattern — expected for a solo project
                As the project scales, prioritize adding a second contributor.
```

---

## Repo maturity detection

Kinlyze classifies repositories to prevent over-alerting:

| Stage | Criteria | Effect |
|-------|----------|--------|
| **Early-stage solo** | 1 contributor, <100 commits | BF=1 alerts → MEDIUM. "Expected single-owner pattern." |
| **Solo project** | 1 contributor, 100+ commits | BF=1 alerts → MEDIUM. Monoculture insight suppressed. |
| **Small team** | 2–3 contributors | Full alerts. Reduced noise. |
| **Team project** | 4+ contributors | Full critical alerts. All signals active. |

The maturity classification is available in JSON output:

```bash
kinlyze --json | jq '.maturity'
# { "stage": "early", "label": "Early-stage solo project", "contributors": 1, "total_commits": 8, "solo": true }
```

---

## Impact classification

Modules are classified as **runtime** (production code) or **low-impact** (examples, templates, docs, generated code):

| Classification | Paths | Risk treatment |
|---------------|-------|----------------|
| **Runtime** | Everything not in low-impact dirs | Full critical/high/medium/low |
| **Low-impact** | `examples/`, `templates/`, `docs/`, `registry/`, `blocks/`, `demos/`, `stories/`, `generated/`, `scripts/`, and 25+ more | Dimmed in heat map, alerts downgraded, separated in summary |

In JSON output:

```bash
kinlyze --json | jq '.summary'
# { "critical": 310, "runtime_critical": 5, "low_impact_count": 305, ... }

kinlyze --json | jq '.modules[] | select(.impact == "runtime" and .risk_level == "critical")'
# Only the 5 modules that actually matter
```

---

## How the scoring works

```
ownership_score = (commit_score  × 0.40)
               + (exclusivity   × 0.40)
               + (criticality   × 0.20)
```

### Significance weighting (Leniency Model)

Not all commits are equal. A 200-line feature represents far more knowledge than a typo fix:

| Lines Changed | Multiplier | What it represents |
|---------------|-----------|-------------------|
| 1–5 | 0.1× | Typo fix, comment, whitespace |
| 6–20 | 0.4× | Small bug fix, config tweak |
| 21–100 | 0.75× | Feature addition, refactor |
| 101+ | 1.0× | Major feature, new module |

### Scoring signals

| Signal | Weight | What it captures |
|--------|--------|-----------------|
| **Commit Score** | 40% | Recency-weighted authorship, scaled by change significance |
| **Exclusivity** | 40% | What fraction of the file's weighted activity this developer owns |
| **Criticality** | 20% | Files in payments/auth/core paths score higher |

**Bus factor** = minimum developers whose removal causes ≥50% knowledge loss in a module.

**Trend** = comparison of bus factors between the most recent 90-day window and the prior 90-day window.

---

## Privacy and security

Kinlyze reads only what Git already knows — it never sees your source code.

**What Kinlyze reads:** author name, author email, commit date, file path, lines added, lines deleted. This is the same information shown by `git log --numstat`.

**What Kinlyze never reads:** file contents, diffs, commit messages, branch names beyond the current branch, remotes, tags, or any data outside the repository directory.

**No network requests.** The CLI runs entirely offline. Nothing is sent anywhere. No telemetry, no analytics, no license checks. Your code stays on your machine.

---

## Architecture

The CLI is open source and runs entirely on your local machine — it requires no account, no server, and no internet connection. The dashboard (kinlyze.com) is a separate commercial product that builds on the same analysis engine and adds team collaboration, trend history, and scheduled reports. The CLI and the dashboard are independent — you can use one without the other.

---

## CI integration

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
          fetch-depth: 0

      - name: Install kinlyze
        run: curl -sSL https://kinlyze.com/install.sh | sh

      - name: Run analysis
        run: |
          kinlyze --json > kinlyze-report.json

          # Check RUNTIME critical — not total critical (avoids template noise)
          CRITICAL=$(jq '.summary.runtime_critical' kinlyze-report.json)
          MATURITY=$(jq -r '.maturity.stage' kinlyze-report.json)

          echo "Maturity: $MATURITY"
          echo "Runtime-critical modules: $CRITICAL"

          if [ "$MATURITY" != "early" ] && [ "$CRITICAL" -gt 0 ]; then
            echo "⚠ Runtime risk detected."
            kinlyze insights --no-color
            exit 1
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
  "repo_root": "/path/to/repo",
  "repo_info": {
    "name": "my-api",
    "branch": "main",
    "total_commits": 1842
  },
  "maturity": {
    "stage": "mature",
    "label": "Team project",
    "contributors": 12,
    "total_commits": 1842,
    "solo": false
  },
  "since_days": 365,
  "files_analyzed": 267,
  "total_modules": 64,
  "summary": {
    "critical": 15,
    "high": 5,
    "medium": 8,
    "low": 36,
    "runtime_critical": 3,
    "runtime_high": 4,
    "low_impact_count": 12
  },
  "insights": [
    {
      "level": "critical",
      "title": "Monoculture — 78% of the repository is owned by Sarah K",
      "detail": "42 of 64 modules have a single contributor.",
      "action": "Prioritize bringing a second contributor into the highest-traffic modules."
    }
  ],
  "modules": [
    {
      "module": "src/payments",
      "bus_factor": 1,
      "risk_level": "critical",
      "impact": "runtime",
      "primary_owner": "Sarah K",
      "primary_pct": 94.0,
      "trend": "stable",
      "contributors": [
        { "name": "Sarah K", "email": "sarah@co.com", "pct": 94.0 }
      ]
    }
  ],
  "flows": [
    {
      "name": "Authentication",
      "bus_factor": 1,
      "risk_level": "critical",
      "primary_owner": "Sarah K",
      "primary_pct": 91.0,
      "coverage": 1,
      "modules": ["src/auth", "src/session", "src/token"]
    }
  ],
  "developers": [
    {
      "name": "Sarah K",
      "knowledge_pct": 31.0,
      "risk": "critical",
      "days_inactive": 0,
      "owned_modules": ["src/payments", "src/auth"],
      "sole_modules": ["src/auth"],
      "owned_flows": ["Authentication", "Payments"]
    }
  ],
  "alerts": [
    {
      "severity": "critical",
      "type": "bus_factor_1",
      "title": "Bus factor 1 — Sarah K is a single point of failure",
      "detail": "3 runtime module(s) with no backup.",
      "action": "Schedule knowledge transfer sessions."
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
make build
./kinlyze --repo /path/to/any/git/repo
```

### Project structure

```
kinlyze/
├── main.go                      Entry point
├── cmd/
│   └── root.go                  Cobra CLI — subcommands and flags
├── internal/
│   ├── git/
│   │   └── git.go               Git history reader (numstat, bot detection, impact paths)
│   ├── scoring/
│   │   └── scoring.go           Scoring, maturity, flows, insights, alerts
│   └── renderer/
│       └── renderer.go          ANSI terminal output
├── scripts/
│   └── install.sh               Universal install script
├── .goreleaser.yaml             Cross-platform release config
├── .github/workflows/
│   ├── ci.yml                   Tests on every push
│   └── release.yml              Build + publish on git tag
└── Makefile                     Developer shortcuts
```

---

## Full installation options

### macOS / Linux — Install script

```bash
curl -sSL https://kinlyze.com/install.sh | sh
KINLYZE_INSTALL_DIR=~/.local/bin curl -sSL https://kinlyze.com/install.sh | sh  # custom dir
```

### Windows — Direct download

Download from [releases](https://github.com/talhakhalidmtk/kinlyze-library/releases/latest), extract `kinlyze.exe`, add to PATH.

### Requirements

| Requirement | Version |
|-------------|---------|
| Git         | 2.0+    |
| OS          | macOS, Linux, Windows |
| Arch        | amd64, arm64 |

No runtime required. The Go binary is self-contained.

---

## License

MIT © [Kinlyze](https://kinlyze.com)

---

## Links

- Website: [kinlyze.com](https://kinlyze.com)
- Waitlist: [kinlyze.com/#waitlist](https://kinlyze.com/#waitlist)
- Issues: [github.com/talhakhalidmtk/kinlyze-library/issues](https://github.com/talhakhalidmtk/kinlyze-library/issues)
