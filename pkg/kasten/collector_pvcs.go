package kasten

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

// collectPVCs collects all PVCs across all non-system namespaces.
func collectPVCs(c *cluster.Client) (PVCSummary, error) {
	ctx := context.Background()
	summary := PVCSummary{}

	pvcs, err := c.Typed.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return summary, err
	}

	for _, pvc := range pvcs.Items {
		// Skip K10 internal PVCs
		if IsSystemNamespace(pvc.Namespace) {
			continue
		}

		info := PVCInfo{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Status:    string(pvc.Status.Phase),
		}

		if pvc.Spec.StorageClassName != nil {
			info.StorageClass = *pvc.Spec.StorageClassName
		}

		// Access modes
		modes := []string{}
		for _, m := range pvc.Spec.AccessModes {
			switch m {
			case corev1.ReadWriteOnce:
				modes = append(modes, "RWO")
			case corev1.ReadWriteMany:
				modes = append(modes, "RWX")
			case corev1.ReadOnlyMany:
				modes = append(modes, "ROX")
			case corev1.ReadWriteOncePod:
				modes = append(modes, "RWOP")
			}
		}
		info.AccessModes = strings.Join(modes, ",")

		// Capacity
		if cap := pvc.Status.Capacity.Storage(); cap != nil {
			info.CapacityGB = float64(cap.Value()) / 1024 / 1024 / 1024
		} else if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			info.CapacityGB = float64(req.Value()) / 1024 / 1024 / 1024
		}

		summary.Items = append(summary.Items, info)
		summary.Total++
		summary.TotalSizeGB += info.CapacityGB

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			summary.Bound++
		case corev1.ClaimPending:
			summary.Pending++
		case corev1.ClaimLost:
			summary.Lost++
		}
	}

	return summary, nil
}

// collectCoverageMatrix builds a namespace×policy coverage table.
func collectCoverageMatrix(namespaces NamespaceSummary, policies []Policy) []PolicyCoverageRow {
	// Build map: namespace → list of (policy name, frequency, last run)
	type policyMatch struct {
		name      string
		frequency string
		lastRun   string
	}
	nsToPolicies := map[string][]policyMatch{}

	for _, pol := range policies {
		if pol.IsSystemPolicy {
			continue
		}
		// Selector may contain namespace names or be empty (covers all)
		if pol.Selector == "" {
			// Covers all — mark every unprotected NS
			continue
		}
		for _, ns := range strings.Split(pol.Selector, ", ") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				nsToPolicies[ns] = append(nsToPolicies[ns], policyMatch{
					name:      pol.Name,
					frequency: pol.Frequency,
					lastRun:   pol.LastRunTime,
				})
			}
		}
	}

	var rows []PolicyCoverageRow

	// Protected namespaces (from apps)
	seen := map[string]bool{}
	for ns, matches := range nsToPolicies {
		if IsSystemNamespace(ns) {
			continue
		}
		seen[ns] = true
		row := PolicyCoverageRow{
			Namespace: ns,
			Protected: true,
		}
		for _, m := range matches {
			row.Policies = append(row.Policies, m.name)
			if row.Frequency == "" {
				row.Frequency = m.frequency
			}
			if row.LastBackup == "" {
				row.LastBackup = m.lastRun
			}
		}
		rows = append(rows, row)
	}

	// Unprotected namespaces
	for _, ns := range namespaces.Unprotected {
		if !seen[ns.Name] {
			rows = append(rows, PolicyCoverageRow{
				Namespace: ns.Name,
				Protected: false,
			})
		}
	}

	// Sort: protected first
	protected := []PolicyCoverageRow{}
	unprotected := []PolicyCoverageRow{}
	for _, r := range rows {
		if r.Protected {
			protected = append(protected, r)
		} else {
			unprotected = append(unprotected, r)
		}
	}
	return append(protected, unprotected...)
}
