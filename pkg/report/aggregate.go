package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"sort"
	"time"
)

// ── Data model ────────────────────────────────────────────────────────────────

type AggregateReport struct {
	GeneratedAt  time.Time         `json:"generatedAt"`
	ToolVersion  string            `json:"toolVersion"`
	TotalClusters int              `json:"totalClusters"`
	Summary      AggregateSummary  `json:"summary"`
	Clusters     []*Data           `json:"clusters"`
}

type AggregateSummary struct {
	TotalApplications  int     `json:"totalApplications"`
	TotalProtected     int     `json:"totalProtected"`
	TotalUnprotected   int     `json:"totalUnprotected"`
	AvgCoverage        float64 `json:"avgCoveragePercent"`
	TotalPolicies      int     `json:"totalPolicies"`
	TotalProfiles      int     `json:"totalProfiles"`
	TotalRestorePoints int     `json:"totalRestorePoints"`
	TotalOrphaned      int     `json:"totalOrphanedRestorePoints"`
	TotalFailedJobs7d  int     `json:"totalFailedJobs7d"`
	ClustersWithNoAuth int     `json:"clustersWithNoAuth"`
	ClustersWithNoDR   int     `json:"clustersWithNoDR"`
	ClustersNeedAction []string `json:"clustersNeedingAction"`
	K10Versions        map[string]int `json:"k10Versions"`
}

// ── Builder ───────────────────────────────────────────────────────────────────

func BuildAggregateReport(clusters []*Data, toolVersion string) *AggregateReport {
	r := &AggregateReport{
		GeneratedAt:   time.Now().UTC(),
		ToolVersion:   toolVersion,
		TotalClusters: len(clusters),
		Clusters:      clusters,
		Summary: AggregateSummary{
			K10Versions: map[string]int{},
		},
	}

	totalCoverage := 0.0
	for _, c := range clusters {
		k := c.Kasten
		r.Summary.TotalApplications += k.Applications.Total
		r.Summary.TotalProtected += k.Applications.Protected
		r.Summary.TotalUnprotected += k.Applications.Unprotected
		r.Summary.TotalPolicies += len(k.Policies)
		r.Summary.TotalProfiles += len(k.Profiles)
		r.Summary.TotalRestorePoints += k.RestorePoints.Total
		r.Summary.TotalOrphaned += k.RestorePoints.Orphaned
		r.Summary.TotalFailedJobs7d += k.Compliance.FailedJobs7d
		totalCoverage += k.Compliance.ProtectionCoverage

		if k.Security.AuthMethod == "None / Passthrough" || k.Security.AuthMethod == "" {
			r.Summary.ClustersWithNoAuth++
		}
		if !k.DR.Enabled {
			r.Summary.ClustersWithNoDR++
		}
		// Flag clusters needing attention
		if k.BestPractices.Critical > 0 || k.Compliance.ProtectionCoverage < 50 {
			r.Summary.ClustersNeedAction = append(r.Summary.ClustersNeedAction, c.Cluster.Name)
		}

		if k.Version != "" && k.Version != "unknown" {
			r.Summary.K10Versions[k.Version]++
		}
	}

	if len(clusters) > 0 {
		r.Summary.AvgCoverage = totalCoverage / float64(len(clusters))
	}

	// Sort clusters: worst coverage first (most attention needed)
	sort.Slice(r.Clusters, func(i, j int) bool {
		return r.Clusters[i].Kasten.Compliance.ProtectionCoverage <
			r.Clusters[j].Kasten.Compliance.ProtectionCoverage
	})

	return r
}

// ── HTML writer ───────────────────────────────────────────────────────────────

func WriteAggregateHTML(path string, d *AggregateReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	funcMap := template.FuncMap{
		"fmtTime": func(t time.Time) string { return t.Format("02 Jan 2006 15:04 UTC") },
		"fmtDate": func(s string) string {
			if s == "" {
				return "—"
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return s
			}
			return t.Format("02 Jan 2006")
		},
		"pct": func(f float64) string { return fmt.Sprintf("%.1f%%", f) },
		"colorPct": func(f float64) string {
			if f >= 80 {
				return "#3fb950"
			} else if f >= 50 {
				return "#d29922"
			}
			return "#f85149"
		},
		"bpColor": func(s string) string {
			switch s {
			case "pass":
				return "#3fb950"
			case "warning":
				return "#d29922"
			default:
				return "#f85149"
			}
		},
		"orDash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		"clusterAnchor": func(name string) string {
			r := ""
			for _, c := range name {
				if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
					r += string(c)
				} else if c >= 'A' && c <= 'Z' {
					r += string(c + 32)
				} else {
					r += "-"
				}
			}
			return r
		},
		"statusClass": func(s string) string {
			switch s {
			case "Complete", "pass":
				return "ok"
			case "Failed", "critical":
				return "fail"
			case "warning":
				return "warn"
			default:
				return "off"
			}
		},
		"join": func(s []string, sep string) string {
			result := ""
			for i, v := range s {
				if i > 0 {
					result += sep
				}
				result += v
			}
			return result
		},
	}

	tmpl, err := template.New("aggregate").Funcs(funcMap).Parse(aggregateHTMLTmpl)
	if err != nil {
		return fmt.Errorf("parsing aggregate template: %w", err)
	}
	return tmpl.Execute(f, d)
}


// WriteAggregateJSON writes the aggregate report as indented JSON.
func WriteAggregateJSON(path string, data *AggregateReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

var aggregateHTMLTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%23FFB800'/%3E%3Ctext x='16' y='22' text-anchor='middle' font-family='monospace' font-weight='700' font-size='13' fill='%23000'%3EK10%3C/text%3E%3C/svg%3E">
<title>K10 Multi-Cluster — Aggregate Report</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap');
:root{
  --bg:#0d1117;--s1:#161b22;--s2:#1c2230;--b:#30363d;
  --t:#e6edf3;--tm:#8b949e;
  --green:#3fb950;--red:#f85149;--yellow:#d29922;
  --blue:#58a6ff;--purple:#bc8cff;--orange:#ffa657;--kasten:#FFB800;
}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'IBM Plex Sans',sans-serif;background:var(--bg);color:var(--t);font-size:14px;line-height:1.6}
.page{max-width:1300px;margin:0 auto;padding:0 24px 80px}
.hdr{border-bottom:1px solid var(--b);padding:24px 0 18px;display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:12px}
.logo{width:38px;height:38px;border-radius:8px;background:var(--kasten);display:flex;align-items:center;justify-content:center;font-family:'IBM Plex Mono',monospace;font-weight:700;font-size:14px;color:#000}
.hdr-brand{display:flex;align-items:center;gap:12px}
.hdr-title h1{font-size:18px;font-weight:600}
.hdr-title p{color:var(--tm);font-size:12px;margin-top:2px}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;font-family:'IBM Plex Mono',monospace;background:rgba(255,184,0,.12);color:var(--kasten);border:1px solid rgba(255,184,0,.3);margin-top:4px}
/* Nav */
.nav{display:flex;gap:4px;padding:14px 0;border-bottom:1px solid var(--b);flex-wrap:wrap;position:sticky;top:0;background:var(--bg);z-index:10}
.nav a{padding:5px 12px;border-radius:6px;font-size:12px;font-weight:500;color:var(--tm);text-decoration:none;transition:background .15s,color .15s}
.nav a:hover,.nav a.active{background:var(--s2);color:var(--t)}
/* KPI */
.kpi-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:10px;margin:20px 0}
.kpi{background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:14px 16px}
.kpi-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:6px}
.kpi-val{font-size:26px;font-weight:700;font-family:'IBM Plex Mono',monospace;line-height:1}
.kpi-sub{font-size:11px;color:var(--tm);margin-top:4px}
/* Cluster cards grid (landing) */
.cluster-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:12px;margin-top:16px}
.cluster-card{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px;text-decoration:none;color:var(--t);transition:border-color .15s,background .15s;display:block}
.cluster-card:hover{border-color:var(--kasten);background:var(--s2)}
.cc-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:12px}
.cc-name{font-size:14px;font-weight:600}
.cc-version{font-size:10px;font-family:'IBM Plex Mono',monospace;color:var(--tm);background:var(--s2);padding:2px 6px;border-radius:4px}
.cc-bar-bg{background:var(--b);border-radius:3px;height:4px;overflow:hidden;margin:6px 0 4px}
.cc-bar-fill{height:100%;border-radius:3px}
.cc-stats{display:grid;grid-template-columns:1fr 1fr 1fr;gap:6px;margin-top:10px}
.cc-stat{text-align:center}
.cc-stat-val{font-size:16px;font-weight:700;font-family:'IBM Plex Mono',monospace}
.cc-stat-label{font-size:9px;color:var(--tm);text-transform:uppercase;letter-spacing:.4px}
.cc-alerts{margin-top:10px;display:flex;flex-wrap:wrap;gap:4px}
.cc-alert{font-size:10px;padding:1px 6px;border-radius:3px;font-weight:600}
.cc-alert.critical{background:rgba(248,81,73,.15);color:var(--red)}
.cc-alert.warning{background:rgba(210,153,34,.15);color:var(--yellow)}
.cc-alert.ok{background:rgba(63,185,80,.15);color:var(--green)}
/* Cluster detail sections */
.cluster-section{margin-top:40px;scroll-margin-top:60px}
.cs-hdr{display:flex;align-items:center;gap:10px;padding-bottom:12px;border-bottom:1px solid var(--b);margin-bottom:16px}
.cs-num{width:28px;height:28px;border-radius:6px;background:rgba(255,184,0,.12);color:var(--kasten);display:flex;align-items:center;justify-content:center;font-size:11px;font-weight:700;flex-shrink:0}
.cs-title{font-size:16px;font-weight:600}
.cs-meta{font-size:11px;color:var(--tm);margin-left:auto}
/* Tables */
.twrap{background:var(--s1);border:1px solid var(--b);border-radius:10px;overflow:hidden}
.tscroll{overflow-x:auto}
table{width:100%;border-collapse:collapse}
thead th{background:var(--s2);padding:8px 12px;text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);border-bottom:1px solid var(--b);white-space:nowrap}
tbody tr{border-bottom:1px solid var(--b)}
tbody tr:last-child{border-bottom:none}
tbody tr:hover{background:var(--s2)}
td{padding:8px 12px;font-size:13px;vertical-align:middle}
.mono{font-family:'IBM Plex Mono',monospace;font-size:12px}
.muted{color:var(--tm)}
.pill{display:inline-flex;align-items:center;gap:4px;padding:2px 7px;border-radius:20px;font-size:10px;font-weight:600}
.pill::before{content:'';width:5px;height:5px;border-radius:50%;display:inline-block}
.ok{background:rgba(63,185,80,.12);color:var(--green);border:1px solid rgba(63,185,80,.3)}.ok::before{background:var(--green)}
.fail{background:rgba(248,81,73,.12);color:var(--red);border:1px solid rgba(248,81,73,.3)}.fail::before{background:var(--red)}
.warn{background:rgba(210,153,34,.12);color:var(--yellow);border:1px solid rgba(210,153,34,.3)}.warn::before{background:var(--yellow)}
.off{background:rgba(139,148,158,.1);color:var(--tm);border:1px solid rgba(139,148,158,.2)}.off::before{background:var(--tm)}
/* Grid layouts */
.two{display:grid;grid-template-columns:1fr 1fr;gap:12px}
.three{display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px}
/* BP mini */
.bp-mini{display:grid;grid-template-columns:1fr 1fr;gap:6px}
.bp-mini-row{display:flex;align-items:center;gap:6px;padding:6px 10px;background:var(--s2);border-radius:6px;font-size:11px}
.bp-dot{width:7px;height:7px;border-radius:50%;flex-shrink:0}
/* Section separator */
.cluster-sep{height:1px;background:var(--b);margin:40px 0}
/* Back to top */
.back-top{display:inline-block;font-size:11px;color:var(--tm);text-decoration:none;padding:4px 10px;border-radius:4px;border:1px solid var(--b);margin-bottom:8px}
.back-top:hover{color:var(--t);background:var(--s2)}
.footer{margin-top:48px;padding-top:16px;border-top:1px solid var(--b);font-size:11px;color:var(--tm);display:flex;justify-content:space-between;flex-wrap:wrap;gap:8px}
@media(max-width:700px){.two{grid-template-columns:1fr}.three{grid-template-columns:1fr 1fr}.cluster-grid{grid-template-columns:1fr}.bp-mini{grid-template-columns:1fr}}
</style>
</head>
<body>
<div class="page">

<!-- Header -->
<div class="hdr">
  <div class="hdr-brand">
    <div class="logo">K10</div>
    <div class="hdr-title">
      <h1>Kasten K10 — Multi-Cluster Report</h1>
      <p>Aggregate view across {{.TotalClusters}} cluster{{if gt .TotalClusters 1}}s{{end}}</p>
    </div>
  </div>
  <div>
    <div style="font-size:11px;color:var(--tm)">{{fmtTime .GeneratedAt}}</div>
    <span class="badge">v{{.ToolVersion}}</span>
  </div>
</div>

<!-- Nav -->
<nav class="nav" id="top">
  <a href="#overview">Overview</a>
  {{range $i,$c := .Clusters}}
  <a href="#cluster-{{clusterAnchor $c.Cluster.Name}}">{{$c.Cluster.Name}}</a>
  {{end}}
</nav>

<!-- ══ OVERVIEW ══ -->
<div id="overview">

  <!-- Global KPIs -->
  <div class="kpi-grid" style="margin-top:20px">
    <div class="kpi"><div class="kpi-label">Clusters</div><div class="kpi-val" style="color:var(--blue)">{{.TotalClusters}}</div><div class="kpi-sub">inspected</div></div>
    <div class="kpi"><div class="kpi-label">Applications</div><div class="kpi-val" style="color:var(--blue)">{{.Summary.TotalApplications}}</div><div class="kpi-sub">across all clusters</div></div>
    <div class="kpi"><div class="kpi-label">Protected</div><div class="kpi-val" style="color:var(--green)">{{.Summary.TotalProtected}}</div><div class="kpi-sub">have a backup policy</div></div>
    <div class="kpi"><div class="kpi-label">Unprotected</div><div class="kpi-val" style="color:var(--red)">{{.Summary.TotalUnprotected}}</div><div class="kpi-sub">no backup policy</div></div>
    <div class="kpi"><div class="kpi-label">Avg coverage</div><div class="kpi-val" style="color:{{colorPct .Summary.AvgCoverage}}">{{pct .Summary.AvgCoverage}}</div><div class="kpi-sub">protection rate</div></div>
    <div class="kpi"><div class="kpi-label">No auth</div><div class="kpi-val" style="color:{{if gt .Summary.ClustersWithNoAuth 0}}var(--red){{else}}var(--green){{end}}">{{.Summary.ClustersWithNoAuth}}</div><div class="kpi-sub">clusters open</div></div>
    <div class="kpi"><div class="kpi-label">No DR</div><div class="kpi-val" style="color:{{if gt .Summary.ClustersWithNoDR 0}}var(--yellow){{else}}var(--green){{end}}">{{.Summary.ClustersWithNoDR}}</div><div class="kpi-sub">clusters without KDR</div></div>
    <div class="kpi"><div class="kpi-label">Failed jobs (7d)</div><div class="kpi-val" style="color:{{if gt .Summary.TotalFailedJobs7d 0}}var(--red){{else}}var(--green){{end}}">{{.Summary.TotalFailedJobs7d}}</div><div class="kpi-sub">across all clusters</div></div>
  </div>

  {{if .Summary.ClustersNeedAction}}
  <div style="padding:12px 16px;background:rgba(248,81,73,.08);border:1px solid rgba(248,81,73,.25);border-radius:8px;margin-bottom:16px;font-size:12px">
    <strong style="color:var(--red)">⚠ Clusters needing immediate attention:</strong>
    {{range .Summary.ClustersNeedAction}}
    <a href="#cluster-{{clusterAnchor .}}" style="color:var(--red);margin-left:8px">{{.}}</a>
    {{end}}
  </div>
  {{end}}

  <!-- Cluster cards grid -->
  <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin:20px 0 10px">All clusters — sorted by coverage (worst first)</div>
  <div class="cluster-grid">
    {{range $i,$c := .Clusters}}
    <a class="cluster-card" href="#cluster-{{clusterAnchor $c.Cluster.Name}}">
      <div class="cc-header">
        <div class="cc-name">{{$c.Cluster.Name}}</div>
        <span class="cc-version">{{$c.Kasten.Version}}</span>
      </div>
      <div style="font-size:11px;color:var(--tm);margin-bottom:4px">{{pct $c.Kasten.Compliance.ProtectionCoverage}} coverage</div>
      <div class="cc-bar-bg"><div class="cc-bar-fill" style="width:{{pct $c.Kasten.Compliance.ProtectionCoverage}};background:{{colorPct $c.Kasten.Compliance.ProtectionCoverage}}"></div></div>
      <div class="cc-stats">
        <div class="cc-stat"><div class="cc-stat-val" style="color:var(--blue)">{{$c.Kasten.Applications.Total}}</div><div class="cc-stat-label">Apps</div></div>
        <div class="cc-stat"><div class="cc-stat-val" style="color:var(--green)">{{$c.Kasten.Applications.Protected}}</div><div class="cc-stat-label">Protected</div></div>
        <div class="cc-stat"><div class="cc-stat-val" style="color:{{if gt $c.Kasten.BestPractices.Critical 0}}var(--red){{else}}var(--tm){{end}}">{{$c.Kasten.BestPractices.Critical}}</div><div class="cc-stat-label">Critical</div></div>
      </div>
      <div class="cc-alerts">
        {{if eq $c.Kasten.Security.AuthMethod "None / Passthrough"}}<span class="cc-alert critical">No auth</span>{{end}}
        {{if not $c.Kasten.DR.Enabled}}<span class="cc-alert warning">No DR</span>{{end}}
        {{if not $c.Kasten.Security.Encryption.Enabled}}<span class="cc-alert warning">No encryption</span>{{end}}
        {{if $c.Kasten.DR.Enabled}}<span class="cc-alert ok">DR ✓</span>{{end}}
      </div>
    </a>
    {{end}}
  </div>

  <!-- Cross-cluster comparison table -->
  <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin:24px 0 10px">Cross-cluster comparison</div>
  <div class="twrap tscroll">
    <table>
      <thead><tr>
        <th>Cluster</th><th>K10</th><th>Platform</th><th>Coverage</th>
        <th>Apps</th><th>Policies</th><th>Profiles</th>
        <th>Auth</th><th>DR</th><th>BP Pass</th><th>BP Critical</th>
        <th>Failed (7d)</th>
      </tr></thead>
      <tbody>
        {{range .Clusters}}<tr>
          <td><a href="#cluster-{{clusterAnchor .Cluster.Name}}" style="color:var(--blue)">{{.Cluster.Name}}</a></td>
          <td class="mono muted">{{.Kasten.Version}}</td>
          <td class="muted">{{.Cluster.Platform}}</td>
          <td style="color:{{colorPct .Kasten.Compliance.ProtectionCoverage}};font-family:'IBM Plex Mono',monospace;font-weight:600">{{pct .Kasten.Compliance.ProtectionCoverage}}</td>
          <td class="mono">{{.Kasten.Applications.Total}}</td>
          <td class="mono">{{len .Kasten.Policies}}</td>
          <td class="mono">{{len .Kasten.Profiles}}</td>
          <td>
            {{if eq .Kasten.Security.AuthMethod "None / Passthrough"}}<span class="pill fail">none</span>
            {{else}}<span class="pill ok">{{.Kasten.Security.AuthMethod}}</span>{{end}}
          </td>
          <td>
            {{if .Kasten.DR.Enabled}}<span class="pill ok">enabled</span>
            {{else}}<span class="pill warn">none</span>{{end}}
          </td>
          <td style="color:var(--green);font-family:'IBM Plex Mono',monospace">{{.Kasten.BestPractices.Passed}}/{{.Kasten.BestPractices.TotalChecks}}</td>
          <td style="color:{{if gt .Kasten.BestPractices.Critical 0}}var(--red){{else}}var(--tm){{end}};font-family:'IBM Plex Mono',monospace;font-weight:600">{{.Kasten.BestPractices.Critical}}</td>
          <td style="color:{{if gt .Kasten.Compliance.FailedJobs7d 0}}var(--red){{else}}var(--tm){{end}};font-family:'IBM Plex Mono',monospace">{{.Kasten.Compliance.FailedJobs7d}}</td>
        </tr>{{end}}
      </tbody>
    </table>
  </div>
</div>

<!-- ══ PER-CLUSTER DETAIL ══ -->
{{range $i,$c := .Clusters}}
<div class="cluster-sep"></div>
<div class="cluster-section" id="cluster-{{clusterAnchor $c.Cluster.Name}}">
  <a class="back-top" href="#top">↑ Back to overview</a>
  <div class="cs-hdr">
    <div class="cs-num">{{$i | printf "%02d"}}</div>
    <div>
      <div class="cs-title">{{$c.Cluster.Name}}</div>
      <div style="font-size:11px;color:var(--tm)">{{$c.Cluster.Platform}} {{$c.Cluster.PlatformVersion}} · {{$c.Cluster.KubernetesVersion}} · {{$c.Cluster.NodeCount}} nodes</div>
    </div>
    <div class="cs-meta">K10 {{$c.Kasten.Version}} · {{fmtTime $c.GeneratedAt}}</div>
  </div>

  <!-- Per-cluster KPIs -->
  <div class="kpi-grid" style="grid-template-columns:repeat(auto-fit,minmax(130px,1fr))">
    <div class="kpi"><div class="kpi-label">Coverage</div><div class="kpi-val" style="color:{{colorPct $c.Kasten.Compliance.ProtectionCoverage}}">{{pct $c.Kasten.Compliance.ProtectionCoverage}}</div></div>
    <div class="kpi"><div class="kpi-label">Applications</div><div class="kpi-val" style="color:var(--blue)">{{$c.Kasten.Applications.Total}}</div><div class="kpi-sub">{{$c.Kasten.Applications.Protected}} protected</div></div>
    <div class="kpi"><div class="kpi-label">Policies</div><div class="kpi-val" style="color:var(--purple)">{{len $c.Kasten.Policies}}</div></div>
    <div class="kpi"><div class="kpi-label">Restore Points</div><div class="kpi-val" style="color:var(--green)">{{$c.Kasten.RestorePoints.Total}}</div><div class="kpi-sub">{{$c.Kasten.RestorePoints.Orphaned}} orphaned</div></div>
    <div class="kpi"><div class="kpi-label">BP Passed</div><div class="kpi-val" style="color:var(--green)">{{$c.Kasten.BestPractices.Passed}}</div><div class="kpi-sub">of {{$c.Kasten.BestPractices.TotalChecks}}</div></div>
    <div class="kpi"><div class="kpi-label">Critical</div><div class="kpi-val" style="color:{{if gt $c.Kasten.BestPractices.Critical 0}}var(--red){{else}}var(--green){{end}}">{{$c.Kasten.BestPractices.Critical}}</div></div>
  </div>

  <div class="two" style="margin-top:14px">
    <!-- Policies -->
    <div>
      <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:8px">Policies</div>
      <div class="twrap tscroll">
        <table><thead><tr><th>Name</th><th>Action</th><th>Freq</th><th>Status</th></tr></thead>
        <tbody>{{range $c.Kasten.Policies}}<tr>
          <td class="mono" style="font-size:11px">{{.Name}}</td>
          <td style="color:var(--blue)">{{.Action}}</td>
          <td class="muted" style="font-size:11px">{{.Frequency}}</td>
          <td>{{if .Enabled}}<span class="pill ok">on</span>{{else}}<span class="pill off">paused</span>{{end}}</td>
        </tr>{{end}}</tbody></table>
      </div>
    </div>
    <!-- Best Practices mini -->
    <div>
      <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:8px">Best Practices</div>
      <div class="bp-mini">
        {{range $c.Kasten.BestPractices.Checks}}
        <div class="bp-mini-row">
          <div class="bp-dot" style="background:{{bpColor .Status}}"></div>
          <span style="font-family:'IBM Plex Mono',monospace;font-size:10px;color:var(--tm);min-width:38px">{{.ID}}</span>
          <span style="font-size:11px;flex:1">{{.Name}}</span>
        </div>
        {{end}}
      </div>
    </div>
  </div>

  <!-- Unprotected namespaces -->
  {{if $c.Kasten.Namespaces.Unprotected}}
  <div style="margin-top:14px">
    <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:8px">Unprotected namespaces ({{len $c.Kasten.Namespaces.Unprotected}})</div>
    <div style="display:flex;flex-wrap:wrap;gap:6px">
      {{range $c.Kasten.Namespaces.Unprotected}}
      <span style="font-family:'IBM Plex Mono',monospace;font-size:11px;padding:3px 8px;border-radius:4px;background:rgba(248,81,73,.1);color:var(--red);border:1px solid rgba(248,81,73,.25)">{{.Name}}</span>
      {{end}}
    </div>
  </div>
  {{end}}

  <!-- Security -->
  <div style="margin-top:14px;display:flex;gap:8px;flex-wrap:wrap">
    <span style="font-size:11px;color:var(--tm)">Auth:</span>
    {{if eq $c.Kasten.Security.AuthMethod "None / Passthrough"}}
      <span class="pill fail">None / Open</span>
    {{else}}
      <span class="pill ok">{{$c.Kasten.Security.AuthMethod}}</span>
    {{end}}
    <span style="font-size:11px;color:var(--tm);margin-left:8px">Encryption:</span>
    {{if $c.Kasten.Security.Encryption.Enabled}}
      <span class="pill ok">{{$c.Kasten.Security.Encryption.Provider}}</span>
    {{else}}
      <span class="pill warn">disabled</span>
    {{end}}
    <span style="font-size:11px;color:var(--tm);margin-left:8px">DR:</span>
    {{if $c.Kasten.DR.Enabled}}
      <span class="pill ok">enabled</span>
    {{else}}
      <span class="pill warn">not configured</span>
    {{end}}
  </div>

</div>
{{end}}

<div class="footer">
  <span>Kasten K10 Inspector v{{.ToolVersion}} · kasten-inspector — independent project, not an official Veeam product</span>
  <span>{{fmtTime .GeneratedAt}}</span>
</div>

</div>
<script>
// Highlight active nav link on scroll
var sections = document.querySelectorAll('[id]');
var navLinks = document.querySelectorAll('.nav a');
window.addEventListener('scroll', function(){
  var pos = window.scrollY + 80;
  sections.forEach(function(s){
    if(s.offsetTop <= pos && s.offsetTop + s.offsetHeight > pos){
      navLinks.forEach(function(l){ l.classList.remove('active'); });
      var active = document.querySelector('.nav a[href="#'+s.id+'"]');
      if(active) active.classList.add('active');
    }
  });
});
</script>
</body>
</html>`
