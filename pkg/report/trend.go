package report

import (
	"fmt"
	"html/template"
	"math"
	"os"
	"time"

	"github.com/veeam/kasten-inspector/pkg/kasten"
)

// ── Data model ────────────────────────────────────────────────────────────────

type TrendData struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	ToolVersion string        `json:"toolVersion"`
	ClusterName string        `json:"clusterName"`
	Before      *Data         `json:"before"`
	After       *Data         `json:"after"`
	Delta       TrendDelta    `json:"delta"`
	Changes     []TrendChange `json:"changes"`
}

type TrendDelta struct {
	ProtectionCoverage float64 `json:"protectionCoveragePP"`
	JobSuccessRate     float64 `json:"jobSuccessRatePP"`
	BPPassed           int     `json:"bestPracticesPassed"`
	BPCritical         int     `json:"bestPracticesCritical"`
	Applications       int     `json:"applications"`
	Protected          int     `json:"protected"`
	Unprotected        int     `json:"unprotected"`
	RestorePoints      int     `json:"restorePoints"`
	Policies           int     `json:"policies"`
	FailedJobs7d       int     `json:"failedJobs7d"`
}

type TrendChange struct {
	Area     string `json:"area"`
	Type     string `json:"type"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// ── Compute ───────────────────────────────────────────────────────────────────

func ComputeTrend(before, after *Data) *TrendData {
	t := &TrendData{
		GeneratedAt: time.Now().UTC(),
		ToolVersion: after.ToolVersion,
		ClusterName: after.Cluster.Name,
		Before:      before,
		After:       after,
	}
	b, a := before.Kasten, after.Kasten

	t.Delta.ProtectionCoverage = a.Compliance.ProtectionCoverage - b.Compliance.ProtectionCoverage
	t.Delta.JobSuccessRate = a.Compliance.SuccessRate7d - b.Compliance.SuccessRate7d
	t.Delta.BPPassed = a.BestPractices.Passed - b.BestPractices.Passed
	t.Delta.BPCritical = a.BestPractices.Critical - b.BestPractices.Critical
	t.Delta.Applications = a.Applications.Total - b.Applications.Total
	t.Delta.Protected = a.Applications.Protected - b.Applications.Protected
	t.Delta.Unprotected = a.Applications.Unprotected - b.Applications.Unprotected
	t.Delta.RestorePoints = a.RestorePoints.Total - b.RestorePoints.Total
	t.Delta.Policies = len(a.Policies) - len(b.Policies)
	t.Delta.FailedJobs7d = a.Compliance.FailedJobs7d - b.Compliance.FailedJobs7d

	t.Changes = append(t.Changes, trendCoverage(b, a)...)
	t.Changes = append(t.Changes, trendSecurity(b, a)...)
	t.Changes = append(t.Changes, trendPolicies(b, a)...)
	t.Changes = append(t.Changes, trendProfiles(b, a)...)
	t.Changes = append(t.Changes, trendBP(b, a)...)
	t.Changes = append(t.Changes, trendStorage(b, a)...)
	t.Changes = append(t.Changes, trendVersion(b, a)...)
	return t
}

func trendCoverage(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	d := a.Compliance.ProtectionCoverage - b.Compliance.ProtectionCoverage
	if math.Abs(d) >= 1 {
		typ, sev := "improved", "info"
		if d < 0 {
			typ = "degraded"
			if d < -10 {
				sev = "critical"
			} else {
				sev = "warning"
			}
		}
		ch = append(ch, TrendChange{"Protection", typ,
			fmt.Sprintf("Coverage: %.1f%% → %.1f%% (%+.1f pp)",
				b.Compliance.ProtectionCoverage, a.Compliance.ProtectionCoverage, d), sev})
	}
	bNS := map[string]bool{}
	for _, ns := range b.Namespaces.Unprotected {
		bNS[ns.Name] = true
	}
	aNS := map[string]bool{}
	for _, ns := range a.Namespaces.Unprotected {
		aNS[ns.Name] = true
		if !bNS[ns.Name] {
			ch = append(ch, TrendChange{"Protection", "degraded",
				"New unprotected namespace: " + ns.Name, "warning"})
		}
	}
	for _, ns := range b.Namespaces.Unprotected {
		if !aNS[ns.Name] {
			ch = append(ch, TrendChange{"Protection", "improved",
				"Namespace now protected: " + ns.Name, "info"})
		}
	}
	return ch
}

func trendSecurity(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	if b.Security.AuthMethod != a.Security.AuthMethod {
		typ, sev := "improved", "info"
		if a.Security.AuthMethod == "None / Passthrough" {
			typ, sev = "degraded", "critical"
		}
		ch = append(ch, TrendChange{"Security", typ,
			fmt.Sprintf("Auth: %s → %s", b.Security.AuthMethod, a.Security.AuthMethod), sev})
	}
	if b.Security.Encryption.Enabled != a.Security.Encryption.Enabled {
		if a.Security.Encryption.Enabled {
			ch = append(ch, TrendChange{"Security", "improved",
				"Encryption enabled (" + a.Security.Encryption.Provider + ")", "info"})
		} else {
			ch = append(ch, TrendChange{"Security", "degraded", "Encryption was disabled", "critical"})
		}
	}
	return ch
}

func trendPolicies(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	bMap := map[string]kasten.Policy{}
	for _, p := range b.Policies {
		bMap[p.Name] = p
	}
	aMap := map[string]kasten.Policy{}
	for _, p := range a.Policies {
		aMap[p.Name] = p
	}
	for name := range aMap {
		if _, ok := bMap[name]; !ok {
			ch = append(ch, TrendChange{"Policy", "new", "New policy: " + name, "info"})
		}
	}
	for name, bp := range bMap {
		ap, ok := aMap[name]
		if !ok {
			ch = append(ch, TrendChange{"Policy", "removed", "Policy removed: " + name, "warning"})
			continue
		}
		if bp.Enabled && !ap.Enabled {
			ch = append(ch, TrendChange{"Policy", "degraded", "Policy disabled: " + name, "warning"})
		} else if !bp.Enabled && ap.Enabled {
			ch = append(ch, TrendChange{"Policy", "improved", "Policy re-enabled: " + name, "info"})
		}
	}
	return ch
}

func trendProfiles(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	bMap := map[string]kasten.Profile{}
	for _, p := range b.Profiles {
		bMap[p.Name] = p
	}
	aMap := map[string]bool{}
	for _, p := range a.Profiles {
		aMap[p.Name] = true
		bp, exists := bMap[p.Name]
		if !exists {
			ch = append(ch, TrendChange{"Profile", "new", "New profile: " + p.Name, "info"})
			continue
		}
		if !bp.Immutability && p.Immutability {
			ch = append(ch, TrendChange{"Profile", "improved",
				fmt.Sprintf("Immutability enabled: %s (%s)", p.Name, p.ImmutabilityPeriod), "info"})
		} else if bp.Immutability && !p.Immutability {
			ch = append(ch, TrendChange{"Profile", "degraded",
				"Immutability removed: " + p.Name, "critical"})
		}
	}
	for _, bp := range b.Profiles {
		if !aMap[bp.Name] {
			ch = append(ch, TrendChange{"Profile", "removed", "Profile removed: " + bp.Name, "warning"})
		}
	}
	return ch
}

func trendBP(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	d := a.BestPractices.Passed - b.BestPractices.Passed
	if d > 0 {
		ch = append(ch, TrendChange{"Best Practices", "improved",
			fmt.Sprintf("%d more checks passing (%d → %d)", d, b.BestPractices.Passed, a.BestPractices.Passed), "info"})
	} else if d < 0 {
		ch = append(ch, TrendChange{"Best Practices", "degraded",
			fmt.Sprintf("%d fewer checks passing (%d → %d)", -d, b.BestPractices.Passed, a.BestPractices.Passed), "warning"})
	}
	cd := a.BestPractices.Critical - b.BestPractices.Critical
	if cd > 0 {
		ch = append(ch, TrendChange{"Best Practices", "degraded",
			fmt.Sprintf("%d new critical issues", cd), "critical"})
	} else if cd < 0 {
		ch = append(ch, TrendChange{"Best Practices", "improved",
			fmt.Sprintf("%d critical issues resolved", -cd), "info"})
	}
	return ch
}

func trendStorage(b, a *kasten.Data) []TrendChange {
	var ch []TrendChange
	if b.Storage.SnapshotSizeBytes > 0 && a.Storage.SnapshotSizeBytes > 0 {
		ratio := float64(a.Storage.SnapshotSizeBytes) / float64(b.Storage.SnapshotSizeBytes)
		if ratio > 2 {
			ch = append(ch, TrendChange{"Storage", "degraded",
				fmt.Sprintf("Snapshot storage doubled: %s → %s", b.Storage.SnapshotSizeHuman, a.Storage.SnapshotSizeHuman), "warning"})
		}
	}
	if b.RestorePoints.Orphaned == 0 && a.RestorePoints.Orphaned > 0 {
		ch = append(ch, TrendChange{"Storage", "degraded",
			fmt.Sprintf("%d new orphaned restore points", a.RestorePoints.Orphaned), "warning"})
	} else if b.RestorePoints.Orphaned > 0 && a.RestorePoints.Orphaned == 0 {
		ch = append(ch, TrendChange{"Storage", "improved",
			"All orphaned restore points cleaned up", "info"})
	}
	return ch
}

func trendVersion(b, a *kasten.Data) []TrendChange {
	if b.Version != a.Version && b.Version != "unknown" && a.Version != "unknown" {
		return []TrendChange{{"K10", "new",
			fmt.Sprintf("K10 upgraded: %s → %s", b.Version, a.Version), "info"}}
	}
	return nil
}

// ── HTML writer ───────────────────────────────────────────────────────────────

func WriteTrendHTML(path string, d *TrendData) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	funcMap := template.FuncMap{
		"fmtDate": func(t time.Time) string { return t.Format("02 Jan 2006") },
		"fmtTime": func(t time.Time) string { return t.Format("02 Jan 2006 15:04 UTC") },
		"delta": func(v float64) string {
			if v > 0 {
				return fmt.Sprintf("+%.1f", v)
			}
			return fmt.Sprintf("%.1f", v)
		},
		"deltaInt": func(v int) string {
			if v > 0 {
				return fmt.Sprintf("+%d", v)
			}
			return fmt.Sprintf("%d", v)
		},
		"deltaClass": func(v float64, positiveIsGood bool) string {
			if v == 0 {
				return "td-neutral"
			}
			good := v > 0
			if !positiveIsGood {
				good = !good
			}
			if good {
				return "td-good"
			}
			return "td-bad"
		},
		"deltaClassInt": func(v int, positiveIsGood bool) string {
			if v == 0 {
				return "td-neutral"
			}
			good := v > 0
			if !positiveIsGood {
				good = !good
			}
			if good {
				return "td-good"
			}
			return "td-bad"
		},
		"changeClass": func(severity string) string {
			switch severity {
			case "critical":
				return "change-critical"
			case "warning":
				return "change-warning"
			default:
				return "change-info"
			}
		},
		"changeIcon": func(typ string) string {
			switch typ {
			case "improved":
				return "↑"
			case "degraded":
				return "↓"
			case "new":
				return "+"
			case "removed":
				return "−"
			default:
				return "~"
			}
		},
		"pct": func(f float64) string { return fmt.Sprintf("%.1f%%", f) },
		"abs": func(v float64) float64 {
			if v < 0 {
				return -v
			}
			return v
		},
	}

	tmpl, err := template.New("trend").Funcs(funcMap).Parse(trendHTMLTmpl)
	if err != nil {
		return fmt.Errorf("parsing trend template: %w", err)
	}
	return tmpl.Execute(f, d)
}

var trendHTMLTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%23FFB800'/%3E%3Ctext x='16' y='22' text-anchor='middle' font-family='monospace' font-weight='700' font-size='13' fill='%23000'%3EK10%3C/text%3E%3C/svg%3E">
<title>K10 Trend — {{.ClusterName}}</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap');
:root{
  --bg:#0d1117;--s1:#161b22;--s2:#1c2230;--b:#30363d;
  --t:#e6edf3;--tm:#8b949e;
  --green:#3fb950;--red:#f85149;--yellow:#d29922;
  --blue:#58a6ff;--kasten:#FFB800;
}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'IBM Plex Sans',sans-serif;background:var(--bg);color:var(--t);font-size:14px;line-height:1.6}
.page{max-width:1100px;margin:0 auto;padding:0 24px 80px}
.hdr{border-bottom:1px solid var(--b);padding:24px 0 18px;display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:12px}
.logo{width:38px;height:38px;border-radius:8px;background:var(--kasten);display:flex;align-items:center;justify-content:center;font-family:'IBM Plex Mono',monospace;font-weight:700;font-size:14px;color:#000}
.hdr-brand{display:flex;align-items:center;gap:12px}
.hdr-title h1{font-size:18px;font-weight:600}
.hdr-title p{color:var(--tm);font-size:12px;margin-top:2px}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;font-family:'IBM Plex Mono',monospace;background:rgba(255,184,0,.12);color:var(--kasten);border:1px solid rgba(255,184,0,.3);margin-top:4px}
.sec{margin-top:28px}
.sec-title{font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:12px}
/* Period comparison bar */
.period-bar{display:grid;grid-template-columns:1fr 60px 1fr;gap:0;background:var(--s1);border:1px solid var(--b);border-radius:10px;overflow:hidden;margin-bottom:20px}
.period-side{padding:16px 20px}
.period-side.before{border-right:1px solid var(--b)}
.period-label{font-size:10px;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:4px}
.period-date{font-size:15px;font-weight:600;font-family:'IBM Plex Mono',monospace;color:var(--t)}
.period-cluster{font-size:11px;color:var(--tm);margin-top:2px}
.period-arrow{display:flex;align-items:center;justify-content:center;font-size:20px;color:var(--kasten)}
/* Delta grid */
.delta-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:10px;margin-bottom:20px}
.dcard{background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:14px 16px}
.dcard-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:6px}
.dcard-val{font-size:22px;font-weight:700;font-family:'IBM Plex Mono',monospace;line-height:1}
.dcard-sub{font-size:11px;color:var(--tm);margin-top:4px}
.td-good{color:var(--green)}
.td-bad{color:var(--red)}
.td-neutral{color:var(--tm)}
/* Changes list */
.changes{display:flex;flex-direction:column;gap:6px}
.change{display:flex;align-items:flex-start;gap:10px;padding:10px 14px;background:var(--s1);border-radius:8px;border-left:3px solid transparent}
.change-info{border-left-color:var(--blue)}
.change-warning{border-left-color:var(--yellow)}
.change-critical{border-left-color:var(--red)}
.change-icon{font-size:13px;font-weight:700;min-width:18px;margin-top:1px}
.change-info .change-icon{color:var(--blue)}
.change-warning .change-icon{color:var(--yellow)}
.change-critical .change-icon{color:var(--red)}
.change-area{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);min-width:90px;margin-top:2px}
.change-msg{font-size:12px;flex:1}
/* Comparison table */
.twrap{background:var(--s1);border:1px solid var(--b);border-radius:10px;overflow:hidden;margin-top:12px}
table{width:100%;border-collapse:collapse}
thead th{background:var(--s2);padding:8px 12px;text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);border-bottom:1px solid var(--b)}
tbody tr{border-bottom:1px solid var(--b)}
tbody tr:last-child{border-bottom:none}
tbody tr:hover{background:var(--s2)}
td{padding:8px 12px;font-size:13px}
.mono{font-family:'IBM Plex Mono',monospace;font-size:12px}
.muted{color:var(--tm)}
/* BP comparison */
.bp-grid{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:12px}
.bp-row{display:flex;align-items:center;gap:8px;padding:8px 12px;background:var(--s1);border-radius:6px;font-size:11px}
.bp-id{font-family:'IBM Plex Mono',monospace;font-size:10px;color:var(--tm);min-width:38px}
.bp-name{flex:1;color:var(--t)}
.bp-before,.bp-after{min-width:60px;text-align:center;font-size:10px;font-weight:600;padding:2px 6px;border-radius:20px}
.bp-pass{background:rgba(63,185,80,.12);color:var(--green)}
.bp-fail{background:rgba(248,81,73,.12);color:var(--red)}
.bp-warn{background:rgba(210,153,34,.12);color:var(--yellow)}
.bp-arrow{color:var(--tm);font-size:10px}
.footer{margin-top:48px;padding-top:16px;border-top:1px solid var(--b);font-size:11px;color:var(--tm);display:flex;justify-content:space-between}
@media(max-width:700px){.bp-grid{grid-template-columns:1fr}.period-bar{grid-template-columns:1fr}}
</style>
</head>
<body>
<div class="page">

<!-- Header -->
<div class="hdr">
  <div class="hdr-brand">
    <div class="logo">K10</div>
    <div class="hdr-title">
      <h1>Kasten K10 — Trend Report</h1>
      <p>Period-over-period comparison · {{.ClusterName}}</p>
    </div>
  </div>
  <div>
    <div style="font-size:11px;color:var(--tm)">Generated {{fmtTime .GeneratedAt}}</div>
    <span class="badge">v{{.ToolVersion}}</span>
  </div>
</div>

<!-- Period bar -->
<div class="sec">
  <div class="period-bar">
    <div class="period-side before">
      <div class="period-label">Before</div>
      <div class="period-date">{{fmtDate .Before.GeneratedAt}}</div>
      <div class="period-cluster">{{.Before.Cluster.Name}} · K10 {{.Before.Kasten.Version}}</div>
    </div>
    <div class="period-arrow">→</div>
    <div class="period-side">
      <div class="period-label">After</div>
      <div class="period-date">{{fmtDate .After.GeneratedAt}}</div>
      <div class="period-cluster">{{.After.Cluster.Name}} · K10 {{.After.Kasten.Version}}</div>
    </div>
  </div>
</div>

<!-- Delta KPIs -->
<div class="sec">
  <div class="sec-title">Key metrics — period delta</div>
  <div class="delta-grid">
    <div class="dcard">
      <div class="dcard-label">Protection coverage</div>
      <div class="dcard-val {{deltaClass .Delta.ProtectionCoverage true}}">{{delta .Delta.ProtectionCoverage}} pp</div>
      <div class="dcard-sub">{{pct .Before.Kasten.Compliance.ProtectionCoverage}} → {{pct .After.Kasten.Compliance.ProtectionCoverage}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Job success rate</div>
      <div class="dcard-val {{deltaClass .Delta.JobSuccessRate true}}">{{delta .Delta.JobSuccessRate}} pp</div>
      <div class="dcard-sub">{{pct .Before.Kasten.Compliance.SuccessRate7d}} → {{pct .After.Kasten.Compliance.SuccessRate7d}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">BP checks passed</div>
      <div class="dcard-val {{deltaClassInt .Delta.BPPassed true}}">{{deltaInt .Delta.BPPassed}}</div>
      <div class="dcard-sub">{{.Before.Kasten.BestPractices.Passed}} → {{.After.Kasten.BestPractices.Passed}} of {{.After.Kasten.BestPractices.TotalChecks}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Critical issues</div>
      <div class="dcard-val {{deltaClassInt .Delta.BPCritical false}}">{{deltaInt .Delta.BPCritical}}</div>
      <div class="dcard-sub">{{.Before.Kasten.BestPractices.Critical}} → {{.After.Kasten.BestPractices.Critical}} critical</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Protected apps</div>
      <div class="dcard-val {{deltaClassInt .Delta.Protected true}}">{{deltaInt .Delta.Protected}}</div>
      <div class="dcard-sub">{{.Before.Kasten.Applications.Protected}} → {{.After.Kasten.Applications.Protected}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Failed jobs (7d)</div>
      <div class="dcard-val {{deltaClassInt .Delta.FailedJobs7d false}}">{{deltaInt .Delta.FailedJobs7d}}</div>
      <div class="dcard-sub">{{.Before.Kasten.Compliance.FailedJobs7d}} → {{.After.Kasten.Compliance.FailedJobs7d}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Restore points</div>
      <div class="dcard-val {{deltaClassInt .Delta.RestorePoints true}}">{{deltaInt .Delta.RestorePoints}}</div>
      <div class="dcard-sub">{{.Before.Kasten.RestorePoints.Total}} → {{.After.Kasten.RestorePoints.Total}}</div>
    </div>
    <div class="dcard">
      <div class="dcard-label">Policies</div>
      <div class="dcard-val td-neutral">{{deltaInt .Delta.Policies}}</div>
      <div class="dcard-sub">{{len .Before.Kasten.Policies}} → {{len .After.Kasten.Policies}}</div>
    </div>
  </div>
</div>

<!-- Changes -->
<div class="sec">
  <div class="sec-title">Notable changes ({{len .Changes}})</div>
  {{if .Changes}}
  <div class="changes">
    {{range .Changes}}
    <div class="change {{changeClass .Severity}}">
      <span class="change-icon">{{changeIcon .Type}}</span>
      <span class="change-area">{{.Area}}</span>
      <span class="change-msg">{{.Message}}</span>
    </div>
    {{end}}
  </div>
  {{else}}
  <div style="padding:20px;text-align:center;color:var(--tm);font-size:12px;background:var(--s1);border-radius:8px">No significant changes detected between the two reports.</div>
  {{end}}
</div>

<!-- Best Practices comparison -->
<div class="sec">
  <div class="sec-title">Best practices — before vs after</div>
  <div class="bp-grid">
    {{range $i, $a := .After.Kasten.BestPractices.Checks}}
    {{$b := index $.Before.Kasten.BestPractices.Checks $i}}
    <div class="bp-row">
      <span class="bp-id">{{$a.ID}}</span>
      <span class="bp-name">{{$a.Name}}</span>
      {{if $b}}<span class="bp-before bp-{{$b.Status}}">{{$b.Status}}</span>
      <span class="bp-arrow">→</span>{{end}}
      <span class="bp-after bp-{{$a.Status}}">{{$a.Status}}</span>
    </div>
    {{end}}
  </div>
</div>

<!-- Policy comparison -->
<div class="sec">
  <div class="sec-title">Policy comparison</div>
  <div class="twrap">
    <table>
      <thead><tr><th>Policy</th><th>Before — Enabled</th><th>Before — Last Run</th><th>After — Enabled</th><th>After — Last Run</th><th>Avg Duration</th></tr></thead>
      <tbody>
        {{range .After.Kasten.Policies}}<tr>
          <td class="mono" style="font-size:11px">{{.Name}}</td>
          <td>—</td><td class="muted">—</td>
          <td>{{if .Enabled}}<span style="color:var(--green)">✓</span>{{else}}<span style="color:var(--red)">✗</span>{{end}}</td>
          <td class="mono muted" style="font-size:11px">{{if .LastRunTime}}{{.LastRunTime | printf "%.10s"}}{{else}}never{{end}}</td>
          <td class="mono muted">{{if .AvgRunDuration}}{{.AvgRunDuration}}{{else}}—{{end}}</td>
        </tr>{{end}}
      </tbody>
    </table>
  </div>
</div>

<div class="footer">
  <span>Kasten K10 Inspector v{{.ToolVersion}} · {{.Author}}</span>
  <span>{{fmtTime .GeneratedAt}}</span>
</div>

</div>
</body>
</html>`
