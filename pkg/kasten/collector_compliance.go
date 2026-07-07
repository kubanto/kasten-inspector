package kasten

import (
	"fmt"
	"strings"
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

// ── Best Practices (25 checks) ────────────────────────────────────────────────

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
		// BP-12..17: ported from Kasten Disco Lite v1.9
		checkSnapshotRetentionHigh(d),
		checkSnapshotRetentionZero(d),
		checkExportRetentionExplicit(d),
		checkCSICoverage(d),
		checkBackupDrift(d),
		checkRestoreTest(d),
		// BP-18..25: from Kasten Best Practices guide
		checkIngressHTTPS(d),
		checkVSCKastenAnnotation(d),
		checkNoWildcardSelector(d),
		checkClusterScopedPolicy(d),
		checkObjectStorageProfile(d),
		checkPolicyPresets(d),
		checkCatalogSpace(d),
		checkPrometheusAlerts(d),
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

// ── BP-18..25: Kasten Best Practices guide ────────────────────────────────────

func checkIngressHTTPS(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-18",
		Name:     "Dashboard exposed via Ingress with HTTPS",
		Severity: "warning",
	}
	access := d.HelmConfig.DashboardAccess
	switch {
	case strings.Contains(access, "Ingress") && d.HelmConfig.IngressTLS:
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("Dashboard accessible via %s with TLS", access)
	case strings.Contains(access, "Ingress") && !d.HelmConfig.IngressTLS:
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("Ingress found (%s) but no TLS configured — expose the dashboard over HTTPS", access)
	case access == "NodePort" || access == "LoadBalancer":
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("Dashboard via %s — consider using an Ingress with HTTPS instead of port-forward or direct exposure", access)
	default:
		ch.Status = "warning"
		ch.Detail = "No Ingress found for the K10 dashboard — do not rely on port-forward for production access"
	}
	return ch
}

func checkVSCKastenAnnotation(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-19",
		Name:     "VolumeSnapshotClass has Kasten annotation",
		Severity: "critical",
	}
	if len(d.VolumeSnapshotClasses) == 0 {
		ch.Status = "pass"
		ch.Detail = "No VolumeSnapshotClasses found (may be restricted by RBAC)"
		return ch
	}
	var missing []string
	for _, vsc := range d.VolumeSnapshotClasses {
		if !vsc.HasKastenAnnotation {
			missing = append(missing, vsc.Name)
		}
	}
	if len(missing) == 0 {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("All %d VolumeSnapshotClass(es) have the k10.kasten.io/is-snapshot-class annotation", len(d.VolumeSnapshotClasses))
	} else {
		ch.Status = "critical"
		ch.Detail = fmt.Sprintf("%d VolumeSnapshotClass(es) missing annotation k10.kasten.io/is-snapshot-class=true: %s — Kasten will not use them for CSI snapshots", len(missing), strings.Join(missing, ", "))
	}
	return ch
}

func checkNoWildcardSelector(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-20",
		Name:     "No policies with wildcard namespace selector",
		Severity: "warning",
	}
	var wildcards []string
	for _, p := range d.Policies {
		if p.IsWildcard {
			wildcards = append(wildcards, p.Name)
		}
	}
	if len(wildcards) == 0 {
		ch.Status = "pass"
		ch.Detail = "No policies with wildcard namespace selectors found"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d policy/ies have a wildcard namespace selector (%s) — this increases load on the K10 and cluster API; use per-application or per-group policies instead", len(wildcards), strings.Join(wildcards, ", "))
	}
	return ch
}

func checkClusterScopedPolicy(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-21",
		Name:     "Dedicated policy for cluster-scoped resources",
		Severity: "warning",
	}
	for _, p := range d.Policies {
		if p.IsClusterScoped && !p.IsSystemPolicy {
			ch.Status = "pass"
			ch.Detail = fmt.Sprintf("Cluster-scoped policy found: %s", p.Name)
			return ch
		}
	}
	ch.Status = "warning"
	ch.Detail = "No cluster-scoped resource policy found — StorageClasses, CRDs, ClusterRoles, and ClusterRoleBindings are not being captured"
	return ch
}

func checkObjectStorageProfile(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-22",
		Name:     "Location profiles use object storage (not NFS/SMB only)",
		Severity: "warning",
	}
	nfsOnly := true
	hasObjectStorage := false
	hasNFS := false
	for _, p := range d.Profiles {
		if p.Type != "Location" {
			continue
		}
		provider := strings.ToLower(p.Provider)
		if provider == "nfs" || provider == "smb" || provider == "filestore" {
			hasNFS = true
		} else {
			hasObjectStorage = true
			nfsOnly = false
		}
	}
	switch {
	case hasObjectStorage && !hasNFS:
		ch.Status = "pass"
		ch.Detail = "All location profiles use object storage (S3/Azure/GCS)"
	case hasObjectStorage && hasNFS:
		ch.Status = "pass"
		ch.Detail = "Mix of object storage and NFS profiles — ensure off-site copy uses object storage for immutability and durability"
	case nfsOnly && hasNFS:
		ch.Status = "warning"
		ch.Detail = "All location profiles use NFS/SMB — object storage (S3, Azure Blob, GCS) is recommended for immutability, scalability, and ransomware protection"
	default:
		ch.Status = "warning"
		ch.Detail = "No location profiles configured"
	}
	return ch
}

func checkPolicyPresets(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-23",
		Name:     "PolicyPresets defined for retention standardization",
		Severity: "info",
	}
	if len(d.PolicyPresets) > 0 {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("%d PolicyPreset(s) defined — backup and export retention is standardized across policies", len(d.PolicyPresets))
	} else {
		ch.Status = "warning"
		ch.Detail = "No PolicyPresets found — consider defining PolicyPresets to standardize retention settings across all policies"
	}
	return ch
}

func checkCatalogSpace(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-24",
		Name:     "Catalog storage has ≥50% free space (upgrade prerequisite)",
		Severity: "warning",
	}
	if d.Catalog.SizeBytes == 0 || d.Catalog.FreePercent == 0 {
		ch.Status = "pass"
		ch.Detail = "Catalog storage metrics not available — check Settings → Support → Upgrade Status in the K10 dashboard"
		return ch
	}
	switch {
	case d.Catalog.FreePercent >= 50:
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("Catalog has %.1f%% free space (%s free of %s) — sufficient for upgrades", d.Catalog.FreePercent, d.Catalog.FreeHuman, d.Catalog.SizeHuman)
	case d.Catalog.FreePercent >= 30:
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("Catalog has %.1f%% free space (%s free of %s) — below the 50%% threshold required for safe upgrades", d.Catalog.FreePercent, d.Catalog.FreeHuman, d.Catalog.SizeHuman)
	default:
		ch.Status = "critical"
		ch.Detail = fmt.Sprintf("Catalog has only %.1f%% free space (%s free of %s) — upgrades will fail; reduce retention or expand the catalog PVC", d.Catalog.FreePercent, d.Catalog.FreeHuman, d.Catalog.SizeHuman)
	}
	return ch
}

func checkPrometheusAlerts(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-25",
		Name:     "Prometheus alert rules configured",
		Severity: "info",
	}
	if d.Prometheus.AlertRules {
		ch.Status = "pass"
		ch.Detail = "PrometheusRule CR(s) found — alert rules are configured for K10 metrics"
	} else if !d.Prometheus.Enabled {
		ch.Status = "warning"
		ch.Detail = "Prometheus not enabled — configure monitoring and alert rules for failed backup actions and catalog space"
	} else {
		ch.Status = "warning"
		ch.Detail = "Prometheus is enabled but no PrometheusRule CRs found — configure alerts for actions where state=failed and catalog volume used space >50%"
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
