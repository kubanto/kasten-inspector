# Changelog

All notable changes to Kasten Inspector are documented here.

---

## [1.3.0] — 2026-06-24

### Added

#### Best Practices (8 new checks — 25 total)
- BP-18: Dashboard exposed via Ingress with HTTPS (warning) — detects port-forward or missing TLS
- BP-19: VolumeSnapshotClass has Kasten annotation `k10.kasten.io/is-snapshot-class=true` (critical)
- BP-20: No policies with wildcard namespace selector (warning) — flags policies targeting all namespaces
- BP-21: Dedicated policy for cluster-scoped resources (warning) — StorageClasses, CRDs, ClusterRoles
- BP-22: Location profiles use object storage, not NFS/SMB only (warning)
- BP-23: PolicyPresets defined for retention standardization (info)
- BP-24: Catalog storage has ≥50% free space — upgrade prerequisite (warning/critical)
- BP-25: Prometheus alert rules (PrometheusRule CRs) configured (info)

#### New "Health Check" tab
- Dedicated tab aggregating all cluster and Kasten installation health information
- Cluster info (platform, K8s version, nodes, storage classes)
- License status with expiry and node usage
- Security configuration (authentication method, encryption provider)
- K10 resource limits per deployment
- Kasten Disaster Recovery status
- StorageClass & VolumeSnapshotClass inventory with CSI cross-check

#### Report tab reorganization
- 8 tabs total (was 7): Overview, Health Check, Protection, Operations, Storage, Configuration, Statistics & QBR, Diagnostics
- PVC status moved from Protection → Storage tab
- Security moved from Protection → Health Check tab
- KDR status moved from Operations → Health Check tab
- License and K10 Resource Limits moved from Configuration → Health Check tab
- StorageClass & VSC Inventory moved from Diagnostics → Health Check tab
- Configuration tab now contains only Kanister Blueprints & TransformSets

#### Data model & collectors
- `HelmConfig.IngressTLS` — detects TLS on the K10 dashboard Ingress
- `VolumeSnapshotClassInfo.HasKastenAnnotation` — checks for `k10.kasten.io/is-snapshot-class=true`
- `PrometheusInfo.AlertRules` — detects PrometheusRule CRs
- `Policy.IsWildcard` — flags policies with no namespace selector
- `Policy.IsClusterScoped` — flags cluster-scoped policies

### Changed
- `collectHelmConfig`: also captures TLS status from Ingress spec
- `collectVolumeProvisionerAudit`: reads Kasten annotation on each VolumeSnapshotClass
- `collectPrometheus`: scans for PrometheusRule CRs in namespace and cluster-scoped

---

## [1.2.0] — 2026-05-22

### Added

#### QBR & Reporting
- PowerPoint QBR deck generation (`--pptx` flag) — 11-slide deck with Executive Summary, Protection Coverage, Job History, Best Practices Assessment, Recovery Readiness Score, Application Risk Matrix, Actions Required, and Next Steps slides
- Recovery Readiness Score (0–100, grade A–F) — composite score across 8 weighted dimensions: protection coverage, backup recency, offsite export, immutability, disaster recovery, authentication, encryption, restore test
- Application Risk Matrix — per-namespace RPO/RTO estimate with risk level indicator and immutability/export status
- Statistics & QBR interactive tab — coverage donut, job outcome donut, monthly job trend (stacked Complete/Failed/Skipped), security posture radar chart, K10 concurrency limiters, weekly SLA trend, gaps to address
- `--tam`, `--customer`, `--meeting-date`, `--cluster-name` flags for PPTX personalisation

#### Best Practices (6 new checks)
- BP-12: Snapshot retention within safe bounds (warning when total retention > 7)
- BP-13: At least one snapshot retained per policy (warning)
- BP-14: Export retention explicitly configured (warning)
- BP-15: CSI provisioners have snapshot capability (warning)
- BP-16: Protected namespaces — backup recency within 7 days (warning)
- BP-17: Restore test performed at least once (critical)

#### Diagnostics Tab (new)
- Recent Failures top-5 — unified ranking across BackupAction, ExportAction, RestoreAction with recursive error unwrapping (up to 5 levels)
- Long-running Actions — running actions older than 24h
- Backup Recency per Namespace — last backup/export/restore timestamps, days since last backup, stale flag
- StorageClass & VolumeSnapshotClass Inventory — CSI cross-check with warning when no VSC matches a CSI provisioner

#### Cluster & Platform
- KubeVirt / OCP Virtualization — VM inventory with protection status (K10 8.5+)
- OpenShift context name cleanup — removes API server URL from context display
- Multi-cluster mode detection (primary / secondary / standalone)
- Storage age banner — contextual indicator for stale K10 report data (7d yellow, 30d+ red, missing red)

#### Usability
- Automatic timestamped output directory (`./reports/report-YYYY-MM-DD-HH-MM-SS/`)
- Cluster name from kubeconfig `current-context` (no longer hardcoded)
- K10 reports filtered by default to last 30 days (30d/90d/All time buttons)
- Restore point detail table with name, application, date, policy
- Filterable job list with re-apply on tab switch
- `--cluster-name` flag to override cluster display name
- Veeam green favicon (`#00C853`) in HTML report browser tab
- K10 Inspector logo updated to Veeam green

### Fixed
- Application total count was incorrect on OCP clusters (showed K10 report total instead of detected namespaces)
- BP-07 "Disaster Recovery configured" showed "last run: never" even when DR policy had run
- Orphaned restore points included KDR backups from `kasten-io` namespace (false positives)
- `openebs.io/local` StorageClass incorrectly flagged as missing VolumeSnapshotClass
- `terminating` namespaces incorrectly included in application count
- Tab switching broken by unbalanced `</div>` in Policy frequencies section
- `actionsByTypeJSON` and other Go template JSON values were not wrapped in `JSON.parse()`, causing chart labels to render as character indices
- `IsSystemNamespace` missing entries for `kasten-io-mc`, `longhorn-system`, `monitoring`, `cattle-*`, `rancher-*`
- K10 version fallback from K10 Report CRD when image tag is not available
- `enrichPolicyLastRun` and `enrichDRFromPolicies` called after compliance/BP evaluation — BP-07 and BP-16 now computed with correct data

### Changed
- Tab structure reorganised: Protection KPI banner moved from Overview to Protection tab
- Best Practices in Overview replaced with compact summary (non-pass checks only) with link to Statistics & QBR
- HTML footer updated: removed TAM name, shows generation timestamp and cluster name
- PPTX footer updated: shows `Customer: <name>  ·  Cluster: <name>` instead of TAM name
- Best Practices Assessment slide updated: shows all 17 checks in 3-column grid
- `kasten-io` KDR backups excluded from orphaned restore point count
- `kasten-io-mc` and infrastructure namespaces excluded from application list

---

## [1.0.0] — 2025-12-01

Initial release.

- HTML, JSON, and Markdown report generation
- 11 automated Best Practice checks (BP-01 to BP-11)
- Cluster info, Authentication, Encryption, Security config
- Policy inventory with frequency, retention, export profile
- Application protection coverage matrix
- Restore point inventory (total, orphaned)
- PVC status with health donut chart
- Job history with time filters
- K10 Disaster Recovery status
- Kanister Blueprints and TransformSets inventory
- K10 Resource Limits per container
- License information
- Location and Infrastructure Profiles
- RBAC manifest for in-cluster execution
