package kasten

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

var gvrK10Reports = schema.GroupVersionResource{
	Group:   "reporting.kio.kasten.io",
	Version: "v1alpha1",
	Resource: "reports",
}

// collectK10Reports reads reports.reporting.kio.kasten.io
// K10 stores all aggregated data in the "results" field (not "status").
// Tries namespace-scoped first, then cluster-scoped (some K10 versions differ).
func collectK10Reports(c *cluster.Client, ns string) ([]K10Report, error) {
	ctx := context.Background()

	// Try namespace-scoped first (standard)
	list, err := c.Dynamic.Resource(gvrK10Reports).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil || len(list.Items) == 0 {
		// Try cluster-scoped (some K10 8.x deployments)
		list2, err2 := c.Dynamic.Resource(gvrK10Reports).Namespace("").List(ctx, metav1.ListOptions{})
		if err2 == nil && len(list2.Items) > 0 {
			list = list2
			err = nil
		}
	}
	if err != nil {
		return nil, err
	}
	if list == nil || len(list.Items) == 0 {
		return nil, nil
	}

	var reports []K10Report
	for _, item := range list.Items {
		obj := item.Object
		spec := GetMap(obj, "spec")
		res := GetMap(obj, "results") // K10 8.x uses "results", not "status"

		r := K10Report{
			Name:        MetaName(obj),
			GeneratedAt: GetString(spec, "reportTimestamp"),
			Period:      fmt.Sprintf("%d day(s)", GetInt(spec, "statsIntervalDays")),
			K10Version:  GetString(GetMap(obj, "results"), "k10Version"),
		}
		if r.K10Version == "" {
			r.K10Version = GetString(obj, "k10Version")
		}
		if r.GeneratedAt == "" {
			r.GeneratedAt = MetaTimestamp(obj)
		}

		// ── Compliance / Applications ─────────────────────────────────────
		comp := GetMap(res, "compliance")
		r.Stats.Apps = K10ReportApps{
			Total:        GetInt(comp, "applicationCount"),
			Compliant:    GetInt(comp, "compliantCount"),
			NonCompliant: GetInt(comp, "nonCompliantCount"),
			Unmanaged:    GetInt(comp, "unmanagedCount"),
		}

		// ── Actions ───────────────────────────────────────────────────────
		// results.actions.countStats.<type>.{completed,failed,skipped,cancelled}
		countStats := GetMap(res, "actions", "countStats")
		var totalCompleted, totalFailed, totalSkipped, totalCancelled int
		var snapshotC, restoreC, exportC, importC int
		for actionType, v := range countStats {
			am, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			c2 := GetInt(am, "completed")
			f := GetInt(am, "failed")
			s := GetInt(am, "skipped")
			cc := GetInt(am, "cancelled")
			totalCompleted += c2
			totalFailed += f
			totalSkipped += s
			totalCancelled += cc
			switch actionType {
			case "backup", "backupCluster":
				snapshotC += c2 + f + s + cc
			case "restore", "restoreCluster":
				restoreC += c2 + f + s + cc
			case "export":
				exportC += c2 + f + s + cc
			case "import":
				importC += c2 + f + s + cc
			}
		}
		total := totalCompleted + totalFailed + totalSkipped + totalCancelled
		r.Stats.Actions = K10ReportActions{
			Total:     total,
			Completed: totalCompleted,
			Failed:    totalFailed,
			Skipped:   totalSkipped,
			Cancelled: totalCancelled,
			Snapshot:  snapshotC,
			Restore:   restoreC,
			Export:    exportC,
			Import:    importC,
		}

		// ── Storage ───────────────────────────────────────────────────────
		stor := GetMap(res, "storage")

		snap := GetMap(stor, "snapshotStorage")
		snapBytes := int64(GetInt(snap, "logicalBytes"))
		snapCount := GetInt(snap, "count")

		obj2 := GetMap(stor, "objectStorage")
		expLogical := int64(GetInt(obj2, "logicalBytes"))
		expPhysical := int64(GetInt(obj2, "physicalBytes"))
		expCount := GetInt(obj2, "count")
		var dedupeRatio float64
		if expPhysical > 0 && expLogical > 0 {
			dedupeRatio = float64(expLogical) / float64(expPhysical)
		}

		pvcStat := GetMap(stor, "pvcStats")

		r.Stats.Storage = K10ReportStorage{
			SnapshotSizeBytes: snapBytes,
			SnapshotCount:     snapCount,
			ExportSizeBytes:   expPhysical,
			ExportCount:       expCount,
			DedupeRatio:       dedupeRatio,
		}

		// Live storage from pvcStats
		r.Stats.PVCBytes = int64(GetInt(pvcStat, "pvcBytes"))
		r.Stats.PVCCount = GetInt(pvcStat, "pvcCount")

		// ── License ───────────────────────────────────────────────────────
		// K10 8.x: licensing data may be at results.licensing or results.license
		lic := GetMap(res, "licensing")
		if len(lic) == 0 {
			lic = GetMap(res, "license")
		}
		licType := GetString(lic, "type")
		if licType == "" {
			licType = GetString(lic, "licenseType")
		}
		licStatus := GetString(lic, "status")
		if licStatus == "" {
			licStatus = GetString(lic, "licenseState")
		}
		licNodeCount := GetInt(lic, "nodeCount")
		if licNodeCount == 0 {
			licNodeCount = GetInt(lic, "nodeUsage")
		}
		licNodeLimit := GetInt(lic, "nodeLimit")
		r.Stats.License = K10ReportLicense{
			Type:      licType,
			Status:    licStatus,
			NodeCount: licNodeCount,
			NodeLimit: licNodeLimit,
		}
		// Expiry: try multiple field names
		for _, expKey := range []string{"expiry", "expiryDate", "expiresAt", "expires"} {
			if exp := lic[expKey]; exp != nil {
				expStr := fmt.Sprintf("%v", exp)
				if expStr != "<nil>" && expStr != "null" && expStr != "" {
					r.Stats.License.ExpiresAt = expStr
					break
				}
			}
		}
		// If no expiry and type is known, mark as no-expiry
		if r.Stats.License.ExpiresAt == "" && r.Stats.License.Type != "" {
			r.Stats.License.ExpiresAt = "No expiry"
		}

		// ── K10 Services disk ─────────────────────────────────────────────
		for _, svcRaw := range GetSlice(res, "k10Services") {
			svcMap, ok := svcRaw.(map[string]interface{})
			if !ok {
				continue
			}
			disk := GetMap(svcMap, "diskUsage")
			free := int64(GetInt(disk, "freeBytes"))
			used := int64(GetInt(disk, "usedBytes"))
			total2 := free + used
			freeP := float64(0)
			if total2 > 0 {
				freeP = float64(free) / float64(total2) * 100
			}
			r.ServicesDisk = append(r.ServicesDisk, ServiceDiskUsage{
				Name:        GetString(svcMap, "name"),
				FreeBytes:   free,
				UsedBytes:   used,
				TotalBytes:  total2,
				FreeHuman:   HumanBytes(free),
				UsedHuman:   HumanBytes(used),
				TotalHuman:  HumanBytes(total2),
				FreePercent: freeP,
			})
		}

		// ── Profiles (enriched data) ───────────────────────────────────────
		for _, profRaw := range GetSlice(res, "profiles", "summaries") {
			pm, ok := profRaw.(map[string]interface{})
			if !ok {
				continue
			}
			r.ProfileSummaries = append(r.ProfileSummaries, K10ReportProfile{
				Name:           GetString(pm, "name"),
				Type:           GetString(pm, "type"),
				Provider:       GetString(pm, "objectStoreType"),
				Bucket:         GetString(pm, "bucket"),
				Region:         GetString(pm, "region"),
				Endpoint:       GetString(pm, "endpoint"),
				SSLVerification: GetString(pm, "sslVerification"),
				Validation:     GetString(pm, "validation"),
				Immutability:   GetString(pm, "immutability", "protection") == "Enabled",
				ImmutabilityDays: GetInt(pm, "immutability", "protectionDays"),
			})
		}

		reports = append(reports, r)
	}

	// Sort newest first so reports[0] is always the most recent report.
	sort.Slice(reports, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, reports[i].GeneratedAt)
		tj, ej := time.Parse(time.RFC3339, reports[j].GeneratedAt)
		if ei != nil || ej != nil {
			return ei == nil // valid timestamps rank before unparseable ones
		}
		return ti.After(tj)
	})

	return reports, nil
}

// collectStorageSummary builds a StorageSummary from K10 reports.
func collectStorageSummary(c *cluster.Client, ns string, reports []K10Report, pvcs PVCSummary) (StorageSummary, error) {
	s := StorageSummary{
		ExportByApp: map[string]int64{},
	}
	if len(reports) == 0 {
		// No K10 report available — live storage only
		s.ReportAgeDays = -1
		s.LiveVolumeCount = pvcs.Total
		s.LiveSizeBytes = int64(pvcs.TotalSizeGB * 1024 * 1024 * 1024)
		s.LiveSizeHuman = HumanBytes(s.LiveSizeBytes)
		return s, nil
	}

	latest := reports[0]

	// Report age — how old is the data we are showing
	s.ReportDate = latest.GeneratedAt
	if ts, err := time.Parse(time.RFC3339, latest.GeneratedAt); err == nil {
		s.ReportAgeDays = int(time.Since(ts).Hours() / 24)
	}

	s.SnapshotSizeBytes = latest.Stats.Storage.SnapshotSizeBytes
	s.SnapshotSizeHuman = HumanBytes(s.SnapshotSizeBytes)
	s.SnapshotCount = latest.Stats.Storage.SnapshotCount

	s.ExportSizeBytes = latest.Stats.Storage.ExportSizeBytes
	s.ExportSizeHuman = HumanBytes(s.ExportSizeBytes)
	s.DedupeRatio = latest.Stats.Storage.DedupeRatio

	// Live storage from report's pvcStats (authoritative)
	if latest.Stats.PVCBytes > 0 {
		s.LiveSizeBytes = latest.Stats.PVCBytes
		s.LiveVolumeCount = latest.Stats.PVCCount
	} else {
		s.LiveSizeBytes = int64(pvcs.TotalSizeGB * 1024 * 1024 * 1024)
		s.LiveVolumeCount = pvcs.Total
	}
	s.LiveSizeHuman = HumanBytes(s.LiveSizeBytes)

	// K10 services disk from report
	s.ServicesDisk = latest.ServicesDisk

	return s, nil
}

// enrichLicenseFromReport fills License fields from K10 report data.
func enrichLicenseFromReport(lic *License, reports []K10Report) {
	if len(reports) == 0 {
		return
	}
	r := reports[0].Stats.License
	if lic.LicenseType == "" && r.Type != "" {
		lic.LicenseType = r.Type
	}
	if !lic.Valid && strings.EqualFold(r.Status, "valid") {
		lic.Valid = true
	}
	if lic.ExpiresAt == "" && r.ExpiresAt != "" && r.ExpiresAt != "<nil>" {
		lic.ExpiresAt = r.ExpiresAt
	}
	if lic.NodeLimit == 0 && r.NodeLimit > 0 {
		lic.NodeLimit = r.NodeLimit
	}
	if lic.NodeUsage == 0 && r.NodeCount > 0 {
		lic.NodeUsage = r.NodeCount
	}
}

// enrichApplicationsFromReport fills app stats from K10 report data.
func enrichApplicationsFromReport(apps *AppSummary, reports []K10Report) {
	if len(reports) == 0 {
		return
	}
	ra := reports[0].Stats.Apps
	if ra.Total > 0 {
		// Always use report data for compliance counts (authoritative from K10)
		apps.Compliant = ra.Compliant
		apps.NonCompliant = ra.NonCompliant
		apps.Unmanaged = ra.Unmanaged
		// NOTE: we intentionally do NOT override apps.Total from the K10 report.
		// The K10 report counts all namespaces including system ones, giving
		// inflated totals (e.g. 97 on OpenShift vs 16 actual user namespaces).
		// Our collector already filters system and Terminating namespaces.
	}
}

// enrichProfilesFromReport adds region, SSL, immutability days from report data.
func enrichProfilesFromReport(profiles []Profile, reports []K10Report) []Profile {
	if len(reports) == 0 {
		return profiles
	}
	// Build lookup by name
	lookup := map[string]K10ReportProfile{}
	for _, p := range reports[0].ProfileSummaries {
		lookup[p.Name] = p
	}
	for i, p := range profiles {
		if rp, ok := lookup[p.Name]; ok {
			if p.Region == "" {
				profiles[i].Region = rp.Region
			}
			if !p.Immutability && rp.Immutability {
				profiles[i].Immutability = true
				if rp.ImmutabilityDays > 0 {
					profiles[i].ImmutabilityPeriod = fmt.Sprintf("%dd", rp.ImmutabilityDays)
				}
			}
			if p.Provider == "" {
				profiles[i].Provider = rp.Provider
			}
		}
	}
	return profiles
}
