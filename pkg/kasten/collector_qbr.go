package kasten

// collector_qbr.go — QBR analytics
//
// Computes:
//   1. Recovery Readiness Score (0-100, grade A-F)
//   2. App Risk Matrix (per-namespace RPO/RTO + red/yellow/green)
//   3. Weekly SLA Trend (last 12 weeks, actionable jobs only)
//   4. BP-17: no restore test ever performed

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ── 1. Recovery Readiness Score ───────────────────────────────────────────────

// computeRecoveryReadiness builds a 0-100 score from weighted components.
func computeRecoveryReadiness(d *Data) RecoveryReadinessScore {
	rrs := RecoveryReadinessScore{
		Components:    map[string]int{},
		MaxComponents: map[string]int{},
	}

	add := func(name string, earned, max int, finding string) {
		rrs.Components[name] = earned
		rrs.MaxComponents[name] = max
		if earned < max && finding != "" {
			rrs.Findings = append(rrs.Findings, finding)
		}
	}

	// ── Coverage (25 pts) ────────────────────────────────────────────────────
	covPct := d.Compliance.ProtectionCoverage
	covPts := int(math.Round(covPct / 100 * 25))
	covFinding := ""
	if covPct < 100 {
		covFinding = fmt.Sprintf("Only %.0f%% of applications are protected (%d unprotected)",
			covPct, d.Applications.Unprotected)
	}
	add("Protection coverage", covPts, 25, covFinding)

	// ── Backup recency (20 pts) ───────────────────────────────────────────────
	// Points proportional to % of protected apps with backup < 7 days
	protectedApps := 0
	recentApps := 0
	neverRun := 0
	now := time.Now().UTC()
	for _, app := range d.Applications.Apps {
		if !app.Protected {
			continue
		}
		protectedApps++
		if app.LastBackup != "" {
			t, err := time.Parse(time.RFC3339, app.LastBackup)
			if err == nil && now.Sub(t) < 7*24*time.Hour {
				recentApps++
			}
		} else {
			neverRun++
		}
	}
	recentPts := 0
	recentFinding := ""
	if protectedApps > 0 {
		recentPts = int(math.Round(float64(recentApps) / float64(protectedApps) * 20))
		switch {
		case neverRun == protectedApps:
			recentFinding = fmt.Sprintf("All %d protected apps have never been backed up — on-demand policies have not been executed", protectedApps)
		case recentApps < protectedApps:
			stale := protectedApps - recentApps - neverRun
			parts := []string{}
			if stale > 0 {
				parts = append(parts, fmt.Sprintf("%d with backup older than 7 days", stale))
			}
			if neverRun > 0 {
				parts = append(parts, fmt.Sprintf("%d never backed up", neverRun))
			}
			if len(parts) > 0 {
				recentFinding = fmt.Sprintf("%d of %d protected apps need attention: %s",
					protectedApps-recentApps, protectedApps, strings.Join(parts, ", "))
			}
		}
	}
	add("Backup recency", recentPts, 20, recentFinding)

	// ── Offsite export (15 pts) ───────────────────────────────────────────────
	exportPolicies := 0
	userPolicies := 0
	for _, p := range d.Policies {
		if p.IsSystemPolicy {
			continue
		}
		userPolicies++
		if len(p.ExportProfiles) > 0 {
			exportPolicies++
		}
	}
	exportPts := 0
	exportFinding := ""
	if userPolicies > 0 {
		exportPts = int(math.Round(float64(exportPolicies) / float64(userPolicies) * 15))
		if exportPolicies < userPolicies {
			exportFinding = fmt.Sprintf("%d of %d policies have no offsite export (3-2-1 rule)",
				userPolicies-exportPolicies, userPolicies)
		}
	}
	add("Offsite export", exportPts, 15, exportFinding)

	// ── Immutability (10 pts) ────────────────────────────────────────────────
	immutableProfiles := 0
	locationProfiles := 0
	for _, p := range d.Profiles {
		if p.Type == "Location" {
			locationProfiles++
			if p.Immutability {
				immutableProfiles++
			}
		}
	}
	immutPts := 0
	immutFinding := ""
	if locationProfiles > 0 {
		if immutableProfiles > 0 {
			immutPts = 10
		} else {
			immutFinding = "No location profiles have immutability / object lock enabled"
		}
	} else {
		immutFinding = "No location profiles configured"
	}
	add("Immutability", immutPts, 10, immutFinding)

	// ── Disaster Recovery (10 pts) ────────────────────────────────────────────
	drPts := 0
	drFinding := "Kasten DR policy not configured — K10 catalog is unprotected"
	if d.DR.Enabled {
		drPts = 10
		drFinding = ""
	}
	add("Disaster recovery", drPts, 10, drFinding)

	// ── Authentication (5 pts) ────────────────────────────────────────────────
	authPts := 0
	authFinding := "No authentication configured — K10 dashboard is open"
	switch d.Security.AuthMethod {
	case "OIDC", "OpenShift OAuth", "LDAP":
		authPts = 5
		authFinding = ""
	case "Token", "Basic":
		authPts = 3
		authFinding = fmt.Sprintf("%s authentication is not recommended for production", d.Security.AuthMethod)
	}
	add("Authentication", authPts, 5, authFinding)

	// ── Encryption (5 pts) ────────────────────────────────────────────────────
	encPts := 0
	encFinding := "Backup encryption not configured"
	if d.Security.Encryption.Enabled {
		encPts = 5
		encFinding = ""
	}
	add("Encryption", encPts, 5, encFinding)

	// ── Restore test (10 pts) ────────────────────────────────────────────────
	// Checks if any RestoreAction has ever completed successfully
	hasRestoreTest := false
	for _, j := range d.Jobs {
		if j.Action == "restore" && j.Status == "Complete" {
			hasRestoreTest = true
			break
		}
	}
	restorePts := 0
	restoreFinding := "No successful restore test found — backups have never been verified"
	if hasRestoreTest {
		restorePts = 10
		restoreFinding = ""
	}
	add("Restore test", restorePts, 10, restoreFinding)

	// ── Final score ───────────────────────────────────────────────────────────
	total := 0
	for _, v := range rrs.Components {
		total += v
	}
	rrs.Score = total
	rrs.Grade = scoreGrade(total)

	return rrs
}

func scoreGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

// ── 2. App Risk Matrix ────────────────────────────────────────────────────────

// computeAppRiskMatrix builds a risk entry for each non-system namespace.
func computeAppRiskMatrix(d *Data) []AppRisk {
	now := time.Now().UTC()

	// Build: policy name → export profiles, namespace selector
	type polInfo struct {
		hasExport    bool
		hasImmutable bool
		avgRTOSec    float64
	}
	polMap := map[string]polInfo{}
	for _, pol := range d.Policies {
		if pol.IsSystemPolicy {
			continue
		}
		pi := polInfo{hasExport: len(pol.ExportProfiles) > 0}
		// Check if any export profile is immutable
		for _, profName := range pol.ExportProfiles {
			for _, prof := range d.Profiles {
				if prof.Name == profName && prof.Immutability {
					pi.hasImmutable = true
				}
			}
		}
		// Avg RTO from job durations
		var totalSec int64
		var count int
		for _, j := range d.Jobs {
			if j.Action == "run" && j.Status == "Complete" &&
				j.PolicyName == pol.Name && j.DurationSec > 0 {
				totalSec += j.DurationSec
				count++
			}
		}
		if count > 0 {
			pi.avgRTOSec = float64(totalSec) / float64(count)
		}
		polMap[pol.Name] = pi
	}

	// Newest RestorePoint per app
	newestRP := map[string]time.Time{}
	for _, rp := range d.RestorePoints.Details {
		if t, err := time.Parse(time.RFC3339, rp.CreatedAt); err == nil {
			if prev, ok := newestRP[rp.AppName]; !ok || t.After(prev) {
				newestRP[rp.AppName] = t
			}
		}
	}

	var matrix []AppRisk
	for _, app := range d.Applications.Apps {
		risk := AppRisk{
			Namespace: app.Namespace,
			Protected: app.Protected,
		}

		if !app.Protected {
			risk.RiskLevel = "red"
			risk.RiskReasons = append(risk.RiskReasons, "No backup policy assigned")
			matrix = append(matrix, risk)
			continue
		}

		// Aggregate policy properties across all policies covering this app
		var rtoSecs []float64
		for _, polName := range app.PolicyNames {
			if pi, ok := polMap[polName]; ok {
				if pi.hasExport {
					risk.HasExport = true
				}
				if pi.hasImmutable {
					risk.HasImmutable = true
				}
				if pi.avgRTOSec > 0 {
					rtoSecs = append(rtoSecs, pi.avgRTOSec)
				}
			}
		}
		if len(rtoSecs) > 0 {
			var sum float64
			for _, s := range rtoSecs {
				sum += s
			}
			risk.RTOMinutes = math.Round(sum/float64(len(rtoSecs))/60*10) / 10
		}

		// RPO from newest RestorePoint
		if app.LastBackup != "" {
			risk.LastBackup = app.LastBackup
		}
		if t, ok := newestRP[app.Namespace]; ok {
			rpoH := now.Sub(t).Hours()
			risk.RPOHours = math.Round(rpoH*10) / 10
			if risk.LastBackup == "" {
				risk.LastBackup = t.Format(time.RFC3339)
			}
		}

		// Risk level
		isRed := false
		isYellow := false

		hasRP := risk.RPOHours > 0 || risk.LastBackup != ""
		if hasRP && risk.RPOHours > 24*7 {
			isRed = true
			risk.RiskReasons = append(risk.RiskReasons,
				fmt.Sprintf("RPO is %.0fh (last RP: %s)", risk.RPOHours,
					formatTimeShortGo(risk.LastBackup)))
		} else if hasRP && risk.RPOHours > 24 {
			isYellow = true
			risk.RiskReasons = append(risk.RiskReasons,
				fmt.Sprintf("RPO is %.0fh — consider more frequent backups", risk.RPOHours))
		} else if !hasRP {
			// No restore point yet — on-demand policy never run
			isYellow = true
			risk.RiskReasons = append(risk.RiskReasons, "No restore point found — policy never executed")
		}

		if !risk.HasExport {
			isYellow = true
			risk.RiskReasons = append(risk.RiskReasons, "No offsite export — single copy only")
		}
		if !risk.HasImmutable {
			isYellow = true
			risk.RiskReasons = append(risk.RiskReasons, "No immutable export profile")
		}

		switch {
		case isRed:
			risk.RiskLevel = "red"
		case isYellow:
			risk.RiskLevel = "yellow"
		default:
			risk.RiskLevel = "green"
		}

		matrix = append(matrix, risk)
	}

	// Sort: red first, then yellow, then green; within group alphabetical
	levelOrder := map[string]int{"red": 0, "yellow": 1, "green": 2}
	sort.Slice(matrix, func(i, j int) bool {
		li, lj := levelOrder[matrix[i].RiskLevel], levelOrder[matrix[j].RiskLevel]
		if li != lj {
			return li < lj
		}
		return matrix[i].Namespace < matrix[j].Namespace
	})
	return matrix
}

func formatTimeShortGo(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("02 Jan 2006")
}

// ── 3. Weekly SLA Trend ───────────────────────────────────────────────────────

// computeWeeklySLA groups the last 12 active weeks by ISO week number.
func computeWeeklySLA(jobs []Job) []WeeklySLA {
	type bucket struct {
		complete, failed, skipped int
	}
	byWeek := map[string]*bucket{}
	weekLabel := map[string]string{}

	for _, j := range jobs {
		if j.StartTime == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, j.StartTime)
		if err != nil {
			continue
		}
		year, week := t.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", year, week)
		if byWeek[key] == nil {
			byWeek[key] = &bucket{}
			// Label: "Mon DD" of the Monday of that week
			weekLabel[key] = isoWeekMonday(year, week).Format("Jan 2")
		}
		switch j.Status {
		case "Complete":
			byWeek[key].complete++
		case "Failed":
			byWeek[key].failed++
		case "Skipped":
			byWeek[key].skipped++
		}
	}

	// Sort keys, take last 12
	keys := make([]string, 0, len(byWeek))
	for k := range byWeek {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 12 {
		keys = keys[len(keys)-12:]
	}

	var trend []WeeklySLA
	for _, k := range keys {
		b := byWeek[k]
		actionable := b.complete + b.failed
		rate := -1.0
		if actionable > 0 {
			rate = float64(b.complete) / float64(actionable) * 100
		}
		trend = append(trend, WeeklySLA{
			Week:        k,
			Label:       weekLabel[k],
			Complete:    b.complete,
			Failed:      b.failed,
			Skipped:     b.skipped,
			SuccessRate: math.Round(rate*10) / 10,
		})
	}
	return trend
}

// isoWeekMonday returns the Monday of the given ISO year/week.
func isoWeekMonday(year, week int) time.Time {
	// Jan 4 is always in week 1
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	// Find Monday of week 1
	_, w1 := jan4.ISOWeek()
	monday := jan4.AddDate(0, 0, -int(jan4.Weekday()-time.Monday))
	if w1 > 1 {
		monday = monday.AddDate(0, 0, 7)
	}
	return monday.AddDate(0, 0, (week-1)*7)
}

// ── 4. BP-17: no restore test ─────────────────────────────────────────────────

func checkRestoreTest(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-17",
		Name:     "Restore test performed at least once",
		Severity: "critical",
	}
	for _, j := range d.Jobs {
		if j.Action == "restore" && j.Status == "Complete" {
			ch.Status = "pass"
			ch.Detail = fmt.Sprintf("At least one successful restore action found (%s)",
				formatTimeShortGo(j.StartTime))
			return ch
		}
	}
	ch.Status = "critical"
	ch.Detail = "No successful restore action found in collected jobs — backups have never been verified recoverable"
	return ch
}

// ── helpers ───────────────────────────────────────────────────────────────────

// rrsComponentsJSON serialises the RRS components for the HTML chart.
func RRSComponentsJSON(rrs RecoveryReadinessScore) string {
	order := []string{
		"Protection coverage", "Backup recency", "Offsite export",
		"Immutability", "Disaster recovery", "Authentication",
		"Encryption", "Restore test",
	}
	var labels, earned, max []string
	for _, k := range order {
		if _, ok := rrs.Components[k]; !ok {
			continue
		}
		labels = append(labels, `"`+k+`"`)
		earned = append(earned, fmt.Sprintf("%d", rrs.Components[k]))
		max = append(max, fmt.Sprintf("%d", rrs.MaxComponents[k]))
	}
	return fmt.Sprintf(`{"labels":[%s],"earned":[%s],"max":[%s]}`,
		strings.Join(labels, ","),
		strings.Join(earned, ","),
		strings.Join(max, ","))
}

// weeklySLAJSON serialises the weekly trend for Chart.js.
func WeeklySLAJSON(trend []WeeklySLA) string {
	var labels, rates, completes, faileds []string
	for _, w := range trend {
		labels = append(labels, `"`+w.Label+`"`)
		if w.SuccessRate < 0 {
			rates = append(rates, "null")
		} else {
			rates = append(rates, fmt.Sprintf("%.1f", w.SuccessRate))
		}
		completes = append(completes, fmt.Sprintf("%d", w.Complete))
		faileds = append(faileds, fmt.Sprintf("%d", w.Failed))
	}
	return fmt.Sprintf(`{"labels":[%s],"rates":[%s],"complete":[%s],"failed":[%s]}`,
		strings.Join(labels, ","),
		strings.Join(rates, ","),
		strings.Join(completes, ","),
		strings.Join(faileds, ","))
}
