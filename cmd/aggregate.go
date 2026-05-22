package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/veeam/kasten-inspector/pkg/cluster"
	"github.com/veeam/kasten-inspector/pkg/kasten"
	"github.com/veeam/kasten-inspector/pkg/report"
)

func runAggregate(args []string) {
	fs := flag.NewFlagSet("aggregate", flag.ExitOnError)
	var (
		kubeconfigs string
		namespace   string
		outputDir   string
		formats     string
		jobLimit    int
		verbose     bool
		parallel    bool
	)

	fs.StringVar(&kubeconfigs, "kubeconfig", "", "Comma-separated list of kubeconfig paths (required)")
	fs.StringVar(&namespace, "namespace", "kasten-io", "Kasten K10 namespace (same for all clusters)")
	fs.StringVar(&outputDir, "output-dir", ".", "Directory where output files will be saved")
	fs.StringVar(&formats, "format", "html", "Output formats: html,json (or 'all')")
	fs.IntVar(&jobLimit, "job-limit", 100, "Max jobs to collect per cluster")
	fs.BoolVar(&verbose, "verbose", false, "Verbose logging")
	fs.BoolVar(&parallel, "parallel", true, "Collect from clusters in parallel")
	fs.Parse(args)

	if kubeconfigs == "" {
		fmt.Fprintf(os.Stderr, "Usage: kasten-inspector aggregate --kubeconfig=c1.yaml,c2.yaml,c3.yaml\n")
		os.Exit(1)
	}

	printBanner(version)
	fmt.Println("→ Multi-cluster aggregation mode")

	paths := splitAndTrim(kubeconfigs)
	fmt.Printf("  Clusters to inspect: %d\n\n", len(paths))

	results := make([]*report.Data, len(paths))
	errors := make([]error, len(paths))

	if parallel {
		var wg sync.WaitGroup
		for i, kc := range paths {
			wg.Add(1)
			go func(idx int, kubeconfigPath string) {
				defer wg.Done()
				results[idx], errors[idx] = inspectCluster(kubeconfigPath, namespace, jobLimit, verbose)
			}(i, kc)
		}
		wg.Wait()
	} else {
		for i, kc := range paths {
			results[i], errors[i] = inspectCluster(kc, namespace, jobLimit, verbose)
		}
	}

	// Report results
	var collected []*report.Data
	for i, res := range results {
		if errors[i] != nil {
			fmt.Printf("  ✗ %s: %v\n", paths[i], errors[i])
			continue
		}
		fmt.Printf("  ✓ %s — K10 %s · %d apps · %d policies\n",
			res.Cluster.Name, res.Kasten.Version,
			res.Kasten.Applications.Total, len(res.Kasten.Policies))
		collected = append(collected, res)
	}

	if len(collected) == 0 {
		fatalf("No clusters collected successfully")
	}

	aggData := report.BuildAggregateReport(collected, version)

	ts := time.Now().Format("2006-01-02-15-04-05")
	if outputDir == "." {
		outputDir = filepath.Join("reports", "report-"+ts)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatalf("Cannot create output directory: %v", err)
	}
	fmt.Println()

	wantFormats := parseFormats(formats)
	for _, f := range wantFormats {
		var outPath string
		var writeErr error
		switch f {
		case "html":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-aggregate-%s.html", ts))
			fmt.Printf("→ Generating aggregate HTML report...\n")
			writeErr = report.WriteAggregateHTML(outPath, aggData)
		case "json":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-aggregate-%s.json", ts))
			fmt.Printf("→ Generating aggregate JSON...\n")
			writeErr = report.WriteAggregateJSON(outPath, aggData)
		}
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", f, writeErr)
		} else if outPath != "" {
			fmt.Printf("  ✓ %s (%s)\n", outPath, fileSize(outPath))
		}
	}

	fmt.Printf("\n✓ Aggregation complete! %d/%d clusters collected.\n",
		len(collected), len(paths))
}

func inspectCluster(kubeconfigPath, namespace string, jobLimit int, verbose bool) (*report.Data, error) {
	c, err := cluster.NewClient(kubeconfigPath, verbose)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	clusterInfo, err := cluster.CollectInfo(c)
	if err != nil {
		return nil, fmt.Errorf("cluster info: %w", err)
	}
	opts := kasten.CollectOptions{
		Namespace: namespace,
		JobLimit:  jobLimit,
		Verbose:   verbose,
	}
	kastenData, err := kasten.CollectAll(c, opts)
	if err != nil {
		return nil, fmt.Errorf("kasten data: %w", err)
	}
	return &report.Data{
		GeneratedAt: time.Now().UTC(),
		ToolVersion: version,
		Cluster:     clusterInfo,
		Kasten:      kastenData,
	}, nil
}

func splitAndTrim(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
