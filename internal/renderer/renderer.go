// Package renderer produces ANSI-coloured terminal output.
// All column formatting uses ANSI-aware padding to handle invisible escape codes.
package renderer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
)

// ── Color ─────────────────────────────────────────────────────────────────────

var noColor = !isTerminal() || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func c(code, text string) string {
	if noColor {
		return text
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", code, text)
}

func bold(t string) string    { return c("1", t) }
func red(t string) string     { return c("31", t) }
func green(t string) string   { return c("32", t) }
func yellow(t string) string  { return c("33", t) }
func cyan(t string) string    { return c("36", t) }
func white(t string) string   { return c("97", t) }
func grey(t string) string    { return c("90", t) }
func boldRed(t string) string { return c("1;31", t) }

var ansiEsc = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// vlen returns the visible (printable) length of a string after stripping ANSI codes.
// Uses rune count (not byte length) so that multi-byte Unicode characters like
// arrows (→ ↑ ↓ = 3 bytes) and middle-dot (· = 2 bytes) are all counted as 1.
func vlen(s string) int {
	return len([]rune(ansiEsc.ReplaceAllString(s, "")))
}

// pad pads text to width printable characters, ANSI-aware.
func pad(text string, width int) string {
	diff := width - vlen(text)
	if diff <= 0 {
		return text
	}
	return text + strings.Repeat(" ", diff)
}

func riskColor(level, text string) string {
	switch level {
	case "critical":
		return boldRed(text)
	case "high":
		return yellow(text)
	case "medium":
		return yellow(text)
	case "low":
		return green(text)
	}
	return text
}

var riskEmoji = map[string]string{
	"critical": "🔴",
	"high":     "🟠",
	"medium":   "🟡",
	"low":      "🟢",
}

var trendArrow = map[string]string{
	"worsening": "↓",
	"improving": "↑",
	"stable":    "→",
	"new":       "·",
}

var sevIcon = map[string]string{
	"critical": "✖",
	"high":     "▲",
	"medium":   "●",
	"low":      "·",
}

// ── Layout ────────────────────────────────────────────────────────────────────

func termWidth() int {
	// Fixed safe width — avoids syscall complexity for cross-platform
	return 96
}

func div() string {
	return grey(strings.Repeat("─", termWidth()))
}

func header(title, subtitle string) {
	label := bold(white(title))
	sub := ""
	if subtitle != "" {
		sub = "  " + grey(subtitle)
	}
	fmt.Printf("\n  %s%s\n  %s\n", label, sub, div())
}

func bar(pct float64, width int) string {
	filled := int(math.Round(math.Min(pct, 100) / 100.0 * float64(width)))
	if filled < 0 {
		filled = 0
	}
	empty := width - filled
	return cyan(strings.Repeat("█", filled)) + grey(strings.Repeat("░", empty))
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return grey("—")
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days == 0:
		return green("today")
	case days == 1:
		return green("yesterday")
	case days <= 7:
		return green(fmt.Sprintf("%dd ago", days))
	case days <= 30:
		return white(fmt.Sprintf("%dd ago", days))
	case days <= 90:
		return yellow(fmt.Sprintf("%dd ago", days))
	default:
		return red(fmt.Sprintf("%dd ago", days))
	}
}

func fmtInactive(days int) string {
	switch {
	case days <= 7:
		return green(fmt.Sprintf("%dd", days))
	case days <= 30:
		return white(fmt.Sprintf("%dd", days))
	case days <= 90:
		return yellow(fmt.Sprintf("%dd", days))
	default:
		return red(fmt.Sprintf("%dd", days))
	}
}

func trendColored(trend string) string {
	arrow := trendArrow[trend]
	switch trend {
	case "worsening":
		return red(arrow)
	case "improving":
		return green(arrow)
	default:
		return grey(arrow)
	}
}

// ── Progress / error ──────────────────────────────────────────────────────────

// PrintProgress prints a grey progress line.
func PrintProgress(msg string) {
	fmt.Printf("  %s %s\n", grey("›"), grey(msg))
}

// PrintError prints a red error message to stderr.
func PrintError(msg string) {
	lines := strings.Split(strings.TrimSpace(msg), "\n")
	fmt.Fprintf(os.Stderr, "\n  %s %s %s\n", red("✖"), bold(red("Error:")), lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(os.Stderr, "    %s\n", line)
	}
	fmt.Fprintln(os.Stderr)
}

// PrintWarning prints a yellow warning.
func PrintWarning(msg string) {
	fmt.Printf("  %s  %s\n", yellow("⚠"), yellow(msg))
}

// ── Banner ────────────────────────────────────────────────────────────────────

func printBanner() {
	fmt.Printf("\n  %s  %s  %s  %s\n",
		bold(cyan("KINLYZE")),
		grey("·"),
		grey("Analyze the kin behind your code"),
		cyan("kinlyze.com"),
	)
}

// ── Section 0: Scan summary ───────────────────────────────────────────────────

func renderSummary(r *scoring.Result) {
	header("SCAN SUMMARY", "")
	fmt.Println()

	alertStr := green("0 — looking good")
	if len(r.Alerts) > 0 {
		alertStr = yellow(fmt.Sprintf("%d", len(r.Alerts)))
	}

	flowStr := white(fmt.Sprintf("%d", len(r.Flows)))
	if len(r.Flows) == 0 {
		flowStr = grey("—")
	}

	// Maturity label
	maturityStr := grey(r.Maturity.Label)

	rows := [][4]string{
		{grey("Repository"), bold(cyan(r.RepoInfo.Name)), grey("Files analyzed"), white(fmt.Sprintf("%d", r.FilesAnalyzed))},
		{grey("Branch"), white(r.RepoInfo.Branch), grey("Modules found"), white(fmt.Sprintf("%d", r.TotalModules))},
		{grey("Total commits"), white(fmt.Sprintf("%d", r.RepoInfo.TotalCommits)), grey("User flows"), flowStr},
		{grey("Contributors"), white(fmt.Sprintf("%d", r.Maturity.Contributors)), grey("Alerts"), alertStr},
		{grey("Repo maturity"), maturityStr, grey("Scoring"), grey("significance-weighted")},
	}
	if r.MuleSoftLayer != "" {
		rows = append(rows, [4]string{grey("API layer"), cyan(r.MuleSoftLayer), grey(""), grey("")})
	}

	for _, row := range rows {
		fmt.Printf("  %s  %s  %s  %s\n",
			pad(row[0], 18), pad(row[1], 26),
			pad(row[2], 18), row[3],
		)
	}

	fmt.Println()

	s := r.Summary
	total := s.Critical + s.High + s.Medium + s.Low
	if total == 0 {
		total = 1
	}

	fmt.Printf("  %s  %s critical    %s  %s high    %s  %s medium    %s  %s healthy\n",
		riskEmoji["critical"], pad(red(bold(fmt.Sprintf("%d", s.Critical))), 4),
		riskEmoji["high"], pad(yellow(fmt.Sprintf("%d", s.High)), 4),
		riskEmoji["medium"], pad(yellow(fmt.Sprintf("%d", s.Medium)), 4),
		riskEmoji["low"], green(fmt.Sprintf("%d", s.Low)),
	)

	// Show runtime vs low-impact breakdown if there's meaningful noise
	if s.LowImpactCount > 0 {
		fmt.Printf("\n  %s  %s runtime-critical    %s  %s in low-impact paths %s\n",
			grey("├"),
			red(bold(fmt.Sprintf("%d", s.RuntimeCritical))),
			grey("·"),
			grey(fmt.Sprintf("%d", s.LowImpactCount)),
			grey("(examples, templates, docs)"),
		)
	}

	// Proportion bar
	barWidth := 56
	cw := int(math.Round(float64(s.Critical) / float64(total) * float64(barWidth)))
	hw := int(math.Round(float64(s.High) / float64(total) * float64(barWidth)))
	mw := int(math.Round(float64(s.Medium) / float64(total) * float64(barWidth)))
	lw := barWidth - cw - hw - mw
	if lw < 0 {
		lw = 0
	}

	propBar := ""
	if !noColor {
		propBar = fmt.Sprintf("\033[31m%s\033[0m\033[33m%s\033[0m\033[93m%s\033[0m\033[32m%s\033[0m",
			strings.Repeat("█", cw), strings.Repeat("█", hw),
			strings.Repeat("█", mw), strings.Repeat("█", lw),
		)
	} else {
		propBar = strings.Repeat("█", barWidth)
	}

	fmt.Printf("\n  %s\n\n", propBar)
}

// ── Section 1: Heat Map ───────────────────────────────────────────────────────

func renderHeatmap(modules []scoring.Module, top int) {
	display := modules
	if top > 0 && top < len(modules) {
		display = modules[:top]
	}

	subtitle := fmt.Sprintf("%d modules  ·  red = high concentration risk", len(display))
	if top > 0 && top < len(modules) {
		subtitle = fmt.Sprintf("showing top %d of %d  ·  use --top %d to see all", top, len(modules), len(modules))
	}
	header("KNOWLEDGE HEAT MAP", subtitle)
	fmt.Println()

	// Column widths
	const (
		cModule  = 34
		cBF      = 4
		cTrend   = 3
		cFiles   = 5
		cCommits = 7
		cOwner   = 22
		cPct     = 7
	)

	fmt.Printf("  %s  %s  %s  %s  %s  %s  %s  %s  %s\n",
		"   ",
		pad(grey("MODULE"), cModule),
		pad(grey("BF"), cBF),
		pad(grey("↕"), cTrend),
		pad(grey("FILES"), cFiles),
		pad(grey("COMMITS"), cCommits),
		pad(grey("PRIMARY OWNER"), cOwner),
		pad(grey("OWNS"), cPct+14),
		grey("LAST ACTIVE"),
	)
	fmt.Printf("  %s\n", div())

	for _, m := range display {
		emoji := riskEmoji[m.RiskLevel]
		mod := m.Module
		if len(mod) > cModule-1 {
			mod = "…" + mod[len(mod)-(cModule-2):]
		}

		// Dim low-impact modules (examples, templates, docs)
		isLowImpact := m.Impact == "low-impact"
		modColor := white(mod)
		if isLowImpact {
			modColor = grey(mod)
			emoji = grey("○")
		}

		bfStr := risk_col(m.RiskLevel, fmt.Sprintf("%d", m.BusFactor))
		if isLowImpact {
			bfStr = grey(fmt.Sprintf("%d", m.BusFactor))
		}
		trendStr := trendColored(m.Trend)
		ownerStr := m.PrimaryOwner
		if len(ownerStr) > cOwner-1 {
			ownerStr = ownerStr[:cOwner-1]
		}
		if isLowImpact {
			ownerStr = grey(ownerStr)
		} else {
			ownerStr = white(ownerStr)
		}
		pctBar := bar(m.PrimaryPct, 10)
		pctStr := cyan(fmt.Sprintf("%.1f%%", m.PrimaryPct))
		lastStr := fmtDate(m.LastActive)

		fmt.Printf("  %s  %s  %s  %s  %s  %s  %s  %s %s  %s\n",
			emoji,
			pad(modColor, cModule),
			pad(bfStr, cBF),
			pad(trendStr, cTrend),
			pad(grey(fmt.Sprintf("%d", m.FileCount)), cFiles),
			pad(grey(fmt.Sprintf("%d", m.CommitCount)), cCommits),
			pad(ownerStr, cOwner),
			pctBar, pad(pctStr, cPct),
			lastStr,
		)
	}

	if top > 0 && top < len(modules) {
		remaining := len(modules) - top
		fmt.Printf("\n  %s\n", grey(fmt.Sprintf("… %d more modules. Run with --top %d to see all.", remaining, len(modules))))
	}

	fmt.Println()
	fmt.Printf("  %s  %s worsening  %s stable  %s improving    %s low-impact (examples/templates/docs)\n",
		grey("BF = Bus Factor  ·"),
		red("↓"), grey("→"), green("↑"),
		grey("○"),
	)
	fmt.Println()
}

// ── Section 2: Bus Factor ─────────────────────────────────────────────────────

func renderBusFactor(modules []scoring.Module) {
	risky := filterModules(modules, func(m scoring.Module) bool {
		return m.RiskLevel == "critical" || m.RiskLevel == "high"
	})

	if len(risky) == 0 {
		header("BUS FACTOR ANALYSIS", "")
		fmt.Printf("\n  %s  %s\n\n", green("✓"), green("All modules have bus factor 3+. Knowledge is well distributed."))
		return
	}

	subtitle := fmt.Sprintf("%d at-risk modules  ·  grouped by primary owner", len(risky))
	header("BUS FACTOR ANALYSIS", subtitle)
	fmt.Println()

	// Group by primary owner
	ownerMods := make(map[string][]scoring.Module)
	ownerOrder := []string{}
	for _, m := range risky {
		if _, exists := ownerMods[m.PrimaryOwner]; !exists {
			ownerOrder = append(ownerOrder, m.PrimaryOwner)
		}
		ownerMods[m.PrimaryOwner] = append(ownerMods[m.PrimaryOwner], m)
	}

	for _, owner := range ownerOrder {
		owned := ownerMods[owner]
		worst := owned[0]

		soleCount := 0
		for _, m := range owned {
			if len(m.Contributors) == 1 {
				soleCount++
			}
		}

		soleStr := ""
		if soleCount > 0 {
			soleStr = fmt.Sprintf("  %s  %s", grey("·"), red(fmt.Sprintf("%d sole owner", soleCount)))
		}

		fmt.Printf("  %s  %s  %s  %s%s\n",
			riskEmoji[worst.RiskLevel],
			bold(white(owner)),
			grey("·"),
			riskColor(worst.RiskLevel, fmt.Sprintf("%d modules at risk", len(owned))),
			soleStr,
		)

		limit := 4
		if len(owned) < limit {
			limit = len(owned)
		}
		for _, m := range owned[:limit] {
			pctBar := bar(m.PrimaryPct, 20)
			pctStr := riskColor(m.RiskLevel, fmt.Sprintf("%.1f%%", m.PrimaryPct))

			mod := m.Module
			if len(mod) > 38 {
				mod = "…" + mod[len(mod)-37:]
			}

			// Other contributors
			othersStr := ""
			for _, contrib := range m.Contributors {
				if contrib.Email != m.PrimaryEmail {
					othersStr = fmt.Sprintf("  %s %s (%.1f%%)", grey("+"), grey(contrib.Name), contrib.Pct)
					break
				}
			}

			fmt.Printf("     %s  %s  BF %s  %s %s%s\n",
				grey("›"),
				pad(white(mod), 40),
				riskColor(m.RiskLevel, fmt.Sprintf("%d", m.BusFactor)),
				pctBar, pctStr,
				othersStr,
			)
		}

		if len(owned) > 4 {
			fmt.Printf("     %s\n", grey(fmt.Sprintf("… and %d more modules", len(owned)-4)))
		}
		fmt.Println()
	}
}

// ── Section 3: Developer Profiles ────────────────────────────────────────────

func renderDeveloperProfiles(developers []scoring.Developer) {
	if len(developers) == 0 {
		return
	}

	header("DEVELOPER PROFILES", "knowledge departure impact")
	fmt.Println()

	const (
		cName     = 24
		cPct      = 7
		cRisk     = 10
		cInactive = 8
	)

	fmt.Printf("  %s  %s  %s  %s  %s\n",
		pad(grey("DEVELOPER"), cName),
		pad(grey("KNOWLEDGE"), cPct+14),
		pad(grey("IF THEY LEAVE"), cRisk),
		pad(grey("INACTIVE"), cInactive),
		grey("MODULES OWNED"),
	)
	fmt.Printf("  %s\n", div())

	limit := 15
	if len(developers) < limit {
		limit = len(developers)
	}

	for _, dev := range developers[:limit] {
		name := dev.Name
		if len(name) > cName-1 {
			name = name[:cName-1]
		}

		pct := dev.KnowledgePct
		pctBar := bar(math.Min(pct, 50), 12) // cap visual at 50%
		pctStr := pad(riskColor(dev.Risk, fmt.Sprintf("%.1f%%", pct)), cPct)
		riskStr := pad(riskColor(dev.Risk, strings.ToUpper(dev.Risk)), cRisk)
		inactStr := fmtInactive(dev.DaysInactive)

		// Module display
		var modParts []string
		for i, m := range dev.OwnedModules {
			if i >= 3 {
				break
			}
			parts := strings.Split(m, "/")
			modParts = append(modParts, parts[len(parts)-1])
		}
		modsStr := strings.Join(modParts, "  ")
		if len(dev.OwnedModules) > 3 {
			modsStr += fmt.Sprintf("  %s", grey(fmt.Sprintf("+%d more", len(dev.OwnedModules)-3)))
		}
		if modsStr == "" {
			modsStr = grey("—")
		}

		fmt.Printf("  %s  %s %s  %s  %s  %s\n",
			pad(white(name), cName),
			pctBar, pctStr,
			riskStr,
			pad(inactStr, cInactive),
			grey(modsStr),
		)

		if len(dev.SoleModules) > 0 {
			examples := []string{}
			for i, m := range dev.SoleModules {
				if i >= 3 {
					break
				}
				parts := strings.Split(m, "/")
				examples = append(examples, fmt.Sprintf("'%s'", parts[len(parts)-1]))
			}
			exStr := strings.Join(examples, ", ")
			if len(dev.SoleModules) > 3 {
				exStr += fmt.Sprintf(" +%d", len(dev.SoleModules)-3)
			}
			fmt.Printf("  %s %s\n", grey("  └─"), yellow(fmt.Sprintf("Sole owner of %d module(s): %s", len(dev.SoleModules), exStr)))
		}

		if len(dev.OwnedFlows) > 0 {
			fmt.Printf("  %s %s\n", grey("  └─"), red(fmt.Sprintf("End-to-end owner of flow(s): %s", strings.Join(dev.OwnedFlows, ", "))))
		}
	}

	if len(developers) > 15 {
		fmt.Printf("\n  %s\n", grey(fmt.Sprintf("… %d more developers with low risk scores", len(developers)-15)))
	}
	fmt.Println()
}

// ── Section 4: User Flow Risk ─────────────────────────────────────────────────

func renderFlowRisk(flows []scoring.Flow) {
	if len(flows) == 0 {
		header("USER FLOW RISK", "end-to-end capability analysis")
		fmt.Printf("\n  %s  %s\n\n", grey("—"), grey("No user flows detected. Flows are identified by keyword patterns in module paths (auth, payment, api, etc.)."))
		return
	}

	subtitle := fmt.Sprintf("%d flows detected  ·  does one person own an entire user journey?", len(flows))
	header("USER FLOW RISK", subtitle)
	fmt.Println()

	const (
		cFlow     = 22
		cBF       = 4
		cOwner    = 22
		cPct      = 7
		cCoverage = 10
	)

	fmt.Printf("  %s  %s  %s  %s  %s  %s  %s\n",
		"   ",
		pad(grey("USER FLOW"), cFlow),
		pad(grey("BF"), cBF),
		pad(grey("PRIMARY OWNER"), cOwner),
		pad(grey("OWNS"), cPct+14),
		pad(grey("COVERAGE"), cCoverage),
		grey("MODULES"),
	)
	fmt.Printf("  %s\n", div())

	for _, f := range flows {
		emoji := riskEmoji[f.RiskLevel]
		flowName := f.Name
		if len(flowName) > cFlow-1 {
			flowName = flowName[:cFlow-1]
		}

		bfStr := risk_col(f.RiskLevel, fmt.Sprintf("%d", f.BusFactor))
		ownerStr := f.PrimaryOwner
		if len(ownerStr) > cOwner-1 {
			ownerStr = ownerStr[:cOwner-1]
		}
		pctBar := bar(f.PrimaryPct, 10)
		pctStr := cyan(fmt.Sprintf("%.1f%%", f.PrimaryPct))

		coverageStr := ""
		switch {
		case f.Coverage <= 1:
			coverageStr = red(fmt.Sprintf("%d person", f.Coverage))
		case f.Coverage <= 2:
			coverageStr = yellow(fmt.Sprintf("%d people", f.Coverage))
		default:
			coverageStr = green(fmt.Sprintf("%d people", f.Coverage))
		}

		modCount := fmt.Sprintf("%d", len(f.Modules))

		fmt.Printf("  %s  %s  %s  %s  %s %s  %s  %s\n",
			emoji,
			pad(white(flowName), cFlow),
			pad(bfStr, cBF),
			pad(white(ownerStr), cOwner),
			pctBar, pad(pctStr, cPct),
			pad(coverageStr, cCoverage),
			grey(modCount),
		)
	}

	fmt.Println()
	fmt.Printf("  %s\n", grey("Coverage = developers with >5% of flow knowledge. Low coverage = entire feature depends on one person."))
	fmt.Println()

	// Show risky flow details
	riskyFlows := filterFlows(flows, func(f scoring.Flow) bool {
		return f.RiskLevel == "critical" || f.RiskLevel == "high"
	})

	if len(riskyFlows) > 0 {
		for _, f := range riskyFlows {
			fmt.Printf("  %s  %s  %s  %s\n",
				riskEmoji[f.RiskLevel],
				bold(white(f.Name)),
				grey("·"),
				riskColor(f.RiskLevel, fmt.Sprintf("BF %d — %s owns %.1f%%", f.BusFactor, f.PrimaryOwner, f.PrimaryPct)),
			)

			// Show modules in this flow
			limit := 5
			if len(f.Modules) < limit {
				limit = len(f.Modules)
			}
			for _, mod := range f.Modules[:limit] {
				fmt.Printf("     %s  %s\n", grey("›"), grey(mod))
			}
			if len(f.Modules) > 5 {
				fmt.Printf("     %s\n", grey(fmt.Sprintf("… and %d more modules", len(f.Modules)-5)))
			}

			// Show contributors
			if len(f.Contributors) > 1 {
				fmt.Printf("     %s  ", grey("Contributors:"))
				for i, contrib := range f.Contributors {
					if i >= 3 {
						fmt.Printf("%s", grey(fmt.Sprintf("+%d more", len(f.Contributors)-3)))
						break
					}
					if i > 0 {
						fmt.Printf("  ")
					}
					fmt.Printf("%s %s", white(contrib.Name), grey(fmt.Sprintf("(%.1f%%)", contrib.Pct)))
				}
				fmt.Println()
			}
			fmt.Println()
		}
	}
}

// ── Section 4.5: Insights ─────────────────────────────────────────────────────

func renderInsights(insights []scoring.Insight) {
	if len(insights) == 0 {
		return
	}

	header("KEY INSIGHTS", "Repository-wide patterns")
	fmt.Println()

	insightIcon := map[string]string{
		"critical": "⚠",
		"warning":  "▸",
		"info":     "ℹ",
	}

	for _, ins := range insights {
		icon := insightIcon[ins.Level]
		if icon == "" {
			icon = "·"
		}

		var iconColored, titleColored string
		switch ins.Level {
		case "critical":
			iconColored = red(icon)
			titleColored = bold(red(ins.Title))
		case "warning":
			iconColored = yellow(icon)
			titleColored = bold(yellow(ins.Title))
		default:
			iconColored = cyan(icon)
			titleColored = bold(white(ins.Title))
		}

		fmt.Printf("  %s  %s\n", iconColored, titleColored)
		if ins.Detail != "" {
			// Wrap long detail text
			words := strings.Fields(ins.Detail)
			line := "     "
			for _, w := range words {
				if len(line)+len(w)+1 > 90 {
					fmt.Printf("%s\n", grey(line))
					line = "     " + w
				} else {
					if line == "     " {
						line += w
					} else {
						line += " " + w
					}
				}
			}
			if line != "     " {
				fmt.Printf("%s\n", grey(line))
			}
		}
		if ins.Action != "" {
			fmt.Printf("     %s %s\n", cyan("→"), ins.Action)
		}
		fmt.Println()
	}
}

// ── Section 5: Alerts ─────────────────────────────────────────────────────────

func renderAlerts(alerts []scoring.Alert) {
	header("RISK ALERTS", "")

	if len(alerts) == 0 {
		fmt.Printf("\n  %s  %s\n\n", green("✓"), green("No alerts. Repository knowledge looks healthy."))
		return
	}

	fmt.Println()
	for _, alert := range alerts {
		icon := sevIcon[alert.Severity]
		sev := pad(riskColor(alert.Severity, strings.ToUpper(alert.Severity)), 10)
		title := bold(white(alert.Title))

		sevIconColored := ""
		switch alert.Severity {
		case "critical":
			sevIconColored = red(icon)
		case "high":
			sevIconColored = yellow(icon)
		case "medium":
			sevIconColored = yellow(icon)
		default:
			sevIconColored = grey(icon)
		}

		fmt.Printf("  %s  %s  %s\n", sevIconColored, sev, title)
		if alert.Detail != "" {
			fmt.Printf("           %s\n", grey(alert.Detail))
		}
		if alert.Action != "" {
			fmt.Printf("           %s %s\n", cyan("→"), alert.Action)
		}
		fmt.Println()
	}
}

// ── Main entry point ──────────────────────────────────────────────────────────

// Render prints the full terminal report with sections ordered by importance:
// Summary → Insights → Alerts → Heat Map → Bus Factor → Developers → Flows
func Render(r *scoring.Result, top int) {
	printBanner()
	fmt.Println()
	renderSummary(r)
	RenderInsights(r)
	RenderAlerts(r)
	RenderHeatmap(r, top)
	RenderBusFactor(r)
	RenderDeveloperProfiles(r)
	RenderFlowRisk(r)

	fmt.Printf("  %s\n", div())
	fmt.Printf("  %s  %s\n\n",
		grey("Full dashboard · Slack/email alerts · GitHub integration →"),
		cyan("kinlyze.com"),
	)
}

// ── Exported section renderers (for subcommands) ──────────────────────────────

// RenderInsights prints the KEY INSIGHTS section.
func RenderInsights(r *scoring.Result) { renderInsights(r.Insights) }

// RenderAlerts prints the RISK ALERTS section.
func RenderAlerts(r *scoring.Result) { renderAlerts(r.Alerts) }

// RenderHeatmap prints the KNOWLEDGE HEAT MAP section.
func RenderHeatmap(r *scoring.Result, top int) { renderHeatmap(r.Modules, top) }

// RenderBusFactor prints the BUS FACTOR ANALYSIS section.
func RenderBusFactor(r *scoring.Result) { renderBusFactor(r.Modules) }

// RenderDeveloperProfiles prints the DEVELOPER PROFILES section.
func RenderDeveloperProfiles(r *scoring.Result) { renderDeveloperProfiles(r.Developers) }

// RenderFlowRisk prints the USER FLOW RISK section.
func RenderFlowRisk(r *scoring.Result) { renderFlowRisk(r.Flows) }

// RenderSummaryOnly prints banner + summary (used by subcommands).
func RenderSummaryOnly(r *scoring.Result) {
	printBanner()
	fmt.Println()
	renderSummary(r)
}

// RenderJSON prints the result as indented JSON.
func RenderJSON(r *scoring.Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func filterModules(modules []scoring.Module, fn func(scoring.Module) bool) []scoring.Module {
	var result []scoring.Module
	for _, m := range modules {
		if fn(m) {
			result = append(result, m)
		}
	}
	return result
}

func filterFlows(flows []scoring.Flow, fn func(scoring.Flow) bool) []scoring.Flow {
	var result []scoring.Flow
	for _, f := range flows {
		if fn(f) {
			result = append(result, f)
		}
	}
	return result
}

// risk_col is an alias to avoid shadowing the package-level riskColor func.
func risk_col(level, text string) string { return riskColor(level, text) }
