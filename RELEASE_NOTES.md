# Release Notes — Kasten Inspector v1.5.2

**Released:** 2026-07-15  
**Repository:** https://github.com/kubanto/kasten-inspector  
**Previous version:** v1.4.0

> This file documents the **current release only**. For the full version history see [CHANGELOG.md](CHANGELOG.md).

---

## Highlights

### New "Recovery" tab (v1.5.0)
A dedicated **Recovery** tab groups everything that answers *"can we actually recover?"* in one place: **Recovery Readiness Score**, **Kasten Disaster Recovery (KDR)**, **Restore Points**, and the **Application Risk Matrix**. It sits between Protection and Operations. Disaster Recovery moved here from Health Check and Restore Points from Operations; Recovery Readiness and the Risk Matrix also remain in the Statistics & QBR tab.

### Data-correctness fixes (v1.5.1 – v1.5.2)
- **"Never backed up" false positive** — applications with real restore points (backed up on-demand, outside the collected job window) were wrongly flagged as *never backed up*. Last-backup now falls back to the newest restore point, so they correctly show as recent or stale.
- **Restore-point total** — now includes restore points in system/DR namespaces (KDR catalog in `kasten-io`, etcd backups), so the total reflects the whole cluster instead of user-application namespaces only.
- **Consistency across outputs** — backup-recency (best practice **BP-16**) and the Diagnostics view now inherit the same corrected last-backup value used by the Recovery Readiness Score and the Application Risk Matrix, so the HTML, JSON, Markdown, and PPTX outputs all tell the same story.

### Breaking Changes

None. All existing flags and JSON fields are unchanged; the additions are additive.

---

## Upgrade from v1.4.0

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
- `IsClusterScoped` policy detection is based on the `subType` field — may not detect all cluster-scoped policies depending on K10 version.

---

> ⚠️ This is an independent personal project — not an official Veeam product.  
> No support, no SLA. Use at your own risk. See [DISCLAIMER.md](DISCLAIMER.md).
