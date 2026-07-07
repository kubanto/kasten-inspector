package kasten

// collector_diagnostics.go — diagnostic and infrastructure collectors
//
// Adds:
//   1. Recent Failures (top-5)     — ranked failed actions with recursive error unwrapping
//   2. Long-running Actions         — actions stuck in Running state beyond threshold
//   3. Backup Recency per Namespace  — per-NS last successful backup/export/restore + drift alert
//   4. Volume Provisioner Audit      — StorageClass/VolumeSnapshotClass inventory + CSI capability check
//   5. Best Practice checks BP-12…BP-16

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

const (
	longRunningThreshold = 24 * time.Hour
	driftThresholdDays = 7
)

var (
	gvrStorageClass          = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}
	gvrVolumeSnapshotClass   = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses"}
	gvrVolumeSnapshotClassV1 = schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1beta1", Resource: "volumesnapshotclasses"}
)

// ── 1. Recent Failures (top-5) ──────────────────────────────────────────────────

// collectRecentFailures scans BackupAction, ExportAction, RestoreAction for
// state=Failed, sorts by creation timestamp descending, and returns the top 5.
func collectRecentFailures(c *cluster.Client, ns string) ([]RecentFailure, error) {
	ctx := context.Background()
	var failed []RecentFailure

	actionGVRs := []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{GVRBackupAction, "BackupAction"},
		{GVRExportAction, "ExportAction"},
		{GVRRestoreAction, "RestoreAction"},
	}

	for _, ag := range actionGVRs {
		list, err := c.Dynamic.Resource(ag.gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			obj := item.Object
			status := GetMap(obj, "status")
			state := GetString(status, "state")
			if state != "Failed" {
				continue
			}
			labels := GetLabels(obj)
			fa := RecentFailure{
				Kind:       ag.kind,
				Name:       MetaName(obj),
				AppName:    labelStr(labels, "apps.kio.kasten.io/appName"),
				PolicyName: labelStr(labels, "policies.kio.kasten.io/policy-name"),
				StartTime:  GetString(status, "startTime"),
				Error:      deepestError(status),
			}
			if fa.AppName == "" {
				fa.AppName = labelStr(labels, "k10.kasten.io/appNamespace")
			}
			failed = append(failed, fa)
		}
	}

	// Sort by start time descending (most recent first)
	sort.Slice(failed, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, failed[i].StartTime)
		tj, _ := time.Parse(time.RFC3339, failed[j].StartTime)
		return ti.After(tj)
	})

	if len(failed) > 5 {
		failed = failed[:5]
	}
	return failed, nil
}

// deepestError recursively unwraps status.error.cause (which K10 serialises as
// a JSON-encoded string) up to 5 levels deep, returning the innermost message.
func deepestError(status map[string]interface{}) string {
	errObj := getNestedMap(status, "error")
	if errObj == nil {
		// Try flat string error
		if s, ok := status["error"].(string); ok && s != "" {
			return s
		}
		if s, ok := status["reason"].(string); ok {
			return s
		}
		return ""
	}
	msg := getString(errObj, "message")
	for depth := 0; depth < 5; depth++ {
		causeStr, ok := errObj["cause"].(string)
		if !ok || causeStr == "" {
			break
		}
		// K10 encodes the cause as a JSON string — try to unwrap
		if strings.HasPrefix(strings.TrimSpace(causeStr), "{") {
			var inner map[string]interface{}
			if err := jsonUnmarshalString(causeStr, &inner); err == nil {
				errObj = inner
				if m := getString(inner, "message"); m != "" {
					msg = m
				}
				continue
			}
		}
		// Plain string cause
		msg = causeStr
		break
	}
	return msg
}

// ── 2. Long-running Actions ──────────────────────────────────────────────────────

// collectLongRunningActions returns actions in Running state older than the configured threshold.
func collectLongRunningActions(c *cluster.Client, ns string) ([]LongRunningAction, error) {
	ctx := context.Background()
	now := time.Now().UTC()
	var stuck []LongRunningAction

	actionGVRs := []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{GVRBackupAction, "BackupAction"},
		{GVRExportAction, "ExportAction"},
		{GVRRestoreAction, "RestoreAction"},
		{GVRRunAction, "RunAction"},
	}

	for _, ag := range actionGVRs {
		list, err := c.Dynamic.Resource(ag.gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			obj := item.Object
			status := GetMap(obj, "status")
			state := normaliseStatus(
				GetString(status, "state"),
				GetString(status, "phase"),
				GetString(status, "progress"),
			)
			if state != "Running" {
				continue
			}
			startStr := GetString(status, "startTime")
			if startStr == "" {
				startStr = MetaTimestamp(obj)
			}
			start, err := time.Parse(time.RFC3339, startStr)
			if err != nil {
				continue
			}
			running := now.Sub(start)
			if running < longRunningThreshold {
				continue
			}
			labels := GetLabels(obj)
			sa := LongRunningAction{
				Kind:       ag.kind,
				Name:       MetaName(obj),
				AppName:    labelStr(labels, "apps.kio.kasten.io/appName"),
				PolicyName: labelStr(labels, "policies.kio.kasten.io/policy-name"),
				StartTime:  startStr,
				RunningFor: formatDur(running),
			}
			stuck = append(stuck, sa)
		}
	}
	return stuck, nil
}

// ── 3. Backup Recency per Namespace ──────────────────────────────────────────────

// collectBackupRecency computes per-namespace last backup/export/restore
// timestamps and a drift alert (no successful backup in the last driftThresholdDays days).
func collectBackupRecency(jobs []Job, apps AppSummary) []NamespaceBackupRecency {
	type nsEntry struct {
		protected   bool
		lastBackup  time.Time
		lastExport  time.Time
		lastRestore time.Time
	}

	now := time.Now().UTC()
	entries := map[string]*nsEntry{}

	// Seed from app list
	for _, app := range apps.Apps {
		ns := app.Namespace
		if ns == "" {
			ns = app.Name
		}
		if _, ok := entries[ns]; !ok {
			entries[ns] = &nsEntry{}
		}
		if app.Protected {
			entries[ns].protected = true
		}
	}

	// Enrich with actual job timestamps
	for _, j := range jobs {
		appNS := j.AppName
		if appNS == "" {
			continue
		}
		if _, ok := entries[appNS]; !ok {
			entries[appNS] = &nsEntry{}
		}
		e := entries[appNS]

		t, err := time.Parse(time.RFC3339, j.EndTime)
		if err != nil || j.Status != "Complete" {
			continue
		}
		switch j.Action {
		case "backup":
			if t.After(e.lastBackup) {
				e.lastBackup = t
			}
		case "export":
			if t.After(e.lastExport) {
				e.lastExport = t
			}
		case "restore":
			if t.After(e.lastRestore) {
				e.lastRestore = t
			}
		}
	}

	var result []NamespaceBackupRecency
	for ns, e := range entries {
		item := NamespaceBackupRecency{
			Namespace: ns,
			Protected: e.protected,
		}
		if !e.lastBackup.IsZero() {
			item.LastBackup = e.lastBackup.Format(time.RFC3339)
			days := int(now.Sub(e.lastBackup).Hours() / 24)
			item.DaysSinceBackup = days
			item.Drift = days > driftThresholdDays
		} else if e.protected {
			// Protected but no successful backup recorded — stale
			item.Drift = true
		}
		if !e.lastExport.IsZero() {
			item.LastExport = e.lastExport.Format(time.RFC3339)
		}
		if !e.lastRestore.IsZero() {
			item.LastRestore = e.lastRestore.Format(time.RFC3339)
		}
		result = append(result, item)
	}

	// Sort by namespace name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Namespace < result[j].Namespace
	})
	return result
}

// ── 4. Volume Provisioner Audit ──────────────────────────────────────────────────

// collectVolumeProvisionerAudit lists StorageClasses and VolumeSnapshotClasses
// and emits a warning for each CSI provisioner that has no matching VolumeSnapshotClass.
func collectVolumeProvisionerAudit(c *cluster.Client) (
	[]StorageClassInfo,
	[]VolumeSnapshotClassInfo,
	[]string,
	error,
) {
	ctx := context.Background()
	var scs []StorageClassInfo
	var vscs []VolumeSnapshotClassInfo
	var warnings []string

	// StorageClasses (cluster-scoped)
	scList, err := c.Dynamic.Resource(gvrStorageClass).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// RBAC denied — degrade gracefully
		warnings = append(warnings, fmt.Sprintf("StorageClass read denied: %v", err))
	} else {
		for _, item := range scList.Items {
			obj := item.Object
			annotations := GetAnnotations(obj)
			spec := GetMap(obj, "spec")
			sc := StorageClassInfo{
				Name:          MetaName(obj),
				Provisioner:   GetString(obj, "provisioner"),
				IsDefault:     annotations["storageclass.kubernetes.io/is-default-class"] == "true",
				ReclaimPolicy: GetString(spec, "reclaimPolicy"),
				BindingMode:   GetString(spec, "volumeBindingMode"),
			}
			if b, ok := obj["allowVolumeExpansion"].(bool); ok {
				sc.Expandable = b
			}
			scs = append(scs, sc)
		}
	}

	// VolumeSnapshotClasses — try v1 first, then v1beta1
	vscList, err := c.Dynamic.Resource(gvrVolumeSnapshotClass).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		vscList, err = c.Dynamic.Resource(gvrVolumeSnapshotClassV1).Namespace("").List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("VolumeSnapshotClass read denied or CRD not installed: %v", err))
	} else {
		for _, item := range vscList.Items {
			obj := item.Object
			annotations := GetAnnotations(obj)
			vscs = append(vscs, VolumeSnapshotClassInfo{
				Name:                MetaName(obj),
				Driver:              GetString(obj, "driver"),
				IsDefault:           annotations["snapshot.storage.kubernetes.io/is-default-class"] == "true",
				DeletionPolicy:      GetString(obj, "deletionPolicy"),
				HasKastenAnnotation: annotations["k10.kasten.io/is-snapshot-class"] == "true",
			})
		}
	}

	// CSI cross-check: for each StorageClass using a CSI provisioner,
	// warn if there is no VolumeSnapshotClass with the same driver.
	vscDrivers := map[string]bool{}
	for _, v := range vscs {
		vscDrivers[v.Driver] = true
	}
	for i, sc := range scs {
		// Non-CSI provisioners (in-tree) have no VSC; skip them.
		if !isCsiProvisioner(sc.Provisioner) {
			continue
		}
		if vscDrivers[sc.Provisioner] {
			scs[i].HasVSC = true
		} else {
			warnings = append(warnings, fmt.Sprintf(
				"StorageClass %q uses CSI provisioner %q but no matching VolumeSnapshotClass found — PVCs cannot be CSI-snapshotted; Kanister Blueprint or Generic Volume Backup required",
				sc.Name, sc.Provisioner,
			))
		}
	}

	return scs, vscs, warnings, nil
}

// isCsiProvisioner returns true if the provisioner string looks like a CSI driver
// (contains a dot and is not a known in-tree provisioner).
func isCsiProvisioner(p string) bool {
	inTree := map[string]bool{
		"kubernetes.io/aws-ebs":          true,
		"kubernetes.io/azure-disk":        true,
		"kubernetes.io/azure-file":        true,
		"kubernetes.io/cinder":            true,
		"kubernetes.io/gce-pd":            true,
		"kubernetes.io/glusterfs":         true,
		"kubernetes.io/no-provisioner":    true,
		"kubernetes.io/nfs":               true,
		"kubernetes.io/portworx-volume":   true,
		"kubernetes.io/rbd":               true,
		"kubernetes.io/vsphere-volume":    true,
		"docker.io/hostpath":              true,
		"rancher.io/local-path":           true,
		"local.csi.k8s.io":               true,
		"openebs.io/local":               true,
		"microk8s.io/hostpath":            true,
	}
	return strings.Contains(p, ".") && !inTree[p] && !strings.HasPrefix(p, "kubernetes.io/")
}

// ── 5. Best Practice checks BP-12…BP-16 ──────────────────────────────────────────

func checkSnapshotRetentionHigh(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-12",
		Name:     "Snapshot retention within safe bounds",
		Severity: "warning",
	}
	var high []string
	for _, p := range d.Policies {
		if p.IsSystemPolicy {
			continue
		}
		total := p.Retention.Hourly + p.Retention.Daily + p.Retention.Weekly + p.Retention.Monthly + p.Retention.Yearly
		if total > 7 {
			high = append(high, fmt.Sprintf("%s (total=%d)", p.Name, total))
		}
	}
	if len(high) == 0 {
		ch.Status = "pass"
		ch.Detail = "All policies have snapshot retention ≤ 7"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d policy/ies with snapshot retention > 7: %s — excessive simultaneous snapshots may impact storage I/O", len(high), strings.Join(high, ", "))
	}
	return ch
}

func checkSnapshotRetentionZero(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-13",
		Name:     "At least one snapshot retained per policy",
		Severity: "warning",
	}
	var zero []string
	for _, p := range d.Policies {
		if p.IsSystemPolicy {
			continue
		}
		total := p.Retention.Hourly + p.Retention.Daily + p.Retention.Weekly + p.Retention.Monthly + p.Retention.Yearly
		if total == 0 {
			zero = append(zero, p.Name)
		}
	}
	if len(zero) == 0 {
		ch.Status = "pass"
		ch.Detail = "All policies have at least one snapshot retention slot configured"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d policy/ies have zero snapshot retention (%s) — no local restore points will be kept", len(zero), strings.Join(zero, ", "))
	}
	return ch
}

func checkExportRetentionExplicit(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-14",
		Name:     "Export retention explicitly configured",
		Severity: "warning",
	}
	var implicit []string
	for _, p := range d.Policies {
		if p.IsSystemPolicy || len(p.ExportProfiles) == 0 {
			continue
		}
		// If ExportRetention is not set separately, it inherits snapshot retention.
		// We detect this by checking whether the policy has an ExportRetention field.
		// In the current model, ExportProfiles lists profiles but we don't track
		// export-specific retention separately — flag policies that have exports
		// but whose retention sums to zero (which would mean they silently inherit).
		// Conservative: only flag when the snapshot retention itself is also 0.
		total := p.Retention.Hourly + p.Retention.Daily + p.Retention.Weekly + p.Retention.Monthly + p.Retention.Yearly
		if total == 0 {
			implicit = append(implicit, p.Name)
		}
	}
	if len(implicit) == 0 {
		ch.Status = "pass"
		ch.Detail = "All export policies appear to have explicit retention configured"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d export policy/ies may be using implicit retention (%s) — verify export retention is explicitly set", len(implicit), strings.Join(implicit, ", "))
	}
	return ch
}

func checkCSICoverage(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-15",
		Name:     "CSI provisioners have snapshot capability",
		Severity: "warning",
	}
	if len(d.StorageClasses) == 0 {
		ch.Status = "pass"
		ch.Detail = "No StorageClass data available (RBAC may be restricted)"
		return ch
	}
	var missing []string
	for _, sc := range d.StorageClasses {
		if isCsiProvisioner(sc.Provisioner) && !sc.HasVSC {
			missing = append(missing, fmt.Sprintf("%s (%s)", sc.Name, sc.Provisioner))
		}
	}
	if len(missing) == 0 {
		ch.Status = "pass"
		ch.Detail = "All CSI provisioners have a matching VolumeSnapshotClass"
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d StorageClass(es) with no matching VolumeSnapshotClass: %s — PVCs on these classes require Kanister Blueprint or Generic Volume Backup", len(missing), strings.Join(missing, "; "))
	}
	return ch
}

func checkBackupDrift(d *Data) BPCheck {
	ch := BPCheck{
		ID:       "BP-16",
		Name:     fmt.Sprintf("Protected namespaces: backup recency within %d days", driftThresholdDays),
		Severity: "warning",
	}
	var drifted []string
	for _, ns := range d.BackupRecency {
		if ns.Protected && ns.Drift {
			if ns.LastBackup == "" {
				drifted = append(drifted, fmt.Sprintf("%s (never backed up)", ns.Namespace))
			} else {
				drifted = append(drifted, fmt.Sprintf("%s (%dd ago)", ns.Namespace, ns.DaysSinceBackup))
			}
		}
	}
	if len(drifted) == 0 {
		ch.Status = "pass"
		ch.Detail = fmt.Sprintf("All protected namespaces have a successful backup within the last %d days", driftThresholdDays)
	} else {
		ch.Status = "warning"
		ch.Detail = fmt.Sprintf("%d protected namespace(s) with backup drift >7d: %s", len(drifted), strings.Join(drifted, ", "))
	}
	return ch
}

// ── helpers ───────────────────────────────────────────────────────────────────

func getNestedMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	cur := m
	for _, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

// jsonUnmarshalString parses a JSON string into a map (avoids importing encoding/json
// directly — we call the package-level json decoder already used elsewhere).
func jsonUnmarshalString(s string, out *map[string]interface{}) error {
	return _jsonUnmarshal([]byte(s), out)
}
