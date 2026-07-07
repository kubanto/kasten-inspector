package kasten

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

// ── Policies ──────────────────────────────────────────────────────────────────

func collectPolicies(c *cluster.Client, ns string) ([]Policy, error) {
	ctx := context.Background()
	list, err := c.Dynamic.Resource(GVRPolicy).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var policies []Policy
	for _, item := range list.Items {
		obj := item.Object
		spec := GetMap(obj, "spec")
		status := GetMap(obj, "status")

		p := Policy{
			Name:          MetaName(obj),
			Namespace:     MetaNamespace(obj),
			Enabled:       !GetBool(spec, "paused"),
			CreatedAt:     MetaTimestamp(obj),
			LastRunTime:   GetString(status, "lastRunTime"),
			LastRunStatus: GetString(status, "lastRunStatus"),
		}

		subType := GetString(spec, "subType")
		p.SubType = subType
		p.IsSystemPolicy = subType == "K10DR" || subType == "Report" ||
			strings.Contains(strings.ToLower(p.Name), "k10dr") ||
			strings.Contains(strings.ToLower(p.Name), "dr-") ||
			strings.Contains(strings.ToLower(p.Name), "report") ||
			strings.Contains(strings.ToLower(p.Name), "system-report")
		p.IsClusterScoped = strings.EqualFold(subType, "ClusterScoped") ||
			strings.Contains(strings.ToLower(subType), "cluster")

		// K10 8.x: frequency is at spec.frequency (not spec.schedule.frequency)
		freq := GetString(spec, "frequency")
		if freq != "" {
			p.Frequency = normFrequency(freq)
		} else if sched, ok := spec["schedule"].(map[string]interface{}); ok {
			p.Frequency = buildFrequency(sched)
		}

		// Actions
		for _, a := range GetSlice(spec, "actions") {
			am, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			action := GetString(am, "action")
			if p.Action == "" {
				p.Action = action
			}
			if action == "export" {
				if ep, ok := am["exportParameters"].(map[string]interface{}); ok {
					if prof, ok := ep["profile"].(map[string]interface{}); ok {
						p.ExportProfiles = append(p.ExportProfiles, GetString(prof, "name"))
					}
				}
			}
		}

		// Selector: K10 8.x uses matchExpressions with key k10.kasten.io/appNamespace
		p.Selector = extractNamespacesFromSelector(spec)
		// A policy with no namespace selector (and not system/cluster-scoped) targets all namespaces
		p.IsWildcard = p.Selector == "" && !p.IsSystemPolicy && !p.IsClusterScoped

		// Retention
		if ret, ok := spec["retention"].(map[string]interface{}); ok {
			p.Retention = RetentionInfo{
				Hourly:  GetInt(ret, "hourly"),
				Daily:   GetInt(ret, "daily"),
				Weekly:  GetInt(ret, "weekly"),
				Monthly: GetInt(ret, "monthly"),
				Yearly:  GetInt(ret, "yearly"),
			}
		}

		policies = append(policies, p)
	}
	return policies, nil
}

// extractNamespacesFromSelector reads matchExpressions for k10.kasten.io/appNamespace.
func extractNamespacesFromSelector(spec map[string]interface{}) string {
	sel, ok := spec["selector"].(map[string]interface{})
	if !ok {
		return ""
	}
	exprs, ok := sel["matchExpressions"].([]interface{})
	if !ok {
		return ""
	}
	var namespaces []string
	for _, e := range exprs {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		key := GetString(em, "key")
		if key != "k10.kasten.io/appNamespace" {
			continue
		}
		for _, v := range GetSlice(em, "values") {
			if s, ok := v.(string); ok {
				namespaces = append(namespaces, s)
			}
		}
	}
	return strings.Join(namespaces, ", ")
}

// normFrequency converts K10 @period strings to human-readable labels.
func normFrequency(freq string) string {
	switch freq {
	case "@hourly":
		return "hourly"
	case "@daily":
		return "daily"
	case "@weekly":
		return "weekly"
	case "@monthly":
		return "monthly"
	case "@yearly", "@annually":
		return "yearly"
	case "@onDemand":
		return "on-demand"
	default:
		if strings.HasPrefix(freq, "@") {
			return strings.TrimPrefix(freq, "@")
		}
		return freq
	}
}

// ── PolicyPresets ─────────────────────────────────────────────────────────────

func collectPolicyPresets(c *cluster.Client, ns string) ([]PolicyPreset, error) {
	ctx := context.Background()
	list, err := c.Dynamic.Resource(GVRPolicyPreset).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var presets []PolicyPreset
	for _, item := range list.Items {
		obj := item.Object
		spec := GetMap(obj, "spec")
		preset := PolicyPreset{
			Name:      MetaName(obj),
			Namespace: MetaNamespace(obj),
			CreatedAt: MetaTimestamp(obj),
		}

		// K10 8.x preset structure: spec.backup.frequency and spec.export
		actions := []string{}
		if backup, ok := spec["backup"].(map[string]interface{}); ok {
			preset.Frequency = normFrequency(GetString(backup, "frequency"))
			if ret, ok := backup["retention"].(map[string]interface{}); ok {
				preset.Retention = RetentionInfo{
					Hourly:  GetInt(ret, "hourly"),
					Daily:   GetInt(ret, "daily"),
					Weekly:  GetInt(ret, "weekly"),
					Monthly: GetInt(ret, "monthly"),
					Yearly:  GetInt(ret, "yearly"),
				}
			}
			actions = append(actions, "backup")
		}
		if _, ok := spec["export"]; ok {
			actions = append(actions, "export")
		}
		if len(actions) > 0 {
			preset.Action = strings.Join(actions, "+")
		}

		// Fallback: legacy structure
		if preset.Frequency == "" {
			preset.Frequency = normFrequency(GetString(spec, "frequency"))
		}
		if preset.Frequency == "" {
			if sched, ok := spec["schedule"].(map[string]interface{}); ok {
				preset.Frequency = buildFrequency(sched)
			}
		}

		presets = append(presets, preset)
	}
	return presets, nil
}

// ── Profiles ──────────────────────────────────────────────────────────────────

func collectProfiles(c *cluster.Client, ns string) ([]Profile, error) {
	ctx := context.Background()
	list, err := c.Dynamic.Resource(GVRProfile).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var profiles []Profile
	for _, item := range list.Items {
		obj := item.Object
		spec := GetMap(obj, "spec")
		status := GetMap(obj, "status")

		p := Profile{
			Name:      MetaName(obj),
			Namespace: MetaNamespace(obj),
			CreatedAt: MetaTimestamp(obj),
			Ready:     GetString(status, "validation") == "Success",
		}

		if loc, ok := spec["locationSpec"].(map[string]interface{}); ok {
			p.Type = "Location"
			if obj2, ok := loc["objectStore"].(map[string]interface{}); ok {
				// K10 8.x uses "objectStoreType" (not "provider")
				p.Provider = GetString(obj2, "objectStoreType")
				if p.Provider == "" {
					p.Provider = GetString(obj2, "provider")
				}
				// bucket name: "name" field in K10 8.x (not "bucketName")
				p.Bucket = GetString(obj2, "name")
				if p.Bucket == "" {
					p.Bucket = GetString(obj2, "bucketName")
				}
				p.Region = GetString(obj2, "region")
				p.Endpoint = GetString(obj2, "endpoint")

				// K10 8.x immutability: "protectionPeriod" field (e.g. "24h0m0s", "360h0m0s")
				if pp := GetString(obj2, "protectionPeriod"); pp != "" && pp != "0s" {
					p.Immutability = true
					p.ImmutabilityPeriod = parseDuration(pp)
				}
				// Legacy: objectLockConfig
				if immut, ok := obj2["objectLockConfig"].(map[string]interface{}); ok {
					p.Immutability = true
					days := GetInt(immut, "lockDuration")
					unit := GetString(immut, "durationUnit")
					if days > 0 {
						p.ImmutabilityPeriod = immutPeriod(days, unit)
					}
				}
				if period := GetString(obj2, "immutabilityPeriod"); period != "" {
					p.Immutability = true
					p.ImmutabilityPeriod = period
				}
			}
			if nfs, ok := loc["fileStore"].(map[string]interface{}); ok {
				p.Provider = "NFS"
				p.Endpoint = GetString(nfs, "server") + ":" + GetString(nfs, "path")
			}
		} else if _, ok := spec["infraSpec"]; ok {
			p.Type = "Infrastructure"
		} else if _, ok := spec["infraParameters"]; ok {
			// K10 8.x: vSphere and other infrastructure profiles
			p.Type = "Infrastructure"
		} else if _, ok := spec["vSphereParameters"]; ok {
			p.Type = "Infrastructure"
		} else if _, ok := spec["kanisterToolsImage"]; ok {
			p.Type = "Kanister"
		} else if p.Type == "" {
			// Last resort: scan spec keys for infra/vsphere hints
			for k := range spec {
				kl := strings.ToLower(k)
				if strings.Contains(kl, "infra") || strings.Contains(kl, "vsphere") || strings.Contains(kl, "vcenter") {
					p.Type = "Infrastructure"
					break
				}
			}
		}

		profiles = append(profiles, p)
	}
	return profiles, nil
}

// parseDuration converts Go duration strings like "24h0m0s", "360h0m0s" to human-readable.
func parseDuration(d string) string {
	// Strip trailing "0m0s", "0s"
	d = strings.TrimSuffix(d, "0s")
	d = strings.TrimSuffix(d, "0m")
	d = strings.TrimSuffix(d, "m")
	// Parse hours
	var hours int
	if n, err := fmt.Sscanf(d, "%dh", &hours); n == 1 && err == nil {
		if hours%24 == 0 {
			return fmt.Sprintf("%dd", hours/24)
		}
		return fmt.Sprintf("%dh", hours)
	}
	return d
}

// immutPeriod formats an immutability duration.
func immutPeriod(value int, unit string) string {
	switch strings.ToLower(unit) {
	case "days", "day", "d":
		return fmt.Sprintf("%dd", value)
	case "hours", "hour", "h":
		return fmt.Sprintf("%dh", value)
	default:
		if value > 0 {
			return fmt.Sprintf("%d%s", value, unit)
		}
		return unit
	}
}

// ── Applications (K10 8.x: derived from namespaces, not a CRD) ───────────────

func collectApplications(c *cluster.Client, ns string, policies []Policy) (AppSummary, error) {
	ctx := context.Background()
	summary := AppSummary{Apps: []AppInfo{}}

	// Build map: namespace → policies that cover it
	policyForNS := map[string][]string{}
	policyFreqForNS := map[string]string{}
	for _, pol := range policies {
		if pol.IsSystemPolicy {
			continue
		}
		for _, targetNS := range strings.Split(pol.Selector, ", ") {
			targetNS = strings.TrimSpace(targetNS)
			if targetNS != "" {
				policyForNS[targetNS] = append(policyForNS[targetNS], pol.Name)
				if policyFreqForNS[targetNS] == "" {
					policyFreqForNS[targetNS] = pol.Frequency
				}
			}
		}
	}

	// List all non-system namespaces as the application inventory
	nsList, err := c.Typed.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}

	for _, n := range nsList.Items {
		if IsSystemNamespace(n.Name) {
			continue
		}
		// Skip Terminating namespaces — they are being deleted
		if !IsActiveNamespace(string(n.Status.Phase)) {
			continue
		}
		polNames := policyForNS[n.Name]
		protected := len(polNames) > 0

		app := AppInfo{
			Name:        n.Name,
			Namespace:   n.Name,
			Protected:   protected,
			PolicyNames: polNames,
			Compliant:   protected,
		}

		summary.Apps = append(summary.Apps, app)
		if protected {
			summary.Protected++
		} else {
			summary.Unprotected++
		}
	}
	// Total = actual application count (non-system, active namespaces only)
	summary.Total = len(summary.Apps)
	return summary, nil
}

// ── Namespaces ────────────────────────────────────────────────────────────────

func collectNamespaces(c *cluster.Client, ns string, apps AppSummary) (NamespaceSummary, error) {
	ctx := context.Background()
	summary := NamespaceSummary{}

	nsList, err := c.Typed.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}
	summary.Total = len(nsList.Items)

	protected := map[string]bool{}
	for _, app := range apps.Apps {
		if app.Protected {
			protected[app.Namespace] = true
		}
	}

	for _, n := range nsList.Items {
		if IsSystemNamespace(n.Name) {
			summary.Excluded = append(summary.Excluded, n.Name)
			continue
		}
		if !protected[n.Name] {
			labels := map[string]string{}
			for k, v := range n.Labels {
				labels[k] = v
			}
			summary.Unprotected = append(summary.Unprotected, UnprotectedNS{
				Name:   n.Name,
				Labels: labels,
			})
		}
	}
	return summary, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func buildFrequency(schedule map[string]interface{}) string {
	if cron := GetString(schedule, "cronExpression"); cron != "" {
		return "cron: " + cron
	}
	for _, f := range []string{"hourly", "daily", "weekly", "monthly", "yearly"} {
		if v, ok := schedule[f]; ok && v != nil {
			return f
		}
	}
	return "custom"
}
