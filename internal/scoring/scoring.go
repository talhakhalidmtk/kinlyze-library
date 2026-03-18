// Package scoring implements the Kinlyze ownership scoring algorithm.
//
// Composite ownership score per developer per file:
//
//	score = (commit_score × 0.40) + (exclusivity × 0.40) + (criticality × 0.20)
//
// Bus factor = minimum developers whose removal causes ≥50% knowledge loss.
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

// ── Public result types ───────────────────────────────────────────────────────

// Contributor is a developer's share of a module.
type Contributor struct {
	Name  string
	Email string
	Pct   float64
}

// Module is the bus factor analysis result for one module path.
type Module struct {
	Module       string
	BusFactor    int
	RiskLevel    string
	PrimaryOwner string
	PrimaryEmail string
	PrimaryPct   float64
	Contributors []Contributor
	FileCount    int
	CommitCount  int
	Trend        string
	LastActive   time.Time
}

// Developer is a developer's org-wide knowledge profile.
type Developer struct {
	Name         string
	Email        string
	KnowledgePct float64
	Risk         string
	OwnedModules []string
	SoleModules  []string
	LastActive   time.Time
	DaysInactive int
}

// Alert is a grouped risk notification.
type Alert struct {
	Severity string
	Type     string
	Title    string
	Detail   string
	Action   string
	Count    int
}

// Summary holds counts by risk level.
type Summary struct {
	Critical int
	High     int
	Medium   int
	Low      int
}

// Result is the full analysis output.
type Result struct {
	RepoRoot      string
	RepoInfo      git.RepoInfo
	SinceDays     int
	FilesAnalyzed int
	TotalModules  int
	Modules       []Module
	Developers    []Developer
	Alerts        []Alert
	Summary       Summary
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

func commitScore(commits []git.Commit) float64 {
	if len(commits) == 0 {
		return 0
	}
	var weighted float64
	for _, c := range commits {
		weighted += recencyWeight(c.Date)
	}
	return math.Min((weighted/20.0)*100.0, 100.0)
}

func exclusivityScore(devCount, totalCount int) float64 {
	if totalCount == 0 {
		return 0
	}
	return (float64(devCount) / float64(totalCount)) * 100.0
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

func namesMatch(a, b string) bool {
	na, nb := normalizeName(a), normalizeName(b)
	if na == nb {
		return true
	}
	partsA := strings.Fields(na)
	partsB := strings.Fields(nb)
	if len(partsA) == 0 || len(partsB) == 0 {
		return false
	}
	// Share at least one word token
	setA := make(map[string]bool)
	for _, p := range partsA {
		setA[p] = true
	}
	for _, p := range partsB {
		if setA[p] {
			return true
		}
	}
	return false
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

// ── Main analysis ─────────────────────────────────────────────────────────────

// AnalyzeRepo runs the full Kinlyze MVP analysis.
func AnalyzeRepo(repoRoot string, sinceDays, minCommits int, progressFn func(string)) (*Result, error) {
	p := func(msg string) {
		if progressFn != nil {
			progressFn(msg)
		}
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
		nameMap[c.Email] = c.Name
		commitCounts[c.Email] = c.Commits
	}

	// Bulk commit load — ONE git call
	p(fmt.Sprintf("Loading commit history (last %d days)...", sinceDays))
	fileCommits, err := git.LoadCommitsBulk(repoRoot, sinceDays, p)
	if err != nil || len(fileCommits) == 0 {
		return nil, fmt.Errorf("no commits found in the last %d days — try --days %d", sinceDays, sinceDays*2)
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

		totalFileCommits := 0
		for _, devCommits := range byAuthor {
			totalFileCommits += len(devCommits)
		}

		if totalFileCommits < minCommits {
			continue
		}

		crit := criticalityScore(filePath, totalFileCommits)
		module := git.GetModulePath(filePath)

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
			eS := exclusivityScore(len(devCommits), totalFileCommits)
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
			if git.GetModulePath(fp) != module {
				continue
			}
			byA := make(map[string]int)
			for _, c := range commits {
				if c.Email != "" {
					canonical := identityMap[c.Email]
					if canonical == "" {
						canonical = c.Email
					}
					byA[canonical]++
				}
			}
			total := 0
			for _, cnt := range byA {
				total += cnt
			}
			for em, cnt := range byA {
				mScores[em] += exclusivityScore(cnt, total)
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

	// Sort: worst first
	riskRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	sort.Slice(modules, func(i, j int) bool {
		ri, rj := riskRank[modules[i].RiskLevel], riskRank[modules[j].RiskLevel]
		if ri != rj {
			return ri < rj
		}
		return modules[i].PrimaryPct > modules[j].PrimaryPct
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
			LastActive:   lastActive,
			DaysInactive: inactive,
		})
	}

	sort.Slice(developers, func(i, j int) bool {
		return developers[i].KnowledgePct > developers[j].KnowledgePct
	})

	// ── Alerts ────────────────────────────────────────────────────────────────
	p("Evaluating risk thresholds...")
	alerts := buildAlerts(modules, developers)

	// ── Summary ───────────────────────────────────────────────────────────────
	summary := Summary{}
	for _, m := range modules {
		switch m.RiskLevel {
		case "critical":
			summary.Critical++
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
	}

	return &Result{
		RepoRoot:      repoRoot,
		RepoInfo:      repoInfo,
		SinceDays:     sinceDays,
		FilesAnalyzed: filesAnalyzed,
		TotalModules:  len(modules),
		Modules:       modules,
		Developers:    developers,
		Alerts:        alerts,
		Summary:       summary,
	}, nil
}

// ── Alert generation — grouped, not per-module ────────────────────────────────

func buildAlerts(modules []Module, developers []Developer) []Alert {
	var alerts []Alert
	severityRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

	// Group BF=1 modules by primary owner
	bf1ByOwner := make(map[string][]string)
	for _, m := range modules {
		if m.BusFactor == 1 {
			bf1ByOwner[m.PrimaryOwner] = append(bf1ByOwner[m.PrimaryOwner], m.Module)
		}
	}
	for owner, mods := range bf1ByOwner {
		count := len(mods)
		examples := joinModules(mods, 3)
		alerts = append(alerts, Alert{
			Severity: "critical",
			Type:     "bus_factor_1",
			Title:    fmt.Sprintf("Bus factor 1 — %s is a single point of failure", owner),
			Detail:   fmt.Sprintf("%d module(s) with no backup: %s.", count, examples),
			Action:   "Schedule knowledge transfer sessions. Encourage code reviews across these areas.",
			Count:    count,
		})
	}

	// High concentration (BF > 1, primary owns 80%+)
	highConcByOwner := make(map[string][]string)
	for _, m := range modules {
		if m.BusFactor > 1 && m.PrimaryPct >= 80 {
			highConcByOwner[m.PrimaryOwner] = append(highConcByOwner[m.PrimaryOwner], m.Module)
		}
	}
	for owner, mods := range highConcByOwner {
		count := len(mods)
		examples := joinModules(mods, 3)
		alerts = append(alerts, Alert{
			Severity: "high",
			Type:     "high_concentration",
			Title:    fmt.Sprintf("High concentration — %s dominates %d module(s)", owner, count),
			Detail:   fmt.Sprintf("80%%+ ownership in: %s.", examples),
			Action:   "Distribute knowledge through mandatory code reviews and documentation.",
			Count:    count,
		})
	}

	// Worsening trend
	var worsening []string
	for _, m := range modules {
		if m.Trend == "worsening" {
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

	// Developer org-level risk
	for _, dev := range developers {
		switch {
		case dev.KnowledgePct > 30:
			alerts = append(alerts, Alert{
				Severity: "critical",
				Type:     "org_knowledge_risk",
				Title:    fmt.Sprintf("Org-wide dependency — %s holds %.1f%% of knowledge", dev.Name, dev.KnowledgePct),
				Detail:   fmt.Sprintf("Primary owner of %d modules. Sole owner of %d with no other contributors.", len(dev.OwnedModules), len(dev.SoleModules)),
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

func joinModules(mods []string, max int) string {
	var parts []string
	for i, m := range mods {
		if i >= max {
			parts = append(parts, fmt.Sprintf("and %d more", len(mods)-max))
			break
		}
		parts = append(parts, fmt.Sprintf("`%s`", m))
	}
	return strings.Join(parts, ", ")
}
