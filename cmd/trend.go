package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/veeam/kasten-inspector/pkg/report"
)

func runTrend(args []string) {
	fs := flag.NewFlagSet("trend", flag.ExitOnError)
	var (
		beforeFile string
		afterFile  string
		outputDir  string
		formats    string
	)

	fs.StringVar(&beforeFile, "before", "", "Path to the older JSON report (required)")
	fs.StringVar(&afterFile, "after", "", "Path to the newer JSON report (required)")
	fs.StringVar(&outputDir, "output-dir", ".", "Directory where output files will be saved")
	fs.StringVar(&formats, "format", "html", "Output formats: html,json,markdown (or 'all')")
	fs.Parse(args)

	if beforeFile == "" || afterFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: kasten-inspector trend --before=<old.json> --after=<new.json>\n")
		os.Exit(1)
	}

	printBanner(version)
	fmt.Println("→ Loading reports...")

	before, err := loadReportData(beforeFile)
	if err != nil {
		fatalf("Cannot load --before file: %v", err)
	}
	after, err := loadReportData(afterFile)
	if err != nil {
		fatalf("Cannot load --after file: %v", err)
	}

	fmt.Printf("  ✓ Before : %s (%s)\n", before.Cluster.Name, before.GeneratedAt.Format("02 Jan 2006"))
	fmt.Printf("  ✓ After  : %s (%s)\n", after.Cluster.Name, after.GeneratedAt.Format("02 Jan 2006"))

	fmt.Println("→ Computing diff...")
	trendData := report.ComputeTrend(before, after)

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fatalf("Cannot create output directory: %v", err)
	}
	ts := time.Now().Format("2006-01-02-15-04")
	fmt.Println()

	wantFormats := parseFormats(formats)
	for _, f := range wantFormats {
		var outPath string
		var writeErr error
		switch f {
		case "html":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-trend-%s.html", ts))
			fmt.Printf("→ Generating trend HTML report...\n")
			writeErr = report.WriteTrendHTML(outPath, trendData)
		case "json":
			outPath = filepath.Join(outputDir, fmt.Sprintf("kasten-trend-%s.json", ts))
			fmt.Printf("→ Generating trend JSON...\n")
			writeErr = writeTrendJSON(outPath, trendData)
		}
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", f, writeErr)
		} else if outPath != "" {
			fmt.Printf("  ✓ %s (%s)\n", outPath, fileSize(outPath))
		}
	}

	fmt.Println("\n✓ Trend analysis complete!")
	fmt.Printf("  Coverage: %+.1f%%  |  Jobs success rate: %+.1f%%  |  BP score: %+d checks passed\n",
		trendData.Delta.ProtectionCoverage,
		trendData.Delta.JobSuccessRate,
		trendData.Delta.BPPassed)
}

func loadReportData(path string) (*report.Data, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var d report.Data
	if err := json.NewDecoder(f).Decode(&d); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &d, nil
}

func writeTrendJSON(path string, d *report.TrendData) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}
