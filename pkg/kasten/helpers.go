package kasten

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ── Kasten GVRs ───────────────────────────────────────────────────────────────

var (
	GVRPolicy           = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "policies"}
	GVRPolicyPreset     = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "policypresets"}
	GVRProfile          = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "profiles"}
	GVRTransformSet     = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "transformsets"}
	GVRRunAction        = schema.GroupVersionResource{Group: "actions.kio.kasten.io", Version: "v1alpha1", Resource: "runactions"}
	GVRBackupAction     = schema.GroupVersionResource{Group: "actions.kio.kasten.io", Version: "v1alpha1", Resource: "backupactions"}
	GVRRestoreAction    = schema.GroupVersionResource{Group: "actions.kio.kasten.io", Version: "v1alpha1", Resource: "restoreactions"}
	GVRExportAction     = schema.GroupVersionResource{Group: "actions.kio.kasten.io", Version: "v1alpha1", Resource: "exportactions"}
	GVRRestorePoint     = schema.GroupVersionResource{Group: "apps.kio.kasten.io", Version: "v1alpha1", Resource: "restorepoints"}
	GVRApplication      = schema.GroupVersionResource{Group: "apps.kio.kasten.io", Version: "v1alpha1", Resource: "applications"}
	GVRBlueprint        = schema.GroupVersionResource{Group: "cr.kanister.io", Version: "v1alpha1", Resource: "blueprints"}
	GVRBlueprintBinding = schema.GroupVersionResource{Group: "config.kio.kasten.io", Version: "v1alpha1", Resource: "blueprintbindings"}
	GVRVMI              = schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
)

// ── JSON path helpers ─────────────────────────────────────────────────────────

// GetMap descends a nested map following the given keys.
func GetMap(obj map[string]interface{}, keys ...string) map[string]interface{} {
	cur := obj
	for _, k := range keys {
		v, ok := cur[k]
		if !ok {
			return map[string]interface{}{}
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return map[string]interface{}{}
		}
		cur = m
	}
	return cur
}

// GetString descends and returns the final value as string.
func GetString(obj map[string]interface{}, keys ...string) string {
	cur := obj
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			switch vv := v.(type) {
			case string:
				return vv
			case json.Number:
				return vv.String()
			case bool:
				if vv {
					return "true"
				}
				return "false"
			default:
				return fmt.Sprintf("%v", v)
			}
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return ""
		}
		cur = m
	}
	return ""
}

// GetBool descends and returns a bool value.
func GetBool(obj map[string]interface{}, keys ...string) bool {
	cur := obj
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return false
		}
		if i == len(keys)-1 {
			b, ok := v.(bool)
			return ok && b
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return false
		}
		cur = m
	}
	return false
}

// GetInt descends and returns an int value.
func GetInt(obj map[string]interface{}, keys ...string) int {
	cur := obj
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return 0
		}
		if i == len(keys)-1 {
			switch vv := v.(type) {
			case int:
				return vv
			case int64:
				return int(vv)
			case float64:
				return int(vv)
			case json.Number:
				n, _ := vv.Int64()
				return int(n)
			}
			return 0
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return 0
		}
		cur = m
	}
	return 0
}

// GetSlice descends and returns a slice.
func GetSlice(obj map[string]interface{}, keys ...string) []interface{} {
	cur := obj
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil
		}
		if i == len(keys)-1 {
			s, ok := v.([]interface{})
			if ok {
				return s
			}
			return nil
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil
		}
		cur = m
	}
	return nil
}

// GetLabels returns the metadata.labels map as map[string]interface{}.
func GetLabels(obj map[string]interface{}) map[string]interface{} {
	meta := GetMap(obj, "metadata")
	labels, _ := meta["labels"].(map[string]interface{})
	if labels == nil {
		return map[string]interface{}{}
	}
	return labels
}

// GetLabel returns a single label value.
func GetLabel(obj map[string]interface{}, key string) string {
	l := GetLabels(obj)
	v, _ := l[key].(string)
	return v
}

// MetaName returns metadata.name.
func MetaName(obj map[string]interface{}) string {
	return GetString(obj, "metadata", "name")
}

// MetaNamespace returns metadata.namespace.
func MetaNamespace(obj map[string]interface{}) string {
	return GetString(obj, "metadata", "namespace")
}

// MetaTimestamp returns metadata.creationTimestamp.
func MetaTimestamp(obj map[string]interface{}) string {
	return GetString(obj, "metadata", "creationTimestamp")
}

// HumanBytes converts bytes to a human-readable string.
func HumanBytes(b int64) string {
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

// GetAnnotations returns the metadata.annotations map.
func GetAnnotations(obj map[string]interface{}) map[string]string {
	meta := GetMap(obj, "metadata")
	raw, _ := meta["annotations"].(map[string]interface{})
	out := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// _jsonUnmarshal is a package-level alias used by collector_diagnostics.go
// to avoid an import cycle while keeping json in one place.
func _jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// IsSystemNamespace returns true for well-known system/kasten namespaces.
func IsSystemNamespace(ns string) bool {
	// OpenShift and Kubernetes system prefixes
	for _, prefix := range []string{
		"openshift-", "kube-", "openshift-", "cattle-", "rancher-",
		"flux-", "argo", "crossplane-",
	} {
		if strings.HasPrefix(ns, prefix) {
			return true
		}
	}
	systemNS := map[string]bool{
		// Kubernetes
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
		"default": false, // default is a user namespace — do NOT exclude
		// Kasten
		"kasten-io": true, "kasten-io-mc": true,
		// OpenShift
		"openshift": true, "openshift-operators": true, "openshift-monitoring": true,
		// Infra
		"cert-manager": true, "ingress-nginx": true, "longhorn-system": true,
		"monitoring": true, "logging": true, "istio-system": true,
		"knative-serving": true, "knative-eventing": true,
	}
	return systemNS[ns]
}

// IsTerminatingNamespace returns true if the namespace is in Terminating state.
// These should be excluded from application counts.
func IsActiveNamespace(phase string) bool {
	return phase == "" || phase == "Active"
}
