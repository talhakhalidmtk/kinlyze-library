// Package git reads local Git history using only os/exec — zero external dependencies.
//
// KEY DESIGN: ONE git log call for the entire analysis window.
// Parsing happens in Go. ~500x faster than calling `git log -- <file>` per file.
package git

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// ── Skip lists ────────────────────────────────────────────────────────────────

var skipExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".ico": true, ".woff": true, ".woff2": true, ".ttf": true,
	".eot": true, ".otf": true, ".zip": true, ".tar": true, ".gz": true,
	".bz2": true, ".7z": true, ".exe": true, ".dll": true, ".so": true,
	".dylib": true, ".class": true, ".pyc": true, ".pyo": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".lock": true, ".sum": true, ".map": true, ".db": true, ".sqlite": true,
	".DS_Store": true,
}

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, "__pycache__": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true, "coverage": true,
	".venv": true, "venv": true, "env": true, ".cache": true, ".idea": true,
	".vscode": true, "bower_components": true, "jspm_packages": true,
}

var criticalPatterns = []string{
	"payment", "billing", "stripe", "checkout", "invoice", "subscription",
	"auth", "authn", "authz", "security", "password", "token",
	"oauth", "jwt", "session", "credential", "secret", "encrypt",
	"migration", "schema", "model", "database",
	"settings", "config",
	"core", "engine", "pipeline", "processor",
	"router", "middleware", "gateway",
	"deploy", "dockerfile",
}

const commitSep = "<<KINLYZE>>"

// ── Public types ──────────────────────────────────────────────────────────────

// RepoInfo holds basic repository metadata.
type RepoInfo struct {
	Name         string
	Branch       string
	TotalCommits int
	FirstCommit  time.Time
	LastCommit   time.Time
}

// Commit is a single commit record attached to a file.
type Commit struct {
	SHA   string
	Email string
	Name  string
	Date  time.Time
}

// Contributor is a developer with their all-time commit count.
type Contributor struct {
	Name    string
	Email   string
	Commits int
}

// ── Git runner ────────────────────────────────────────────────────────────────

func runGit(args []string, cwd string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", args[0], string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git not found: %w", err)
	}
	return string(out), nil
}

// ── Repo validation ───────────────────────────────────────────────────────────

// IsGitRepo returns true if path is inside a git repository.
func IsGitRepo(path string) bool {
	_, err := runGit([]string{"rev-parse", "--git-dir"}, path)
	return err == nil
}

// GetRepoRoot returns the absolute path to the repo root.
func GetRepoRoot(path string) (string, error) {
	out, err := runGit([]string{"rev-parse", "--show-toplevel"}, path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetRepoInfo returns basic repo metadata.
func GetRepoInfo(repoRoot string) (RepoInfo, error) {
	info := RepoInfo{}

	// Branch
	branch, err := runGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, repoRoot)
	if err != nil {
		info.Branch = "unknown"
	} else {
		info.Branch = strings.TrimSpace(branch)
	}

	// Total commits
	countOut, err := runGit([]string{"rev-list", "--count", "HEAD"}, repoRoot)
	if err != nil {
		return info, fmt.Errorf("repository has no commits")
	}
	fmt.Sscanf(strings.TrimSpace(countOut), "%d", &info.TotalCommits)
	if info.TotalCommits == 0 {
		return info, fmt.Errorf("repository has no commits")
	}

	// Name from root path
	info.Name = filepath.Base(repoRoot)

	// First and last commit dates
	if last, err := runGit([]string{"log", "--format=%ai", "--max-count=1"}, repoRoot); err == nil {
		info.LastCommit = parseDate(strings.TrimSpace(last))
	}
	if first, err := runGit([]string{"log", "--reverse", "--format=%ai", "--max-count=1"}, repoRoot); err == nil {
		info.FirstCommit = parseDate(strings.TrimSpace(first))
	}

	return info, nil
}

// ── File helpers ──────────────────────────────────────────────────────────────

// ShouldAnalyze returns true if this file path is worth analyzing.
func ShouldAnalyze(path string) bool {
	// Normalize separators
	normalized := filepath.ToSlash(path)
	parts := strings.Split(normalized, "/")

	// Check parent directories
	for _, part := range parts[:max(0, len(parts)-1)] {
		if skipDirs[strings.ToLower(part)] {
			return false
		}
	}

	// Check extension
	ext := strings.ToLower(filepath.Ext(path))
	if skipExtensions[ext] {
		return false
	}

	return len(parts[len(parts)-1]) > 0
}

// IsCriticalFile returns true if the file path matches a critical pattern.
func IsCriticalFile(path string) bool {
	lower := strings.ToLower(path)
	for _, p := range criticalPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// GetModulePath returns the parent directory of a file path.
// "src/payments/processor.go" → "src/payments"
// "main.go"                   → "root"
func GetModulePath(filePath string) string {
	normalized := filepath.ToSlash(filePath)
	idx := strings.LastIndex(normalized, "/")
	if idx < 0 {
		return "root"
	}
	return normalized[:idx]
}

// GetContributors returns all-time contributors sorted by commit count.
func GetContributors(repoRoot string) ([]Contributor, error) {
	out, err := runGit([]string{"shortlog", "-sne", "--all", "--no-merges"}, repoRoot)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`^\s*(\d+)\s+(.+?)\s+<(.+?)>\s*$`)
	var contributors []Contributor

	for _, line := range strings.Split(out, "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var count int
		fmt.Sscanf(m[1], "%d", &count)
		contributors = append(contributors, Contributor{
			Commits: count,
			Name:    strings.TrimSpace(m[2]),
			Email:   strings.ToLower(strings.TrimSpace(m[3])),
		})
	}

	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].Commits > contributors[j].Commits
	})
	return contributors, nil
}

// ── Bulk commit loader ────────────────────────────────────────────────────────

// LoadCommitsBulk loads ALL commits for the window in a single git log call.
// Returns map[filePath][]Commit.
func LoadCommitsBulk(repoRoot string, sinceDays int, progressFn func(string)) (map[string][]Commit, error) {
	if progressFn != nil {
		progressFn(fmt.Sprintf("Running git log (last %d days)...", sinceDays))
	}

	format := commitSep + "%n%H%n%ae%n%an%n%ai"
	args := []string{
		"log",
		fmt.Sprintf("--since=%d days ago", sinceDays),
		fmt.Sprintf("--format=%s", format),
		"--name-only",
		"--no-merges",
		"--diff-filter=AM",
	}

	out, err := runGit(args, repoRoot)
	if err != nil {
		return map[string][]Commit{}, nil // non-fatal — return empty
	}

	if progressFn != nil {
		progressFn("Parsing commit history...")
	}

	return parseBulkLog(out), nil
}

// LoadCommitsWindow loads commits in [untilDays, sinceDays] window for trend comparison.
func LoadCommitsWindow(repoRoot string, sinceDays, untilDays int) map[string][]Commit {
	format := commitSep + "%n%H%n%ae%n%an%n%ai"
	args := []string{
		"log",
		fmt.Sprintf("--since=%d days ago", sinceDays),
		fmt.Sprintf("--format=%s", format),
		"--name-only",
		"--no-merges",
		"--diff-filter=AM",
	}
	if untilDays > 0 {
		args = append(args[:2], append([]string{fmt.Sprintf("--until=%d days ago", untilDays)}, args[2:]...)...)
	}

	out, err := runGit(args, repoRoot)
	if err != nil {
		return map[string][]Commit{}
	}
	return parseBulkLog(out)
}

// parseBulkLog parses git log output into {filePath: []Commit}.
// State machine: seek → sha → email → name → date → files → repeat
func parseBulkLog(raw string) map[string][]Commit {
	result := make(map[string][]Commit)

	type commitState struct {
		sha   string
		email string
		name  string
		date  time.Time
	}

	var current *commitState
	state := "seek"

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		if line == commitSep {
			current = &commitState{}
			state = "sha"
			continue
		}

		if current == nil {
			continue
		}

		switch state {
		case "sha":
			if len(line) >= 8 {
				current.sha = line[:8]
			}
			state = "email"
		case "email":
			current.email = strings.ToLower(strings.TrimSpace(line))
			state = "name"
		case "name":
			current.name = strings.TrimSpace(line)
			state = "date"
		case "date":
			current.date = parseDate(line)
			state = "files"
		case "files":
			if line == "" {
				continue
			}
			if ShouldAnalyze(line) && current.email != "" {
				result[line] = append(result[line], Commit{
					SHA:   current.sha,
					Email: current.email,
					Name:  current.name,
					Date:  current.date,
				})
			}
		}
	}

	return result
}

// ── Date parsing ──────────────────────────────────────────────────────────────

var dateFormats = []string{
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05 -07:00",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05Z",
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	// Normalize "2024-01-15 14:30:00 +0530" timezone format
	re := regexp.MustCompile(`(\d{2}:\d{2}:\d{2}) ([+-]\d{2})(\d{2})$`)
	s = re.ReplaceAllString(s, "$1 $2:$3")

	for _, format := range dateFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// IsWindows returns true on Windows (used for terminal width detection).
func IsWindows() bool {
	return runtime.GOOS == "windows"
}
