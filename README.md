# Kasten Inspector

> ⚠️ **DISCLAIMER — Read before use**
>
> This is an **independent personal project** by [Antonio Caputo](https://github.com/kubanto).
> It is **not** an official Veeam or Kasten product and is not affiliated with,
> endorsed by, or supported by Veeam Software in any way.
> The software is provided **"as is"** with **no warranty** and **no maintenance commitment**.
> Use at your own risk. See [DISCLAIMER.md](DISCLAIMER.md) for full details.

A standalone read-only binary that inspects a [Veeam Kasten K10](https://www.kasten.io)
installation and generates self-contained reports in **HTML**, **JSON**, **Markdown**, and **PowerPoint** —
no runtime dependencies, no changes to the cluster, no `kubectl` required.

---

## What's New in v1.2

### QBR & Reporting
- **PowerPoint QBR deck** (`--pptx`) — 11-slide deck ready to present: Executive Summary, Protection Coverage, Job History, Best Practices Assessment (all 17 checks), Recovery Readiness Score, Application Risk Matrix, Actions Required, Next Steps
- **Recovery Readiness Score** — composite score (0–100, grade A–F) across 8 weighted dimensions: protection coverage, backup recency, offsite export, immutability, disaster recovery, authentication, encryption, restore test
- **Application Risk Matrix** — per-namespace RPO/RTO estimate with risk level (🔴/🟡/🟢), export and immutability indicators
- **Statistics & QBR dashboard** — interactive HTML tab with coverage donut, job outcome breakdown, monthly trend (stacked Complete/Failed/Skipped), security posture radar, concurrency limiters, weekly SLA trend, gaps to address

### Best Practices (17 total, up from 11)
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

### Diagnostics
- **Recent Failures (top-5)** — unified ranking of failed BackupAction, ExportAction, RestoreAction with deepest cause-chain error (unwraps up to 5 levels of nested K10 error JSON)
- **Long-running Actions** — running actions older than 24h flagged (likely hung Kanister jobs)
- **Backup Recency per Namespace** — last successful backup/export/restore per namespace, days since last backup, stale flag (>7 days)
- **StorageClass & VSC Inventory** — CSI cross-check: warns when a CSI provisioner has no matching VolumeSnapshotClass

### Cluster & Platform
- **KubeVirt / OCP Virtualization** — VM inventory with protection status (K10 8.5+)
- **OpenShift support** — context name cleanup, platform detection, Red Hat CoreOS node display
- **Multi-cluster detection** — primary/secondary/standalone KDR mode
- **Storage age banner** — contextual banner when K10 system report data is stale (>7 days = yellow, >30 days = red, missing = red)

### Usability
- **Automatic output directory** — reports saved to `./reports/report-YYYY-MM-DD-HH-MM-SS/`
- **Cluster name from kubeconfig** — reads `current-context` instead of showing `k8s-cluster`
- **`--cluster-name` flag** — override cluster display name in report and PPTX
- **Filterable K10 reports** — default 30-day view with 30d/90d/All time filters
- **Restore point detail table** — name, application, creation date, policy
- **Veeam green favicon** — browser tab icon in Veeam brand colour

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

```bash
git clone https://github.com/kubanto/kasten-inspector.git
cd kasten-inspector

# Download dependencies
GONOSUMDB=* GOFLAGS=-mod=mod go mod download

# Build for current platform
make build

# Build all platforms → dist/
make all
```

### Manual build (Apple Silicon)
```bash
GONOSUMDB=* GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
  go build -o kasten-inspector ./cmd/
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
