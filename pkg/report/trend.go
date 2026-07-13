package report

import (
	"fmt"
	"html/template"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/veeam/kasten-inspector/pkg/kasten"
)

// ── Data model ────────────────────────────────────────────────────────────────

type TrendData struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	ToolVersion string        `json:"toolVersion"`
	ClusterName string        `json:"clusterName"`
	GapHours    float64       `json:"gapHoursApart"`
	Narrative   string        `json:"narrative"`
	Before      *Data         `json:"before"`
	After       *Data         `json:"after"`
	Delta       TrendDelta    `json:"delta"`
	Changes     []TrendChange `json:"changes"`
}

type TrendDelta struct {
	ProtectionCoverage float64 `json:"protectionCoveragePP"`
	JobSuccessRate     float64 `json:"jobSuccessRatePP"`
	BPPassed           int     `json:"bestPracticesPassed"`
	BPCritical         int     `json:"bestPracticesCritical"`
	Applications       int     `json:"applications"`
	Protected          int     `json:"protected"`
	Unprotected        int     `json:"unprotected"`
	RestorePoints      int     `json:"restorePoints"`
	Policies           int     `json:"policies"`
	FailedJobs7d       int     `json:"failedJobs7d"`
	ReadinessScore     int     `json:"readinessScore"`
	CatalogFreePercent float64 `json:"catalogFreePercentPP"`
	SnapshotGrowthPct  float64 `json:"snapshotGrowthPct"`
}

// TrendChange represents one detected change, with optional detail and action.
type TrendChange struct {
	Area     string `json:"area"`
	Type     string `json:"type"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"` // explains the cause / context
	Action   string `json:"action,omitempty"` // recommended next step
	Severity string `json:"severity"`
}

// ── Compute ───────────────────────────────────────────────────────────────────

func ComputeTrend(before, after *Data) *TrendData {
	t := &TrendData{
		GeneratedAt: time.Now().UTC(),
		ToolVersion: after.ToolVersion,
		ClusterName: after.Cluster.Name,
		Before:      before,
		After:       after,
	}
	b, a := before.Kasten, after.Kasten

	gap := after.GeneratedAt.Sub(before.GeneratedAt)
	t.GapHours = gap.Hours()

	t.Delta.ProtectionCoverage = a.Compliance.ProtectionCoverage - b.Compliance.ProtectionCoverage
	t.Delta.JobSuccessRate = a.Compliance.SuccessRate7d - b.Compliance.SuccessRate7d
	t.Delta.BPPassed = a.BestPractices.Passed - b.BestPractices.Passed
	t.Delta.BPCritical = a.BestPractices.Critical - b.BestPractices.Critical
	t.Delta.Applications = a.Applications.Total - b.Applications.Total
	t.Delta.Protected = a.Applications.Protected - b.Applications.Protected
	t.Delta.Unprotected = a.Applications.Unprotected - b.Applications.Unprotected
	t.Delta.RestorePoints = a.RestorePoints.Total - b.RestorePoints.Total
	t.Delta.Policies = len(a.Policies) - len(b.Policies)
	t.Delta.FailedJobs7d = a.Compliance.FailedJobs7d - b.Compliance.FailedJobs7d
	t.Delta.ReadinessScore = a.RecoveryReadiness.Score - b.RecoveryReadiness.Score
	t.Delta.CatalogFreePercent = a.Catalog.FreePercent - b.Catalog.FreePercent
	if b.Storage.SnapshotSizeBytes > 0 {
		t.Delta.SnapshotGrowthPct = float64(a.Storage.SnapshotSizeBytes-b.Storage.SnapshotSizeBytes) / float64(b.Storage.SnapshotSizeBytes) * 100
	}

	t.Changes = append(t.Changes, trendCoverage(b, a)...)
	t.Changes = append(t.Changes, trendSecurity(b, a)...)
	t.Changes = append(t.Changes, trendPolicies(b, a)...)
	t.Changes = append(t.Changes, trendProfiles(b, a)...)
	t.Changes = append(t.Changes, trendBP(b, a)...)
	t.Changes = append(t.Changes, trendStorage(b, a)...)
	t.Changes = append(t.Changes, trendReadiness(b, a)...)
	t.Changes = append(t.Changes, trendVersion(b, a)...)

	t.Narrative = buildNarrative(t, gap)
	return t
}

// ── Change detectors ──────────────────────────────────────────────────────────

func trendCoverage(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange

	bNS := map[string]bool{}
	for _, ns := range b.Namespaces.Unprotected {
		bNS[ns.Name] = true
	}
	aNS := map[string]bool{}
	for _, ns := range a.Namespaces.Unprotected {
		aNS[ns.Name] = true
	}

	// Collect new/fixed namespace names for the aggregate message
	var newNS, fixedNS []string
	for _, ns := range a.Namespaces.Unprotected {
		if !bNS[ns.Name] {
			newNS = append(newNS, "'"+ns.Name+"'")
		}
	}
	for _, ns := range b.Namespaces.Unprotected {
		if !aNS[ns.Name] {
			fixedNS = append(fixedNS, "'"+ns.Name+"'")
		}
	}

	// Aggregate coverage delta
	d := a.Compliance.ProtectionCoverage - b.Compliance.ProtectionCoverage
	if math.Abs(d) >= 1 {
		typ, sev := "improved", "info"
		detail := ""
		if d < 0 {
			typ = "degraded"
			if d < -10 {
				sev = "critical"
			} else {
				sev = "warning"
			}
			if len(newNS) > 0 {
				detail = fmt.Sprintf("Root cause: %d new namespace(s) appeared without a backup policy: %s.", len(newNS), strings.Join(newNS, ", "))
			}
		} else {
			if len(fixedNS) > 0 {
				detail = fmt.Sprintf("Improvement driven by %d namespace(s) that are now covered by a policy: %s.", len(fixedNS), strings.Join(fixedNS, ", "))
			}
		}
		ch = append(ch, TrendChange{
			Area:     "Protection",
			Type:     typ,
			Message:  fmt.Sprintf("Coverage: %.1f%% → %.1f%% (%+.1f pp)", b.Compliance.ProtectionCoverage, a.Compliance.ProtectionCoverage, d),
			Detail:   detail,
			Severity: sev,
		})
	}

	// Per new unprotected namespace
	for _, ns := range a.Namespaces.Unprotected {
		if !bNS[ns.Name] {
			ch = append(ch, TrendChange{
				Area:    "Protection",
				Type:    "degraded",
				Message: "New unprotected namespace: " + ns.Name,
				Detail:  fmt.Sprintf("'%s' appeared in the cluster since the previous scan but is not covered by any backup policy. This is the direct cause of the coverage drop above.", ns.Name),
				Action:  fmt.Sprintf("Add '%s' to an existing policy selector, or create a dedicated backup policy for this namespace.", ns.Name),
				Severity: "warning",
			})
		}
	}
	// Per newly protected namespace
	for _, ns := range b.Namespaces.Unprotected {
		if !aNS[ns.Name] {
			ch = append(ch, TrendChange{
				Area:    "Protection",
				Type:    "improved",
				Message: "Namespace now protected: " + ns.Name,
				Detail:  fmt.Sprintf("'%s' was unprotected in the previous scan and is now covered by at least one backup policy.", ns.Name),
				Severity: "info",
			})
		}
	}

	return ch
}

func trendSecurity(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	if b.Security.AuthMethod != a.Security.AuthMethod {
		typ, sev := "improved", "info"
		detail := fmt.Sprintf("Authentication changed from '%s' to '%s'.", b.Security.AuthMethod, a.Security.AuthMethod)
		action := ""
		if a.Security.AuthMethod == "None / Passthrough" {
			typ, sev = "degraded", "critical"
			detail = "Authentication was disabled — the K10 dashboard is now open without any authentication."
			action = "Re-enable authentication immediately. Configure an auth method in K10 Settings → Authentication."
		}
		ch = append(ch, TrendChange{
			Area:     "Security",
			Type:     typ,
			Message:  fmt.Sprintf("Auth method: %s → %s", b.Security.AuthMethod, a.Security.AuthMethod),
			Detail:   detail,
			Action:   action,
			Severity: sev,
		})
	}
	if b.Security.Encryption.Enabled != a.Security.Encryption.Enabled {
		if a.Security.Encryption.Enabled {
			ch = append(ch, TrendChange{
				Area:     "Security",
				Type:     "improved",
				Message:  "Backup encryption enabled (" + a.Security.Encryption.Provider + ")",
				Detail:   "Data at rest is now encrypted. This satisfies BP-02.",
				Severity: "info",
			})
		} else {
			ch = append(ch, TrendChange{
				Area:     "Security",
				Type:     "degraded",
				Message:  "Backup encryption was disabled",
				Detail:   "Backup data is no longer encrypted at rest. This violates BP-02 and is a critical security regression.",
				Action:   "Re-enable encryption in K10 Settings → Encryption before the next backup run.",
				Severity: "critical",
			})
		}
	}
	return ch
}

func trendPolicies(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	bMap := map[string]kasten.Policy{}
	for _, p := range b.Policies {
		bMap[p.Name] = p
	}
	aMap := map[string]kasten.Policy{}
	for _, p := range a.Policies {
		aMap[p.Name] = p
	}
	for name, ap := range aMap {
		if _, ok := bMap[name]; !ok {
			ch = append(ch, TrendChange{
				Area:     "Policy",
				Type:     "new",
				Message:  "New policy created: " + name,
				Detail:   fmt.Sprintf("Policy '%s' (action: %s, frequency: %s) did not exist in the previous scan.", name, ap.Action, ap.Frequency),
				Severity: "info",
			})
		}
	}
	for name, bp := range bMap {
		ap, ok := aMap[name]
		if !ok {
			ch = append(ch, TrendChange{
				Area:     "Policy",
				Type:     "removed",
				Message:  "Policy removed: " + name,
				Detail:   fmt.Sprintf("Policy '%s' existed in the previous scan but is gone now. Any namespaces it protected may no longer have backups.", name),
				Action:   "Verify that the affected namespaces are still protected by another policy.",
				Severity: "warning",
			})
			continue
		}
		if bp.Enabled && !ap.Enabled {
			ch = append(ch, TrendChange{
				Area:     "Policy",
				Type:     "degraded",
				Message:  "Policy disabled: " + name,
				Detail:   fmt.Sprintf("Policy '%s' was active in the previous scan but is now disabled. Backups for its target namespaces have stopped.", name),
				Action:   "Re-enable the policy or confirm this was intentional.",
				Severity: "warning",
			})
		} else if !bp.Enabled && ap.Enabled {
			ch = append(ch, TrendChange{
				Area:     "Policy",
				Type:     "improved",
				Message:  "Policy re-enabled: " + name,
				Detail:   fmt.Sprintf("Policy '%s' was disabled in the previous scan and has been re-enabled.", name),
				Severity: "info",
			})
		}
	}
	return ch
}

func trendProfiles(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	bMap := map[string]kasten.Profile{}
	for _, p := range b.Profiles {
		bMap[p.Name] = p
	}
	aMap := map[string]bool{}
	for _, p := range a.Profiles {
		aMap[p.Name] = true
		bp, exists := bMap[p.Name]
		if !exists {
			ch = append(ch, TrendChange{
				Area:     "Profile",
				Type:     "new",
				Message:  "New location profile: " + p.Name,
				Detail:   fmt.Sprintf("Profile '%s' (type: %s, provider: %s) was added since the previous scan.", p.Name, p.Type, p.Provider),
				Severity: "info",
			})
			continue
		}
		if !bp.Immutability && p.Immutability {
			ch = append(ch, TrendChange{
				Area:     "Profile",
				Type:     "improved",
				Message:  fmt.Sprintf("Immutability enabled on profile: %s (%s)", p.Name, p.ImmutabilityPeriod),
				Detail:   fmt.Sprintf("Profile '%s' now has immutable backups with a %s retention lock. This satisfies BP-03.", p.Name, p.ImmutabilityPeriod),
				Severity: "info",
			})
		} else if bp.Immutability && !p.Immutability {
			ch = append(ch, TrendChange{
				Area:     "Profile",
				Type:     "degraded",
				Message:  "Immutability removed from profile: " + p.Name,
				Detail:   fmt.Sprintf("Profile '%s' had immutability enabled before but it is now disabled. Backups to this profile can be deleted or modified.", p.Name),
				Action:   "Re-enable immutability on the profile if this was unintentional.",
				Severity: "critical",
			})
		}
	}
	for _, bp := range b.Profiles {
		if !aMap[bp.Name] {
			ch = append(ch, TrendChange{
				Area:     "Profile",
				Type:     "removed",
				Message:  "Profile removed: " + bp.Name,
				Detail:   fmt.Sprintf("Profile '%s' existed in the previous scan but is gone. Any policies using it as export target will fail.", bp.Name),
				Action:   "Check policies that referenced this profile and update them to use a valid target.",
				Severity: "warning",
			})
		}
	}
	return ch
}

func trendBP(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange

	bMap := map[string]kasten.BPCheck{}
	for _, c := range b.BestPractices.Checks {
		bMap[c.ID] = c
	}
	for _, ac := range a.BestPractices.Checks {
		bc, exists := bMap[ac.ID]
		if !exists {
			continue
		}
		if bc.Status == "pass" && ac.Status != "pass" {
			sev := "warning"
			if ac.Severity == "critical" {
				sev = "critical"
			}
			ch = append(ch, TrendChange{
				Area:     "Best Practices",
				Type:     "degraded",
				Message:  fmt.Sprintf("%s %s: pass → %s", ac.ID, ac.Name, ac.Status),
				Detail:   ac.Detail,
				Severity: sev,
			})
		} else if bc.Status != "pass" && ac.Status == "pass" {
			ch = append(ch, TrendChange{
				Area:     "Best Practices",
				Type:     "improved",
				Message:  fmt.Sprintf("%s %s: %s → pass", ac.ID, ac.Name, bc.Status),
				Detail:   ac.Detail,
				Severity: "info",
			})
		}
	}

	// Fallback summary when no individual checks changed but counts differ
	cd := a.BestPractices.Critical - b.BestPractices.Critical
	if len(ch) == 0 && cd != 0 {
		if cd > 0 {
			ch = append(ch, TrendChange{
				Area:     "Best Practices",
				Type:     "degraded",
				Message:  fmt.Sprintf("%d new critical issue(s) detected (total: %d)", cd, a.BestPractices.Critical),
				Detail:   "The individual check that regressed could not be matched. Review the Best Practices section of the full report.",
				Severity: "critical",
			})
		} else {
			ch = append(ch, TrendChange{
				Area:     "Best Practices",
				Type:     "improved",
				Message:  fmt.Sprintf("%d critical issue(s) resolved (remaining: %d)", -cd, a.BestPractices.Critical),
				Severity: "info",
			})
		}
	}
	return ch
}

func trendStorage(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange

	if b.Storage.SnapshotSizeBytes > 0 && a.Storage.SnapshotSizeBytes > 0 {
		pct := float64(a.Storage.SnapshotSizeBytes-b.Storage.SnapshotSizeBytes) / float64(b.Storage.SnapshotSizeBytes) * 100
		if pct >= 50 {
			ch = append(ch, TrendChange{
				Area:    "Storage",
				Type:    "degraded",
				Message: fmt.Sprintf("Snapshot storage grew +%.0f%%: %s → %s", pct, b.Storage.SnapshotSizeHuman, a.Storage.SnapshotSizeHuman),
				Detail:  "Snapshot storage increased significantly between scans. This could indicate new applications being backed up, longer retention, or larger workloads.",
				Action:  "Review retention policies to ensure they are within intended bounds. Consider whether the growth is expected.",
				Severity: "warning",
			})
		} else if pct <= -20 {
			ch = append(ch, TrendChange{
				Area:    "Storage",
				Type:    "improved",
				Message: fmt.Sprintf("Snapshot storage reduced %.0f%%: %s → %s", -pct, b.Storage.SnapshotSizeHuman, a.Storage.SnapshotSizeHuman),
				Detail:  "Snapshot storage decreased, likely due to retention cleanup or policy changes.",
				Severity: "info",
			})
		}
	}

	if b.RestorePoints.Orphaned == 0 && a.RestorePoints.Orphaned > 0 {
		ch = append(ch, TrendChange{
			Area:    "Storage",
			Type:    "degraded",
			Message: fmt.Sprintf("%d new orphaned restore point(s)", a.RestorePoints.Orphaned),
			Detail:  "Orphaned restore points consume storage but cannot be used for recovery because their source application no longer exists.",
			Action:  "Remove orphaned restore points via K10 → Restore Points → filter by 'Orphaned'.",
			Severity: "warning",
		})
	} else if b.RestorePoints.Orphaned > 0 && a.RestorePoints.Orphaned == 0 {
		ch = append(ch, TrendChange{
			Area:    "Storage",
			Type:    "improved",
			Message: "All orphaned restore points cleaned up",
			Detail:  fmt.Sprintf("%d orphaned restore points that existed in the previous scan have been removed.", b.RestorePoints.Orphaned),
			Severity: "info",
		})
	}

	// Catalog free space (only when data is available)
	if a.Catalog.FreePercent > 0 && b.Catalog.FreePercent > 0 {
		diff := a.Catalog.FreePercent - b.Catalog.FreePercent
		if diff <= -10 {
			sev := "warning"
			if a.Catalog.FreePercent < 30 {
				sev = "critical"
			}
			ch = append(ch, TrendChange{
				Area:    "Storage",
				Type:    "degraded",
				Message: fmt.Sprintf("Catalog free space dropped: %.0f%% → %.0f%%", b.Catalog.FreePercent, a.Catalog.FreePercent),
				Detail:  fmt.Sprintf("The K10 catalog PVC now has %.0f%% free space. Upgrades require at least 50%% free — below that, the upgrade will fail.", a.Catalog.FreePercent),
				Action:  "Reduce retention in system policies or expand the catalog PVC before attempting any K10 upgrade.",
				Severity: sev,
			})
		}
	}

	return ch
}

func trendReadiness(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	diff := a.RecoveryReadiness.Score - b.RecoveryReadiness.Score
	if diff == 0 {
		return ch
	}

	dir := "improved"
	typ := "improved"
	sev := "info"
	if diff < 0 {
		dir = "dropped"
		typ = "degraded"
		if diff <= -15 {
			sev = "critical"
		} else if diff <= -5 {
			sev = "warning"
		}
	}

	// Build component breakdown
	var compChanges []string
	comps := make([]string, 0, len(a.RecoveryReadiness.Components))
	for k := range a.RecoveryReadiness.Components {
		comps = append(comps, k)
	}
	sort.Strings(comps)
	for _, comp := range comps {
		aScore := a.RecoveryReadiness.Components[comp]
		if bScore, ok := b.RecoveryReadiness.Components[comp]; ok && aScore != bScore {
			compChanges = append(compChanges, fmt.Sprintf("%s: %d → %d pts", comp, bScore, aScore))
		}
	}

	detail := ""
	if len(compChanges) > 0 {
		detail = "Component changes: " + strings.Join(compChanges, "; ") + "."
	}

	absD := diff
	if absD < 0 {
		absD = -absD
	}
	ch = append(ch, TrendChange{
		Area:     "Readiness",
		Type:     typ,
		Message:  fmt.Sprintf("Recovery Readiness Score %s %d pt(s): %d (%s) → %d (%s)", dir, absD, b.RecoveryReadiness.Score, b.RecoveryReadiness.Grade, a.RecoveryReadiness.Score, a.RecoveryReadiness.Grade),
		Detail:   detail,
		Severity: sev,
	})
	return ch
}

func trendVersion(b, a *kasten.Data) []TrendChange {
	if b.Version != a.Version && b.Version != "unknown" && a.Version != "unknown" {
		return []TrendChange{{
			Area:     "K10",
			Type:     "new",
			Message:  fmt.Sprintf("K10 upgraded: %s → %s", b.Version, a.Version),
			Detail:   "A new version of Kasten K10 was deployed between the two scans.",
			Severity: "info",
		}}
	}
	return nil
}

// ── Narrative builder ─────────────────────────────────────────────────────────

func buildNarrative(t *TrendData, gap time.Duration) string {
	b, a := t.Before.Kasten, t.After.Kasten
	var parts []string

	// Coverage
	covDelta := a.Compliance.ProtectionCoverage - b.Compliance.ProtectionCoverage
	if math.Abs(covDelta) >= 1 {
		bNS := map[string]bool{}
		for _, ns := range b.Namespaces.Unprotected {
			bNS[ns.Name] = true
		}
		var newNS []string
		for _, ns := range a.Namespaces.Unprotected {
			if !bNS[ns.Name] {
				newNS = append(newNS, "'"+ns.Name+"'")
			}
		}
		cause := ""
		if len(newNS) > 0 {
			cause = fmt.Sprintf(" (root cause: %d new unprotected namespace(s): %s)", len(newNS), strings.Join(newNS, ", "))
		}
		dir := "improved"
		if covDelta < 0 {
			dir = "dropped"
		}
		parts = append(parts, fmt.Sprintf("protection coverage %s %.1f pp (%.0f%% → %.0f%%)%s",
			dir, math.Abs(covDelta), b.Compliance.ProtectionCoverage, a.Compliance.ProtectionCoverage, cause))
	}

	// Best practices
	bpCritDelta := a.BestPractices.Critical - b.BestPractices.Critical
	bpPassDelta := a.BestPractices.Passed - b.BestPractices.Passed
	if bpCritDelta > 0 {
		parts = append(parts, fmt.Sprintf("%d new critical BP issue(s) appeared — total now: %d critical, %d of %d checks passing",
			bpCritDelta, a.BestPractices.Critical, a.BestPractices.Passed, a.BestPractices.TotalChecks))
	} else if bpCritDelta < 0 {
		parts = append(parts, fmt.Sprintf("%d critical BP issue(s) resolved — total now: %d critical, %d of %d checks passing",
			-bpCritDelta, a.BestPractices.Critical, a.BestPractices.Passed, a.BestPractices.TotalChecks))
	} else if bpPassDelta != 0 {
		dir := "improved"
		if bpPassDelta < 0 {
			dir = "degraded"
		}
		parts = append(parts, fmt.Sprintf("BP score %s: now %d of %d checks passing (%+d)", dir, a.BestPractices.Passed, a.BestPractices.TotalChecks, bpPassDelta))
	}

	// Readiness score
	rDelta := a.RecoveryReadiness.Score - b.RecoveryReadiness.Score
	if rDelta != 0 {
		dir := "improved"
		if rDelta < 0 {
			dir = "fell"
		}
		absR := rDelta
		if absR < 0 {
			absR = -absR
		}
		parts = append(parts, fmt.Sprintf("Recovery Readiness Score %s %d pt(s) to %d/100 (grade %s)",
			dir, absR, a.RecoveryReadiness.Score, a.RecoveryReadiness.Grade))
	}

	// Security
	if b.Security.AuthMethod != a.Security.AuthMethod {
		parts = append(parts, fmt.Sprintf("authentication method changed ('%s' → '%s')", b.Security.AuthMethod, a.Security.AuthMethod))
	}

	// Storage
	if b.Storage.SnapshotSizeBytes > 0 && a.Storage.SnapshotSizeBytes > 0 {
		pct := float64(a.Storage.SnapshotSizeBytes-b.Storage.SnapshotSizeBytes) / float64(b.Storage.SnapshotSizeBytes) * 100
		if math.Abs(pct) >= 10 {
			dir := "grew"
			if pct < 0 {
				dir = "shrank"
			}
			parts = append(parts, fmt.Sprintf("snapshot storage %s %.0f%% (%s → %s)", dir, math.Abs(pct), b.Storage.SnapshotSizeHuman, a.Storage.SnapshotSizeHuman))
		}
	}

	// Failed jobs
	jobDelta := a.Compliance.FailedJobs7d - b.Compliance.FailedJobs7d
	if jobDelta > 0 {
		parts = append(parts, fmt.Sprintf("%d more failed job(s) in the last 7 days (total: %d)", jobDelta, a.Compliance.FailedJobs7d))
	} else if jobDelta < 0 {
		parts = append(parts, fmt.Sprintf("%d fewer failed job(s) in the last 7 days (total: %d)", -jobDelta, a.Compliance.FailedJobs7d))
	}

	// Stable items
	var stable []string
	if math.Abs(a.Compliance.SuccessRate7d-b.Compliance.SuccessRate7d) < 1 {
		stable = append(stable, fmt.Sprintf("job success rate (%.0f%%)", a.Compliance.SuccessRate7d))
	}
	if a.RestorePoints.Total == b.RestorePoints.Total {
		stable = append(stable, fmt.Sprintf("restore points (%d)", a.RestorePoints.Total))
	}
	if len(a.Policies) == len(b.Policies) {
		stable = append(stable, fmt.Sprintf("policies (%d)", len(a.Policies)))
	}

	// Compose
	var sb strings.Builder
	if len(parts) == 0 {
		sb.WriteString("No significant changes detected between the two snapshots.")
	} else {
		// Capitalize first part
		first := parts[0]
		if len(first) > 0 {
			first = strings.ToUpper(string([]rune(first)[:1])) + string([]rune(first)[1:])
		}
		sb.WriteString(first)
		for _, p := range parts[1:] {
			sb.WriteString("; ")
			sb.WriteString(p)
		}
		sb.WriteString(".")
		if len(stable) > 0 {
			sb.WriteString(" Unchanged: ")
			sb.WriteString(strings.Join(stable, ", "))
			sb.WriteString(".")
		}
	}

	return sb.String()
}

// ── HTML writer ───────────────────────────────────────────────────────────────

func WriteTrendHTML(path string, d *TrendData) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	funcMap := template.FuncMap{
		"fmtDate": func(t time.Time) string { return t.Format("02 Jan 2006") },
		"fmtDateTime": func(t time.Time) string { return t.Format("02 Jan 2006 · 15:04 UTC") },
		"fmtTime": func(t time.Time) string { return t.Format("02 Jan 2006 15:04 UTC") },
		"fmtTimeOnly": func(t time.Time) string { return t.Format("15:04 UTC") },
		"gapStr": func(h float64) string {
			if h < 1 {
				return fmt.Sprintf("%.0f minutes", h*60)
			}
			if h < 24 {
				return fmt.Sprintf("%.1f hours", h)
			}
			return fmt.Sprintf("%.1f days", h/24)
		},
		"gapUnder24h": func(h float64) bool { return h < 24 },
		"delta": func(v float64) string {
			if v > 0 {
				return fmt.Sprintf("+%.1f", v)
			}
			return fmt.Sprintf("%.1f", v)
		},
		"deltaInt": func(v int) string {
			if v > 0 {
				return fmt.Sprintf("+%d", v)
			}
			return fmt.Sprintf("%d", v)
		},
		"deltaClass": func(v float64, positiveIsGood bool) string {
			if v == 0 {
				return "td-neutral"
			}
			good := v > 0
			if !positiveIsGood {
				good = !good
			}
			if good {
				return "td-good"
			}
			return "td-bad"
		},
		"deltaClassInt": func(v int, positiveIsGood bool) string {
			if v == 0 {
				return "td-neutral"
			}
			good := v > 0
			if !positiveIsGood {
				good = !good
			}
			if good {
				return "td-good"
			}
			return "td-bad"
		},
		"changeClass": func(severity string) string {
			switch severity {
			case "critical":
				return "change-critical"
			case "warning":
				return "change-warning"
			default:
				return "change-info"
			}
		},
		"changeIcon": func(typ string) string {
			switch typ {
			case "improved":
				return "↑"
			case "degraded":
				return "↓"
			case "new":
				return "+"
			case "removed":
				return "−"
			default:
				return "~"
			}
		},
		"pct": func(f float64) string { return fmt.Sprintf("%.1f%%", f) },
		"hasCatalogData": func(before, after *Data) bool {
			return before.Kasten.Catalog.FreePercent > 0 && after.Kasten.Catalog.FreePercent > 0
		},
		"abs": func(v float64) float64 {
			if v < 0 {
				return -v
			}
			return v
		},
	}

	tmpl, err := template.New("trend").Funcs(funcMap).Parse(trendHTMLTmpl)
	if err != nil {
		return fmt.Errorf("parsing trend template: %w", err)
	}
	return tmpl.Execute(f, d)
}

var trendHTMLTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%23FFB800'/%3E%3Ctext x='16' y='22' text-anchor='middle' font-family='monospace' font-weight='700' font-size='13' fill='%23000'%3EK10%3C/text%3E%3C/svg%3E">
<title>K10 Trend — {{.ClusterName}}</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap');
:root{
  --bg:#0d1117;--s1:#161b22;--s2:#1c2230;--b:#30363d;
  --t:#e6edf3;--tm:#8b949e;
  --green:#3fb950;--red:#f85149;--yellow:#d29922;
  --blue:#58a6ff;--kasten:#FFB800;
}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'IBM Plex Sans',sans-serif;background:var(--bg);color:var(--t);font-size:14px;line-height:1.6}
.page{max-width:1100px;margin:0 auto;padding:0 24px 80px}
.hdr{border-bottom:1px solid var(--b);padding:24px 0 18px;display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:12px}
.logo{width:38px;height:38px;border-radius:8px;background:var(--kasten);display:flex;align-items:center;justify-content:center;font-family:'IBM Plex Mono',monospace;font-weight:700;font-size:14px;color:#000}
.hdr-brand{display:flex;align-items:center;gap:12px}
.hdr-title h1{font-size:18px;font-weight:600}
.hdr-title p{color:var(--tm);font-size:12px;margin-top:2px}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;font-family:'IBM Plex Mono',monospace;background:rgba(255,184,0,.12);color:var(--kasten);border:1px solid rgba(255,184,0,.3);margin-top:4px}
.sec{margin-top:28px}
.sec-title{font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:12px}
/* Period bar */
.period-bar{display:grid;grid-template-columns:1fr 60px 1fr;gap:0;background:var(--s1);border:1px solid var(--b);border-radius:10px;overflow:hidden;margin-bottom:20px}
.period-side{padding:16px 20px}
.period-side.before{border-right:1px solid var(--b)}
.period-label{font-size:10px;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:4px}
.period-date{font-size:15px;font-weight:600;font-family:'IBM Plex Mono',monospace;color:var(--t)}
.period-time{font-size:11px;color:var(--tm);margin-top:1px;font-family:'IBM Plex Mono',monospace}
.period-cluster{font-size:11px;color:var(--tm);margin-top:4px}
.period-arrow{display:flex;align-items:center;justify-content:center;font-size:20px;color:var(--kasten)}
/* Narrative */
.narrative{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:18px 22px;font-size:13px;line-height:1.75;color:var(--t)}
.narrative-gap{margin-top:12px;padding:10px 14px;background:rgba(210,153,34,.07);border:1px solid rgba(210,153,34,.2);border-radius:6px;font-size:12px;color:var(--yellow)}
/* Delta grid */
.delta-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;margin-bottom:20px}
.dcard{background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:14px 16px}
.dcard-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:6px}
.dcard-val{font-size:22px;font-weight:700;font-family:'IBM Plex Mono',monospace;line-height:1}
.dcard-sub{font-size:11px;color:var(--tm);margin-top:4px}
.td-good{color:var(--green)}
.td-bad{color:var(--red)}
.td-neutral{color:var(--tm)}
/* Changes list */
.changes{display:flex;flex-direction:column;gap:8px}
.change{padding:12px 16px;background:var(--s1);border-radius:8px;border-left:3px solid transparent}
.change-info{border-left-color:var(--blue)}
.change-warning{border-left-color:var(--yellow)}
.change-critical{border-left-color:var(--red)}
.change-header{display:flex;align-items:flex-start;gap:10px}
.change-icon{font-size:13px;font-weight:700;min-width:18px;margin-top:1px}
.change-info .change-icon{color:var(--blue)}
.change-warning .change-icon{color:var(--yellow)}
.change-critical .change-icon{color:var(--red)}
.change-area{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);min-width:96px;margin-top:2px}
.change-msg{font-size:13px;font-weight:500;flex:1}
.change-detail{font-size:12px;color:var(--tm);margin-top:6px;padding-left:28px;line-height:1.5}
.change-action{font-size:12px;color:var(--blue);margin-top:4px;padding-left:28px;font-style:italic}
.change-action::before{content:"→ "}
/* Tables */
.twrap{background:var(--s1);border:1px solid var(--b);border-radius:10px;overflow:hidden;margin-top:12px}
table{width:100%;border-collapse:collapse}
thead th{background:var(--s2);padding:8px 12px;text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);border-bottom:1px solid var(--b)}
tbody tr{border-bottom:1px solid var(--b)}
tbody tr:last-child{border-bottom:none}
tbody tr:hover{background:var(--s2)}
td{padding:8px 12px;font-size:13px}
.mono{font-family:'IBM Plex Mono',monospace;font-size:12px}
.muted{color:var(--tm)}
/* BP grid */
.bp-grid{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:12px}
.bp-row{display:flex;align-items:center;gap:8px;padding:8px 12px;background:var(--s1);border-radius:6px;font-size:11px}
.bp-row.bp-changed{background:var(--s2);outline:1px solid var(--b)}
.bp-id{font-family:'IBM Plex Mono',monospace;font-size:10px;color:var(--tm);min-width:38px}
.bp-name{flex:1;color:var(--t)}
.bp-before,.bp-after{min-width:60px;text-align:center;font-size:10px;font-weight:600;padding:2px 6px;border-radius:20px}
.bp-pass{background:rgba(63,185,80,.12);color:var(--green)}
.bp-fail{background:rgba(248,81,73,.12);color:var(--red)}
.bp-warn{background:rgba(210,153,34,.12);color:var(--yellow)}
.bp-critical{background:rgba(248,81,73,.12);color:var(--red)}
.bp-arrow{color:var(--tm);font-size:10px}
.footer{margin-top:48px;padding-top:16px;border-top:1px solid var(--b);font-size:11px;color:var(--tm);display:flex;justify-content:space-between}
@media(max-width:700px){.bp-grid{grid-template-columns:1fr}.period-bar{grid-template-columns:1fr}}
</style>
</head>
<body>
<div class="page">

<!-- Header -->
<div class="hdr">
  <div class="hdr-brand">
    <div class="logo">K10</div>
    <div class="hdr-title">
      <h1>Kasten K10 — Trend Report</h1>
      <p>Period-over-period comparison · {{.ClusterName}}</p>
    </div>
  </div>
  <div>
    <div style="font-size:11px;color:var(--tm)">Generated {{fmtTime .GeneratedAt}}</div>
    <span class="badge">v{{.ToolVersion}}</span>
  </div>
</div>

<!-- Period bar -->
<div class="sec">
  <div class="period-bar">
    <div class="period-side before">
      <div class="period-label">Before</div>
      <div class="period-date">{{fmtDate .Before.GeneratedAt}}</div>
      <div class="period-time">{{fmtTimeOnly .Before.GeneratedAt}}</div>
      <div class="period-cluster">{{.Before.Cluster.Name}} · K10 {{.Before.Kasten.Version}}</div>
    </div>
    <div class="period-arrow">→</div>
    <div class="period-side">
      <div class="period-label">After</div>
      <div class="period-date">{{fmtDate .After.GeneratedAt}}</div>
      <div class="period-time">{{fmtTimeOnly .After.GeneratedAt}}</div>
      <div class="period-cluster">{{.After.Cluster.Name}} · K10 {{.After.Kasten.Version}}</div>
    </div>
  </div>
</div>

<!-- Executive summary / narrative -->
<div class="sec">
  <div class="sec-title">Executive summary</div>
  <div class="narrative">
    {{.Narrative}}
    {{if gapUnder24h .GapHours}}
    <div class="narrative-gap">⚠ These reports are only {{gapStr .GapHours}} apart. For meaningful trend analysis, compare snapshots taken days or weeks apart — short gaps will show only near-real-time cluster changes.</div>
    {{end}}
  </div>
</div>

<!-- Delta KPIs -->
<div class="sec">
  <div class="sec-title">Key metrics — period delta</div>
  <div class="delta-grid">
    <div class="dcard">
      <div class="dcard-label">Protection coverage</div>
      <div class="dcard-val {{deltaClass .Delta.ProtectionCoverage true}}">{{delta .Delta.ProtectionCoverage}} pp</div>
      <div class="dcard-sub">{{pct .Before.Kasten.Compliance.ProtectionCoverage}} → {{pct .After.Kasten.Compliance.ProtectionCoverage}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Job success rate</div>
      <div class="dcard-val {{deltaClass .Delta.JobSuccessRate true}}">{{delta .Delta.JobSuccessRate}} pp</div>
      <div class="dcard-sub">{{pct .Before.Kasten.Compliance.SuccessRate7d}} → {{pct .After.Kasten.Compliance.SuccessRate7d}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">BP checks passed</div>
      <div class="dcard-val {{deltaClassInt .Delta.BPPassed true}}">{{deltaInt .Delta.BPPassed}}</div>
      <div class="dcard-sub">{{.Before.Kasten.BestPractices.Passed}} → {{.After.Kasten.BestPractices.Passed}} of {{.After.Kasten.BestPractices.TotalChecks}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Critical issues</div>
      <div class="dcard-val {{deltaClassInt .Delta.BPCritical false}}">{{deltaInt .Delta.BPCritical}}</div>
      <div class="dcard-sub">{{.Before.Kasten.BestPractices.Critical}} → {{.After.Kasten.BestPractices.Critical}} critical</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Protected apps</div>
      <div class="dcard-val {{deltaClassInt .Delta.Protected true}}">{{deltaInt .Delta.Protected}}</div>
      <div class="dcard-sub">{{.Before.Kasten.Applications.Protected}} → {{.After.Kasten.Applications.Protected}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Failed jobs (7d)</div>
      <div class="dcard-val {{deltaClassInt .Delta.FailedJobs7d false}}">{{deltaInt .Delta.FailedJobs7d}}</div>
      <div class="dcard-sub">{{.Before.Kasten.Compliance.FailedJobs7d}} → {{.After.Kasten.Compliance.FailedJobs7d}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Restore points</div>
      <div class="dcard-val {{deltaClassInt .Delta.RestorePoints true}}">{{deltaInt .Delta.RestorePoints}}</div>
      <div class="dcard-sub">{{.Before.Kasten.RestorePoints.Total}} → {{.After.Kasten.RestorePoints.Total}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Policies</div>
      <div class="dcard-val td-neutral">{{deltaInt .Delta.Policies}}</div>
      <div class="dcard-sub">{{len .Before.Kasten.Policies}} → {{len .After.Kasten.Policies}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Readiness Score</div>
      <div class="dcard-val {{deltaClassInt .Delta.ReadinessScore true}}">{{deltaInt .Delta.ReadinessScore}}</div>
      <div class="dcard-sub">{{.Before.Kasten.RecoveryReadiness.Score}} ({{.Before.Kasten.RecoveryReadiness.Grade}}) → {{.After.Kasten.RecoveryReadiness.Score}} ({{.After.Kasten.RecoveryReadiness.Grade}})</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Catalog free space</div>
      {{if hasCatalogData .Before .After}}
      <div class="dcard-val {{deltaClass .Delta.CatalogFreePercent true}}">{{delta .Delta.CatalogFreePercent}} pp</div>
      <div class="dcard-sub">{{pct .Before.Kasten.Catalog.FreePercent}} → {{pct .After.Kasten.Catalog.FreePercent}}</div>
      {{else}}
      <div class="dcard-val td-neutral">N/A</div>
      <div class="dcard-sub">Catalog usage data unavailable</div>
      {{end}}
    </div>
    <div class="dcard">
      <div class="dcard-label">Snapshot growth</div>
      <div class="dcard-val {{deltaClass .Delta.SnapshotGrowthPct false}}">{{delta .Delta.SnapshotGrowthPct}}%</div>
      <div class="dcard-sub">{{.Before.Kasten.Storage.SnapshotSizeHuman}} → {{.After.Kasten.Storage.SnapshotSizeHuman}}</div>
    </div>
  </div>
</div>

<!-- Notable changes -->
<div class="sec">
  <div class="sec-title">Notable changes ({{len .Changes}})</div>
  {{if .Changes}}
  <div class="changes">
    {{range .Changes}}
    <div class="change {{changeClass .Severity}}">
      <div class="change-header">
        <span class="change-icon">{{changeIcon .Type}}</span>
        <span class="change-area">{{.Area}}</span>
        <span class="change-msg">{{.Message}}</span>
      </div>
      {{if .Detail}}<div class="change-detail">{{.Detail}}</div>{{end}}
      {{if .Action}}<div class="change-action">{{.Action}}</div>{{end}}
    </div>
    {{end}}
  </div>
  {{else}}
  <div style="padding:20px;text-align:center;color:var(--tm);font-size:12px;background:var(--s1);border-radius:8px">No significant changes detected between the two reports.</div>
  {{end}}
</div>

<!-- Best practices comparison -->
<div class="sec">
  <div class="sec-title">Best practices — before vs after</div>
  <div class="bp-grid">
    {{range $i, $a := .After.Kasten.BestPractices.Checks}}
    {{$b := index $.Before.Kasten.BestPractices.Checks $i}}
    {{$changed := false}}
    {{if $b}}{{if ne $b.Status $a.Status}}{{$changed = true}}{{end}}{{end}}
    <div class="bp-row{{if $changed}} bp-changed{{end}}">
      <span class="bp-id">{{$a.ID}}</span>
      <span class="bp-name">{{$a.Name}}</span>
      {{if $b}}<span class="bp-before bp-{{$b.Status}}">{{$b.Status}}</span>
      <span class="bp-arrow">→</span>{{end}}
      <span class="bp-after bp-{{$a.Status}}">{{$a.Status}}</span>
    </div>
    {{end}}
  </div>
</div>

<!-- Policy comparison -->
<div class="sec">
  <div class="sec-title">Policy comparison</div>
  <div class="twrap">
    <table>
      <thead><tr><th>Policy</th><th>Before — Enabled</th><th>Before — Last Run</th><th>After — Enabled</th><th>After — Last Run</th><th>Change</th></tr></thead>
      <tbody>
        {{$bPolicies := .Before.Kasten.Policies}}
        {{range .After.Kasten.Policies}}
        {{$name := .Name}}{{$aEnabled := .Enabled}}{{$aRun := .LastRunTime}}
        {{$bEnabled := false}}{{$bRun := ""}}{{$found := false}}
        {{range $bPolicies}}{{if eq .Name $name}}{{$bEnabled = .Enabled}}{{$bRun = .LastRunTime}}{{$found = true}}{{end}}{{end}}
        <tr>
          <td class="mono" style="font-size:11px">{{.Name}}</td>
          <td>{{if $found}}{{if $bEnabled}}<span style="color:var(--green)">✓</span>{{else}}<span style="color:var(--red)">✗</span>{{end}}{{else}}<span style="color:var(--blue)">new</span>{{end}}</td>
          <td class="mono muted" style="font-size:11px">{{if $bRun}}{{$bRun | printf "%.10s"}}{{else}}—{{end}}</td>
          <td>{{if $aEnabled}}<span style="color:var(--green)">✓</span>{{else}}<span style="color:var(--red)">✗</span>{{end}}</td>
          <td class="mono muted" style="font-size:11px">{{if $aRun}}{{$aRun | printf "%.10s"}}{{else}}never{{end}}</td>
          <td style="font-size:11px">
            {{if not $found}}<span style="color:var(--blue)">+ added</span>
            {{else if and $bEnabled (not $aEnabled)}}<span style="color:var(--red)">disabled</span>
            {{else if and (not $bEnabled) $aEnabled}}<span style="color:var(--green)">re-enabled</span>
            {{else}}<span style="color:var(--tm)">—</span>{{end}}
          </td>
        </tr>{{end}}
        {{range .Before.Kasten.Policies}}
        {{$name := .Name}}{{$found := false}}
        {{range $.After.Kasten.Policies}}{{if eq .Name $name}}{{$found = true}}{{end}}{{end}}
        {{if not $found}}<tr>
          <td class="mono" style="font-size:11px;text-decoration:line-through;color:var(--tm)">{{.Name}}</td>
          <td>{{if .Enabled}}<span style="color:var(--green)">✓</span>{{else}}<span style="color:var(--red)">✗</span>{{end}}</td>
          <td class="mono muted" style="font-size:11px">{{if .LastRunTime}}{{.LastRunTime | printf "%.10s"}}{{else}}—{{end}}</td>
          <td colspan="2" style="color:var(--tm);font-size:11px">— removed —</td>
          <td><span style="color:var(--yellow)">− removed</span></td>
        </tr>{{end}}{{end}}
      </tbody>
    </table>
  </div>
</div>

<div class="footer">
  <span>Kasten K10 Inspector v{{.ToolVersion}}</span>
  <span>{{fmtTime .GeneratedAt}}</span>
</div>

</div>
</body>
</html>`
