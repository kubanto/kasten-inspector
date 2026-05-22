package kasten

import (
	"fmt"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

// CollectAll runs every collector and assembles a complete Data struct.
func CollectAll(c *cluster.Client, opts CollectOptions) (*Data, error) {
	d := &Data{}
	ns := opts.Namespace

	warn := func(area string, err error) {
		if err != nil {
			fmt.Printf("  [warn] %-28s %v\n", area+":", err)
		}
	}

	var err error

	// ── Core ──────────────────────────────────────────────────────────────────
	d.Version, err = collectVersion(c, ns)
	warn("kasten version", err)

	d.HelmConfig, err = collectHelmConfig(c, ns)
	warn("helm config", err)

	d.License, err = collectLicense(c, ns)
	warn("license", err)

	// ── Security ──────────────────────────────────────────────────────────────
	d.Security, err = collectSecurity(c, ns)
	warn("security", err)

	// ── Policies (must come before apps, DR, coverage) ────────────────────────
	d.Policies, err = collectPolicies(c, ns)
	warn("policies", err)

	d.PolicyPresets, err = collectPolicyPresets(c, ns)
	warn("policy presets", err)

	d.Profiles, err = collectProfiles(c, ns)
	warn("profiles", err)

	d.Applications, err = collectApplications(c, ns, d.Policies)
	warn("applications", err)

	d.Namespaces, err = collectNamespaces(c, ns, d.Applications)
	warn("namespaces", err)

	// ── Multi-cluster / DR ────────────────────────────────────────────────────
	d.MultiCluster, err = collectMultiCluster(c, ns)
	warn("multi-cluster", err)

	d.DR, err = collectDR(c, ns, d.Policies)
	warn("disaster recovery", err)

	// ── Jobs ──────────────────────────────────────────────────────────────────
	d.Jobs, err = collectJobs(c, ns, opts.JobLimit)
	warn("jobs", err)
	d.JobSummary = computeJobSummary(d.Jobs)

	// ── Restore Points ────────────────────────────────────────────────────────
	d.RestorePoints, err = collectRestorePoints(c, ns, d.Applications)
	warn("restore points", err)

	// ── KubeVirt ──────────────────────────────────────────────────────────────
	d.KubeVirt, err = collectKubeVirt(c, d.Applications)
	warn("kubevirt", err)

	// ── Kanister / Transforms ─────────────────────────────────────────────────
	d.Blueprints, err = collectBlueprints(c)
	warn("blueprints", err)

	d.Bindings, err = collectBlueprintBindings(c, ns)
	warn("blueprint bindings", err)

	d.TransformSets, err = collectTransformSets(c, ns)
	warn("transform sets", err)

	// ── Infrastructure ────────────────────────────────────────────────────────
	d.Resources, err = collectK10Resources(c, ns)
	warn("k10 resources", err)

	d.Catalog, err = collectCatalog(c, ns)
	warn("catalog", err)

	d.Prometheus, err = collectPrometheus(c, ns)
	warn("prometheus", err)

	// ── PVCs & Coverage Matrix ────────────────────────────────────────────────
	d.PVCs, err = collectPVCs(c)
	warn("pvcs", err)

	d.CoverageMatrix = collectCoverageMatrix(d.Namespaces, d.Policies)

	// ── K10 Reports CRD (storage, license, action stats) ─────────────────────
	d.K10Reports, err = collectK10Reports(c, ns)
	warn("k10 reports", err)
	// Version fallback: use K10Report data if not detected from image tags
	if (d.Version == "" || d.Version == "unknown" || d.Version == "None") && len(d.K10Reports) > 0 {
		if d.K10Reports[0].K10Version != "" {
			d.Version = d.K10Reports[0].K10Version
		}
	}

	// Enrich from K10 report data (authoritative source for license, apps, profiles)
	enrichLicenseFromReport(&d.License, d.K10Reports)
	enrichApplicationsFromReport(&d.Applications, d.K10Reports)
	d.Profiles = enrichProfilesFromReport(d.Profiles, d.K10Reports)

	// Storage summary (combines reports CRD + live PVC data)
	d.Storage, err = collectStorageSummary(c, ns, d.K10Reports, d.PVCs)
	warn("storage summary", err)

	// ── Diagnostics (ported from Kasten Disco Lite v1.9) ─────────────────────
	d.RecentFailures, err = collectRecentFailures(c, ns)
	warn("failed actions top-5", err)

	d.LongRunningActions, err = collectLongRunningActions(c, ns)
	warn("stuck actions", err)

	d.BackupRecency = collectBackupRecency(d.Jobs, d.Applications)

	d.StorageClasses, d.VolumeSnapshotClasses, d.CSIWarnings, err = collectVolumeProvisionerAudit(c)
	warn("storageclass/vsc inventory", err)

	// Enrich before computing compliance so BP-07, BP-16 etc. have correct data
	enrichPolicyDurations(d)
	enrichPolicyLastRun(d)
	enrichAppLastBackup(d)
	enrichDRFromPolicies(d)

	// ── Compliance & Best Practices ───────────────────────────────────────────
	d.Compliance = computeCompliance(d)
	d.BestPractices = evaluateBestPractices(d)

	// ── QBR analytics ────────────────────────────────────────────────────────
	d.RecoveryReadiness = computeRecoveryReadiness(d)
	d.AppRiskMatrix = computeAppRiskMatrix(d)
	d.WeeklySLATrend = computeWeeklySLA(d.Jobs)

	return d, nil
}

// computeJobSummary aggregates job stats by action and status.
func computeJobSummary(jobs []Job) JobSummary {
	s := JobSummary{
		ByAction: map[string]int{},
		ByStatus: map[string]int{},
	}
	for _, j := range jobs {
		s.Total++
		s.ByAction[j.Action]++
		s.ByStatus[j.Status]++
		switch j.Status {
		case "Complete", "Success":
			s.Completed++
		case "Failed", "Error":
			s.Failed++
		case "Skipped":
			s.Skipped++
		case "Cancelled", "Canceled":
			s.Cancelled++
		case "Running", "InProgress":
			s.Running++
		}
	}
	return s
}
