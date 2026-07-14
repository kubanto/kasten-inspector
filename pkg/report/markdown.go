package report

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// WriteMarkdown writes a human-readable Markdown report.
func WriteMarkdown(path string, d *Data) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := func(format string, args ...interface{}) {
		fmt.Fprintf(f, format+"\n", args...)
	}
	hr := func() { w("---") }
	h1 := func(s string) { w("# %s", s) }
	h2 := func(s string) { w("\n## %s", s) }
	h3 := func(s string) { w("\n### %s", s) }

	h1("Kasten K10 Inspector Report")
	w("*Generated: %s | Tool: v%s | Cluster: %s*",
		d.GeneratedAt.Format("02 Jan 2006 15:04 UTC"), d.ToolVersion, d.Cluster.Name)
	hr()

	// ── Executive Summary ────────────────────────────────────────────
	h2("Executive Summary")
	k := d.Kasten
	cl := d.Cluster
	w("| Item | Value |")
	w("|------|-------|")
	w("| Kasten Version | `%s` |", k.Version)
	w("| Cluster | %s |", cl.Name)
	w("| Kubernetes Version | `%s` |", cl.KubernetesVersion)
	w("| Platform | %s %s |", cl.Platform, cl.PlatformVersion)
	w("| Nodes | %d total (%d control-plane, %d workers) |", cl.NodeCount, cl.ControlPlaneNodes, cl.WorkerNodes)
	w("| Namespaces | %d |", cl.NamespaceCount)
	w("| Multi-cluster Mode | %s |", k.MultiCluster.Mode)
	w("| Dashboard Access | %s |", k.HelmConfig.DashboardAccess)

	// ── Compliance ───────────────────────────────────────────────────
	h2("Compliance Overview")
	w("| Metric | Value |")
	w("|--------|-------|")
	w("| Protection Coverage | %.1f%% (%d/%d apps) |",
		k.Compliance.ProtectionCoverage, k.Applications.Protected, k.Applications.Total)
	w("| Policy Compliance | %.1f%% |", k.Compliance.PolicyCompliance)
	w("| Job Success Rate (7d) | %.1f%% |", k.Compliance.SuccessRate7d)
	w("| Failed Jobs (24h) | %d |", k.Compliance.FailedJobs24h)
	w("| Failed Jobs (7d) | %d |", k.Compliance.FailedJobs7d)
	w("| Restore Points | %d total, %d orphaned |", k.RestorePoints.Total, k.RestorePoints.Orphaned)

	// ── Best Practices ───────────────────────────────────────────────
	h2("Best Practices")
	w("**%d checks** · %d passed · %d warnings · %d critical",
		k.BestPractices.TotalChecks, k.BestPractices.Passed,
		k.BestPractices.Warnings, k.BestPractices.Critical)
	w("")
	w("| ID | Check | Status | Detail |")
	w("|----|-------|--------|--------|")
	for _, ch := range k.BestPractices.Checks {
		icon := statusIcon(ch.Status)
		w("| %s | %s | %s %s | %s |", ch.ID, ch.Name, icon, ch.Status, ch.Detail)
	}

	// ── Security ─────────────────────────────────────────────────────
	h2("Security Configuration")
	w("| Setting | Value |")
	w("|---------|-------|")
	w("| Authentication | %s |", k.Security.AuthMethod)
	if k.Security.OIDCConfig != nil {
		w("| OIDC Provider | `%s` |", k.Security.OIDCConfig.ProviderURL)
	}
	w("| Encryption | %v (%s) |", k.Security.Encryption.Enabled, k.Security.Encryption.Provider)
	w("| FIPS Mode | %v |", k.HelmConfig.FIPSMode)
	w("| Audit Logging | %v |", k.HelmConfig.AuditLogging)
	w("| Network Policies | %v |", k.HelmConfig.NetworkPolicies)

	// ── Policies ─────────────────────────────────────────────────────
	h2(fmt.Sprintf("Policies (%d)", len(k.Policies)))
	w("| Name | Enabled | Action | Frequency | Last Run | Status | Avg Duration |")
	w("|------|---------|--------|-----------|----------|--------|--------------|")
	for _, p := range k.Policies {
		enabled := "✓"
		if !p.Enabled {
			enabled = "✗"
		}
		w("| `%s` | %s | %s | %s | %s | %s | %s |",
			p.Name, enabled, p.Action, p.Frequency,
			formatTimeShort(p.LastRunTime), p.LastRunStatus, p.AvgRunDuration)
	}

	// ── Applications ─────────────────────────────────────────────────
	h2(fmt.Sprintf("Applications (%d)", k.Applications.Total))
	w("*%d protected · %d unprotected*", k.Applications.Protected, k.Applications.Unprotected)
	w("")
	w("| Application | Namespace | Protected | Policies | Last Backup |")
	w("|-------------|-----------|-----------|----------|-------------|")
	for _, app := range k.Applications.Apps {
		protected := "✓"
		if !app.Protected {
			protected = "✗"
		}
		w("| `%s` | `%s` | %s | %s | %s |",
			app.Name, app.Namespace, protected,
			strings.Join(app.PolicyNames, ", "),
			formatTimeShort(app.LastBackup))
	}

	// ── Unprotected Namespaces ────────────────────────────────────────
	if len(k.Namespaces.Unprotected) > 0 {
		h2(fmt.Sprintf("Unprotected Namespaces (%d)", len(k.Namespaces.Unprotected)))
		w("| Namespace | Labels |")
		w("|-----------|--------|")
		for _, ns := range k.Namespaces.Unprotected {
			labels := []string{}
			for k, v := range ns.Labels {
				labels = append(labels, k+"="+v)
			}
			w("| `%s` | %s |", ns.Name, strings.Join(labels, ", "))
		}
	}

	// ── Profiles ─────────────────────────────────────────────────────
	h2(fmt.Sprintf("Location Profiles (%d)", len(k.Profiles)))
	w("| Name | Type | Provider | Bucket/Endpoint | Immutable | Ready |")
	w("|------|------|----------|-----------------|-----------|-------|")
	for _, p := range k.Profiles {
		immut := ""
		if p.Immutability {
			immut = fmt.Sprintf("✓ (%s)", p.ImmutabilityPeriod)
		}
		ep := p.Bucket
		if ep == "" {
			ep = p.Endpoint
		}
		ready := "✓"
		if !p.Ready {
			ready = "✗"
		}
		w("| `%s` | %s | %s | `%s` | %s | %s |",
			p.Name, p.Type, p.Provider, ep, immut, ready)
	}

	// ── KubeVirt ─────────────────────────────────────────────────────
	if k.KubeVirt.Enabled {
		h2(fmt.Sprintf("KubeVirt / OCP Virtualization (%d VMs)", k.KubeVirt.TotalVMs))
		w("*%d protected · %d unprotected*",
			k.KubeVirt.ProtectedVMs, k.KubeVirt.UnprotectedVMs)
		w("")
		w("| VM | Namespace | Status | Protected | Policy |")
		w("|----|-----------|--------|-----------|--------|")
		for _, vm := range k.KubeVirt.VMs {
			protected := "✓"
			if !vm.Protected {
				protected = "✗"
			}
			w("| `%s` | `%s` | %s | %s | %s |",
				vm.Name, vm.Namespace, vm.Status, protected, vm.Policy)
		}
	}

	// ── Disaster Recovery ─────────────────────────────────────────────
	h2("Disaster Recovery (KDR)")
	if k.DR.Enabled {
		w("| Setting | Value |")
		w("|---------|-------|")
		w("| Status | ✓ Enabled |")
		w("| Mode | %s |", k.DR.Mode)
		w("| Policy | `%s` |", k.DR.BackupPolicy)
		w("| Export Profile | `%s` |", k.DR.ExportProfile)
		w("| Last Run | %s |", formatTimeShort(k.DR.LastRunTime))
		w("| Last Status | %s |", k.DR.LastRunStatus)
	} else {
		w("> ⚠️ No Kasten DR policy configured — the K10 catalog is not being backed up.")
	}

	// ── Recent Jobs ───────────────────────────────────────────────────
	h2(fmt.Sprintf("Recent Jobs (%d)", len(k.Jobs)))
	w("| Action | Policy | App | Status | Start | Duration |")
	w("|--------|--------|-----|--------|-------|----------|")
	for _, j := range k.Jobs {
		w("| %s | `%s` | `%s` | %s %s | %s | %s |",
			j.Action, j.PolicyName, j.AppName,
			statusIcon(strings.ToLower(j.Status)), j.Status,
			formatTimeShort(j.StartTime), j.Duration)
	}

	// ── Restore Points ─────────────────────────────────────────────────
	h2("Restore Points")
	w("**Total: %d** | Orphaned: %d | Oldest: %s | Newest: %s",
		k.RestorePoints.Total, k.RestorePoints.Orphaned,
		formatTimeShort(k.RestorePoints.Oldest),
		formatTimeShort(k.RestorePoints.Newest))
	if len(k.RestorePoints.ByApp) > 0 {
		h3("By Application")
		w("| Application | Restore Points |")
		w("|-------------|----------------|")
		for app, count := range k.RestorePoints.ByApp {
			w("| `%s` | %d |", app, count)
		}
	}

	// ── Blueprints & TransformSets ────────────────────────────────────
	if len(k.Blueprints) > 0 {
		h2(fmt.Sprintf("Kanister Blueprints (%d)", len(k.Blueprints)))
		w("| Name | Namespace | Actions |")
		w("|------|-----------|---------|")
		for _, bp := range k.Blueprints {
			w("| `%s` | `%s` | %s |", bp.Name, bp.Namespace, strings.Join(bp.Actions, ", "))
		}
	}
	if len(k.TransformSets) > 0 {
		h2(fmt.Sprintf("TransformSets (%d)", len(k.TransformSets)))
		w("| Name | Transforms |")
		w("|------|-----------|")
		for _, ts := range k.TransformSets {
			w("| `%s` | %d |", ts.Name, ts.Transforms)
		}
	}


	// ── Storage Summary ─────────────────────────────────────────────────────
	h2("Storage Summary")
	stor := d.Kasten.Storage
	w("| Metric | Value |")
	w("|--------|-------|")
	if stor.SnapshotSizeBytes > 0 {
		w("| Snapshot Storage | %s (%d snapshots) |", stor.SnapshotSizeHuman, stor.SnapshotCount)
	} else {
		w("| Snapshot Storage | — |")
	}
	if stor.ExportSizeBytes > 0 {
		w("| Export Storage | %s |", stor.ExportSizeHuman)
		if stor.DedupeRatio > 0 {
			w("| Deduplication Ratio | %.2fx |", stor.DedupeRatio)
		}
	} else {
		w("| Export Storage | — |")
	}
	if stor.LiveSizeBytes > 0 {
		w("| Live Storage (PVCs) | %s (%d volumes) |", stor.LiveSizeHuman, stor.LiveVolumeCount)
	}
	if len(stor.ServicesDisk) > 0 {
		w("")
		w("**K10 Services Disk Usage:**")
		w("")
		w("| Service | Used | Free | Total | Free %% |")
		w("|---------|------|------|-------|---------|")
		for _, svc := range stor.ServicesDisk {
			w("| `%s` | %s | %s | %s | %.0f%% |",
				svc.Name, svc.UsedHuman, svc.FreeHuman, svc.TotalHuman, svc.FreePercent)
		}
	}

	// ── Actions Summary ──────────────────────────────────────────────────────
	h2("Actions Summary")
	jsum := d.Kasten.JobSummary
	w("| Status | Count |")
	w("|--------|-------|")
	w("| Complete | **%d** |", jsum.Completed)
	w("| Failed | **%d** |", jsum.Failed)
	w("| Skipped | %d |", jsum.Skipped)
	w("| Cancelled | %d |", jsum.Cancelled)
	w("| **Total** | **%d** |", jsum.Total)
	if len(jsum.ByAction) > 0 {
		w("")
		w("By action type:")
		for action, count := range jsum.ByAction {
			w("- **%s**: %d", action, count)
		}
	}
	if len(jsum.SuccessByAction) > 0 {
		w("")
		w("Success rate by action (in K10 `backup` = snapshot):")
		for _, a := range jsum.SuccessByAction {
			if a.Total > 0 {
				w("- **%s**: %.0f%% (%d/%d)", a.Action, a.SuccessRate, a.Completed, a.Total)
			} else {
				w("- **%s**: n/a", a.Action)
			}
		}
	}

	// ── K10 Generated Reports ────────────────────────────────────────────────
	if len(d.Kasten.K10Reports) > 0 {
		h2(fmt.Sprintf("K10 Generated Reports (%d)", len(d.Kasten.K10Reports)))
		w("| Name | Date | Apps | Completed | Failed | Snapshot | Export | Dedup |")
		w("|------|------|------|-----------|--------|----------|--------|-------|")
		for _, r := range d.Kasten.K10Reports {
			dedup := "—"
			if r.Stats.Storage.DedupeRatio > 0 {
				dedup = fmt.Sprintf("%.2fx", r.Stats.Storage.DedupeRatio)
			}
			w("| `%s` | %s | %d | %d | %d | %s | %s | %s |",
				r.Name, formatTimeShort(r.GeneratedAt),
				r.Stats.Apps.Total,
				r.Stats.Actions.Completed, r.Stats.Actions.Failed,
				formatStorageBytes(r.Stats.Storage.SnapshotSizeBytes),
				formatStorageBytes(r.Stats.Storage.ExportSizeBytes),
				dedup)
		}
	}

	// ── K10 Resources ─────────────────────────────────────────────────
	h2("K10 Resource Limits")
	w("| Deployment | Replicas | Container | CPU Req | CPU Lim | Mem Req | Mem Lim |")
	w("|------------|----------|-----------|---------|---------|---------|---------|")
	for _, dep := range k.Resources.Deployments {
		for i, cont := range dep.Containers {
			name := dep.Name
			replicas := fmt.Sprintf("%d/%d", dep.Ready, dep.Replicas)
			if i > 0 {
				name = ""
				replicas = ""
			}
			w("| `%s` | %s | `%s` | %s | %s | %s | %s |",
				name, replicas, cont.Name,
				orDash(cont.CPURequest), orDash(cont.CPULimit),
				orDash(cont.MemRequest), orDash(cont.MemLimit))
		}
	}

	// ── License ───────────────────────────────────────────────────────
	h2("License")
	w("| Field | Value |")
	w("|-------|-------|")
	w("| Company | %s |", k.License.Company)
	w("| Type | %s |", k.License.LicenseType)
	w("| Expires | %s |", k.License.ExpiresAt)
	w("| Valid | %v |", k.License.Valid)

	// ── Failed Actions Top-5 ──────────────────────────────────────────
	if len(k.RecentFailures) > 0 {
		h2(fmt.Sprintf("Recent Failures — Top %d", len(k.RecentFailures)))
		w("| Kind | App | Policy | When | Error |")
		w("|------|-----|--------|------|-------|")
		for _, fa := range k.RecentFailures {
			errShort := fa.Error
			if len(errShort) > 80 {
				errShort = errShort[:77] + "..."
			}
			w("| `%s` | %s | %s | %s | %s |",
				fa.Kind, orDash(fa.AppName), orDash(fa.PolicyName),
				formatTimeShort(fa.StartTime), orDash(errShort))
		}
	}

	// ── Long-running Actions ─────────────────────────────────────────────────
	if len(k.LongRunningActions) > 0 {
		h2(fmt.Sprintf("Long-running Actions (%d)", len(k.LongRunningActions)))
		w("> Actions in Running state beyond the threshold (24h) — likely a hung Kanister job or unresponsive exec.")
		w("")
		w("| Kind | Name | App | Policy | Started | Running for |")
		w("|------|------|-----|--------|---------|-------------|")
		for _, sa := range k.LongRunningActions {
			w("| `%s` | `%s` | %s | %s | %s | **%s** |",
				sa.Kind, sa.Name, orDash(sa.AppName), orDash(sa.PolicyName),
				formatTimeShort(sa.StartTime), sa.RunningFor)
		}
	}

	// ── Backup Recency per Namespace ───────────────────────────────────
	if len(k.BackupRecency) > 0 {
		driftCount := 0
		for _, ns := range k.BackupRecency {
			if ns.Drift && ns.Protected {
				driftCount++
			}
		}
		h2(fmt.Sprintf("Backup Recency per Namespace (%d with drift)", driftCount))
		w("| Namespace | Protected | Last Backup | Days Since | Export | Drift |")
		w("|-----------|-----------|-------------|------------|--------|-------|")
		for _, ns := range k.BackupRecency {
			driftStr := ""
			if ns.Drift && ns.Protected {
				driftStr = "⚠️ DRIFT"
			}
			w("| `%s` | %v | %s | %s | %s | %s |",
				ns.Namespace,
				ns.Protected,
				formatTimeShort(ns.LastBackup),
				func() string {
					if ns.DaysSinceBackup > 0 {
						return fmt.Sprintf("%dd", ns.DaysSinceBackup)
					}
					return "—"
				}(),
				formatTimeShort(ns.LastExport),
				driftStr)
		}
	}

	// ── StorageClass / VolumeSnapshotClass inventory ──────────────────
	if len(k.StorageClasses) > 0 || len(k.VolumeSnapshotClasses) > 0 {
		h2("StorageClass & VolumeSnapshotClass Inventory")

		if len(k.StorageClasses) > 0 {
			h3("StorageClasses")
			w("| Name | Provisioner | Default | Expandable | Reclaim | VSC |")
			w("|------|-------------|---------|------------|---------|-----|")
			for _, sc := range k.StorageClasses {
				var vsc string
				switch {
				case sc.HasVSC:
					vsc = "✅"
				case !sc.HasVSC && sc.Provisioner != "":
					vsc = "⚠️ missing"
				default:
					vsc = "—"
				}
				w("| `%s` | `%s` | %v | %v | %s | %s |",
					sc.Name, sc.Provisioner, sc.IsDefault, sc.Expandable,
					orDash(sc.ReclaimPolicy), vsc)
			}
		}

		if len(k.VolumeSnapshotClasses) > 0 {
			h3("VolumeSnapshotClasses")
			w("| Name | Driver | Default | Deletion Policy |")
			w("|------|--------|---------|-----------------|")
			for _, vsc := range k.VolumeSnapshotClasses {
				w("| `%s` | `%s` | %v | %s |",
					vsc.Name, vsc.Driver, vsc.IsDefault, orDash(vsc.DeletionPolicy))
			}
		}

		if len(k.CSIWarnings) > 0 {
			h3("CSI Warnings")
			for _, warn := range k.CSIWarnings {
				w("- ⚠️ %s", warn)
			}
		}
	}

	w("\n---\n*Kasten Inspector v%s — %s — Independent tool, not an official Veeam product.*", d.ToolVersion, d.GeneratedAt.Format(time.RFC3339))
	return nil
}

func statusIcon(status string) string {
	switch strings.ToLower(status) {
	case "pass", "complete", "success":
		return "✅"
	case "warning":
		return "⚠️"
	case "critical", "failed", "error":
		return "❌"
	case "running", "pending":
		return "🔄"
	default:
		return "❓"
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func formatStorageBytes(b int64) string {
	if b == 0 {
		return "—"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatTimeShort(ts string) string {
	if ts == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("02 Jan 2006")
}
