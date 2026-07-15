# Kasten Inspector

> ⚠️ **DISCLAIMER — Read before use**
>
> This is an **independent personal project and not a Veeam or Kasten product**.
> It is not affiliated with, endorsed by, or supported by Veeam Software in any way.
> The software is provided **"as is"** with **no warranty** and **no maintenance commitment**.
> Use at your own risk. See [DISCLAIMER.md](DISCLAIMER.md) for full details.

A standalone read-only binary that inspects a [Veeam Kasten K10](https://www.kasten.io)
installation and generates self-contained reports in **HTML**, **JSON**, **Markdown**, and **PowerPoint** —
no runtime dependencies, no changes to the cluster, no `kubectl` required.

---

## What's New in v1.5

### Recovery tab (new)
A dedicated **Recovery** tab groups everything that answers *"can we actually recover?"* — Recovery Readiness Score, Kasten Disaster Recovery (KDR), Restore Points, and the Application Risk Matrix — sitting between Protection and Operations in the nav.

### Data-correctness fixes
- **"Never backed up" false positive** — applications with real restore points (backed up on-demand, outside the collected job window) were wrongly flagged as *never backed up*. Last-backup now falls back to the newest restore point, so they correctly show as recent or stale.
- **Restore-point total** — now includes restore points in system/DR namespaces (KDR catalog in `kasten-io`, etcd backups), reflecting the whole cluster instead of user-app namespaces only.
- **Consistency** — backup-recency (BP-16) and the Diagnostics view now inherit the same corrected last-backup value used by the Recovery Readiness Score and Risk Matrix, so all four outputs (HTML / JSON / Markdown / PPTX) tell the same story.

📖 Full version history: see [CHANGELOG.md](CHANGELOG.md).

---

## Quick Start

Download the binary for your platform from the [Releases](../../releases) page — no installation required.

```bash
# Basic run (HTML + JSON + Markdown)
./kasten-inspector

# With QBR PowerPoint
./kasten-inspector --pptx \
  --tam="Your Name" \
  --customer="Customer Name" \
  --meeting-date="Q2 2026" \
  --cluster-name="prod-cluster"
```

Open the generated `.html` file in any browser — fully self-contained, no external dependencies.

---

## All Flags

```
--kubeconfig      Path to kubeconfig (auto-detected: $KUBECONFIG, ~/.kube/config)
--namespace       Kasten K10 namespace (default: kasten-io)
--output-dir      Output directory (default: ./reports/report-YYYY-MM-DD-HH-MM-SS/)
--job-limit       Max jobs to collect (default: 200)
--verbose         Enable debug logging
--version         Print version and exit

--pptx            Generate QBR PowerPoint deck
--tam             TAM name (shown on PPTX cover and footer)
--customer        Customer name (shown on PPTX cover)
--meeting-date    Meeting date for PPTX cover (e.g. "Q2 2026")
--cluster-name    Override cluster display name in report and PPTX
```

---

## Output Files

Each run creates a timestamped directory with four files:

| File | Description |
|------|-------------|
| `kasten-report-*.html` | Interactive tabbed report — open in any browser |
| `kasten-report-*.json` | Structured data — feed into Grafana, Splunk, Power BI, or scripts |
| `kasten-report-*.md` | Text summary for Confluence, Jira, or email |
| `kasten-qbr-*.pptx` | 11-slide QBR deck (only with `--pptx`) |

The JSON output can be used to regenerate the PPTX later without reconnecting to the cluster:
```bash
./kasten-inspector --pptx --from-json=kasten-report-2026-05-15.json \
  --customer="Customer Name" --meeting-date="Q2 2026"
```

---

## HTML Report — Tab Structure

| Tab | Content |
|-----|---------|
| **Overview** | Cluster info, Executive Summary, Best Practices (summary), Compliance & SLA |
| **Health Check** | Cluster, License, Security (auth + encryption), K10 Resource Limits |
| **Protection** | KPI banner, Policies, Applications, Location Profiles, KubeVirt VMs, Policy frequencies, Coverage matrix |
| **Recovery** | Recovery Readiness Score, Kasten Disaster Recovery (KDR), Restore Points, Application Risk Matrix |
| **Operations** | Recent Jobs (filterable), Job Execution Trend, K10 Generated Reports, Actions summary |
| **Storage** | Storage Overview (with report age banner), Catalog, Storage breakdown, PVC status, StorageClass & VSC inventory |
| **Configuration** | Kanister Blueprints & TransformSets |
| **Statistics & QBR** | KPI grid, NS Protection chart, Job Outcome chart, BP Score, Monthly trend, Security posture, Concurrency Limiters, Recovery Readiness Score, App Risk Matrix, Gaps, Weekly SLA trend |
| **Diagnostics** | Recent Failures, Failures by Policy, Long-running Actions, Backup Recency |

---

## No kubectl Required

The tool communicates directly with the Kubernetes API via `client-go`.
The only requirement is a **kubeconfig file** — the `kubectl` or `oc` binaries are never invoked.

```bash
# Uses ~/.kube/config or $KUBECONFIG by default
./kasten-inspector

# Explicit kubeconfig
./kasten-inspector --kubeconfig=/path/to/kubeconfig

# In-cluster (no kubeconfig needed — uses ServiceAccount automatically)
kubectl apply -f deploy/rbac-and-job.yaml
```

---

## Permissions

The tool is strictly **read-only** — only `get` and `list` RBAC verbs are used.
Review `deploy/rbac-and-job.yaml` before applying it to any cluster.

---

## Build from Source

**Prerequisites:** Go 1.21+  
No CGO, no system libraries — pure Go. Cross-compilation works out of the box from any platform.

```bash
git clone https://github.com/kubanto/kasten-inspector.git
cd kasten-inspector

# Download dependencies
GONOSUMDB=* GOFLAGS=-mod=mod go mod download

# Build for current platform
make build

# Build all platforms at once → dist/
make all
```

Individual platform targets:

| Command | Output |
|---------|--------|
| `make darwin-arm64` | `dist/kasten-inspector-darwin-arm64` (Apple Silicon) |
| `make darwin-amd64` | `dist/kasten-inspector-darwin-amd64` (Intel Mac) |
| `make linux-amd64` | `dist/kasten-inspector-linux-amd64` (Linux x86\_64) |
| `make linux-arm64` | `dist/kasten-inspector-linux-arm64` (Linux ARM64) |
| `make windows-amd64` | `dist/kasten-inspector-windows-amd64.exe` (Windows x64) |

### Cross-compile without Make

```bash
# Linux x86_64 (from Mac or any platform)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o kasten-inspector-linux-amd64 ./cmd/

# Windows x64 (from Mac or Linux)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o kasten-inspector.exe ./cmd/

# Apple Silicon
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o kasten-inspector ./cmd/
```

### Running on Windows

```powershell
# PowerShell — uses %USERPROFILE%\.kube\config automatically
.\kasten-inspector-windows-amd64.exe

# Explicit kubeconfig
.\kasten-inspector-windows-amd64.exe --kubeconfig="C:\Users\you\.kube\config"
```

> **Note for Windows users:** The tool writes reports to `.\reports\report-YYYY-MM-DD-HH-MM-SS\`.
> Open the `.html` file in any browser. No WSL or additional tools required.

### Running on Linux

```bash
chmod +x kasten-inspector-linux-amd64

# Uses ~/.kube/config or $KUBECONFIG
./kasten-inspector-linux-amd64

# Explicit kubeconfig
./kasten-inspector-linux-amd64 --kubeconfig=/path/to/kubeconfig
```

---

## Compatibility

| Platform | Status |
|----------|--------|
| Kasten K10 8.x | ✅ Tested |
| Kasten K10 6.x – 7.x | ⚠️ Partial |
| Kubernetes 1.28+ | ✅ Tested |
| OpenShift 4.x | ✅ Tested |
| k3s | ✅ Tested |
| GKE / EKS / AKS / RKE | ✅ Compatible |

---

## Known Limitations

- **Storage metrics** — snapshot and export size depend on the `k10-system-reports-policy` having run at least once. Without it, only live PVC storage is shown.
- **Historical trending** — weekly SLA trend and monthly job history require the tool to be run periodically. A single run shows only current state.
- **Multi-cluster** — must be run once per cluster; there is no cross-cluster aggregation in a single report.
- **Large clusters** — clusters with 500+ namespaces have not been tested.

---

## Disclaimer

> This tool is an independent personal project and is **not** affiliated with,
> endorsed by, or supported by Veeam Software or any of its subsidiaries.
> It is provided **"as is"** with no warranty of any kind and no commitment to
> maintenance or support. See [DISCLAIMER.md](DISCLAIMER.md) for full details.

---

## License

[MIT](LICENSE) © 2026 kubanto
