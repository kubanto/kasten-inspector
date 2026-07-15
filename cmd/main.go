package main

import (
	"fmt"
	"os"
)

var version = "1.5.1"

const usage = `Kasten K10 Inspector v%s — cluster health & data protection reporter
by kubanto — independent project, not an official Veeam product

Usage:
  kasten-inspector <command> [flags]

Commands:
  inspect     Inspect a cluster and generate reports (default)
  trend       Compare two JSON reports and show what changed
  aggregate   Inspect multiple clusters and generate a unified report

Examples:
  kasten-inspector inspect --format=all --output-dir=./reports
  kasten-inspector inspect --pptx --customer="Acme Corp" --meeting-date="June 2026"
  kasten-inspector inspect --json=report.json --pptx --customer="Acme Corp"
  kasten-inspector trend --before=jan.json --after=feb.json
  kasten-inspector aggregate --kubeconfig=c1.yaml,c2.yaml --output-dir=./reports

Run 'kasten-inspector <command> --help' for command-specific flags.
`

func main() {
	if len(os.Args) < 2 {
		runInspect(os.Args[1:])
		return
	}

	switch os.Args[1] {
	case "inspect":
		runInspect(os.Args[2:])
	case "trend":
		runTrend(os.Args[2:])
	case "aggregate":
		runAggregate(os.Args[2:])
	case "--version", "-version", "version":
		fmt.Printf("kasten-inspector v%s\n", version)
	case "--help", "-help", "help":
		fmt.Printf(usage, version)
	default:
		if len(os.Args[1]) > 0 && os.Args[1][0] == '-' {
			runInspect(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
			fmt.Fprintf(os.Stderr, usage, version)
			os.Exit(1)
		}
	}
}
