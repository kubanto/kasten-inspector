package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"encoding/json"
	"github.com/veeam/kasten-inspector/pkg/cluster"
	"github.com/veeam/kasten-inspector/pkg/kasten"
	"github.com/veeam/kasten-inspector/pkg/report"
)

func runInspect(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	var (
		kubeconfig string
		namespace  string
		outputDir  string
		formats    string
		showVer    bool
		verbose    bool
		jobLimit      int
		pptxMode      bool
		pptxCustomer  string
		pptxMeeting   string
		pptxLogo      string
		pptxFromJSON  string
		pptxTAM       string
	)

	fs.StringVar(&kubeconfig, "kubeconfig", defaultKubeconfig(), "Path to kubeconfig (auto-detected if empty)")
	fs.StringVar(&namespace, "namespace", "kasten-io", "Namespace where Kasten K10 is installed")
	fs.StringVar(&outputDir, "output-dir", ".", "Output directory (default: auto-creates reports/report-YYYY-MM-DD-HH-MM/)")
	fs.StringVar(&formats, "format", "html,json,markdown", "Comma-separated formats: html,json,markdown (or 'all')")
	fs.IntVar(&jobLimit, "job-limit", 200, "Max number of recent jobs to collect")
	fs.BoolVar(&showVer, "version", false, "Print version and exit")
	fs.BoolVar(&verbose, "verbose", false, "Verbose logging")
	fs.BoolVar(&pptxMode, "pptx", false, "Generate a QBR PowerPoint presentation")
	fs.StringVar(&pptxCustomer, "customer", "", "Customer name for the PowerPoint cover slide")
	fs.StringVar(&pptxMeeting, "meeting-date", "", "Meeting date for the PowerPoint (e.g. \"June 2026\")")
	fs.StringVar(&pptxLogo, "logo", "", "Path to customer logo image (optional)")
	fs.StringVar(&pptxFromJSON, "json", "", "Generate PowerPoint from existing JSON report (skips cluster connection)")
	fs.StringVar(&pptxTAM, "tam", "<TAM name>", "TAM name shown on cover and closing slide")
	var clusterNameOverride string
	fs.StringVar(&clusterNameOverride, "cluster-name", "", "Override the cluster name shown in the report (default: kubeconfig current-context)")
	fs.Parse(args)

	if showVer {
		fmt.Printf("kasten-inspector v%s\n", version)
		os.Exit(0)
	}

	// JSON-only mode: generate PPTX from existing report without connecting to cluster
	if pptxFromJSON != "" {
		printBanner(version)
		fmt.Printf("→ Loading report from %s...\n", pptxFromJSON)
		rpt, err := loadReportJSON(pptxFromJSON, version)
		if err != nil {
			fatalf("Cannot load JSON: %v", err)
		}
		ts := time.Now().Format("2006-01-02-15-04-05")
		if outputDir == "." {
			outputDir = filepath.Join("reports", "report-"+ts)
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			fatalf("Cannot create output dir: %v", err)
		}
		pptxPath := filepath.Join(outputDir, fmt.Sprintf("kasten-qbr-%s.pptx", ts))
		fmt.Printf("→ Generating QBR PowerPoint...\n")
		pptxOpts := report.PPTXOptions{
			Customer:    pptxCustomer,
			TAM:         pptxTAM,
			MeetingDate: pptxMeeting,
			LogoPath:    pptxLogo,
		}
		if err := report.WritePPTX(pptxPath, rpt, pptxOpts); err != nil {
			fatalf("PowerPoint generation failed: %v", err)
		}
		fmt.Printf("  ✓ %s (%s)\n", pptxPath, fileSize(pptxPath))
		fmt.Println("\n✓ Done!")
		return
	}

	wantFormats := parseFormats(formats)
	printBanner(version)

	fmt.Println("→ Connecting to Kubernetes cluster...")
	c, err := cluster.NewClient(kubeconfig, verbose)
	if err != nil {
		fatalf("Cannot connect to cluster: %v", err)
	}
	fmt.Printf("  ✓ Connected (%s)\n", c.Mode)

	fmt.Println("→ Collecting cluster information...")
	clusterInfo, err := cluster.CollectInfo(c)
	if err != nil {
		fatalf("Cluster info: %v", err)
	}
	fmt.Printf("  ✓ %s · %s · %d nodes\n",
		clusterInfo.Name, clusterInfo.KubernetesVersion, clusterInfo.NodeCount)

	fmt.Printf("→ Collecting Kasten K10 data (namespace: %s)...\n", namespace)
	opts := kasten.CollectOptions{
		Namespace: namespace,
		JobLimit:  jobLimit,
		Verbose:   verbose,
	}
	kastenData, err := kasten.CollectAll(c, opts)
	if err != nil {
		fatalf("Kasten data collection failed: %v", err)
	}
	printKastenSummary(kastenData)

	// Apply cluster name override if specified
	if clusterNameOverride != "" {
		clusterInfo.Name = clusterNameOverride
	}

	rpt := &report.Data{
		GeneratedAt: time.Now().UTC(),
		ToolVersion: version,
		Author:      pptxTAM,
		Cluster:     clusterInfo,
		Kasten:      kastenData,
	}

	ts := time.Now().Format("2006-01-02-15-04-05")
	if outputDir == "." {
		outputDir = filepath.Join("reports", "report-"+ts)
		fmt.Printf("→ Output directory: %s\n", outputDir)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatalf("Cannot create output directory: %v", err)
	}
	fmt.Println()

	for _, f := range wantFormats {
		var outPath string
		var writeErr error
		switch f {
		case "html":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-report-%s.html", ts))
			fmt.Printf("→ Generating HTML report...\n")
			writeErr = report.WriteHTML(outPath, rpt)
		case "json":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-report-%s.json", ts))
			fmt.Printf("→ Generating JSON report...\n")
			writeErr = report.WriteJSON(outPath, rpt)
		case "markdown":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-report-%s.md", ts))
			fmt.Printf("→ Generating Markdown report...\n")
			writeErr = report.WriteMarkdown(outPath, rpt)
		}
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", f, writeErr)
		} else if outPath != "" {
			fmt.Printf("  ✓ %s (%s)\n", outPath, fileSize(outPath))
		}
	}

	// ── PowerPoint generation ─────────────────────────────────────────────────
	if pptxMode {
		pptxPath := filepath.Join(outputDir, fmt.Sprintf("kasten-qbr-%s.pptx", ts))
		fmt.Printf("→ Generating QBR PowerPoint...\n")
		pptxOpts := report.PPTXOptions{
			Customer:    pptxCustomer,
			TAM:         pptxTAM,
			MeetingDate: pptxMeeting,
			LogoPath:    pptxLogo,
		}
		if err := report.WritePPTX(pptxPath, rpt, pptxOpts); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ pptx: %v\n", err)
		} else {
			fmt.Printf("  ✓ %s (%s)\n", pptxPath, fileSize(pptxPath))
		}
	}

	fmt.Println("\n✓ Inspection complete!")
}

// ── shared helpers ────────────────────────────────────────────────────────────

func parseFormats(s string) []string {
	if s == "all" {
		return []string{"html", "json", "markdown"}
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(strings.ToLower(f))
		if f == "md" {
			f = "markdown"
		}
		if (f == "html" || f == "json" || f == "markdown") && !seen[f] {
			out = append(out, f)
			seen[f] = true
		}
	}
	if len(out) == 0 {
		return []string{"html", "json"}
	}
	return out
}

func defaultKubeconfig() string {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func fileSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "?"
	}
	s := info.Size()
	switch {
	case s < 1024:
		return fmt.Sprintf("%d B", s)
	case s < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(s)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(s)/1024/1024)
	}
}

func printBanner(v string) {
	fmt.Println("┌─────────────────────────────────────────────┐")
	fmt.Printf( "│  Kasten K10 Inspector  %-21s│\n", "v"+v)
	fmt.Println("│  kubanto · cluster health reporter          │")
	fmt.Println("└─────────────────────────────────────────────┘")
	fmt.Println()
}

func printKastenSummary(d *kasten.Data) {
	fmt.Printf("  ✓ Kasten version  : %s\n", d.Version)
	fmt.Printf("  ✓ Mode            : %s\n", d.MultiCluster.Mode)
	fmt.Printf("  ✓ Applications    : %d total / %d protected / %d unprotected\n",
		d.Applications.Total, d.Applications.Protected, d.Applications.Unprotected)
	fmt.Printf("  ✓ Policies        : %d\n", len(d.Policies))
	fmt.Printf("  ✓ Profiles        : %d\n", len(d.Profiles))
	fmt.Printf("  ✓ Jobs collected  : %d\n", len(d.Jobs))
	fmt.Printf("  ✓ Restore points  : %d\n", d.RestorePoints.Total)
	if d.KubeVirt.Enabled {
		fmt.Printf("  ✓ KubeVirt VMs    : %d total / %d protected\n",
			d.KubeVirt.TotalVMs, d.KubeVirt.ProtectedVMs)
	}
	fmt.Printf("  ✓ Best practices  : %d checks · %d warnings · %d critical\n",
		d.BestPractices.TotalChecks, d.BestPractices.Warnings, d.BestPractices.Critical)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "\nERROR: "+format+"\n", args...)
	os.Exit(1)
}

// loadReportJSON loads an existing report JSON file.
func loadReportJSON(path, toolVersion string) (*report.Data, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var d report.Data
	if err := json.NewDecoder(f).Decode(&d); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if d.ToolVersion == "" {
		d.ToolVersion = toolVersion
	}
	return &d, nil
}
