package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps both typed and dynamic K8s clients.
type Client struct {
	Typed   *kubernetes.Clientset
	Dynamic dynamic.Interface
	Config  *rest.Config
	Mode    string // "in-cluster" or "kubeconfig"
	Verbose bool
}

// Info holds cluster-level metadata.
type Info struct {
	Name              string         `json:"name"`
	KubernetesVersion string         `json:"kubernetesVersion"`
	Platform          string         `json:"platform"`
	PlatformVersion   string         `json:"platformVersion,omitempty"`
	NodeCount         int            `json:"nodeCount"`
	ControlPlaneNodes int            `json:"controlPlaneNodes"`
	WorkerNodes       int            `json:"workerNodes"`
	Nodes             []NodeInfo     `json:"nodes"`
	NamespaceCount    int            `json:"namespaceCount"`
	StorageClasses    []StorageClass `json:"storageClasses"`
}

// NodeInfo holds per-node metadata.
type NodeInfo struct {
	Name             string `json:"name"`
	Role             string `json:"role"`
	KubeletVersion   string `json:"kubeletVersion"`
	OSImage          string `json:"osImage"`
	ContainerRuntime string `json:"containerRuntime"`
	Architecture     string `json:"architecture"`
	Ready            bool   `json:"ready"`
}

// StorageClass represents a K8s storage class.
type StorageClass struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`
	Default     bool   `json:"isDefault"`
}

// NewClient auto-detects in-cluster config first, then falls back to kubeconfig.
func NewClient(kubeconfigPath string, verbose bool) (*Client, error) {
	c := &Client{Verbose: verbose}

	if cfg, err := rest.InClusterConfig(); err == nil {
		c.logf("Using in-cluster config")
		c.Mode = "in-cluster"
		c.Config = cfg
		return c.buildClients()
	}

	if kubeconfigPath == "" {
		return nil, fmt.Errorf("not running in-cluster and no kubeconfig found; use --kubeconfig")
	}
	c.logf("Using kubeconfig: %s", kubeconfigPath)
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("building config from %s: %w", kubeconfigPath, err)
	}
	c.Mode = "kubeconfig"
	c.Config = cfg
	return c.buildClients()
}

func (c *Client) buildClients() (*Client, error) {
	typed, err := kubernetes.NewForConfig(c.Config)
	if err != nil {
		return nil, fmt.Errorf("typed client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	c.Typed = typed
	c.Dynamic = dyn
	return c, nil
}

// CollectInfo collects cluster-level information.
func CollectInfo(c *Client) (*Info, error) {
	ctx := context.Background()
	info := &Info{}

	ver, err := c.Typed.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("server version: %w", err)
	}
	info.KubernetesVersion = ver.GitVersion
	info.Platform, info.PlatformVersion = detectPlatform(c, ver.GitVersion)
	info.Name = clusterName()

	nodes, err := c.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	info.NodeCount = len(nodes.Items)
	for _, n := range nodes.Items {
		ni := parseNode(n)
		info.Nodes = append(info.Nodes, ni)
		if ni.Role == "control-plane" {
			info.ControlPlaneNodes++
		} else {
			info.WorkerNodes++
		}
	}

	nsList, err := c.Typed.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		info.NamespaceCount = len(nsList.Items)
	}

	scs, err := c.Typed.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, sc := range scs.Items {
			info.StorageClasses = append(info.StorageClasses, StorageClass{
				Name:        sc.Name,
				Provisioner: sc.Provisioner,
				Default:     sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true",
			})
		}
	}
	return info, nil
}

func detectPlatform(c *Client, k8sVer string) (platform, version string) {
	ctx := context.Background()
	raw, err := c.Typed.RESTClient().Get().
		AbsPath("/apis/config.openshift.io/v1/clusterversions").DoRaw(ctx)
	if err == nil && len(raw) > 0 {
		body := string(raw)
		ver := extractBetween(body, `"version":"`, `"`)
		if ver != "" {
			return "OpenShift", ver
		}
		return "OpenShift", "unknown"
	}
	switch {
	case strings.Contains(k8sVer, "-gke"):
		return "GKE", ""
	case strings.Contains(k8sVer, "-eks"):
		return "EKS", ""
	case strings.Contains(k8sVer, "-azure"), os.Getenv("AKS_CLUSTER_NAME") != "":
		return "AKS", ""
	case strings.Contains(k8sVer, "-rke"):
		return "RKE (Rancher)", ""
	default:
		return "Kubernetes", ""
	}
}

func parseNode(n corev1.Node) NodeInfo {
	role := "worker"
	for l := range n.Labels {
		if l == "node-role.kubernetes.io/master" || l == "node-role.kubernetes.io/control-plane" {
			role = "control-plane"
			break
		}
	}
	ready := false
	for _, cond := range n.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			ready = true
		}
	}
	return NodeInfo{
		Name:             n.Name,
		Role:             role,
		KubeletVersion:   n.Status.NodeInfo.KubeletVersion,
		OSImage:          n.Status.NodeInfo.OSImage,
		ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
		Architecture:     n.Status.NodeInfo.Architecture,
		Ready:            ready,
	}
}

func clusterName() string {
	// 1. Environment variable override
	for _, env := range []string{"CLUSTER_NAME", "KUBE_CLUSTER_NAME"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	// 2. Current context name from kubeconfig (e.g. "kubanto", "my-ocp-cluster")
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{})
	rawConfig, err := clientConfig.RawConfig()
	if err == nil && rawConfig.CurrentContext != "" {
		ctx := rawConfig.CurrentContext
		// OCP contexts are formatted as "namespace/api-server/username"
		// Extract just the meaningful part (api server hostname)
		if parts := strings.Split(ctx, "/"); len(parts) >= 2 {
			// Try to get the API server hostname as the cluster identifier
			for _, part := range parts {
				if strings.Contains(part, ":") || strings.HasPrefix(part, "api-") {
					// Clean up: remove port, replace dashes with readable form
					host := strings.Split(part, ":")[0]
					if host != "" && host != "api" {
						return host
					}
				}
			}
		}
		return ctx
	}
	return "k8s-cluster"
}

// extractBetween returns the string between prefix and suffix in body.
func extractBetween(body, prefix, suffix string) string {
	idx := strings.Index(body, prefix)
	if idx < 0 {
		return ""
	}
	s := body[idx+len(prefix):]
	end := strings.Index(s, suffix)
	if end < 0 {
		return ""
	}
	return s[:end]
}

func (c *Client) logf(format string, args ...interface{}) {
	if c.Verbose {
		fmt.Printf("[cluster] "+format+"\n", args...)
	}
}
