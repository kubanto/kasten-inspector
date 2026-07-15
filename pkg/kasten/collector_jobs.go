package kasten

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

func collectJobs(c *cluster.Client, ns string, limit int) ([]Job, error) {
	ctx := context.Background()
	var jobs []Job

	actionGVRs := []struct {
		gvr    schema.GroupVersionResource
		action string
	}{
		{GVRRunAction, "run"},
		{GVRBackupAction, "backup"},
		{GVRRestoreAction, "restore"},
		{GVRExportAction, "export"},
	}

	for _, ag := range actionGVRs {
		list, err := c.Dynamic.Resource(ag.gvr).Namespace(ns).List(ctx, metav1.ListOptions{
			Limit: int64(limit),
		})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			obj := item.Object
			labels := GetLabels(obj)
			status := GetMap(obj, "status")
			spec := GetMap(obj, "spec")

			j := Job{
				Name:      MetaName(obj),
				Namespace: MetaNamespace(obj),
				Action:    ag.action,
				StartTime: GetString(status, "startTime"),
				EndTime:   GetString(status, "endTime"),
			}

			switch ag.action {
			case "run":
				// K10 8.x RunAction: spec.subject.name = policy name
				// spec.subject.kind = "Policy"
				subjectName := GetString(spec, "subject", "name")
				subjectKind := GetString(spec, "subject", "kind")
				if subjectKind == "Policy" || subjectName != "" {
					j.PolicyName = subjectName
				}
				// AppName for run actions is not meaningful — leave empty
				j.AppName = ""

			case "backup":
				// BackupAction: labels have appName and policy
				j.PolicyName = labelStr(labels, "policies.kio.kasten.io/policy-name")
				if j.PolicyName == "" {
					j.PolicyName = labelStr(labels, "kasten.io/policy")
				}
				j.AppName = labelStr(labels, "apps.kio.kasten.io/appName")
				if j.AppName == "" {
					// Fallback: namespace of the action
					j.AppName = MetaNamespace(obj)
				}

			case "restore":
				j.PolicyName = labelStr(labels, "policies.kio.kasten.io/policy-name")
				// For restore, subject is a RestorePoint — show its namespace
				j.AppName = GetString(spec, "targetNamespace")
				if j.AppName == "" {
					j.AppName = GetString(spec, "subject", "name")
				}

			case "export":
				j.PolicyName = labelStr(labels, "policies.kio.kasten.io/policy-name")
				if j.PolicyName == "" {
					j.PolicyName = labelStr(labels, "kasten.io/policy")
				}
				// Export app: prefer namespace label over UUID
				j.AppName = labelStr(labels, "apps.kio.kasten.io/appName")
				if j.AppName == "" {
					// Try spec.subject.namespace (K10 8.x)
					j.AppName = GetString(spec, "subject", "namespace")
				}
				if j.AppName == "" {
					// Last resort: spec.subject.name (may be UUID — omit if UUID-like)
					name := GetString(spec, "subject", "name")
					if !isUUID(name) {
						j.AppName = name
					}
				}
			}

			j.Status = normaliseStatus(
				GetString(status, "state"),
				GetString(status, "phase"),
				GetString(status, "progress"),
			)

			if j.StartTime != "" && j.EndTime != "" {
				if start, err2 := time.Parse(time.RFC3339, j.StartTime); err2 == nil {
					if end, err2 := time.Parse(time.RFC3339, j.EndTime); err2 == nil {
						d := end.Sub(start)
						j.DurationSec = int64(d.Seconds())
						j.Duration = formatDur(d)
					}
				}
			}

			if j.Status == "Failed" {
				j.Error = firstNonEmpty(
					GetString(status, "error", "message"),
					GetString(status, "error"),
					GetString(status, "reason"),
				)
			}

			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func enrichPolicyDurations(d *Data) {
	type bucket struct {
		total int64
		count int
	}
	m := map[string]*bucket{}

	for _, j := range d.Jobs {
		if j.PolicyName == "" || j.DurationSec == 0 || j.Status != "Complete" {
			continue
		}
		if m[j.PolicyName] == nil {
			m[j.PolicyName] = &bucket{}
		}
		m[j.PolicyName].total += j.DurationSec
		m[j.PolicyName].count++
	}

	for i, pol := range d.Policies {
		if b, ok := m[pol.Name]; ok && b.count > 0 {
			avg := time.Duration(b.total/int64(b.count)) * time.Second
			d.Policies[i].AvgRunDuration = formatDur(avg)
		}
	}

	if d.Compliance.AvgPolicyDuration == nil {
		d.Compliance.AvgPolicyDuration = map[string]string{}
	}
	for name, b := range m {
		if b.count > 0 {
			avg := time.Duration(b.total/int64(b.count)) * time.Second
			d.Compliance.AvgPolicyDuration[name] = formatDur(avg)
		}
	}
}

// enrichPolicyLastRun populates LastRunStatus and LastRunTime for each policy
// from the collected RunActions, since K10 8.x does not always write these back
// to the Policy CRD status field.
func enrichPolicyLastRun(d *Data) {
	type lastRun struct {
		time   string
		status string
	}
	latest := map[string]lastRun{}

	for _, job := range d.Jobs {
		if job.Action != "run" || job.PolicyName == "" {
			continue
		}
		// Skip skipped — not informative for compliance
		if job.Status == "Skipped" {
			continue
		}
		prev, exists := latest[job.PolicyName]
		if !exists || job.StartTime > prev.time {
			latest[job.PolicyName] = lastRun{time: job.StartTime, status: job.Status}
		}
	}

	for i, pol := range d.Policies {
		if lr, ok := latest[pol.Name]; ok {
			if d.Policies[i].LastRunTime == "" {
				d.Policies[i].LastRunTime = lr.time
			}
			if d.Policies[i].LastRunStatus == "" {
				d.Policies[i].LastRunStatus = lr.status
			}
		}
	}
}


// ── helpers ───────────────────────────────────────────────────────────────────

func labelStr(labels map[string]interface{}, key string) string {
	v, _ := labels[key].(string)
	return v
}

func normaliseStatus(state, phase, progress string) string {
	for _, s := range []string{state, phase, progress} {
		switch s {
		case "Complete", "Succeeded", "Success":
			return "Complete"
		case "Failed", "Error":
			return "Failed"
		case "Running", "InProgress", "Active":
			return "Running"
		case "Skipped":
			return "Skipped"
		case "Cancelled", "Canceled":
			return "Cancelled"
		case "Pending":
			return "Pending"
		}
	}
	if state != "" {
		return state
	}
	return "Unknown"
}

func formatDur(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isUUID returns true if s looks like a UUID (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// enrichAppLastBackup populates LastBackup for each application by finding
// the most recent completed RunAction for policies that cover that namespace.
// In K10 8.x, individual BackupActions are garbage-collected quickly, but
// RunActions persist and carry the policy name, which maps to a namespace selector.
func enrichAppLastBackup(d *Data) {
	// Build map: namespace → most recent successful run time
	latestRun := map[string]string{}

	for _, job := range d.Jobs {
		if job.Action != "run" || job.Status != "Complete" || job.PolicyName == "" {
			continue
		}
		// Find which namespaces this policy covers
		for _, pol := range d.Policies {
			if pol.Name != job.PolicyName || pol.IsSystemPolicy {
				continue
			}
			for _, targetNS := range splitSelector(pol.Selector) {
				prev := latestRun[targetNS]
				if prev == "" || job.StartTime > prev {
					latestRun[targetNS] = job.StartTime
				}
			}
		}
	}

	// Fallback: most recent restore point per app. A restore point proves a backup
	// occurred even when the completing "run" action is outside the collected job
	// window or never reached Complete (e.g. on-demand policies). Without this, apps
	// with real restore points were wrongly reported as "never backed up".
	newestRP := map[string]string{}
	for _, rp := range d.RestorePoints.Details {
		if rp.AppName == "" || rp.CreatedAt == "" {
			continue
		}
		if prev, ok := newestRP[rp.AppName]; !ok || rp.CreatedAt > prev {
			newestRP[rp.AppName] = rp.CreatedAt
		}
	}

	for i, app := range d.Applications.Apps {
		if t, ok := latestRun[app.Namespace]; ok {
			d.Applications.Apps[i].LastBackup = t
			continue
		}
		if d.Applications.Apps[i].LastBackup != "" {
			continue
		}
		if t, ok := newestRP[app.Name]; ok {
			d.Applications.Apps[i].LastBackup = t
		} else if t, ok := newestRP[app.Namespace]; ok {
			d.Applications.Apps[i].LastBackup = t
		}
	}
}

// splitSelector splits a comma-separated namespace selector string.
func splitSelector(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
