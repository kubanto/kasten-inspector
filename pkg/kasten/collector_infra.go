package kasten

import (
	"context"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

var (
	gvrClusterProfile = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "clusterprofiles"}
	gvrKubeVirt       = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "kubevirts"}
	gvrLicense        = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "licenses"}

	// K10 8.x: RestorePoints live in the application namespace (not kasten-io)
	// RestorepointContents live in kasten-io as cluster-scoped index
	gvrRestorePointContent = schema.GroupVersionResource{
		Group: "apps.kio.kasten.io", Version: "v1alpha1", Resource: "restorepointcontents",
	}
)

// ── Restore Points ────────────────────────────────────────────────────────────

func collectRestorePoints(c *cluster.Client, ns string, apps AppSummary) (RestorePointInfo, error) {
	ctx := context.Background()
	info := RestorePointInfo{
		ByApp:    map[string]int{},
		ByPolicy: map[string]int{},
	}

	knownApps := map[string]bool{}
	for _, app := range apps.Apps {
		knownApps[app.Name] = true
	}

	// Strategy 1: list RestorePoints in every app namespace (K10 8.x)
	var times []time.Time
	found := 0

	for _, app := range apps.Apps {
		rList, err := c.Dynamic.Resource(GVRRestorePoint).Namespace(app.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range rList.Items {
			obj := item.Object
			labels := GetLabels(obj)
			appName := labelStr(labels, "apps.kio.kasten.io/appName")
			if appName == "" {
				appName = app.Name
			}
			polName := labelStr(labels, "policies.kio.kasten.io/policy-name")
			ts := MetaTimestamp(obj)

			info.Total++
			info.ByApp[appName]++
			if polName != "" {
				info.ByPolicy[polName]++
			}
			info.Details = append(info.Details, RestorePoint{
				Name:      MetaName(obj),
				AppName:   appName,
				Policy:    polName,
				CreatedAt: ts,
				Orphaned:  !knownApps[appName],
			})
			found++
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				times = append(times, t)
			}
		}
	}

	// Strategy 2: fallback — restorepointcontents in kasten-io (cluster-scoped index)
	if found == 0 {
		rpcList, err := c.Dynamic.Resource(gvrRestorePointContent).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, item := range rpcList.Items {
				obj := item.Object
				labels := GetLabels(obj)
				appName := labelStr(labels, "apps.kio.kasten.io/appName")
				if appName == "" {
					// Extract from name: "<appname>-scheduled-XXXXX"
					name := MetaName(obj)
					appName = extractAppNameFromRP(name)
				}
				polName := labelStr(labels, "policies.kio.kasten.io/policy-name")
				ts := MetaTimestamp(obj)

				info.Total++
				info.ByApp[appName]++
				if polName != "" {
					info.ByPolicy[polName]++
				}
				info.Details = append(info.Details, RestorePoint{
					Name:      MetaName(obj),
					AppName:   appName,
					Policy:    polName,
					CreatedAt: ts,
					Orphaned:  !knownApps[appName],
				})
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					times = append(times, t)
				}
			}
		}
	}

	// Strategy 3: all-namespaces scan for restorepoints
	if info.Total == 0 {
		rList, err := c.Dynamic.Resource(GVRRestorePoint).Namespace("").List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, item := range rList.Items {
				obj := item.Object
				labels := GetLabels(obj)
				appName := labelStr(labels, "apps.kio.kasten.io/appName")
				if appName == "" {
					appName = MetaNamespace(obj)
				}
				polName := labelStr(labels, "policies.kio.kasten.io/policy-name")
				ts := MetaTimestamp(obj)

				info.Total++
				info.ByApp[appName]++
				if polName != "" {
					info.ByPolicy[polName]++
				}
				// KDR backups live under kasten-io namespace and are never "orphaned"
				isKDRBackup := appName == "kasten-io" || MetaNamespace(obj) == ns
				orphaned := appName != "" && !knownApps[appName] && !isKDRBackup
				info.Details = append(info.Details, RestorePoint{
					Name:      MetaName(obj),
					AppName:   appName,
					Policy:    polName,
					CreatedAt: ts,
					Orphaned:  orphaned,
				})
				if orphaned {
					info.Orphaned++
				}
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					times = append(times, t)
				}
			}
		}
	}

	// Count orphaned
	for _, d := range info.Details {
		if d.Orphaned {
			info.Orphaned++
		}
	}
	// deduplicate orphan count if already counted above
	if info.Total > 0 && info.Orphaned > info.Total {
		info.Orphaned = 0
		for _, d := range info.Details {
			if d.Orphaned {
				info.Orphaned++
			}
		}
	}

	if len(times) > 0 {
		oldest, newest := times[0], times[0]
		for _, t := range times[1:] {
			if t.Before(oldest) {
				oldest = t
			}
			if t.After(newest) {
				newest = t
			}
		}
		info.Oldest = oldest.Format(time.RFC3339)
		info.Newest = newest.Format(time.RFC3339)
	}

	return info, nil
}

func extractAppNameFromRP(name string) string {
	// Pattern: "<appname>-scheduled-XXXXX" or "<appname>-XXXXX"
	for _, suffix := range []string{"-scheduled-", "-exported-", "-snapshot-"} {
		if idx := strings.LastIndex(name, suffix); idx > 0 {
			return name[:idx]
		}
	}
	// Fallback: strip last segment after last "-"
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		return name[:idx]
	}
	return name
}

// ── Multi-cluster ─────────────────────────────────────────────────────────────

func collectMultiCluster(c *cluster.Client, ns string) (MultiClusterInfo, error) {
	ctx := context.Background()
	info := MultiClusterInfo{Mode: "standalone"}

	// K10 8.x uses dist.kio.kasten.io/v1alpha1 clusters
	gvrCluster := schema.GroupVersionResource{Group: "dist.kio.kasten.io", Version: "v1alpha1", Resource: "clusters"}
	clusterList, err := c.Dynamic.Resource(gvrCluster).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err == nil && len(clusterList.Items) > 0 {
		for _, item := range clusterList.Items {
			obj := item.Object
			spec := GetMap(obj, "spec")
			status := GetMap(obj, "status")
			rc := RemoteCluster{
				Name:    MetaName(obj),
				URL:     GetString(spec, "k10Url"),
				Status:  GetString(status, "state"),
				Version: GetString(status, "version"),
			}
			info.Clusters = append(info.Clusters, rc)
			if info.Mode == "standalone" {
				info.Mode = "primary"
			}
		}
	}

	// Fallback: old clusterprofiles CRD
	if len(info.Clusters) == 0 {
		cpList, err := c.Dynamic.Resource(gvrClusterProfile).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			for _, item := range cpList.Items {
				obj := item.Object
				spec := GetMap(obj, "spec")
				status := GetMap(obj, "status")
				info.Clusters = append(info.Clusters, RemoteCluster{
					Name:    MetaName(obj),
					URL:     GetString(spec, "k10Url"),
					Status:  GetString(status, "state"),
					Version: GetString(status, "version"),
				})
				if info.Mode == "standalone" {
					info.Mode = "primary"
				}
			}
		}
	}

	// mc-config configmap
	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "mc-config", metav1.GetOptions{})
	if err == nil {
		data := cm.Data
		switch {
		case data["isPrimary"] == "true" || data["role"] == "primary":
			info.Mode = "primary"
		case data["isPrimary"] == "false" || data["role"] == "secondary":
			info.Mode = "secondary"
			info.PrimaryURL = data["primaryURL"]
		}
	}

	return info, nil
}

// ── Disaster Recovery ─────────────────────────────────────────────────────────

func collectDR(c *cluster.Client, ns string, policies []Policy) (DRInfo, error) {
	info := DRInfo{}
	ctx := context.Background()

	rawList, err := c.Dynamic.Resource(GVRPolicy).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, item := range rawList.Items {
			obj := item.Object
			spec := GetMap(obj, "spec")
			name := MetaName(obj)
			nl := strings.ToLower(name)

			kdrCfg := GetMap(obj, "spec", "kdrSnapshotConfiguration")
			isKDR := GetString(spec, "subType") == "K10DR" ||
				strings.Contains(nl, "k10dr") ||
				strings.Contains(nl, "disaster-recovery") ||
				strings.Contains(nl, "disaster_recovery") ||
				len(kdrCfg) > 0

			if !isKDR {
				continue
			}

			info.Enabled = true
			info.BackupPolicy = name

			for _, pol := range policies {
				if pol.Name == name {
					info.LastRunTime = pol.LastRunTime
					info.LastRunStatus = pol.LastRunStatus
					if len(pol.ExportProfiles) > 0 {
						info.ExportProfile = pol.ExportProfiles[0]
					}
					break
				}
			}

			if info.ExportProfile == "" {
				for _, a := range GetSlice(spec, "actions") {
					am, ok := a.(map[string]interface{})
					if !ok {
						continue
					}
					if GetString(am, "action") == "export" {
						if ep, ok2 := am["exportParameters"].(map[string]interface{}); ok2 {
							if prof, ok3 := ep["profile"].(map[string]interface{}); ok3 {
								info.ExportProfile = GetString(prof, "name")
							}
						}
					}
				}
			}

			switch {
			case strings.Contains(nl, "primary"):
				info.Mode = "primary"
			case strings.Contains(nl, "secondary"):
				info.Mode = "secondary"
			default:
				info.Mode = "primary"
			}
			break
		}
	}

	if !info.Enabled {
		for _, pol := range policies {
			nl := strings.ToLower(pol.Name)
			if pol.SubType == "K10DR" ||
				strings.Contains(nl, "k10dr") ||
				strings.Contains(nl, "disaster-recovery") {
				info.Enabled = true
				info.BackupPolicy = pol.Name
				info.LastRunTime = pol.LastRunTime
				info.LastRunStatus = pol.LastRunStatus
				if len(pol.ExportProfiles) > 0 {
					info.ExportProfile = pol.ExportProfiles[0]
				}
				break
			}
		}
	}

	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "k10-dr-config", metav1.GetOptions{})
	if err == nil {
		if cm.Data["enabled"] == "true" {
			info.Enabled = true
		}
		if v, ok := cm.Data["mode"]; ok && info.Mode == "" {
			info.Mode = v
		}
	}

	return info, nil
}

// ── KubeVirt ──────────────────────────────────────────────────────────────────

func collectKubeVirt(c *cluster.Client, apps AppSummary) (KubeVirtInfo, error) {
	ctx := context.Background()
	info := KubeVirtInfo{}

	vmList, err := c.Dynamic.Resource(GVRVMI).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return info, nil
	}

	info.Enabled = true

	protectedNS := map[string]bool{}
	for _, app := range apps.Apps {
		if app.Protected {
			protectedNS[app.Namespace] = true
		}
	}

	for _, item := range vmList.Items {
		obj := item.Object
		status := GetMap(obj, "status")
		vmNS := MetaNamespace(obj)

		if info.Version == "" {
			info.Version = GetLabel(obj, "kubevirt.io/version")
		}

		protected := protectedNS[vmNS]
		vm := VMInfo{
			Name:      MetaName(obj),
			Namespace: vmNS,
			Status:    GetString(status, "printableStatus"),
			Protected: protected,
		}
		for _, app := range apps.Apps {
			if app.Namespace == vmNS && app.Protected {
				vm.Policy = strings.Join(app.PolicyNames, ", ")
				break
			}
		}

		info.VMs = append(info.VMs, vm)
		info.TotalVMs++
		if protected {
			info.ProtectedVMs++
		} else {
			info.UnprotectedVMs++
		}
	}

	if info.Version == "" {
		kvList, err := c.Dynamic.Resource(gvrKubeVirt).Namespace("").List(ctx, metav1.ListOptions{})
		if err == nil && len(kvList.Items) > 0 {
			status := GetMap(kvList.Items[0].Object, "status")
			info.Version = GetString(status, "observedKubeVirtVersion")
		}
	}

	return info, nil
}

// enrichDRFromPolicies re-reads LastRunTime/LastRunStatus for the DR policy
// after enrichPolicyLastRun has populated them from RunActions.
// collectDR runs before enrichPolicyLastRun, so DR.LastRun may be empty on first pass.
func enrichDRFromPolicies(d *Data) {
	if !d.DR.Enabled || d.DR.BackupPolicy == "" {
		return
	}
	// Already populated — skip
	if d.DR.LastRunTime != "" {
		return
	}
	for _, pol := range d.Policies {
		if pol.Name == d.DR.BackupPolicy {
			if pol.LastRunTime != "" {
				d.DR.LastRunTime = pol.LastRunTime
				d.DR.LastRunStatus = pol.LastRunStatus
			}
			break
		}
	}
}
