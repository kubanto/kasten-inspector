# Release Notes — Kasten Inspector v1.4.0

**Released:** 2026-07-14  
**Repository:** https://github.com/kubanto/kasten-inspector  
**Previous version:** v1.3.1

---

## Highlights

### Per-action success rate in Markdown & PPTX
The "%snapshot success / %export success" KPI (added to HTML/JSON in v1.3.1) now also appears in the **Markdown** summary and, most importantly, in the **PPTX QBR deck**: the "Job History & Success Rate" slide gains **Snapshot success** and **Export success** KPI cards (in K10 the `backup` action = snapshot).

### Report tab reorganization
Clearer separation of concerns in the HTML report: Catalog and CSI/VSC inventory moved to **Storage**; Restore Points consolidated under **Operations**; **Failures by Policy** now rendered in **Diagnostics**; security flags (FIPS/Audit/Network Policies) shown once in **Security**.

### Fixed
- QBR PowerPoint no longer opens with a "repair" prompt when `--tam` is omitted or when the customer/TAM name contains `<`, `>` or `&` (the document-author field is now XML-escaped).

### Breaking Changes

None. All existing flags and JSON fields are unchanged; additions are additive.

---

# Release Notes — Kasten Inspector v1.3.1

**Released:** 2026-07-14  
**Repository:** https://github.com/kubanto/kasten-inspector  
**Previous version:** v1.3.0

---

## Highlights

### Per-action success rate (snapshot / export)

The report now surfaces the **success rate broken down by action** — the direct answer to KPIs like "%snapshot success rate" and "%export success rate":

- **JSON** — `kasten.jobSummary.successByAction` gives, per action (`backup`/snapshot, `export`, `restore`), the completed/failed counts and the `successRate` (%). Skipped, Running and Cancelled are excluded from the denominator, matching the existing 7-day success-rate semantics.
- **Authoritative source** — `kasten.k10Reports[].stats.actions` now retains per-action `snapshotCompleted/Failed`, `exportCompleted/Failed` and `restoreCompleted/Failed` straight from the K10 report, independent of the job-collection window (ideal for multi-cluster aggregation).
- **HTML** — new **"Success Rate by Action"** rows in the Overview → Compliance & SLA card.

> Note: in K10 the `backup` action corresponds to snapshots.

### Breaking Changes

None. All existing flags and JSON fields are unchanged; the new fields are additive.

---

# Release Notes — Kasten Inspector v1.3.0

**Released:** 2026-06-24  
**Repository:** https://github.com/kubanto/kasten-inspector  
**Previous version:** v1.2.0

---

## Highlights

### New "Health Check" tab

All cluster and Kasten installation health information is now consolidated in a single dedicated tab: **Cluster info**, **License**, **Security** (auth + encryption), **K10 Resource Limits**, **Disaster Recovery status**, and **StorageClass & VSC Inventory**. No more hunting across multiple tabs to assess the health of an installation.

### 8 new Best Practice checks (25 total)

Built directly from the [Veeam Kasten Best Practices guide](https://docs.kasten.io/latest/references/best-practices):

| ID | Check | Severity |
|----|-------|----------|
| BP-18 | Dashboard exposed via Ingress with HTTPS | warning |
| BP-19 | VolumeSnapshotClass has Kasten annotation `k10.kasten.io/is-snapshot-class=true` | **critical** |
| BP-20 | No policies with wildcard namespace selector | warning |
| BP-21 | Dedicated policy for cluster-scoped resources | warning |
| BP-22 | Location profiles use object storage (not NFS/SMB only) | warning |
| BP-23 | PolicyPresets defined for retention standardization | info |
| BP-24 | Catalog storage ≥50% free (upgrade prerequisite) | warning |
| BP-25 | Prometheus alert rules (PrometheusRule CRs) configured | info |

**BP-19** is particularly important: without the `k10.kasten.io/is-snapshot-class=true` annotation on VolumeSnapshotClasses, Kasten silently skips CSI snapshots — a common misconfiguration that is otherwise invisible.

**BP-24** flags clusters at risk of failing a Kasten upgrade because the catalog PVC has less than 50% free space.

### Reorganized report tabs

The 7 tabs have been reorganized into 8, with clearer separation of concerns:

| Tab | Content |
|-----|---------|
| Overview | Executive Summary, Best Practices alerts, Compliance & SLA |
| **Health Check** ← new | Cluster, License, Security, Resource Limits, DR status, CSI inventory |
| Protection | Policies, Applications, Profiles, KubeVirt, Coverage matrix |
| Operations | Jobs, Restore Points, K10 Reports, Actions summary |
| Storage | Storage Overview, Breakdown, **PVC status** (moved here) |
| Configuration | Kanister Blueprints & TransformSets only |
| Statistics & QBR | Unchanged |
| Diagnostics | Recent Failures, Long-running Actions, Backup Recency |

---

## Breaking Changes

None. All existing flags continue to work.

---

## Upgrade from v1.2

Replace the binary and run as before. No other changes required.

---

## Download

| Platform | Binary |
|----------|--------|
| macOS Apple Silicon | `kasten-inspector-darwin-arm64` |
| macOS Intel | `kasten-inspector-darwin-amd64` |
| Linux x86\_64 | `kasten-inspector-linux-amd64` |
| Linux ARM64 | `kasten-inspector-linux-arm64` |
| Windows x86\_64 | `kasten-inspector-windows-amd64.exe` |

SHA256 checksums are provided in `checksums.txt`.

---

## Known Issues

- K10 version is not detectable on OpenShift clusters using the Red Hat registry (image uses SHA digest instead of version tag).
- Storage snapshot and export size metrics require the `k10-system-reports-policy` to have run at least once.
- `IsClusterScoped` policy detection is based on `subType` field — may not detect all cluster-scoped policies depending on K10 version.

---

> ⚠️ This is an independent personal project — not an official Veeam product.  
> No support, no SLA. Use at your own risk. See [DISCLAIMER.md](DISCLAIMER.md).
