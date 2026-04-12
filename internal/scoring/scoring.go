// Package scoring implements the Kinlyze ownership scoring algorithm.
//
// Composite ownership score per developer per file:
//
//	score = (commit_score × 0.40) + (exclusivity × 0.40) + (criticality × 0.20)
//
// Each commit is weighted by both recency AND change significance.
// A 2-line typo fix contributes 10% of what a 200-line feature does.
//
// Bus factor = minimum developers whose removal causes ≥50% knowledge loss.
//
// User flows group related modules into end-to-end capabilities (auth, payments, etc.)
// and compute flow-level bus factors to detect full-feature SPOFs.
package scoring

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/talhakhalidmtk/kinlyze-library/internal/git"
)

// ── Weights ───────────────────────────────────────────────────────────────────

const (
	weightCommit      = 0.40
	weightExclusivity = 0.40
	weightCriticality = 0.20
)

type recencyBand struct {
	days   int
	weight float64
}

var recencyBands = []recencyBand{
	{30, 3.0},
	{90, 2.0},
	{180, 1.0},
	{365, 0.5},
}

// ── Significance tiers (Leniency Model) ───────────────────────────────────────

// significance returns a multiplier [0.1, 1.0] based on how many lines a commit
// changed. Trivial changes (typos, whitespace) barely register; only meaningful
// modifications build real ownership.
//
//	1–5 lines   → 0.1×  (trivial: typo fix, comment, whitespace)
//	6–20 lines  → 0.4×  (minor: small bug fix, config tweak)
//	21–100      → 0.75× (moderate: feature addition, refactor)
//	101+        → 1.0×  (significant: major feature, new module)
func significance(linesChanged int) float64 {
	switch {
	case linesChanged <= 0:
		return 0.1 // binary file or empty — treat as trivial
	case linesChanged <= 5:
		return 0.1
	case linesChanged <= 20:
		return 0.4
	case linesChanged <= 100:
		return 0.75
	default:
		return 1.0
	}
}

// ── User Flow Definitions ─────────────────────────────────────────────────────

// FlowDefinition maps a flow name to the keywords that identify belonging modules.
type FlowDefinition struct {
	Name     string
	Keywords []string
}

// generalFlowDefinitions applies to all repositories.
var generalFlowDefinitions = []FlowDefinition{
	{
		Name:     "Authentication",
		Keywords: []string{"auth", "authn", "authz", "session", "login", "signup", "oauth", "jwt", "credential", "sso", "saml"},
		// Note: "token" removed — too generic, matches unrelated paths
	},
	{
		Name:     "Payments",
		Keywords: []string{"payment", "billing", "stripe", "checkout", "subscription", "invoice", "charge", "refund", "pricing"},
	},
	{
		Name:     "Data Persistence",
		Keywords: []string{"model", "database", "migration", "schema", "repository", "entity", "dao"},
		// Note: "db" and "orm" removed — too short, cause false positives
	},
	{
		Name:     "API Layer",
		Keywords: []string{"api", "router", "handler", "controller", "endpoint", "graphql", "grpc", "rest"},
		// Note: "route" removed — matches too many non-API paths
	},
	{
		Name:     "Infrastructure",
		Keywords: []string{"config", "setting", "deploy", "dockerfile", "docker", "kubernetes", "terraform", "ci", "cd"},
		// Note: "k8s", "infra" removed — too short/broad
	},
	{
		Name:     "Core Engine",
		Keywords: []string{"core", "engine", "processor", "pipeline", "queue", "scheduler", "cron"},
		// Note: "worker" removed — too generic
	},
	{
		Name:     "Middleware",
		Keywords: []string{"middleware", "interceptor", "gateway", "proxy", "rate-limit"},
		// Note: "filter" removed — too generic
	},
	{
		Name:     "Testing",
		Keywords: []string{"test", "spec", "__test", "fixture", "mock", "stub", "e2e", "integration"},
	},
}

// mulesoftFlowDefinitions applies only when a MuleSoft repo is detected.
// These replace generalFlowDefinitions entirely — MuleSoft has its own
// architectural patterns that don't map to general web app flows.
var mulesoftFlowDefinitions = []FlowDefinition{
	{
		Name:     "Experience Layer",
		Keywords: []string{"-exp-api", "exp-api", "/exp/", "experience-api"},
	},
	{
		Name:     "Process Layer",
		Keywords: []string{"-prc-api", "prc-api", "/prc/", "process-api"},
	},
	{
		Name:     "System Layer",
		Keywords: []string{"-sys-api", "sys-api", "/sys/", "system-api", "salesforce", "sap", "workday", "servicenow", "oracle"},
	},
	{
		Name:     "API Interface",
		Keywords: []string{"api-main", "interface", "api-router", "src/main/api", "raml", "openapi"},
	},
	{
		Name:     "Authentication & Security",
		Keywords: []string{"oauth2", "client-id", "client-credentials", "security", "jwt", "saml", "token"},
	},
	{
		Name:     "DataWeave Transforms",
		Keywords: []string{"src/main/resources/dwl", "dataweave", ".dwl", "transform", "mapping", "canonical"},
	},
	{
		Name:     "Error Handling",
		Keywords: []string{"error-handler", "error-handling", "on-error", "exception", "fault", "global-error"},
	},
	{
		Name:     "Schedulers & Batch",
		Keywords: []string{"scheduler", "batch", "polling", "watermark", "cron"},
	},
	{
		Name:     "Messaging & Events",
		Keywords: []string{"anypoint-mq", "amq", "jms", "kafka", "queue", "topic", "pubsub"},
	},
	{
		Name:     "Global Configuration",
		Keywords: []string{"global-config", "global.xml", "mule-artifact", "cloudhub", "properties", "src/main/resources"},
	},
	{
		Name:     "MUnit Testing",
		Keywords: []string{"munit", "src/test/munit"},
	},
}

// ── Public result types ───────────────────────────────────────────────────────

// Contributor is a developer's share of a module.
type Contributor struct {
	Name  string  `json:"name"`
	Email string  `json:"email"`
	Pct   float64 `json:"pct"`
}

// Module is the bus factor analysis result for one module path.
type Module struct {
	Module       string        `json:"module"`
	BusFactor    int           `json:"bus_factor"`
	RiskLevel    string        `json:"risk_level"`
	Impact       string        `json:"impact"` // "runtime" or "low-impact"
	PrimaryOwner string        `json:"primary_owner"`
	PrimaryEmail string        `json:"primary_email"`
	PrimaryPct   float64       `json:"primary_pct"`
	Contributors []Contributor `json:"contributors"`
	FileCount    int           `json:"file_count"`
	CommitCount  int           `json:"commit_count"`
	Trend        string        `json:"trend"`
	LastActive   time.Time     `json:"last_active"`
}

// Flow is the bus factor analysis result for a user flow (end-to-end capability).
type Flow struct {
	Name         string        `json:"name"`
	BusFactor    int           `json:"bus_factor"`
	RiskLevel    string        `json:"risk_level"`
	PrimaryOwner string        `json:"primary_owner"`
	PrimaryPct   float64       `json:"primary_pct"`
	Coverage     int           `json:"coverage"`
	Modules      []string      `json:"modules"`
	Contributors []Contributor `json:"contributors"`
}

// Developer is a developer's org-wide knowledge profile.
type Developer struct {
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	KnowledgePct float64   `json:"knowledge_pct"`
	Risk         string    `json:"risk"`
	OwnedModules []string  `json:"owned_modules"`
	SoleModules  []string  `json:"sole_modules"`
	OwnedFlows   []string  `json:"owned_flows,omitempty"`
	LastActive   time.Time `json:"last_active"`
	DaysInactive int       `json:"days_inactive"`
}

// Alert is a grouped risk notification.
type Alert struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Action   string `json:"action"`
	Count    int    `json:"count"`
}

// Insight is a high-level, CEO-readable observation about the repository.
// Unlike alerts (which are per-module/per-dev), insights describe repo-wide patterns.
type Insight struct {
	Level  string `json:"level"` // "critical", "warning", "info"
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"`
}

// Maturity classifies the repository's development stage.
// This affects how risk signals are interpreted and communicated.
type Maturity struct {
	Stage        string `json:"stage"`        // "early", "growing", "mature"
	Label        string `json:"label"`        // human-readable label
	Contributors int    `json:"contributors"` // unique contributor count
	TotalCommits int    `json:"total_commits"`
	Solo         bool   `json:"solo"` // true if single contributor
}

// Summary holds counts by risk level, split by impact classification.
type Summary struct {
	Critical        int `json:"critical"`
	High            int `json:"high"`
	Medium          int `json:"medium"`
	Low             int `json:"low"`
	RuntimeCritical int `json:"runtime_critical"`
	RuntimeHigh     int `json:"runtime_high"`
	LowImpactCount  int `json:"low_impact_count"`
}

// Result is the full analysis output.
type Result struct {
	RepoRoot      string       `json:"repo_root"`
	RepoInfo      git.RepoInfo `json:"repo_info"`
	Maturity      Maturity     `json:"maturity"`
	MuleSoftLayer string       `json:"mulesoft_layer,omitempty"` // "Experience Layer", "Process Layer", "System Layer", or ""
	SinceDays     int          `json:"since_days"`
	FilesAnalyzed int          `json:"files_analyzed"`
	TotalModules  int          `json:"total_modules"`
	Modules       []Module     `json:"modules"`
	Flows         []Flow       `json:"flows"`
	Developers    []Developer  `json:"developers"`
	Insights      []Insight    `json:"insights"`
	Alerts        []Alert      `json:"alerts"`
	Summary       Summary      `json:"summary"`
}

// ── Repo maturity detection ───────────────────────────────────────────────────

// detectMaturity classifies the repository by development stage.
// This prevents over-alerting on solo projects and early-stage repos
// where BF=1 everywhere is structurally expected, not dangerous.
func detectMaturity(repoInfo git.RepoInfo, contributorCount int) Maturity {
	m := Maturity{
		Contributors: contributorCount,
		TotalCommits: repoInfo.TotalCommits,
		Solo:         contributorCount <= 1,
	}

	switch {
	case contributorCount <= 1 && repoInfo.TotalCommits < 100:
		m.Stage = "early"
		m.Label = "Early-stage solo project"
	case contributorCount <= 1:
		m.Stage = "early"
		m.Label = "Solo project"
	case contributorCount <= 3 && repoInfo.TotalCommits < 200:
		m.Stage = "growing"
		m.Label = "Small team, early history"
	case contributorCount <= 3:
		m.Stage = "growing"
		m.Label = "Small team"
	default:
		m.Stage = "mature"
		m.Label = "Team project"
	}

	return m
}

// ── Signal scorers ────────────────────────────────────────────────────────────

func daysAgo(t time.Time) int {
	if t.IsZero() {
		return 9999
	}
	d := int(time.Since(t).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

func recencyWeight(t time.Time) float64 {
	d := daysAgo(t)
	for _, band := range recencyBands {
		if d <= band.days {
			return band.weight
		}
	}
	return 0.1
}

// commitScore measures recency-weighted authorship scaled by change significance.
// A 200-line feature committed yesterday scores much higher than a 1-line typo fix
// from 6 months ago.
func commitScore(commits []git.Commit) float64 {
	if len(commits) == 0 {
		return 0
	}
	var weighted float64
	for _, c := range commits {
		weighted += recencyWeight(c.Date) * significance(c.LinesChanged())
	}
	return math.Min((weighted/20.0)*100.0, 100.0)
}

// weightedTotal returns the significance × recency weighted sum for a set of commits.
func weightedTotal(commits []git.Commit) float64 {
	var total float64
	for _, c := range commits {
		total += significance(c.LinesChanged()) * recencyWeight(c.Date)
	}
	return total
}

// exclusivityScore measures what fraction of a file's total weighted activity
// this developer owns. Uses significance-weighted counts so trivial fixes
// don't dilute real ownership.
func exclusivityScore(devWeighted, totalWeighted float64) float64 {
	if totalWeighted == 0 {
		return 0
	}
	return (devWeighted / totalWeighted) * 100.0
}

func criticalityScore(filePath string, totalCommits int) float64 {
	base := 0.0
	if git.IsCriticalFile(filePath) {
		base = 60.0
	}
	freq := math.Min((float64(totalCommits)/50.0)*40.0, 40.0)
	return math.Min(base+freq, 100.0)
}

func ownershipScore(cScore, eScore, crit float64) float64 {
	return cScore*weightCommit + eScore*weightExclusivity + crit*weightCriticality
}

// ── Bus factor ────────────────────────────────────────────────────────────────

type bfResult struct {
	BusFactor   int
	Percentages map[string]float64
}

func calcBusFactor(scores map[string]float64) bfResult {
	if len(scores) == 0 {
		return bfResult{BusFactor: 1, Percentages: map[string]float64{}}
	}

	total := 0.0
	for _, s := range scores {
		total += s
	}
	if total == 0 {
		return bfResult{BusFactor: 1, Percentages: map[string]float64{}}
	}

	// Sort by score descending
	type kv struct {
		email string
		score float64
	}
	var ranked []kv
	for e, s := range scores {
		ranked = append(ranked, kv{e, s})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	percentages := make(map[string]float64, len(ranked))
	for _, r := range ranked {
		percentages[r.email] = (r.score / total) * 100.0
	}

	cumulative := 0.0
	bf := 0
	for _, r := range ranked {
		cumulative += percentages[r.email]
		bf++
		if cumulative >= 50.0 {
			break
		}
	}

	return bfResult{BusFactor: bf, Percentages: percentages}
}

func riskLevel(bf int) string {
	switch {
	case bf <= 1:
		return "critical"
	case bf <= 2:
		return "high"
	case bf <= 3:
		return "medium"
	default:
		return "low"
	}
}

func trendStr(currentBF int, prevBF int, hasPrev bool) string {
	if !hasPrev {
		return "new"
	}
	if currentBF < prevBF {
		return "worsening"
	}
	if currentBF > prevBF {
		return "improving"
	}
	return "stable"
}

// ── Developer deduplication ───────────────────────────────────────────────────

func normalizeName(name string) string {
	re := regexp.MustCompile(`[^a-z0-9 ]`)
	return re.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "")
}

// nameSeparators splits on whitespace, dots, underscores, and hyphens —
// the common separators in usernames and display names.
var nameSeparators = regexp.MustCompile(`[\s._\-]+`)

// extractNameTokens splits a display name or username into lowercase alphabetic
// tokens, handling corporate identity patterns:
//
//	"Sohaib Tanveer"         → ["sohaib", "tanveer"]
//	"DOMAIN\Sohaib"          → ["sohaib"]          (domain prefix stripped)
//	"SPLHRLAP-1218\Sohaib"   → ["sohaib"]          (domain + numeric noise stripped)
//	"talha.mahmood"          → ["talha", "mahmood"]
//	"talha_mahmood_systems"  → ["talha", "mahmood", "systems"]
func extractNameTokens(name string) []string {
	// Strip Windows domain or path prefix: "DOMAIN\user" / "DOMAIN/user" → "user"
	if idx := strings.LastIndexAny(name, `\/`); idx >= 0 {
		name = name[idx+1:]
	}

	parts := nameSeparators.Split(strings.ToLower(strings.TrimSpace(name)), -1)

	var tokens []string
	for _, p := range parts {
		if len(p) < 2 {
			continue
		}
		// Skip purely numeric tokens (e.g. "1218" from "SPLHRLAP-1218")
		allDigits := true
		for _, r := range p {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if !allDigits {
			tokens = append(tokens, p)
		}
	}
	return tokens
}

// namesMatch returns true if two display names likely belong to the same person.
//
// Rules (conservative to avoid false merges):
//  1. Exact normalized match → true
//  2. Two or more shared tokens → true (e.g. "Jesse Vincent" & "Jesse R Vincent")
//  3. One name's tokens are a complete subset of the other → true
//     (handles "DOMAIN\Sohaib" matching "Sohaib Tanveer", or "talha.mahmood"
//     matching "talha_mahmood_systems")
//  4. Otherwise → false
//     (prevents "John Smith" merging with "John Doe" on a single shared "john")
func namesMatch(a, b string) bool {
	na, nb := normalizeName(a), normalizeName(b)
	if na == nb {
		return true
	}

	tokA := extractNameTokens(a)
	tokB := extractNameTokens(b)
	if len(tokA) == 0 || len(tokB) == 0 {
		return false
	}

	setA := make(map[string]bool, len(tokA))
	for _, p := range tokA {
		setA[p] = true
	}

	// Count shared tokens
	shared := 0
	for _, p := range tokB {
		if setA[p] {
			shared++
		}
	}

	// Rule 2: two or more shared tokens
	if shared >= 2 {
		return true
	}

	if shared == 0 {
		return false
	}

	// Rule 3: complete subset — all tokens of the shorter name appear in the longer
	smaller, larger := tokA, tokB
	if len(tokA) > len(tokB) {
		smaller, larger = tokB, tokA
	}
	largerSet := make(map[string]bool, len(larger))
	for _, p := range larger {
		largerSet[p] = true
	}
	for _, p := range smaller {
		if !largerSet[p] {
			return false
		}
	}
	return true
}

// buildIdentityMap groups emails that likely belong to the same person.
// Returns map[email]canonicalEmail where canonical = email with most commits.
func buildIdentityMap(nameMap map[string]string, commitCounts map[string]int) map[string]string {
	emails := make([]string, 0, len(nameMap))
	for e := range nameMap {
		emails = append(emails, e)
	}

	// Union-Find
	parent := make(map[string]string, len(emails))
	for _, e := range emails {
		parent[e] = e
	}

	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}

	union := func(x, y string) {
		px, py := find(x), find(y)
		if px == py {
			return
		}
		// Canonical = whichever has more commits
		if commitCounts[px] >= commitCounts[py] {
			parent[py] = px
		} else {
			parent[px] = py
		}
	}

	// Check each pair for name similarity
	for i, e1 := range emails {
		for _, e2 := range emails[i+1:] {
			n1, n2 := nameMap[e1], nameMap[e2]
			if n1 != "" && n2 != "" && namesMatch(n1, n2) {
				union(e1, e2)
			}
		}
	}

	result := make(map[string]string, len(emails))
	for _, e := range emails {
		result[e] = find(e)
	}
	return result
}

// ── User flow mapping ─────────────────────────────────────────────────────────

// mapModulesToFlows assigns each module path to zero or more user flows
// by keyword matching against the module path.
func mapModulesToFlows(modulePaths []string, flowDefs []FlowDefinition) map[string][]string {
	flowModules := make(map[string][]string)

	for _, modPath := range modulePaths {
		lower := strings.ToLower(modPath)

		for _, fd := range flowDefs {
			for _, kw := range fd.Keywords {
				if strings.Contains(lower, kw) {
					flowModules[fd.Name] = append(flowModules[fd.Name], modPath)
					break
				}
			}
		}
	}

	return flowModules
}

// ── Main analysis ─────────────────────────────────────────────────────────────

// AnalyzeRepo runs the full Kinlyze analysis including module scoring,
// user flow analysis, developer profiles, and risk alerts.
func AnalyzeRepo(repoRoot string, sinceDays, minCommits int, excludeBots bool, excludeEmails []string, progressFn func(string)) (*Result, error) {
	p := func(msg string) {
		if progressFn != nil {
			progressFn(msg)
		}
	}

	// Build email exclusion set
	excludeSet := make(map[string]bool, len(excludeEmails))
	for _, e := range excludeEmails {
		excludeSet[strings.ToLower(strings.TrimSpace(e))] = true
	}

	// Repo info
	p("Reading repository info...")
	repoInfo, err := git.GetRepoInfo(repoRoot)
	if err != nil {
		return nil, err
	}

	// Contributors (for name map)
	contributors, _ := git.GetContributors(repoRoot)
	nameMap := make(map[string]string, len(contributors))
	commitCounts := make(map[string]int, len(contributors))
	for _, c := range contributors {
		if excludeBots && git.IsBotEmail(c.Email) {
			continue
		}
		if excludeSet[c.Email] {
			continue
		}
		nameMap[c.Email] = c.Name
		commitCounts[c.Email] = c.Commits
	}

	// Detect repo maturity
	maturity := detectMaturity(repoInfo, len(nameMap))

	// Bulk commit load — ONE git call
	p(fmt.Sprintf("Loading commit history (last %d days)...", sinceDays))
	fileCommits, err := git.LoadCommitsBulk(repoRoot, sinceDays, p)
	if err != nil || len(fileCommits) == 0 {
		return nil, fmt.Errorf("no commits found in the last %d days — try --days %d", sinceDays, sinceDays*2)
	}

	// Filter out bot/excluded commits from loaded data
	if excludeBots || len(excludeSet) > 0 {
		for filePath, commits := range fileCommits {
			var filtered []git.Commit
			for _, c := range commits {
				if excludeBots && git.IsBotEmail(c.Email) {
					continue
				}
				if excludeSet[c.Email] {
					continue
				}
				filtered = append(filtered, c)
			}
			if len(filtered) == 0 {
				delete(fileCommits, filePath)
			} else {
				fileCommits[filePath] = filtered
			}
		}
	}

	if len(fileCommits) == 0 {
		return nil, fmt.Errorf("no commits found in the last %d days after filtering — try --days %d", sinceDays, sinceDays*2)
	}

	// Enrich nameMap from commit data
	for _, commits := range fileCommits {
		for _, c := range commits {
			if c.Email != "" && c.Name != "" {
				if _, exists := nameMap[c.Email]; !exists {
					nameMap[c.Email] = c.Name
					commitCounts[c.Email]++
				}
			}
		}
	}

	p(fmt.Sprintf("Analyzing %d files...", len(fileCommits)))

	// Detect repo type early — governs module grouping and flow definitions
	repoType := git.DetectRepoType(repoRoot)

	// MuleSoft: use filename-level module keys for Mule XML and DWL files
	moduleKeyFn := git.GetModulePath
	if repoType == git.RepoTypeMuleSoft {
		moduleKeyFn = git.GetMuleModulePath
	}

	// MuleSoft: detect API-led connectivity layer from repo name
	muleSoftLayer := ""
	if repoType == git.RepoTypeMuleSoft {
		muleSoftLayer = string(git.DetectMuleSoftLayer(repoInfo.Name))
	}

	// Build identity map (deduplicate same-person emails)
	identityMap := buildIdentityMap(nameMap, commitCounts)

	// Canonical name lookup
	canonicalNames := make(map[string]string)
	for email, canonical := range identityMap {
		if _, exists := canonicalNames[canonical]; !exists {
			if name, ok := nameMap[canonical]; ok && name != "" {
				canonicalNames[canonical] = name
			} else if name, ok := nameMap[email]; ok && name != "" {
				canonicalNames[canonical] = name
			}
		}
	}

	// ── Per-file scoring ──────────────────────────────────────────────────────
	type moduleAccum struct {
		scores      map[string]float64
		fileCount   int
		commitCount int
		lastActive  time.Time
	}

	moduleMap := make(map[string]*moduleAccum)
	devTotals := make(map[string]float64)
	grandTotal := 0.0
	filesAnalyzed := 0

	// Track all commits per canonical email for last-active calculation
	devAllDates := make(map[string][]time.Time)

	for filePath, commits := range fileCommits {
		if len(commits) == 0 {
			continue
		}

		// Group by canonical author
		byAuthor := make(map[string][]git.Commit)
		for _, c := range commits {
			if c.Email == "" {
				continue
			}
			canonical := identityMap[c.Email]
			if canonical == "" {
				canonical = c.Email
			}
			byAuthor[canonical] = append(byAuthor[canonical], c)

			if !c.Date.IsZero() {
				devAllDates[canonical] = append(devAllDates[canonical], c.Date)
			}
		}

		// Raw commit count for the minCommits filter (not weighted)
		totalFileCommits := 0
		for _, devCommits := range byAuthor {
			totalFileCommits += len(devCommits)
		}

		if totalFileCommits < minCommits {
			continue
		}

		// Weighted totals for exclusivity scoring
		totalWeighted := 0.0
		devWeightedMap := make(map[string]float64, len(byAuthor))
		for canonical, devCommits := range byAuthor {
			w := weightedTotal(devCommits)
			devWeightedMap[canonical] = w
			totalWeighted += w
		}

		crit := criticalityScore(filePath, totalFileCommits)
		module := moduleKeyFn(filePath)

		if _, exists := moduleMap[module]; !exists {
			moduleMap[module] = &moduleAccum{scores: make(map[string]float64)}
		}
		acc := moduleMap[module]
		acc.fileCount++
		acc.commitCount += totalFileCommits

		// Last active for this module
		for _, c := range commits {
			if !c.Date.IsZero() && c.Date.After(acc.lastActive) {
				acc.lastActive = c.Date
			}
		}

		for canonical, devCommits := range byAuthor {
			cS := commitScore(devCommits)
			eS := exclusivityScore(devWeightedMap[canonical], totalWeighted)
			oS := ownershipScore(cS, eS, crit)

			acc.scores[canonical] += oS
			devTotals[canonical] += oS
			grandTotal += oS
		}

		filesAnalyzed++
	}

	if filesAnalyzed == 0 || grandTotal == 0 {
		return nil, fmt.Errorf("no commits found in the last %d days — try --days %d", sinceDays, sinceDays*2)
	}

	// ── Trend windows ─────────────────────────────────────────────────────────
	p("Computing risk trends...")
	recentFC := git.LoadCommitsWindow(repoRoot, 90, 0)
	priorFC := git.LoadCommitsWindow(repoRoot, 180, 90)

	quickBF := func(fc map[string][]git.Commit, module string) (int, bool) {
		mScores := make(map[string]float64)
		for fp, commits := range fc {
			if moduleKeyFn(fp) != module {
				continue
			}

			byA := make(map[string][]git.Commit)
			for _, c := range commits {
				if c.Email != "" {
					canonical := identityMap[c.Email]
					if canonical == "" {
						canonical = c.Email
					}
					byA[canonical] = append(byA[canonical], c)
				}
			}

			totalW := 0.0
			devW := make(map[string]float64)
			for em, devCommits := range byA {
				w := weightedTotal(devCommits)
				devW[em] = w
				totalW += w
			}

			for em := range byA {
				mScores[em] += exclusivityScore(devW[em], totalW)
			}
		}
		if len(mScores) == 0 {
			return 0, false
		}
		r := calcBusFactor(mScores)
		return r.BusFactor, true
	}

	// ── Build module results ──────────────────────────────────────────────────
	p("Building module profiles...")

	var modules []Module
	for modulePath, acc := range moduleMap {
		bf := calcBusFactor(acc.scores)
		rl := riskLevel(bf.BusFactor)

		// Primary owner
		type ownerScore struct {
			email string
			score float64
		}
		var ranked []ownerScore
		for e, s := range acc.scores {
			ranked = append(ranked, ownerScore{e, s})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

		primaryEmail := ranked[0].email
		primaryName := canonicalNames[primaryEmail]
		if primaryName == "" {
			primaryName = nameFromEmail(primaryEmail)
		}
		primaryPct := round1(bf.Percentages[primaryEmail])

		// Top contributors
		var contribs []Contributor
		for _, r := range ranked {
			if len(contribs) >= 5 {
				break
			}
			name := canonicalNames[r.email]
			if name == "" {
				name = nameFromEmail(r.email)
			}
			contribs = append(contribs, Contributor{
				Name:  name,
				Email: r.email,
				Pct:   round1(bf.Percentages[r.email]),
			})
		}

		// Trend
		recentBF, hasRecent := quickBF(recentFC, modulePath)
		priorBF, hasPrior := quickBF(priorFC, modulePath)
		var trend string
		if !hasRecent {
			trend = "new"
		} else {
			trend = trendStr(recentBF, priorBF, hasPrior)
		}

		modules = append(modules, Module{
			Module:       modulePath,
			BusFactor:    bf.BusFactor,
			RiskLevel:    rl,
			Impact:       classifyImpact(modulePath),
			PrimaryOwner: primaryName,
			PrimaryEmail: primaryEmail,
			PrimaryPct:   primaryPct,
			Contributors: contribs,
			FileCount:    acc.fileCount,
			CommitCount:  acc.commitCount,
			Trend:        trend,
			LastActive:   acc.lastActive,
		})
	}

	// ── Hidden modules ────────────────────────────────────────────────────────
	// These modules are excluded from all display sections (heatmap, bus factor,
	// developer profiles, alerts) but their files still contributed to ownership
	// scores above, so developer knowledge % remains accurate.
	hiddenModules := map[string]bool{
		"root": true, // repo-root files (pom.xml, mule-artifact.json, etc.) — not meaningful as a module
	}
	var visibleModules []Module
	for _, m := range modules {
		if !hiddenModules[m.Module] {
			visibleModules = append(visibleModules, m)
		}
	}
	modules = visibleModules

	// ── User Flow Analysis ────────────────────────────────────────────────────
	p("Analyzing user flows...")

	// Select flow definitions based on repo type (detected earlier)
	activeFlows := generalFlowDefinitions
	if repoType == git.RepoTypeMuleSoft {
		activeFlows = mulesoftFlowDefinitions
	}

	// Collect module paths
	modulePaths := make([]string, 0, len(moduleMap))
	for mp := range moduleMap {
		modulePaths = append(modulePaths, mp)
	}

	// Map modules to flows
	flowModuleMap := mapModulesToFlows(modulePaths, activeFlows)

	var flows []Flow

	for _, fd := range activeFlows {
		flowMods, exists := flowModuleMap[fd.Name]
		if !exists || len(flowMods) == 0 {
			continue
		}

		// Aggregate module scores to flow level
		flowScores := make(map[string]float64)
		for _, modPath := range flowMods {
			acc, ok := moduleMap[modPath]
			if !ok {
				continue
			}
			for dev, score := range acc.scores {
				flowScores[dev] += score
			}
		}

		if len(flowScores) == 0 {
			continue
		}

		bf := calcBusFactor(flowScores)
		rl := riskLevel(bf.BusFactor)

		// Rank contributors
		type ownerScore struct {
			email string
			score float64
		}

		var ranked []ownerScore
		for e, s := range flowScores {
			ranked = append(ranked, ownerScore{e, s})
		}

		sort.Slice(ranked, func(i, j int) bool {
			return ranked[i].score > ranked[j].score
		})

		// Primary owner
		primaryEmail := ranked[0].email
		primaryName := canonicalNames[primaryEmail]
		if primaryName == "" {
			primaryName = nameFromEmail(primaryEmail)
		}

		primaryPct := round1(bf.Percentages[primaryEmail])

		// Coverage (>5%)
		coverage := 0
		for _, pct := range bf.Percentages {
			if pct > 5.0 {
				coverage++
			}
		}

		// Top contributors
		var contribs []Contributor
		for _, r := range ranked {
			if len(contribs) >= 5 {
				break
			}

			name := canonicalNames[r.email]
			if name == "" {
				name = nameFromEmail(r.email)
			}

			contribs = append(contribs, Contributor{
				Name:  name,
				Email: r.email,
				Pct:   round1(bf.Percentages[r.email]),
			})
		}

		// Sort modules for consistency
		sort.Strings(flowMods)

		flows = append(flows, Flow{
			Name:         fd.Name,
			BusFactor:    bf.BusFactor,
			RiskLevel:    rl,
			PrimaryOwner: primaryName,
			PrimaryPct:   primaryPct,
			Coverage:     coverage,
			Modules:      flowMods,
			Contributors: contribs,
		})
	}

	// ✅ Deduplicate flows (fix bug)
	seen := make(map[string]bool)
	var dedupedFlows []Flow

	for _, f := range flows {
		if !seen[f.Name] {
			seen[f.Name] = true
			dedupedFlows = append(dedupedFlows, f)
		}
	}

	flows = dedupedFlows

	// Sort flows by risk
	riskRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

	sort.Slice(flows, func(i, j int) bool {
		ri, rj := riskRank[flows[i].RiskLevel], riskRank[flows[j].RiskLevel]
		if ri != rj {
			return ri < rj
		}
		return flows[i].PrimaryPct > flows[j].PrimaryPct
	})
	// ── Developer profiles ────────────────────────────────────────────────────
	p("Building developer profiles...")

	var developers []Developer
	for canonicalEmail, total := range devTotals {
		pct := round1((total / grandTotal) * 100.0)
		name := canonicalNames[canonicalEmail]
		if name == "" {
			name = nameFromEmail(canonicalEmail)
		}

		risk := "low"
		switch {
		case pct > 30:
			risk = "critical"
		case pct > 15:
			risk = "high"
		case pct > 5:
			risk = "medium"
		}

		var ownedModules, soleModules []string
		for _, m := range modules {
			if len(m.Contributors) > 0 && m.Contributors[0].Email == canonicalEmail {
				ownedModules = append(ownedModules, m.Module)
			}
			if len(m.Contributors) == 1 && m.Contributors[0].Email == canonicalEmail {
				soleModules = append(soleModules, m.Module)
			}
		}

		// Flows where this dev is the primary owner
		var ownedFlows []string
		for _, f := range flows {
			if len(f.Contributors) > 0 && f.Contributors[0].Email == canonicalEmail {
				ownedFlows = append(ownedFlows, f.Name)
			}
		}

		// Last active
		var lastActive time.Time
		for _, d := range devAllDates[canonicalEmail] {
			if d.After(lastActive) {
				lastActive = d
			}
		}
		inactive := daysAgo(lastActive)

		if len(ownedModules) > 8 {
			ownedModules = ownedModules[:8]
		}

		developers = append(developers, Developer{
			Name:         name,
			Email:        canonicalEmail,
			KnowledgePct: pct,
			Risk:         risk,
			OwnedModules: ownedModules,
			SoleModules:  soleModules,
			OwnedFlows:   ownedFlows,
			LastActive:   lastActive,
			DaysInactive: inactive,
		})
	}

	sort.Slice(developers, func(i, j int) bool {
		return developers[i].KnowledgePct > developers[j].KnowledgePct
	})

	// ── Alerts ────────────────────────────────────────────────────────────────
	p("Evaluating risk thresholds...")
	alerts := buildAlerts(modules, flows, developers, maturity)

	// ── Insights ──────────────────────────────────────────────────────────────
	p("Generating insights...")
	insights := buildInsights(modules, developers, maturity)

	// ── Summary ───────────────────────────────────────────────────────────────
	summary := Summary{}
	for _, m := range modules {
		switch m.RiskLevel {
		case "critical":
			summary.Critical++
			if m.Impact == "runtime" {
				summary.RuntimeCritical++
			}
		case "high":
			summary.High++
			if m.Impact == "runtime" {
				summary.RuntimeHigh++
			}
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
		if m.Impact == "low-impact" {
			summary.LowImpactCount++
		}
	}

	return &Result{
		RepoRoot:      repoRoot,
		RepoInfo:      repoInfo,
		Maturity:      maturity,
		MuleSoftLayer: muleSoftLayer,
		SinceDays:     sinceDays,
		FilesAnalyzed: filesAnalyzed,
		TotalModules:  len(modules),
		Modules:       modules,
		Flows:         flows,
		Developers:    developers,
		Insights:      insights,
		Alerts:        alerts,
		Summary:       summary,
	}, nil
}

// ── Module impact classification ──────────────────────────────────────────────

// classifyImpact returns "runtime" or "low-impact" based on the module path.
// Low-impact modules (examples, templates, docs, generated code) are still
// analyzed but their risk signals are surfaced differently to avoid noise.
func classifyImpact(modulePath string) string {
	if git.IsLowImpactPath(modulePath) {
		return "low-impact"
	}
	return "runtime"
}

// ── Insight generation — CEO-level repo-wide signals ──────────────────────────

func buildInsights(modules []Module, developers []Developer, maturity Maturity) []Insight {
	var insights []Insight
	totalModules := len(modules)
	if totalModules == 0 {
		return insights
	}

	// ── Early-stage / solo repo context ───────────────────────────────────
	if maturity.Solo {
		insights = append(insights, Insight{
			Level:  "info",
			Title:  fmt.Sprintf("Solo project detected — %s (%d commits, 1 contributor)", maturity.Label, maturity.TotalCommits),
			Detail: "All risk signals below are structurally expected for a single-developer repository. Bus factor = 1 everywhere is normal at this stage — it is not an operational failure.",
			Action: "As the project scales, consider adding redundancy to the highest-traffic modules first. These signals become actionable once a second contributor joins.",
		})
	} else if maturity.Stage == "growing" {
		insights = append(insights, Insight{
			Level:  "info",
			Title:  fmt.Sprintf("Small team detected — %s (%d contributors, %d commits)", maturity.Label, maturity.Contributors, maturity.TotalCommits),
			Detail: "Some knowledge concentration is expected in small teams. Focus on modules where the primary owner holds >80% — those are the real dependency risks.",
		})
	}

	// ── Monoculture detection ─────────────────────────────────────────────
	ownerModCount := make(map[string]int)
	for _, m := range modules {
		if m.BusFactor == 1 {
			ownerModCount[m.PrimaryOwner]++
		}
	}
	for owner, count := range ownerModCount {
		pct := round1(float64(count) / float64(totalModules) * 100.0)
		if pct >= 80 && !maturity.Solo {
			// Only flag monoculture as critical for team repos — for solo it's already covered
			insights = append(insights, Insight{
				Level:  "critical",
				Title:  fmt.Sprintf("Monoculture — %.0f%% of the repository is owned by %s", pct, owner),
				Detail: fmt.Sprintf("%d of %d modules have a single contributor (%s). This creates extreme maintainer dependency risk — if this person becomes unavailable, the majority of the codebase has zero knowledgeable backup.", count, totalModules, owner),
				Action: "This is not a per-module problem — it is a structural dependency. Prioritize bringing a second contributor into the highest-traffic modules first.",
			})
		} else if pct >= 50 && !maturity.Solo {
			insights = append(insights, Insight{
				Level:  "warning",
				Title:  fmt.Sprintf("High maintainer concentration — %s owns %.0f%% of modules", owner, pct),
				Detail: fmt.Sprintf("%d of %d modules are solely owned by %s.", count, totalModules, owner),
				Action: "Begin cross-training on the most actively changing modules.",
			})
		}
	}

	// ── Low-impact noise detection ────────────────────────────────────────
	lowImpactCount := 0
	lowImpactCritical := 0
	runtimeCritical := 0
	for _, m := range modules {
		if m.Impact == "low-impact" {
			lowImpactCount++
			if m.RiskLevel == "critical" {
				lowImpactCritical++
			}
		} else if m.RiskLevel == "critical" {
			runtimeCritical++
		}
	}

	if lowImpactCritical > 0 && totalModules > 0 {
		noisePct := round1(float64(lowImpactCritical) / float64(totalModules) * 100.0)
		if noisePct >= 30 {
			insights = append(insights, Insight{
				Level:  "info",
				Title:  fmt.Sprintf("%.0f%% of critical modules are in low-impact paths", noisePct),
				Detail: fmt.Sprintf("%d of %d critical modules are in examples, templates, docs, or generated code. These inflate the risk count but do not represent production operational risk. %d critical modules affect runtime code.", lowImpactCritical, lowImpactCritical+runtimeCritical, runtimeCritical),
			})
		}
	}

	// ── Shallow ownership depth ──────────────────────────────────────────
	if !maturity.Solo {
		singleContribModules := 0
		for _, m := range modules {
			if len(m.Contributors) == 1 {
				singleContribModules++
			}
		}
		if totalModules > 0 {
			shallowPct := round1(float64(singleContribModules) / float64(totalModules) * 100.0)
			if shallowPct >= 60 {
				insights = append(insights, Insight{
					Level:  "warning",
					Title:  fmt.Sprintf("Shallow ownership — %.0f%% of modules have only one contributor", shallowPct),
					Detail: fmt.Sprintf("%d of %d modules have never been touched by a second developer. Knowledge depth across the codebase is thin.", singleContribModules, totalModules),
					Action: "Require code reviews from non-authors. Rotate reviewers across modules to build secondary knowledge.",
				})
			}
		}
	}

	return insights
}

// ── Alert generation — impact-aware, grouped ──────────────────────────────────

func buildAlerts(modules []Module, flows []Flow, developers []Developer, maturity Maturity) []Alert {
	var alerts []Alert
	severityRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

	// In solo/early repos, downgrade severity — BF=1 is structurally expected
	soloDowngrade := maturity.Solo

	// Group BF=1 modules by primary owner, split by impact
	type bf1Group struct {
		runtime   []string
		lowImpact []string
	}
	bf1ByOwner := make(map[string]*bf1Group)
	for _, m := range modules {
		if m.BusFactor == 1 {
			if _, ok := bf1ByOwner[m.PrimaryOwner]; !ok {
				bf1ByOwner[m.PrimaryOwner] = &bf1Group{}
			}
			if m.Impact == "runtime" {
				bf1ByOwner[m.PrimaryOwner].runtime = append(bf1ByOwner[m.PrimaryOwner].runtime, m.Module)
			} else {
				bf1ByOwner[m.PrimaryOwner].lowImpact = append(bf1ByOwner[m.PrimaryOwner].lowImpact, m.Module)
			}
		}
	}

	for owner, group := range bf1ByOwner {
		runtimeCount := len(group.runtime)
		lowImpactCount := len(group.lowImpact)

		if runtimeCount > 0 {
			sev := "critical"
			title := fmt.Sprintf("Bus factor 1 — %s is a single point of failure", owner)
			action := "Schedule knowledge transfer sessions for runtime modules. Encourage code reviews across these areas."

			if soloDowngrade {
				sev = "medium"
				title = fmt.Sprintf("Single-owner pattern — %s owns all %d runtime module(s)", owner, runtimeCount)
				action = "Expected for a solo project. As the project scales, prioritize adding a second contributor to the highest-traffic modules."
			}

			detail := fmt.Sprintf("%d runtime module(s) with no backup: %s.", runtimeCount, joinModules(group.runtime, 3))
			if lowImpactCount > 0 {
				detail += fmt.Sprintf(" (%d additional low-impact modules in examples/templates/docs.)", lowImpactCount)
			}
			alerts = append(alerts, Alert{
				Severity: sev,
				Type:     "bus_factor_1",
				Title:    title,
				Detail:   detail,
				Action:   action,
				Count:    runtimeCount,
			})
		} else if lowImpactCount > 0 {
			alerts = append(alerts, Alert{
				Severity: "low",
				Type:     "bus_factor_1_low_impact",
				Title:    fmt.Sprintf("Bus factor 1 — %s owns %d low-impact module(s)", owner, lowImpactCount),
				Detail:   fmt.Sprintf("%d module(s) in examples/templates/docs with no backup: %s. These are not runtime-critical.", lowImpactCount, joinModules(group.lowImpact, 3)),
				Action:   "Consider whether these modules need maintainer redundancy. For template/example code, this may be acceptable.",
				Count:    lowImpactCount,
			})
		}
	}

	// Flow-level BF=1 alerts
	for _, f := range flows {
		if f.BusFactor == 1 {
			sev := "critical"
			title := fmt.Sprintf("Flow risk — %s is one resignation away from unmaintainable", f.Name)
			action := "Prioritize cross-training across this flow. Assign secondary reviewers to all modules in this flow."

			if soloDowngrade {
				sev = "medium"
				title = fmt.Sprintf("Flow ownership — %s flow is entirely owned by %s", f.Name, f.PrimaryOwner)
				action = "Expected in a solo project. Plan for flow-level redundancy as the team grows."
			}

			alerts = append(alerts, Alert{
				Severity: sev,
				Type:     "flow_bus_factor_1",
				Title:    title,
				Detail:   fmt.Sprintf("%s owns %.1f%% of the entire %s flow (%d modules).", f.PrimaryOwner, f.PrimaryPct, f.Name, len(f.Modules)),
				Action:   action,
				Count:    1,
			})
		}
	}

	// High concentration (BF > 1, primary owns 80%+) — runtime only
	highConcByOwner := make(map[string][]string)
	for _, m := range modules {
		if m.BusFactor > 1 && m.PrimaryPct >= 80 && m.Impact == "runtime" {
			highConcByOwner[m.PrimaryOwner] = append(highConcByOwner[m.PrimaryOwner], m.Module)
		}
	}
	for owner, mods := range highConcByOwner {
		count := len(mods)
		examples := joinModules(mods, 3)
		alerts = append(alerts, Alert{
			Severity: "high",
			Type:     "high_concentration",
			Title:    fmt.Sprintf("High concentration — %s dominates %d runtime module(s)", owner, count),
			Detail:   fmt.Sprintf("80%%+ ownership in: %s.", examples),
			Action:   "Distribute knowledge through mandatory code reviews and documentation.",
			Count:    count,
		})
	}

	// Worsening trend — runtime only
	var worsening []string
	for _, m := range modules {
		if m.Trend == "worsening" && m.Impact == "runtime" {
			worsening = append(worsening, m.Module)
		}
	}
	if len(worsening) > 0 {
		alerts = append(alerts, Alert{
			Severity: "medium",
			Type:     "trend_worsening",
			Title:    fmt.Sprintf("Concentration increasing in %d module(s)", len(worsening)),
			Detail:   fmt.Sprintf("Knowledge becoming more concentrated: %s.", joinModules(worsening, 4)),
			Action:   "Review contribution patterns. Are new team members being onboarded to these areas?",
			Count:    len(worsening),
		})
	}

	// Developer org-level risk — skip for solo repos (already covered by insights)
	if !soloDowngrade {
		for _, dev := range developers {
			switch {
			case dev.KnowledgePct > 30:
				flowNote := ""
				if len(dev.OwnedFlows) > 0 {
					flowNote = fmt.Sprintf(" End-to-end owner of flow(s): %s.", strings.Join(dev.OwnedFlows, ", "))
				}
				alerts = append(alerts, Alert{
					Severity: "critical",
					Type:     "org_knowledge_risk",
					Title:    fmt.Sprintf("Org-wide dependency — %s holds %.1f%% of knowledge", dev.Name, dev.KnowledgePct),
					Detail:   fmt.Sprintf("Primary owner of %d modules. Sole owner of %d with no other contributors.%s", len(dev.OwnedModules), len(dev.SoleModules), flowNote),
					Action:   "Create a knowledge transfer plan. Prioritise onboarding others to critical modules.",
					Count:    1,
				})
			case dev.KnowledgePct > 15:
				alerts = append(alerts, Alert{
					Severity: "high",
					Type:     "developer_concentration",
					Title:    fmt.Sprintf("High dependency — %s holds %.1f%% of knowledge", dev.Name, dev.KnowledgePct),
					Detail:   fmt.Sprintf("Primary owner of %d modules.", len(dev.OwnedModules)),
					Action:   "Encourage cross-team reviews and documentation contributions.",
					Count:    1,
				})
			}
		}
	}

	sort.Slice(alerts, func(i, j int) bool {
		return severityRank[alerts[i].Severity] < severityRank[alerts[j].Severity]
	})
	return alerts
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func nameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	return parts[0]
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}

func joinModules(mods []string, maxCount int) string {
	var parts []string
	for i, m := range mods {
		if i >= maxCount {
			parts = append(parts, fmt.Sprintf("and %d more", len(mods)-maxCount))
			break
		}
		parts = append(parts, fmt.Sprintf("`%s`", m))
	}
	return strings.Join(parts, ", ")
}
