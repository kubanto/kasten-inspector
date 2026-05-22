# Release Notes — Kasten Inspector v1.2.0

**Released:** 2026-05-22  
**Repository:** https://github.com/kubanto/kasten-inspector  
**Previous version:** v1.0.0

---

## Highlights

### QBR-ready PowerPoint output

Run the tool once, get a presentation-ready deck:

```bash
./kasten-inspector --pptx \
  --tam="Your Name" \
  --customer="Customer Corp" \
  --meeting-date="Q2 2026" \
  --cluster-name="prod-ocp"
```

The generated `kasten-qbr-*.pptx` contains 11 slides including a **Recovery Readiness Score** with per-component breakdown and an **Application Risk Matrix** with per-namespace RPO/RTO estimates — ready to open in PowerPoint or Google Slides.

### Recovery Readiness Score

A single number (0–100) and letter grade (A–F) that answers *"how recoverable is this cluster right now?"* across 8 dimensions. Built for QBR conversations with customer management.

### 17 automated Best Practice checks

6 new checks added to the original 11:
- **BP-15** flags CSI StorageClasses with no matching VolumeSnapshotClass — PVCs on these classes require a Kanister Blueprint
- **BP-16** flags protected namespaces that have never been successfully backed up (policy exists but never ran)
- **BP-17** flags clusters where no restore test has ever been performed — the most critical gap in a data protection posture

### Diagnostics tab

New tab with four sections: Recent Failures (top-5 with root cause), Long-running Actions (hung Kanister jobs), Backup Recency per Namespace, and StorageClass/VSC Inventory.

---

## Breaking Changes

None. All existing flags continue to work. New flags are additive.

---

## Upgrade from v1.0

No migration needed. Replace the binary and run as before. The output directory structure is unchanged; new output files are only created when the corresponding flag is passed (`--pptx`).

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

- K10 version is not detectable on OpenShift clusters using the Red Hat registry (image uses SHA digest instead of version tag). The report shows the version from the K10 system report CRD if available, or `unknown` otherwise.
- Storage snapshot and export size metrics require the `k10-system-reports-policy` to have run at least once. A contextual banner in the Storage tab indicates when data is stale or missing.

---

> ⚠️ This is an independent personal project — not an official Veeam product.  
> No support, no SLA. Use at your own risk. See [DISCLAIMER.md](DISCLAIMER.md).
