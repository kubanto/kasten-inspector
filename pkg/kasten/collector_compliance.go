package kasten

import (
	"fmt"
	"time"
)

// ── Compliance ────────────────────────────────────────────────────────────────

func computeCompliance(d *Data) ComplianceInfo {
	c := ComplianceInfo{
		PolicyStatus:    map[string]string{},
		UnprotectedApps: []string{},
	}

	// Protection coverage (exclude system policies from denominator)
	if d.Applications.Total > 0 {
		c.ProtectionCoverage = float64(d.Applications.Protected) / float64(d.Applications.Total) * 100
	}
	for _, app := range d.Applications.Apps {
		if !app.Protected {
			c.UnprotectedApps = append(c.UnprotectedApps, app.Name)
		}
	}

	// Policy compliance (exclude system policies: DR, report)
	now := time.Now().UTC()
	compliant := 0
	total := 0
	for _, pol := range d.Policies {
		if pol.IsSystemPolicy {
			continue
		}
		total++
		status := "unknown"
		switch pol.LastRunStatus {
		case "Complete", "Success":
			status = "compliant"
			compliant++
		case "Failed":
			status = "non-compliant"
		default:
			if !pol.Enabled {
				status = "disabled"
			}
		}
		c.PolicyStatus[pol.Name] = status
	}
	if total > 0 {
		c.PolicyCompliance = float64(compliant) / float64(total) * 100
	}

	// Job stats
	total7d := 0
	success7d := 0
	for _, job := range d.Jobs {
		if job.StartTime == "" {
			continue
		}
		start, err := time.Parse(time.RFC3339, job.StartTime)
		if err != nil {
			continue
		}
		age := now.Sub(start)
		if age <= 24*time.Hour && job.Status == "Failed" {
			c.FailedJobs24h++
		}
		if age <= 7*24*time.Hour {
			switch job.Status {
			case "Complete", "Success":
				success7d++
				total7d++ // only count actionable outcomes (Complete + Failed)
			case "Failed":
				c.FailedJobs7d++
				total7d++
			// Skipped: excluded from denominator — not an actionable outcome
			}
		}
	}
	if total7d > 0 {
		c.SuccessRate7d = float64(success7d) / float64(total7d) * 100
	}

	return c
}

// ── Best Practices (11 checks) ────────────────────────────────────────────────

func evaluateBestPractices(d *Data) BestPractices {
	bp := BestPractices{}

	checks := []BPCheck{
		checkProtectionCoverage(d),
		checkEncryption(d),
		checkImmutableBackups(d),
		checkMultipleProfiles(d),
		checkPolicyRetention(d),
		checkAuthMethod(d),
		checkDREnabled(d),
		checkPrometheusEnabled(d),
		checkOrphanedRestorePoints(d),
		checkUnprotectedNamespaces(d),
		checkResourceLimits(d),
		// BP-12..16: ported from Kasten Disco Lite v1.9
		checkSnapshotRetentionHigh(d),
		checkSnapshotRetentionZero(d),
		checkExportRetentionExplicit(d),
		checkCSICoverage(d),
		checkBackupDrift(d),
		checkRestoreTest(d),
	}

	bp.TotalChecks = len(checks)
	bp.Checks = checks
	for _, ch := range checks {
		switch ch.Status {
		case "pass":
			bp.Passed++
		case "warning":
			bp.Warnings++
		case "critical":
			bp.Critical++
		}
	}
	return bp
}

func checkProtectionCoverage(d *Data) BPCheck {
	pct := d.Compliance.ProtectionCoverage
	ch := BPCheck{
		ID:       "BP-01",
		Name:     "Application protection coverage",
		Severity: "critical",
	}
	switch {
	case pct >= 90:
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("%.1f%% of applications are protected", pct)
	case pct >= 70:
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%.1f%% protected — %d apps without a policy", pct, d.Applications.Unprotected)
	default:
		ch.Status = "critical"
		ch.Detail = fmt.Sprintf("Only %.1f%% protected — %d apps at risk", pct, d.Applications.Unprotected)
	}
	return ch
}

func checkEncryption(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-02",
		Name:     "Backup encryption at rest",
		Severity: "critical",
	}
	if d.Security.Encryption.Enabled {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("Encryption enabled (%s)", d.Security.Encryption.Provider)
	} else {
		ch.Status = "warning"
		ch.Detail = "No encryption configured — configure AWS KMS, Azure Key Vault, HashiCorp Vault, or a K10 Passphrase"
	}
	return ch
}

func checkImmutableBackups(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-03",
		Name:     "Immutable backup storage",
		Severity: "warning",
	}
	immutableCount := 0
	for _, p := range d.Profiles {
		if p.Immutability {
			immutableCount++
		}
	}
	if immutableCount > 0 {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("%d profile(s) have immutability enabled", immutableCount)
	} else {
		ch.Status = "warning"
		ch.Detail = "No location profiles with object lock / immutability configured"
	}
	return ch
}

func checkMultipleProfiles(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-04",
		Name:     "Multiple location profiles (3-2-1 rule)",
		Severity: "warning",
	}
	locCount := 0
	for _, p := range d.Profiles {
		if p.Type == "Location" {
			locCount++
		}
	}
	switch {
	case locCount >= 2:
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("%d location profiles configured", locCount)
	case locCount == 1:
		ch.Status = "warning"
		ch.Detail = "Only 1 location profile — consider adding a secondary export target (3-2-1 rule)"
	default:
		ch.Status = "critical"
		ch.Detail = "No location profiles configured — backups cannot be exported"
	}
	return ch
}

func checkPolicyRetention(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-05",
		Name:     "Policies have export (offsite copy)",
		Severity: "warning",
	}
	withExport := 0
	userPolicies := 0
	for _, p := range d.Policies {
		if p.IsSystemPolicy {
			continue
		}
		userPolicies++
		if len(p.ExportProfiles) > 0 {
			withExport++
		}
	}
	if userPolicies == 0 {
		ch.Status = "warning"
		ch.Detail = "No user-defined policies found"
		return ch
	}
	pct := float64(withExport) / float64(userPolicies) * 100
	if pct >= 80 {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("%d/%d policies have an export action", withExport, userPolicies)
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("Only %d/%d policies export to a secondary location", withExport, userPolicies)
	}
	return ch
}

func checkAuthMethod(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-06",
		Name:     "Authentication method configured",
		Severity: "critical",
	}
	switch d.Security.AuthMethod {
	case "OIDC", "OpenShift OAuth", "LDAP":
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("Using %s authentication", d.Security.AuthMethod)
	case "Token":
		ch.Status = "warning"
		ch.Detail = "Token auth in use — consider OIDC for production"
	case "Basic":
		ch.Status = "warning"
		ch.Detail = "Basic auth in use — not recommended for production"
	default:
		ch.Status = "critical"
		ch.Detail = "No authentication configured — dashboard is open"
	}
	return ch
}

func checkDREnabled(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-07",
		Name:     "Disaster Recovery (KDR) configured",
		Severity: "warning",
	}
	if d.DR.Enabled {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("DR policy active: %s (last run: %s)", d.DR.BackupPolicy, formatTimeShort(d.DR.LastRunTime))
	} else {
		ch.Status = "warning"
		ch.Detail = "No Kasten DR policy found — K10 catalog is not being backed up"
	}
	return ch
}

func checkPrometheusEnabled(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-08",
		Name:     "Prometheus monitoring enabled",
		Severity: "warning",
	}
	if d.Prometheus.Enabled {
		ch.Status = "pass"
		ch.Detail = "Prometheus integration active"
		if d.Prometheus.GrafanaDashboard {
			ch.Detail += " with Grafana dashboard"
		}
	} else {
		ch.Status = "warning"
		ch.Detail = "No Prometheus ServiceMonitor found — backup metrics not being collected"
	}
	return ch
}

func checkOrphanedRestorePoints(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-09",
		Name:     "No orphaned restore points",
		Severity: "warning",
	}
	if d.RestorePoints.Orphaned == 0 {
		ch.Status = "pass"
		ch.Detail = "All restore points are associated with known applications"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d orphaned restore points found (consuming storage without a matching app)", d.RestorePoints.Orphaned)
	}
	return ch
}

func checkUnprotectedNamespaces(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-10",
		Name:     "All non-system namespaces protected",
		Severity: "warning",
	}
	count := len(d.Namespaces.Unprotected)
	if count == 0 {
		ch.Status = "pass"
		ch.Detail = "All non-system namespaces have backup coverage"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d non-system namespace(s) have no backup policy", count)
	}
	return ch
}

func checkResourceLimits(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-11",
		Name:     "K10 pods have resource limits",
		Severity: "warning",
	}
	noLimits := 0
	for _, dep := range d.Resources.Deployments {
		for _, cont := range dep.Containers {
			if cont.CPULimit == "" || cont.MemLimit == "" {
				noLimits++
			}
		}
	}
	if noLimits == 0 {
		ch.Status = "pass"
		ch.Detail = "All K10 containers have CPU and memory limits"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d container(s) missing resource limits — may impact cluster stability", noLimits)
	}
	return ch
}

// ── helpers ───────────────────────────────────────────────────────────────────

func formatTimeShort(ts string) string {
	if ts == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("02 Jan 2006")
}
