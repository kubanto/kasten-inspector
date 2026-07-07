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

## What's New in v1.3

### Health Check tab (new)
All cluster and K10 installation health information is now consolidated in a single dedicated tab: Cluster info, License, Security (auth + encryption), K10 Resource Limits, Disaster Recovery status, StorageClass & VSC inventory.

### Best Practices (25 total, up from 17)
| ID | Check | Severity |
|----|-------|----------|
| BP-01 | Application protection coverage | critical |
| BP-02 | Backup encryption at rest | warning |
| BP-03 | Immutable backup storage | info |
| BP-04 | Multiple location profiles (3-2-1 rule) | warning |
| BP-05 | Policies have export (offsite copy) | warning |
| BP-06 | Authentication method configured | critical |
| BP-07 | Disaster Recovery (KDR) configured | warning |
| BP-08 | Prometheus monitoring enabled | info |
| BP-09 | No orphaned restore points | info |
| BP-10 | All non-system namespaces protected | warning |
| BP-11 | K10 pods have resource limits | warning |
| BP-12 | Snapshot retention within safe bounds | warning |
| BP-13 | At least one snapshot retained per policy | warning |
| BP-14 | Export retention explicitly configured | warning |
| BP-15 | CSI provisioners have snapshot capability | warning |
| BP-16 | Protected namespaces: backup recency within 7 days | warning |
| BP-17 | Restore test performed at least once | critical |
| BP-18 | Dashboard exposed via Ingress with HTTPS | warning |
| BP-19 | VolumeSnapshotClass has Kasten annotation (`k10.kasten.io/is-snapshot-class=true`) | **critical** |
| BP-20 | No policies with wildcard namespace selector | warning |
| BP-21 | Dedicated policy for cluster-scoped resources | warning |
| BP-22 | Location profiles use object storage (not NFS/SMB only) | warning |
| BP-23 | PolicyPresets defined for retention standardization | info |
| BP-24 | Catalog storage ≥50% free (upgrade prerequisite) | warning |
| BP-25 | Prometheus alert rules (PrometheusRule CRs) configured | info |

### Report tab structure (v1.3)
| Tab | Content |
|-----|---------|
| **Overview** | Executive Summary, Best Practices alerts, Compliance & SLA |
| **Health Check** ← new | Cluster, License, Security, Resource Limits, DR status, CSI inventory |
| **Protection** | Policies, Applications, Profiles, KubeVirt, Coverage matrix |
| **Operations** | Jobs (Today/date-range filters), Restore Points, K10 Reports, Actions summary |
| **Storage** | Storage Overview, Breakdown, PVC status |
| **Configuration** | Kanister Blueprints & TransformSets |
| **Statistics & QBR** | KPI grid, charts, Recovery Readiness Score, App Risk Matrix |
| **Diagnostics** | Recent Failures, Long-running Actions, Backup Recency |

### Fixes
- Storage report date now always picks the most recent K10 Report CR (was using arbitrary ordering)
- K10 version detection: SHA256 digest image tags no longer shown as version (falls back to K10 Report CRD)
- License section populated from K10 8.x `status.licenseInfo` nested fields
- Infrastructure profiles (vSphere) correctly classified
- Restore points by namespace: bar chart added to left panel

---

## What's New in v1.2

### QBR & Reporting
- **PowerPoint QBR deck** (`--pptx`) — 11-slide deck ready to present
- **Recovery Readiness Score** — composite score (0–100, grade A–F)
- **Application Risk Matrix** — per-namespace RPO/RTO estimate with risk level
- **Statistics & QBR dashboard** — interactive HTML tab with charts and trends

### Diagnostics
- Recent Failures, Long-running Actions, Backup Recency per Namespace, StorageClass & VSC Inventory

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
| **Protection** | KPI banner, Security config, Policies, Applications, Location Profiles, KubeVirt VMs, Policy frequencies, Coverage matrix, PVC status, Restore points |
| **Operations** | Kasten Disaster Recovery, Recent Jobs (filterable), Restore Points detail, K10 Generated Reports, Actions summary |
| **Storage** | Storage Overview (with report age banner), Storage breakdown |
| **Configuration** | Kanister Blueprints & TransformSets, K10 Resource Limits, License |
| **Statistics & QBR** | KPI grid, NS Protection chart, Job Outcome chart, BP Score, Monthly trend, Security posture, Concurrency Limiters, Recovery Readiness Score, App Risk Matrix, Gaps, Weekly SLA trend |
| **Diagnostics** | Recent Failures, Long-running Actions, Backup Recency, StorageClass & VSC Inventory |

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
