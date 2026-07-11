# Kinlyze CLI Reference

Auto-generated reference of every `kinlyze` command and flag, and which
plan tier each one requires. Source of truth: `go/cmd/root.go`,
`go/cmd/agent.go`, `go/internal/upload/upload.go`, `go/internal/agent/`.

## Plan tiers

| Tier | What it means |
|---|---|
| **Free (local)** | No account needed. Runs entirely on the user's machine; nothing is sent anywhere. |
| **Free (account)** | Needs a Kinlyze Dashboard account and `kinlyze login --token <TOKEN>`, but works on the free plan — no Pro/Team required. |
| **Pro / Team** | Needs a Dashboard account **and** an active Pro/Team plan. The CLI attempts these regardless of plan and surfaces the Dashboard's `402 Payment Required` response as a clear error if the account isn't on Pro/Team. |

Everything is opt-in and additive: with no login at all, every scan command
runs fully locally and never phones home.

---

## Commands

| Command | Aliases | Description | Tier |
|---|---|---|---|
| `kinlyze` | — | Full scan of the current directory (same as `kinlyze scan`) | Free (local)¹ |
| `kinlyze scan` | — | Full scan — all sections | Free (local)¹ |
| `kinlyze insights` | — | Key insights and risk alerts only (executive summary) | Free (local)² |
| `kinlyze heatmap` | — | Knowledge heat map — every module ranked by risk | Free (local)² |
| `kinlyze busfactor` | `bf` | Bus factor deep dive, grouped by owner | Free (local)² |
| `kinlyze developers` | `devs` | Developer departure impact profiles | Free (local)² |
| `kinlyze flows` | — | User flow risk — end-to-end feature ownership | Free (local)² |
| `kinlyze version` | — | Print the CLI version | Free (local) |
| `kinlyze login` | — | Save a Dashboard token locally | Free (account) |
| `kinlyze logout` | — | Remove the saved Dashboard token | Free (local) |
| `kinlyze agent install` | — | Save a token, register the OS scheduler, run an initial scan | **Pro / Team** |
| `kinlyze agent run` | — | Run one scheduled scan pass (invoked by the OS scheduler, not for interactive use) | **Pro / Team** |
| `kinlyze agent uninstall` | — | Remove the scheduler entry registered by `agent install` | Free (local)³ |

¹ Scanning a **single** repo is free/local. Scanning **multiple** repos via `--repo a,b,c` or `--discover` requires a Dashboard account (see [Flags](#flags) below) — still free plan, not Pro/Team.
² These only ever scan one repo (`--repo <path>`, default `.`), so they're always free/local.
³ Uninstalling doesn't call the Dashboard at all — it only touches the local OS scheduler and local state, so it works even on an expired plan or while logged out.

### Automatic Dashboard sync (not a separate command)

Any scan command additionally **uploads its report to the Dashboard** if the
user is logged in (`isLoggedIn()` — a saved token). This is best-effort and
silent-fails to stderr; it never blocks or fails the local scan. The upload
itself is gated **Pro/Team**: a logged-in free-plan account gets a
`402` from `upload-report` and a `✖ Dashboard sync failed: 402 upgrade to
Pro/Team...` message on stderr, while the local report/exit code are
unaffected.

| Tier | Behavior |
|---|---|
| Free (local) — not logged in | Scan runs, no upload attempted. |
| Free (account) — logged in, no Pro/Team | Scan runs, upload attempted, Dashboard rejects with 402, warning printed to stderr, scan still succeeds. |
| **Pro / Team** — logged in | Scan runs, report uploaded and appears on the Dashboard. |

---

## Flags

### Shared by `kinlyze`, `scan`, `insights`, `heatmap`, `busfactor`/`bf`, `developers`/`devs`, `flows`

| Flag | Short | Default | Description | Tier |
|---|---|---|---|---|
| `--days` | `-d` | `365` | Days of history to analyze | Free (local) |
| `--top` | `-t` | `0` (all) | Show only top N riskiest modules | Free (local) |
| `--min-commits` | — | `2` | Minimum commits for a file to be included | Free (local) |
| `--no-color` | — | `false` | Disable colored output | Free (local) |
| `--json` | — | `false` | Output raw JSON (full result) | Free (local) |
| `--no-bots` | — | `true` | Exclude bot/CI commits | Free (local) |
| `--exclude-emails` | — | `nil` | Comma-separated emails to exclude | Free (local) |

### Repo selection

| Flag | Short | Applies to | Default | Description | Tier |
|---|---|---|---|---|---|
| `--repo` | `-r` | `insights`, `heatmap`, `busfactor`, `developers`, `flows` | `.` | Path to a single git repository | Free (local) |
| `--repo` | `-r` | `kinlyze` / `scan` | `nil` | Comma-separated path(s). **A single path is free/local; two or more paths require a Dashboard account.** | 1 path: Free (local) · 2+ paths: Free (account) |
| `--discover` | — | `kinlyze` / `scan` | `""` | Search this path's subdirectories for git repos and prompt to select which to scan | Free (account) |

If `--repo`/`--discover` are omitted and the current directory isn't itself a
git repo, `scan` looks for repos in immediate subdirectories and prompts
interactively — that auto-discovery path also requires a Dashboard account;
without one it falls through to the normal "not a git repository" error.

### `kinlyze login`

| Flag | Default | Description | Tier |
|---|---|---|---|
| `--token` | `""` (required) | Dashboard API token | Free (account) |

### `kinlyze agent install`

| Flag | Default | Description | Tier |
|---|---|---|---|
| `--token` | `""` (required) | Dashboard API token — validated live against the Dashboard before anything is installed (`401` → invalid/revoked token, `402` → no active Pro/Team plan) | **Pro / Team** |

`kinlyze agent run` and `kinlyze agent uninstall` take no flags.

---

## Agent scheduling details (`kinlyze agent install` / `run` / `uninstall`)

- Runs weekdays at **9am local time**.
- If the machine is off/asleep at that time, the run is skipped — but a
  catch-up trigger fires the next time the machine is available (once per
  weekday, tracked in `~/.kinlyze/agent-state.json`):
  - **macOS**: launchd `RunAtLoad` (runs at login/agent reload)
  - **Linux**: cron `@reboot`
  - **Windows**: Task Scheduler "At startup" trigger + `StartWhenAvailable`
- `agent install` also runs an initial scan immediately, rather than waiting
  for the first scheduled trigger.
- Per-run logs are written to `~/.kinlyze/agent.log`.
- The Dashboard token is shared with `kinlyze login`/`logout` — `agent
  uninstall` removes only the scheduler entry, not the saved token.
