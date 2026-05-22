package kasten

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/veeam/kasten-inspector/pkg/cluster"
)

func collectBlueprints(c *cluster.Client) ([]Blueprint, error) {
	ctx := context.Background()
	list, err := c.Dynamic.Resource(GVRBlueprint).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var bps []Blueprint
	for _, item := range list.Items {
		obj := item.Object
		var actions []string
		if am, ok := obj["actions"].(map[string]interface{}); ok {
			for k := range am {
				actions = append(actions, k)
			}
		}
		bps = append(bps, Blueprint{
			Name:      MetaName(obj),
			Namespace: MetaNamespace(obj),
			Actions:   actions,
			CreatedAt: MetaTimestamp(obj),
		})
	}
	return bps, nil
}

func collectBlueprintBindings(c *cluster.Client, ns string) ([]BlueprintBinding, error) {
	ctx := context.Background()
	list, err := c.Dynamic.Resource(GVRBlueprintBinding).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var bindings []BlueprintBinding
	for _, item := range list.Items {
		obj := item.Object
		spec := GetMap(obj, "spec")
		bindings = append(bindings, BlueprintBinding{
			Name:      MetaName(obj),
			Namespace: MetaNamespace(obj),
			Blueprint: GetString(spec, "blueprintRef", "name"),
			Subject:   GetString(spec, "subject", "resource") + "/" + GetString(spec, "subject", "name"),
			CreatedAt: MetaTimestamp(obj),
		})
	}
	return bindings, nil
}

func collectTransformSets(c *cluster.Client, ns string) ([]TransformSet, error) {
	ctx := context.Background()
	list, err := c.Dynamic.Resource(GVRTransformSet).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var sets []TransformSet
	for _, item := range list.Items {
		obj := item.Object
		spec := GetMap(obj, "spec")
		count := 0
		if transforms, ok := spec["transforms"].([]interface{}); ok {
			count = len(transforms)
		}
		sets = append(sets, TransformSet{
			Name:       MetaName(obj),
			Namespace:  MetaNamespace(obj),
			Transforms: count,
			CreatedAt:  MetaTimestamp(obj),
		})
	}
	return sets, nil
}

func collectK10Resources(c *cluster.Client, ns string) (K10Resources, error) {
	ctx := context.Background()
	res := K10Resources{}

	deps, err := c.Typed.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return res, err
	}

	for _, dep := range deps.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		di := DeploymentInfo{
			Name:     dep.Name,
			Replicas: replicas,
			Ready:    dep.Status.ReadyReplicas,
		}
		for _, cont := range dep.Spec.Template.Spec.Containers {
			cr := ContainerRes{Name: cont.Name}
			req := cont.Resources.Requests
			lim := cont.Resources.Limits
			if req != nil {
				if cpu, ok := req[corev1.ResourceCPU]; ok {
					cr.CPURequest = cpu.String()
				}
				if mem, ok := req[corev1.ResourceMemory]; ok {
					cr.MemRequest = mem.String()
				}
			}
			if lim != nil {
				if cpu, ok := lim[corev1.ResourceCPU]; ok {
					cr.CPULimit = cpu.String()
				}
				if mem, ok := lim[corev1.ResourceMemory]; ok {
					cr.MemLimit = mem.String()
				}
			}
			di.Containers = append(di.Containers, cr)
		}
		res.Deployments = append(res.Deployments, di)
	}
	return res, nil
}

func collectCatalog(c *cluster.Client, ns string) (CatalogInfo, error) {
	ctx := context.Background()
	info := CatalogInfo{}

	pvcs, err := c.Typed.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return info, err
	}
	for _, pvc := range pvcs.Items {
		if pvc.Name == "catalog-pv-claim" || pvc.Name == "catalog" ||
			containsAny(pvc.Name, "catalog") {
			if cap := pvc.Status.Capacity.Storage(); cap != nil {
				info.SizeBytes = cap.Value()
				info.SizeHuman = HumanBytes(info.SizeBytes)
			}
			if pvc.Spec.StorageClassName != nil {
				info.StorageClass = *pvc.Spec.StorageClassName
			}
			break
		}
	}

	cm, err := c.Typed.CoreV1().ConfigMaps(ns).Get(ctx, "catalog-info", metav1.GetOptions{})
	if err == nil {
		var used int64
		fmt.Sscanf(cm.Data["usedBytes"], "%d", &used)
		if used > 0 && info.SizeBytes > 0 {
			info.FreeBytes = info.SizeBytes - used
			info.FreeHuman = HumanBytes(info.FreeBytes)
			info.FreePercent = float64(info.FreeBytes) / float64(info.SizeBytes) * 100
			info.LowSpaceAlert = info.FreePercent < 20
		}
	}
	return info, nil
}

func collectPrometheus(c *cluster.Client, ns string) (PrometheusInfo, error) {
	ctx := context.Background()
	info := PrometheusInfo{}

	smGVR := schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"}
	smList, err := c.Dynamic.Resource(smGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err == nil && len(smList.Items) > 0 {
		info.Enabled = true
		info.ServiceMonitor = true
	}

	gdGVR := schema.GroupVersionResource{Group: "integreatly.org", Version: "v1alpha1", Resource: "grafanadashboards"}
	gdList, err := c.Dynamic.Resource(gdGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err == nil && len(gdList.Items) > 0 {
		info.GrafanaDashboard = true
	}

	svc, err := c.Typed.CoreV1().Services(ns).Get(ctx, "prometheus-server", metav1.GetOptions{})
	if err == nil {
		info.Enabled = true
		for _, port := range svc.Spec.Ports {
			if port.Port == 9090 {
				info.Endpoint = fmt.Sprintf("http://prometheus-server.%s.svc:9090", ns)
				break
			}
		}
	}

	_, err = c.Typed.CoreV1().Services(ns).Get(ctx, "prometheus-k10-metrics", metav1.GetOptions{})
	if err == nil {
		info.Enabled = true
	}

	return info, nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
