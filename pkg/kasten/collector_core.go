package kasten

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

// isHexDigest returns true if s is a raw SHA256 hex string (64 lowercase hex chars)
// used as an image tag on some registries instead of a proper version tag.
func isHexDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func collectVersion(c *cluster.Client, ns string) (string, error) {
	ctx := context.Background()
	for _, name := range []string{"catalog-svc", "catalog", "gateway-svc", "gateway", "executor-svc", "executor", "frontend-svc", "frontend"} {
		dep, err := c.Typed.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		for _, cont := range dep.Spec.Template.Spec.Containers {
			img := cont.Image
			if strings.Contains(img, "kasten") || strings.Contains(img, "k10") {
				// Standard tag (e.g. :6.5.0)
			if idx := strings.LastIndex(img, ":"); idx >= 0 {
					tag := img[idx+1:]
					if tag != "" && tag != "latest" &&
						!strings.HasPrefix(tag, "sha256:") &&
						!isHexDigest(tag) {
						return tag, nil
					}
				}
				// Red Hat registry: image@sha256:... — fall through to label check
				if strings.Contains(img, "@sha256:") {
					_ = img
				}
			}
		}
		// Check version from annotations too
		for _, annotKey := range []string{
			"app.kubernetes.io/version",
			"helm.sh/chart",
			"k10.kasten.io/version",
		} {
			if dep.Annotations != nil {
				if v, ok := dep.Annotations[annotKey]; ok && v != "" {
					v = strings.TrimPrefix(v, "k10-")
					if v != "" && !strings.Contains(v, "sha256") {
						return v, nil
					}
				}
			}
		}
		// Check version labels
		for _, labelKey := range []string{
			"app.kubernetes.io/version",
			"helm.sh/chart",
			"chart",
		} {
			if v, ok := dep.Labels[labelKey]; ok && v != "" {
				// helm.sh/chart is like "k10-6.5.0" — extract version
				if strings.Contains(v, "k10-") {
					v = strings.TrimPrefix(v, "k10-")
				}
				if v != "" {
					return v, nil
				}
			}
			if v, ok := dep.Spec.Template.Labels[labelKey]; ok && v != "" {
				if strings.Contains(v, "k10-") {
					v = strings.TrimPrefix(v, "k10-")
				}
				return v, nil
			}
		}
		// Check env vars for version
		for _, cont := range dep.Spec.Template.Spec.Containers {
			for _, env := range cont.Env {
				if env.Name == "K10_VERSION" || env.Name == "APP_VERSION" {
					if env.Value != "" {
						return env.Value, nil
					}
				}
			}
		}
	}
	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "k10-config", metav1.GetOptions{})
	if err == nil {
		if v, ok := cm.Data["version"]; ok {
			return v, nil
		}
	}
	return "unknown", nil
}

func collectHelmConfig(c *cluster.Client, ns string) (HelmConfig, error) {
	ctx := context.Background()
	cfg := HelmConfig{Namespace: ns}

	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "k10-config", metav1.GetOptions{})
	if err == nil {
		cfg.Values = make(map[string]string)
		for k, v := range cm.Data {
			cfg.Values[k] = v
		}
		if v, ok := cm.Data["concurrencyLimit"]; ok {
			fmt.Sscanf(v, "%d", &cfg.ConcurrencyLimit)
		}
		if v, ok := cm.Data["backupTimeout"]; ok {
			cfg.BackupTimeout = v
		}
		if v, ok := cm.Data["restoreTimeout"]; ok {
			cfg.RestoreTimeout = v
		}
		if v, ok := cm.Data["datastoreParallelism"]; ok {
			fmt.Sscanf(v, "%d", &cfg.DatastoreParallelism)
		}
		cfg.FIPSMode = cm.Data["fips"] == "true" || cm.Data["fipsMode"] == "true"
		cfg.AuditLogging = cm.Data["auditLogging"] == "true"
		cfg.NetworkPolicies = cm.Data["networkPolicies"] == "true" || cm.Data["networkPolicy"] == "enabled"
	}

	svc, err := c.Typed.CoreV1().Services(ns).Get(ctx, "gateway", metav1.GetOptions{})
	if err == nil {
		switch svc.Spec.Type {
		case "NodePort":
			cfg.DashboardAccess = "NodePort"
		case "LoadBalancer":
			cfg.DashboardAccess = "LoadBalancer"
		default:
			cfg.DashboardAccess = "ClusterIP"
		}
	}
	ingresses, err := c.Typed.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err == nil && len(ingresses.Items) > 0 {
		cfg.DashboardAccess = "Ingress"
		ing := ingresses.Items[0]
		if len(ing.Spec.Rules) > 0 {
			cfg.DashboardAccess = fmt.Sprintf("Ingress (%s)", ing.Spec.Rules[0].Host)
		}
		cfg.IngressTLS = len(ing.Spec.TLS) > 0
	}

	// Helm release name from secrets
	secrets, err := c.Typed.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, s := range secrets.Items {
			if strings.HasPrefix(s.Name, "sh.helm.release") {
				if rel, ok := s.Labels["name"]; ok {
					cfg.ReleaseName = rel
				}
			}
			if v, ok := s.Labels["helm.sh/chart"]; ok && strings.Contains(v, "k10") {
				cfg.ChartVersion = v
			}
		}
	}

	return cfg, nil
}

func collectLicense(c *cluster.Client, ns string) (License, error) {
	ctx := context.Background()
	lic := License{}

	for _, name := range []string{"k10-license", "kasten-license", "license"} {
		secret, err := c.Typed.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		for _, field := range []string{"license", "license.txt", "key"} {
			if v, ok := secret.Data[field]; ok {
				parseLicenseData(string(v), &lic)
				return lic, nil
			}
		}
	}

	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "k10-license", metav1.GetOptions{})
	if err == nil {
		lic.Company = cm.Data["company"]
		lic.LicenseType = cm.Data["type"]
		lic.ExpiresAt = cm.Data["expiresAt"]
		return lic, nil
	}

	// License CRD (Kasten 6+)
	licList, err := c.Dynamic.Resource(gvrLicense).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err == nil && len(licList.Items) > 0 {
		item := licList.Items[0].Object
		spec := GetMap(item, "spec")
		status := GetMap(item, "status")

		// K10 6.x field names
		lic.Company = GetString(spec, "company")
		lic.LicenseType = GetString(spec, "licenseType")
		lic.ExpiresAt = GetString(spec, "expiryDate")
		lic.NodeLimit = GetInt(spec, "nodeLimit")

		// K10 8.x: licenseState is in status.conditions or status.licenseState
		state := GetString(status, "licenseState")
		if state == "" {
			// try status.conditions[0].reason or type
			for _, cond := range GetSlice(status, "conditions") {
				cm2, ok := cond.(map[string]interface{})
				if !ok {
					continue
				}
				if GetString(cm2, "type") == "LicenseValid" || GetString(cm2, "type") == "Valid" {
					state = GetString(cm2, "status") // "True" / "False"
					if strings.EqualFold(state, "true") {
						state = "Valid"
					}
					break
				}
			}
		}
		lic.Valid = strings.EqualFold(state, "Valid")

		// K10 8.x: license info is nested under status.licenseInfo
		licInfo := GetMap(status, "licenseInfo")
		if lic.Company == "" {
			lic.Company = GetString(licInfo, "company")
			if lic.Company == "" {
				lic.Company = GetString(status, "company")
			}
		}
		if lic.LicenseType == "" {
			lic.LicenseType = GetString(licInfo, "licenseType")
			if lic.LicenseType == "" {
				lic.LicenseType = GetString(licInfo, "type")
			}
			if lic.LicenseType == "" {
				lic.LicenseType = GetString(status, "licenseType")
			}
			if lic.LicenseType == "" {
				lic.LicenseType = GetString(status, "type")
			}
		}
		if lic.ProductName == "" {
			lic.ProductName = GetString(licInfo, "product")
			if lic.ProductName == "" {
				lic.ProductName = GetString(licInfo, "productName")
			}
		}
		if lic.ExpiresAt == "" {
			for _, key := range []string{"expirationTimestamp", "expiryDate", "expiry", "expiresAt"} {
				if v := GetString(licInfo, key); v != "" {
					lic.ExpiresAt = v
					break
				}
				if v := GetString(status, key); v != "" {
					lic.ExpiresAt = v
					break
				}
			}
		}
		if lic.NodeLimit == 0 {
			lic.NodeLimit = GetInt(licInfo, "nodeCount")
			if lic.NodeLimit == 0 {
				lic.NodeLimit = GetInt(licInfo, "nodeLimit")
			}
			if lic.NodeLimit == 0 {
				lic.NodeLimit = GetInt(status, "nodeLimit")
			}
		}

		// Only return if we got something useful; otherwise fall through to report enrichment
		if lic.Company != "" || lic.LicenseType != "" || lic.ExpiresAt != "" || lic.NodeLimit > 0 || lic.Valid {
			return lic, nil
		}
	}

	return lic, nil
}

func parseLicenseData(data string, lic *License) {
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch strings.ToLower(k) {
		case "company", "organization":
			lic.Company = v
		case "product", "productname":
			lic.ProductName = v
		case "expirydate", "expires", "expiry":
			lic.ExpiresAt = v
		case "type", "licensetype":
			lic.LicenseType = v
		case "valid":
			lic.Valid = strings.ToLower(v) == "true"
		}
	}
}
