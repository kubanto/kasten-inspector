package report

import (
	"fmt"
	"html/template"
	"math"
	"os"
	"strings"
	"time"

	"github.com/veeam/kasten-inspector/pkg/kasten"
)

// WriteHTML generates a self-contained HTML report.
func WriteHTML(path string, data *Data) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	funcMap := template.FuncMap{
		"fmtTime": func(s string) string {
			if s == "" {
				return "—"
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return s
			}
			return t.Format("02 Jan 2006 15:04 UTC")
		},
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
		"generatedAt": func() string {
			return data.GeneratedAt.Format("02 Jan 2006 15:04:05 UTC")
		},
		"pct": func(f float64) string {
			return fmt.Sprintf("%.1f%%", f)
		},
		"pctInt": func(f float64) int {
			return int(math.Round(f))
		},
		"statusClass": func(s string) string {
			switch strings.ToLower(s) {
			case "complete", "success", "pass", "compliant", "valid":
				return "ok"
			case "failed", "error", "critical", "non-compliant":
				return "fail"
			case "running", "pending":
				return "run"
			case "warning":
				return "warn"
			case "disabled", "paused":
				return "off"
			default:
				return "unk"
			}
		},
		"yesno": func(b bool) string {
			if b {
				return "Yes"
			}
			return "No"
		},
		"join":    strings.Join,
		"orDash":  orDash,
		"formatTimeShort": formatTimeShort,
		"sub":     func(a, b int) int { return a - b },
		"mul100":  func(f float64) int { return int(math.Round(f * 100)) },
		"colorPct": func(f float64) string {
			switch {
			case f >= 90:
				return "#3fb950"
			case f >= 70:
				return "#d29922"
			default:
				return "#f85149"
			}
		},
		"jsStr": func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			s = strings.ReplaceAll(s, "\n", `\n`)
			return `"` + s + `"`
		},
		"helmLimiters": func(vals map[string]string) string {
			// K10 limiter keys and their display labels
			type limiter struct{ key, label string }
			limiters := []limiter{
				{"K10LimiterCsiSnapshotsPerCluster",        "CSI snapshots / cluster"},
				{"K10LimiterVolumeRestoresPerCluster",       "Volume restores / cluster"},
				{"K10LimiterSnapshotExportsPerCluster",      "Snapshot exports / cluster"},
				{"K10LimiterGenericVolumeBackupsPerCluster", "Generic volume backups"},
				{"K10LimiterImageCopiesPerCluster",          "Image copies / cluster"},
				{"K10LimiterVMSnapshotsPerCluster",          "VM snapshots / cluster"},
			}
			pairs := []string{}
			for _, l := range limiters {
				v := "10" // K10 default
				if val, ok := vals[l.key]; ok && val != "" {
					v = val
				}
				pairs = append(pairs, `"`+l.label+`":`+v)
			}
			if len(pairs) == 0 {
				return "{}"
			}
			return "{" + strings.Join(pairs, ",") + "}"
		},
		"storageHuman": func(b int64) string {
			if b == 0 {
				return "—"
			}
			const unit = 1024
			if b < unit {
				return fmt.Sprintf("%d B", b)
			}
			div, exp := int64(unit), 0
			for n := b / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
		},
		"expByAppJSON": func(m map[string]int64) string {
			if len(m) == 0 {
				return "{}"
			}
			pairs := []string{}
			for k, v := range m {
				pairs = append(pairs, `"`+k+`":`+fmt.Sprintf("%d", v))
			}
			return "{" + strings.Join(pairs, ",") + "}"
		},
		"actionsByTypeJSON": func(m map[string]int) string {
			if len(m) == 0 {
				return "{}"
			}
			pairs := []string{}
			for k, v := range m {
				pairs = append(pairs, `"`+k+`":`+fmt.Sprintf("%d", v))
			}
			return "{" + strings.Join(pairs, ",") + "}"
		},
		"rpByAppJSON": func(m map[string]int) string {
			if len(m) == 0 {
				return "{}"
			}
			pairs := []string{}
			for k, v := range m {
				pairs = append(pairs, `"`+k+`":`+fmt.Sprintf("%d", v))
			}
			return "{" + strings.Join(pairs, ",") + "}"
		},
		"noResourceLimits": func(deps []kasten.DeploymentInfo) int {
			count := 0
			for _, dep := range deps {
				for _, c := range dep.Containers {
					if c.CPULimit == "" || c.MemLimit == "" {
						count++
					}
				}
			}
			return count
		},
		"rrsJSON": func(rrs kasten.RecoveryReadinessScore) string {
			return kasten.RRSComponentsJSON(rrs)
		},
		"weeklySLAJSON": func(trend []kasten.WeeklySLA) string {
			return kasten.WeeklySLAJSON(trend)
		},
		"gradeColor": func(grade string) string {
			switch grade {
			case "A": return "#3fb950"
			case "B": return "#58a6ff"
			case "C": return "#ffa657"
			case "D", "F": return "#f85149"
			default: return "#8b949e"
			}
		},
		"riskColor": func(level string) string {
			switch level {
			case "green":  return "#3fb950"
			case "yellow": return "#ffa657"
			case "red":    return "#f85149"
			default:       return "#8b949e"
			}
		},
		"riskIcon": func(level string) string {
			switch level {
			case "green":  return "✅"
			case "yellow": return "⚠️"
			case "red":    return "🔴"
			default:       return "❓"
			}
		},
		"locationProfiles": func(profiles []kasten.Profile) []kasten.Profile {
			var out []kasten.Profile
			for _, p := range profiles {
				if p.Type == "Location" {
					out = append(out, p)
				}
			}
			return out
		},
		"infraProfiles": func(profiles []kasten.Profile) []kasten.Profile {
			var out []kasten.Profile
			for _, p := range profiles {
				if p.Type != "Location" && p.Type != "" {
					out = append(out, p)
				}
			}
			return out
		},
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTmpl)
	if err != nil {
		return fmt.Errorf("parsing HTML template: %w", err)
	}
	return tmpl.Execute(f, data)
}

var htmlTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%2300C853'/%3E%3Ctext x='16' y='22' text-anchor='middle' font-family='monospace' font-weight='700' font-size='13' fill='%23ffffff'%3EK10%3C/text%3E%3C/svg%3E">
<title>Kasten K10 Inspector — {{.Cluster.Name}}</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600&family=IBM+Plex+Sans:wght@300;400;500;600&display=swap');
:root{
  --bg:#0d1117;--s1:#161b22;--s2:#1c2230;--b:#30363d;
  --t:#e6edf3;--tm:#8b949e;
  --blue:#58a6ff;--green:#3fb950;--red:#f85149;
  --yellow:#d29922;--purple:#bc8cff;--orange:#ffa657;
  --kasten:#00C853;--kd:rgba(0,200,83,.12);
}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:'IBM Plex Sans',sans-serif;background:var(--bg);color:var(--t);font-size:14px;line-height:1.6}
.page{max-width:1400px;margin:0 auto;padding:0 24px 80px}
a{color:var(--blue);text-decoration:none}

/* ── Header ── */
.hdr{border-bottom:1px solid var(--b);padding:28px 0 20px;display:flex;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;gap:16px}
.hdr-brand{display:flex;align-items:center;gap:14px}
.logo{width:42px;height:42px;background:var(--kasten);border-radius:10px;display:flex;align-items:center;justify-content:center;font-family:'IBM Plex Mono',monospace;font-weight:600;font-size:17px;color:#fff}
.hdr-title h1{font-size:20px;font-weight:600;letter-spacing:-.2px}
.hdr-title p{color:var(--tm);font-size:12px;margin-top:2px}
.hdr-meta{text-align:right}
.cluster-name{font-family:'IBM Plex Mono',monospace;font-size:13px;color:var(--blue)}
.gen-time{font-size:11px;color:var(--tm);margin-top:3px}
.ver-badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;font-family:'IBM Plex Mono',monospace;margin-top:5px;background:var(--kd);color:var(--kasten);border:1px solid rgba(255,184,0,.3)}

/* ── Tab nav ── */
.tabnav{display:flex;gap:2px;padding:12px 0 0;border-bottom:1px solid var(--b);flex-wrap:wrap}
.tablink{background:none;border:none;padding:8px 16px;font-size:13px;font-weight:500;color:var(--tm);cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px;transition:color .15s,border-color .15s;font-family:'IBM Plex Sans',sans-serif}
.tablink:hover{color:var(--t)}
.tablink.active{color:var(--t);border-bottom-color:var(--kasten)}
/* ── Tab panels ── */
.tabpanel{display:none}.tabpanel.active{display:block}

/* ── Sections ── */
.sec{margin-top:36px;scroll-margin-top:16px}
.sec-hdr{display:flex;align-items:center;gap:8px;margin-bottom:14px}
.sec-icon{width:26px;height:26px;border-radius:6px;display:flex;align-items:center;justify-content:center;font-size:13px;flex-shrink:0}
.sec-hdr h2{font-size:15px;font-weight:600}
.sec-count{font-size:11px;color:var(--tm);margin-left:auto;font-family:'IBM Plex Mono',monospace}
/* ── Tooltip ── */
.tip-wrap{position:relative;display:inline-flex;align-items:center;margin-left:6px}
.tip-btn{width:16px;height:16px;border-radius:50%;background:var(--s2);border:1px solid var(--b);color:var(--tm);font-size:10px;font-weight:700;cursor:help;display:inline-flex;align-items:center;justify-content:center;flex-shrink:0;line-height:1}
.tip-btn:hover{background:var(--b);color:var(--t)}
.tip-box{display:none;position:absolute;left:22px;top:50%;transform:translateY(-50%);background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:10px 13px;width:280px;font-size:11px;line-height:1.6;color:var(--t);z-index:100;box-shadow:0 4px 16px rgba(0,0,0,.4)}
.tip-wrap:hover .tip-box{display:block}
.tip-box strong{color:var(--kasten);font-weight:600}

/* ── Score Grid ── */
.sgrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px}
.scard{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px;position:relative;overflow:hidden}
.scard::before{content:'';position:absolute;top:0;left:0;right:0;height:3px}
.scard.blue::before{background:var(--blue)}
.scard.green::before{background:var(--green)}
.scard.red::before{background:var(--red)}
.scard.yellow::before{background:var(--yellow)}
.scard.purple::before{background:var(--purple)}
.scard.orange::before{background:var(--orange)}
.scard.kasten::before{background:var(--kasten)}
.slabel{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.8px;color:var(--tm);margin-bottom:6px}
.sval{font-size:28px;font-weight:700;font-family:'IBM Plex Mono',monospace;line-height:1}
.scard.blue .sval{color:var(--blue)}
.scard.green .sval{color:var(--green)}
.scard.red .sval{color:var(--red)}
.scard.yellow .sval{color:var(--yellow)}
.scard.purple .sval{color:var(--purple)}
.scard.orange .sval{color:var(--orange)}
.scard.kasten .sval{color:var(--kasten)}
.ssub{font-size:11px;color:var(--tm);margin-top:4px}
.pbar{height:3px;background:var(--b);border-radius:2px;overflow:hidden;margin-top:8px}
.pfill{height:100%;border-radius:2px}

/* ── Tables ── */
.twrap{background:var(--s1);border:1px solid var(--b);border-radius:10px;overflow:hidden}
.tscroll{overflow-x:auto}
table{width:100%;border-collapse:collapse}
thead th{background:var(--s2);padding:8px 12px;text-align:left;font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);border-bottom:1px solid var(--b);white-space:nowrap}
tbody tr{border-bottom:1px solid var(--b);transition:background .12s}
tbody tr:last-child{border-bottom:none}
tbody tr:hover{background:var(--s2)}
td{padding:8px 12px;font-size:13px;vertical-align:middle}
.mono{font-family:'IBM Plex Mono',monospace;font-size:12px}
.muted{color:var(--tm)}

/* ── Status Pills ── */
.pill{display:inline-flex;align-items:center;gap:4px;padding:2px 7px;border-radius:20px;font-size:10px;font-weight:600}
.pill::before{content:'';width:5px;height:5px;border-radius:50%;display:inline-block}
.ok{background:rgba(63,185,80,.12);color:var(--green);border:1px solid rgba(63,185,80,.3)}.ok::before{background:var(--green)}
.fail{background:rgba(248,81,73,.12);color:var(--red);border:1px solid rgba(248,81,73,.3)}.fail::before{background:var(--red)}
.warn{background:rgba(210,153,34,.12);color:var(--yellow);border:1px solid rgba(210,153,34,.3)}.warn::before{background:var(--yellow)}
.run{background:rgba(88,166,255,.12);color:var(--blue);border:1px solid rgba(88,166,255,.3)}.run::before{background:var(--blue)}
.off{background:rgba(139,148,158,.1);color:var(--tm);border:1px solid rgba(139,148,158,.2)}.off::before{background:var(--tm)}
.unk{background:rgba(210,153,34,.08);color:var(--yellow);border:1px solid rgba(210,153,34,.2)}.unk::before{background:var(--yellow)}

/* ── Best-practice checks ── */
.bpgrid{display:grid;grid-template-columns:1fr 1fr;gap:8px}
@media(max-width:900px){.bpgrid{grid-template-columns:1fr}}
.bpcard{background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:12px 14px;display:flex;align-items:flex-start;gap:10px}
.bpcard.ok{border-left:3px solid var(--green)}
.bpcard.warn{border-left:3px solid var(--yellow)}
.bpcard.fail,.bpcard.critical{border-left:3px solid var(--red)}
.bpid{font-family:'IBM Plex Mono',monospace;font-size:10px;color:var(--tm);min-width:40px}
.bpname{font-size:12px;font-weight:600;margin-bottom:2px}
.bpdetail{font-size:11px;color:var(--tm)}

/* ── Compliance ring ── */
.cring-wrap{display:grid;grid-template-columns:220px 1fr;gap:14px;align-items:start}
@media(max-width:700px){.cring-wrap{grid-template-columns:1fr}}
.cring-card{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:20px;display:flex;flex-direction:column;align-items:center}
.ring-lbl{position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);text-align:center}
.ring-pct{font-size:24px;font-weight:700;font-family:'IBM Plex Mono',monospace}
.ring-sub{font-size:10px;color:var(--tm)}
.ring-title{margin-top:10px;font-size:12px;font-weight:600}
.cstats{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px}
.stat-row{display:flex;justify-content:space-between;align-items:center;padding:8px 0;border-bottom:1px solid var(--b)}
.stat-row:last-child{border-bottom:none}
.stat-label{color:var(--tm);font-size:12px}
.stat-val{font-family:'IBM Plex Mono',monospace;font-size:12px;font-weight:600}

/* ── Two-col grid ── */
.two{display:grid;grid-template-columns:1fr 1fr;gap:14px}
@media(max-width:700px){.two{grid-template-columns:1fr}}

/* ── Info cards ── */
.igrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:10px}
.icard{background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:14px}
.icard-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:4px}
.icard-val{font-family:'IBM Plex Mono',monospace;font-size:13px;word-break:break-all}

/* ── Retention tags ── */
.ret{display:flex;flex-wrap:wrap;gap:3px}
.ret span{background:var(--s2);border:1px solid var(--b);border-radius:3px;padding:1px 5px;font-size:10px;font-family:'IBM Plex Mono',monospace}

/* ── Node grid ── */
.ngrid{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:8px;margin-top:12px}
.ncard{background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:12px;display:flex;align-items:flex-start;gap:8px}
.nicon{width:28px;height:28px;border-radius:6px;background:var(--s2);display:flex;align-items:center;justify-content:center;font-size:13px;flex-shrink:0}
.nname{font-family:'IBM Plex Mono',monospace;font-size:11px;font-weight:600}
.nrole{font-size:10px;color:var(--tm);margin-top:1px}
.ndetail{font-size:10px;color:var(--tm);margin-top:3px;line-height:1.5}

/* ── Empty state ── */
.empty{padding:28px;text-align:center;color:var(--tm);font-size:12px}

/* ── Alert box ── */
.alert{padding:10px 14px;border-radius:6px;font-size:12px;margin-bottom:14px;display:flex;align-items:center;gap:8px}
.alert.warn{background:rgba(210,153,34,.1);border:1px solid rgba(210,153,34,.3);color:var(--yellow)}
.alert.ok{background:rgba(63,185,80,.1);border:1px solid rgba(63,185,80,.3);color:var(--green)}

/* ── Footer ── */
.footer{margin-top:56px;padding-top:20px;border-top:1px solid var(--b);display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:10px}
.footer-l{font-size:11px;color:var(--tm)}
.footer-r{font-family:'IBM Plex Mono',monospace;font-size:10px;color:var(--tm)}
/* ── Filter buttons ── */
.job-filter-btn{background:var(--s2);border:1px solid var(--b);color:var(--tm);padding:3px 10px;border-radius:4px;font-size:11px;cursor:pointer;font-family:'IBM Plex Sans',sans-serif}
.job-filter-btn.active,.job-filter-btn:hover{background:var(--b);color:var(--t)}
.job-filter-btn.active{border-color:var(--kasten);color:var(--kasten)}
.job-pdf-btn{background:var(--kasten);border:1px solid var(--kasten);color:#0b1e12;font-weight:600;padding:3px 12px;border-radius:4px;font-size:11px;cursor:pointer;font-family:'IBM Plex Sans',sans-serif}
.job-pdf-btn:hover{filter:brightness(1.08)}
.print-only{display:none}
@media print{
  .no-print{display:none!important}
  .tabpanel{display:none!important}
  .tabpanel.active{display:block!important}
  .tscroll{max-height:none!important;overflow:visible!important}
  .print-only{display:block!important}
  .sec{break-inside:avoid}
  .page{max-width:none;padding:0}
  html,body{-webkit-print-color-adjust:exact;print-color-adjust:exact}
  #print-filter-summary{margin:0 0 12px;font-size:12px;color:var(--tm)}
}
</style>
<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.js"></script>
</head>
<body>
<div class="page">

<!-- Header -->
<div class="hdr">
  <div class="hdr-brand">
    <div class="logo">K10</div>
    <div class="hdr-title">
      <h1>Kasten K10 Inspector</h1>
      <p>Cluster health &amp; data protection report</p>
    </div>
  </div>
  <div class="hdr-meta">
    <div class="cluster-name">{{.Cluster.Name}}</div>
    <div class="gen-time">Generated {{generatedAt}}</div>
    <span class="ver-badge">v{{.ToolVersion}}</span>
  </div>
</div>

<!-- Tab Nav -->
<nav class="tabnav" id="tabnav">
  <button class="tablink active" data-tab="tab-overview">Overview</button>
  <button class="tablink" data-tab="tab-health">Health Check</button>
  <button class="tablink" data-tab="tab-protection">Protection</button>
  <button class="tablink" data-tab="tab-recovery">Recovery</button>
  <button class="tablink" data-tab="tab-operations">Operations</button>
  <button class="tablink" data-tab="tab-storage">Storage</button>
  <button class="tablink" data-tab="tab-config">Configuration</button>
  <button class="tablink" data-tab="tab-statistics">Statistics &amp; QBR</button>
  <button class="tablink" data-tab="tab-diagnostics">Diagnostics</button>
</nav>

<div class="tabpanel active" id="tab-overview">



<div class="sec" id="summary">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(255,184,0,.1)">📊</div>
    <h2>Executive Summary</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">High-level snapshot of the cluster and K10 installation. <strong>Applications</strong>: namespaces with workloads found by K10. <strong>Protected</strong>: have at least one active backup policy. <strong>Restore Points</strong>: saved backup copies available for recovery.</span></span>
  </div>
  <div class="sgrid">
    <div class="scard blue"><div class="slabel">Applications</div><div class="sval">{{.Kasten.Applications.Total}}</div><div class="ssub">across all namespaces</div></div>

    <div class="scard kasten"><div class="slabel">Kasten Version</div><div class="sval" style="font-size:12px;word-break:break-all;line-height:1.3">{{.Kasten.Version}}</div><div class="ssub">{{.Cluster.Platform}} {{.Cluster.PlatformVersion}}</div></div>
    <div class="scard blue"><div class="slabel">Multi-cluster Mode</div><div class="sval" style="font-size:18px">{{.Kasten.MultiCluster.Mode}}</div><div class="ssub">{{len .Kasten.MultiCluster.Clusters}} remote cluster(s)</div></div>
  </div>
</div>

<!-- ══ BEST PRACTICES SUMMARY ══ -->
<div class="sec" id="bp-summary-overview">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(63,185,80,.1)">✅</div>
    <h2>Best Practices</h2>
    <span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Quick status of automated best practice checks. See <strong>Statistics &amp; QBR</strong> for the full breakdown with remediation guidance.</span></span>
    <span class="sec-count">{{.Kasten.BestPractices.TotalChecks}} checks · {{.Kasten.BestPractices.Passed}} pass · {{.Kasten.BestPractices.Warnings}} warn · {{.Kasten.BestPractices.Critical}} critical</span>
  </div>
  <div style="display:flex;gap:12px;flex-wrap:wrap">
    {{range .Kasten.BestPractices.Checks}}{{if ne .Status "pass"}}
    <div style="flex:1;min-width:280px;padding:10px 14px;border-radius:8px;border-left:3px solid {{if eq .Status "critical"}}var(--red){{else}}var(--yellow){{end}};background:var(--s2)">
      <div style="font-size:11px;font-weight:600;color:{{if eq .Status "critical"}}var(--red){{else}}var(--yellow){{end}}">{{.ID}} · {{if eq .Status "critical"}}❌ critical{{else}}⚠️ warning{{end}}</div>
      <div style="font-size:12px;font-weight:600;margin:4px 0 2px">{{.Name}}</div>
      <div style="font-size:11px;color:var(--tm)">{{.Detail}}</div>
    </div>
    {{end}}{{end}}
    {{if eq .Kasten.BestPractices.Critical 0}}{{if eq .Kasten.BestPractices.Warnings 0}}
    <div style="padding:16px;text-align:center;color:var(--green);font-size:14px;width:100%">✅ All {{.Kasten.BestPractices.TotalChecks}} checks passed — environment is in good health.</div>
    {{end}}{{end}}
  </div>
  <div style="margin-top:12px;font-size:11px;color:var(--tm)">→ Full details with remediation guidance in <strong>Statistics &amp; QBR</strong> tab.</div>
</div>

<!-- ══ COMPLIANCE ══ -->
<div class="sec" id="compliance">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(63,185,80,.1)">🛡️</div><h2>Compliance &amp; SLA Overview</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Measures how well the backup environment meets protection goals. <strong>Coverage</strong>: % of namespaces with a policy. <strong>Success Rate</strong>: % of jobs completed in the last 7 days. <strong>Orphaned RPs</strong>: restore points with no matching app &mdash; wasting storage.</span></span></div>
  <div class="cring-wrap">
    <div class="cring-card">
      <div style="position:relative;width:130px;height:130px">
        <svg width="130" height="130" viewBox="0 0 130 130">
          <circle cx="65" cy="65" r="52" fill="none" stroke="var(--b)" stroke-width="9"/>
          <circle cx="65" cy="65" r="52" fill="none" stroke="{{colorPct .Kasten.Compliance.ProtectionCoverage}}"
            stroke-width="9" stroke-linecap="round"
            style="stroke-dasharray:calc({{pctInt .Kasten.Compliance.ProtectionCoverage}} * 3.267) 326.7;stroke-dashoffset:0"
            transform="rotate(-90 65 65)"/>
        </svg>
        <div class="ring-lbl">
          <div class="ring-pct" style="color:{{colorPct .Kasten.Compliance.ProtectionCoverage}}">{{pct .Kasten.Compliance.ProtectionCoverage}}</div>
          <div class="ring-sub">coverage</div>
        </div>
      </div>
      <div class="ring-title">Protection Coverage</div>
    </div>
    <div class="cstats">
      <div class="stat-row"><span class="stat-label">Policy Compliance Rate</span><span class="stat-val" style="color:{{colorPct .Kasten.Compliance.PolicyCompliance}}">{{pct .Kasten.Compliance.PolicyCompliance}}</span></div>
      <div class="stat-row"><span class="stat-label">Job Success Rate (7d)</span><span class="stat-val" style="color:{{colorPct .Kasten.Compliance.SuccessRate7d}}">{{pct .Kasten.Compliance.SuccessRate7d}}</span></div>
      <div class="stat-row"><span class="stat-label">Failed Jobs (24h)</span><span class="stat-val" style="color:{{if gt .Kasten.Compliance.FailedJobs24h 0}}var(--red){{else}}var(--green){{end}}">{{.Kasten.Compliance.FailedJobs24h}}</span></div>
      <div class="stat-row"><span class="stat-label">Failed Jobs (7d)</span><span class="stat-val" style="color:{{if gt .Kasten.Compliance.FailedJobs7d 0}}var(--red){{else}}var(--green){{end}}">{{.Kasten.Compliance.FailedJobs7d}}</span></div>
      <div class="stat-row"><span class="stat-label">Restore Points / Orphaned</span><span class="stat-val">{{.Kasten.RestorePoints.Total}} / <span style="color:{{if gt .Kasten.RestorePoints.Orphaned 0}}var(--yellow){{else}}var(--green){{end}}">{{.Kasten.RestorePoints.Orphaned}}</span></span></div>
      {{if .Kasten.RestorePoints.Oldest}}<div class="stat-row"><span class="stat-label">Oldest Restore Point</span><span class="stat-val mono" style="font-size:11px">{{fmtDate .Kasten.RestorePoints.Oldest}}</span></div>{{end}}
      {{if .Kasten.RestorePoints.Newest}}<div class="stat-row"><span class="stat-label">Newest Restore Point</span><span class="stat-val mono" style="font-size:11px">{{fmtDate .Kasten.RestorePoints.Newest}}</span></div>{{end}}
      {{if .Kasten.DR.Enabled}}
      <div class="stat-row"><span class="stat-label">DR Last Run</span><span class="stat-val mono" style="font-size:11px">{{fmtDate .Kasten.DR.LastRunTime}} <span class="pill {{statusClass .Kasten.DR.LastRunStatus}}">{{.Kasten.DR.LastRunStatus}}</span></span></div>
      {{end}}
      {{if .Kasten.JobSummary.SuccessByAction}}
      <div class="stat-row" style="border-top:1px solid var(--b);margin-top:6px;padding-top:10px"><span class="stat-label" style="font-weight:600">Success Rate by Action</span><span class="stat-val muted" style="font-size:10px">completed / total</span></div>
      {{range .Kasten.JobSummary.SuccessByAction}}
      <div class="stat-row"><span class="stat-label" style="text-transform:capitalize">{{.Action}}</span><span class="stat-val">{{if ge .SuccessRate 0.0}}<span style="color:{{colorPct .SuccessRate}}">{{pct .SuccessRate}}</span> <span class="muted" style="font-size:11px">({{.Completed}} / {{.Total}})</span>{{else}}<span class="muted">n/a</span>{{end}}</span></div>
      {{end}}
      {{end}}
    </div>
  </div>
</div>


</div><!-- /tab-overview -->

<div class="tabpanel" id="tab-health">

<!-- ══ CLUSTER ══ -->
<div class="sec" id="cluster">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(88,166,255,.1)">🖥️</div><h2>Cluster</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Kubernetes infrastructure details. <strong>Platform</strong>: detected distribution (GKE, EKS, AKS, OpenShift, k3s). <strong>Dashboard Access</strong>: how the K10 UI is exposed (NodePort, LoadBalancer, Ingress).</span></span><span class="sec-count">{{.Cluster.Platform}} {{.Cluster.PlatformVersion}}</span></div>
  <div class="igrid">
    <div class="icard"><div class="icard-label">Kubernetes Version</div><div class="icard-val">{{.Cluster.KubernetesVersion}}</div></div>
    <div class="icard"><div class="icard-label">Platform</div><div class="icard-val">{{.Cluster.Platform}} {{.Cluster.PlatformVersion}}</div></div>
    <div class="icard"><div class="icard-label">Nodes</div><div class="icard-val">{{.Cluster.NodeCount}} ({{.Cluster.ControlPlaneNodes}} CP / {{.Cluster.WorkerNodes}} workers)</div></div>
    <div class="icard"><div class="icard-label">Namespaces</div><div class="icard-val">{{.Cluster.NamespaceCount}}</div></div>
    <div class="icard"><div class="icard-label">Storage Classes</div><div class="icard-val">{{len .Cluster.StorageClasses}}</div></div>
    <div class="icard"><div class="icard-label">Dashboard Access</div><div class="icard-val">{{.Kasten.HelmConfig.DashboardAccess}}</div></div>
    <div class="icard"><div class="icard-label">Concurrency Limit</div><div class="icard-val">{{if .Kasten.HelmConfig.ConcurrencyLimit}}{{.Kasten.HelmConfig.ConcurrencyLimit}}{{else}}—{{end}}</div></div>
    <div class="icard"><div class="icard-label">Backup Timeout</div><div class="icard-val">{{orDash .Kasten.HelmConfig.BackupTimeout}}</div></div>
    <div class="icard"><div class="icard-label">Datastore Parallelism</div><div class="icard-val">{{if .Kasten.HelmConfig.DatastoreParallelism}}{{.Kasten.HelmConfig.DatastoreParallelism}}{{else}}—{{end}}</div></div>
  </div>
  <div class="ngrid">
    {{range .Cluster.Nodes}}
    <div class="ncard">
      <div class="nicon">{{if eq .Role "control-plane"}}🎮{{else}}⚙️{{end}}</div>
      <div>
        <div class="nname">{{.Name}}</div>
        <div class="nrole">{{.Role}} · {{if .Ready}}<span style="color:var(--green)">Ready</span>{{else}}<span style="color:var(--red)">Not Ready</span>{{end}}</div>
        <div class="ndetail">{{.OSImage}}<br>kubelet {{.KubeletVersion}} · {{.Architecture}}</div>
      </div>
    </div>
    {{end}}
  </div>
  {{if .Cluster.StorageClasses}}
  <div class="twrap tscroll" style="margin-top:14px">
    <table><thead><tr><th>Storage Class</th><th>Provisioner</th><th>Default</th></tr></thead>
    <tbody>{{range .Cluster.StorageClasses}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="mono muted">{{.Provisioner}}</td>
      <td>{{if .Default}}<span class="pill ok">default</span>{{else}}—{{end}}</td>
    </tr>{{end}}</tbody></table>
  </div>{{end}}
</div>

<!-- ══ LICENSE ══ -->
<div class="sec" id="license">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(255,184,0,.1)">🔑</div><h2>License</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">K10 license status. Expired or invalid licenses stop new backup jobs from running. Contact Veeam support if shown as invalid.</span></span></div>
  <div class="twrap">
    <table><tbody>
      <tr><td>Company</td><td>{{orDash .Kasten.License.Company}}</td></tr>
      {{if .Kasten.License.NodeUsage}}<tr><td>Node Usage</td><td class="mono">{{.Kasten.License.NodeUsage}} / {{.Kasten.License.NodeLimit}}</td></tr>{{end}}
      <tr><td>Product</td><td>{{orDash .Kasten.License.ProductName}}</td></tr>
      <tr><td>Type</td><td>{{orDash .Kasten.License.LicenseType}}</td></tr>
      <tr><td>Expires</td><td class="mono muted">{{orDash .Kasten.License.ExpiresAt}}</td></tr>
      <tr><td>Valid</td><td>{{if .Kasten.License.Valid}}<span class="pill ok">valid</span>{{else}}<span class="pill warn">unknown</span>{{end}}</td></tr>
      {{if .Kasten.License.NodeLimit}}<tr><td>Node Limit</td><td class="mono">{{.Kasten.License.NodeLimit}}</td></tr>{{end}}
    </tbody></table>
  </div>
</div>

<!-- ══ SECURITY ══ -->
<div class="sec" id="security">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(248,81,73,.1)">🔐</div><h2>Security</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box"><strong>Authentication</strong>: how users log into K10. None/Passthrough means the dashboard is open to anyone on the network &mdash; critical risk. <strong>Encryption</strong>: whether backup data is encrypted at rest using an external KMS.</span></span></div>
  <div class="two">
    <div class="twrap">
      <table><thead><tr><th>Auth Setting</th><th>Value</th></tr></thead>
      <tbody>
        <tr><td>Authentication Method</td><td><strong>{{.Kasten.Security.AuthMethod}}</strong></td></tr>
        {{if .Kasten.Security.OIDCConfig}}
        <tr><td>OIDC Provider URL</td><td class="mono muted">{{.Kasten.Security.OIDCConfig.ProviderURL}}</td></tr>
        <tr><td>OIDC Client ID</td><td class="mono muted">{{.Kasten.Security.OIDCConfig.ClientID}}</td></tr>
        <tr><td>Username Claim</td><td class="mono muted">{{orDash .Kasten.Security.OIDCConfig.UsernameClaim}}</td></tr>
        <tr><td>Groups Claim</td><td class="mono muted">{{orDash .Kasten.Security.OIDCConfig.GroupsClaim}}</td></tr>
        {{end}}
        {{if .Kasten.Security.LDAPConfig}}
        <tr><td>LDAP Host</td><td class="mono muted">{{.Kasten.Security.LDAPConfig.Host}}</td></tr>
        <tr><td>LDAP BindDN</td><td class="mono muted">{{.Kasten.Security.LDAPConfig.BindDN}}</td></tr>
        {{end}}
      </tbody></table>
    </div>
    <div class="twrap">
      <table><thead><tr><th>Encryption Setting</th><th>Value</th></tr></thead>
      <tbody>
        <tr><td>Encryption Enabled</td><td>{{if .Kasten.Security.Encryption.Enabled}}<span class="pill ok">enabled</span>{{else}}<span class="pill warn">disabled</span>{{end}}</td></tr>
        <tr><td>Provider</td><td>{{orDash .Kasten.Security.Encryption.Provider}}</td></tr>
        <tr><td>Key ID / Path</td><td class="mono muted">{{orDash .Kasten.Security.Encryption.KeyID}}</td></tr>
        <tr><td>Vault URL</td><td class="mono muted">{{orDash .Kasten.Security.Encryption.VaultURL}}</td></tr>
        <tr><td>FIPS Mode</td><td>{{if .Kasten.HelmConfig.FIPSMode}}<span class="pill ok">enabled</span>{{else}}—{{end}}</td></tr>
        <tr><td>Audit Logging</td><td>{{if .Kasten.HelmConfig.AuditLogging}}<span class="pill ok">enabled</span>{{else}}—{{end}}</td></tr>
        <tr><td>Network Policies</td><td>{{if .Kasten.HelmConfig.NetworkPolicies}}<span class="pill ok">enabled</span>{{else}}—{{end}}</td></tr>
      </tbody></table>
    </div>
  </div>
</div>

<!-- ══ RESOURCES ══ -->
<div class="sec" id="resources">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(88,166,255,.1)">📈</div><h2>K10 Resource Limits</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">CPU and memory settings for each K10 microservice container. <strong>Yellow dashes</strong>: no limit set &mdash; the container can consume unbounded resources, potentially causing OOM kills or starving other workloads.</span></span></div>
  <div class="twrap tscroll">
    <table><thead><tr><th>Deployment</th><th>Replicas</th><th>Container</th><th>CPU Req</th><th>CPU Lim</th><th>Mem Req</th><th>Mem Lim</th></tr></thead>
    <tbody>{{range .Kasten.Resources.Deployments}}{{$dep := .}}{{range $i,$cont := .Containers}}<tr>
      <td class="mono" style="font-size:11px">{{if eq $i 0}}{{$dep.Name}}{{end}}</td>
      <td class="mono muted">{{if eq $i 0}}<span style="color:{{if lt $dep.Ready $dep.Replicas}}var(--yellow){{else}}var(--green){{end}}">{{$dep.Ready}}/{{$dep.Replicas}}</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{$cont.Name}}</td>
      <td class="mono muted">{{orDash $cont.CPURequest}}</td>
      <td class="mono muted">{{if $cont.CPULimit}}{{$cont.CPULimit}}{{else}}<span style="color:var(--yellow)">—</span>{{end}}</td>
      <td class="mono muted">{{orDash $cont.MemRequest}}</td>
      <td class="mono muted">{{if $cont.MemLimit}}{{$cont.MemLimit}}{{else}}<span style="color:var(--yellow)">—</span>{{end}}</td>
    </tr>{{end}}{{end}}</tbody></table>
  </div>
  <div class="igrid" style="margin-top:12px">
    <div class="icard"><div class="icard-label">Prometheus</div><div class="icard-val">{{if .Kasten.Prometheus.Enabled}}<span style="color:var(--green)">Enabled</span>{{else}}<span style="color:var(--tm)">Not detected</span>{{end}}</div></div>
    <div class="icard"><div class="icard-label">ServiceMonitor</div><div class="icard-val">{{yesno .Kasten.Prometheus.ServiceMonitor}}</div></div>
    <div class="icard"><div class="icard-label">Grafana Dashboard</div><div class="icard-val">{{yesno .Kasten.Prometheus.GrafanaDashboard}}</div></div>
    {{if .Kasten.Prometheus.Endpoint}}<div class="icard"><div class="icard-label">Endpoint</div><div class="icard-val mono" style="font-size:11px">{{.Kasten.Prometheus.Endpoint}}</div></div>{{end}}
  </div>
</div>

</div><!-- /tab-health -->

<div class="tabpanel" id="tab-protection">

<!-- ══ PROTECTION KPI BANNER ══ -->
<div class="sgrid" style="margin-bottom:20px">
  <div class="scard green"><div class="slabel">Protected</div><div class="sval">{{.Kasten.Applications.Protected}}</div><div class="ssub">have an active policy</div><div class="pbar"><div class="pfill" style="width:{{pctInt .Kasten.Compliance.ProtectionCoverage}}%;background:{{colorPct .Kasten.Compliance.ProtectionCoverage}}"></div></div></div>
  <div class="scard red"><div class="slabel">Unprotected</div><div class="sval">{{.Kasten.Applications.Unprotected}}</div><div class="ssub">no backup policy</div></div>
  <div class="scard yellow"><div class="slabel">Policies</div><div class="sval">{{len .Kasten.Policies}}</div><div class="ssub">configured</div></div>
  <div class="scard purple"><div class="slabel">Restore Points</div><div class="sval">{{.Kasten.RestorePoints.Total}}</div><div class="ssub">{{.Kasten.RestorePoints.Orphaned}} orphaned</div></div>
  <div class="scard orange"><div class="slabel">Jobs (collected)</div><div class="sval">{{len .Kasten.Jobs}}</div><div class="ssub">{{.Kasten.Compliance.FailedJobs7d}} failed in 7d</div></div>
</div>


<!-- ══ POLICIES ══ -->
<div class="sec" id="policies">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(188,140,255,.1)">📋</div><h2>Policies</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Backup rules defining what to protect, how often, and where to store it. <strong>Frequency</strong>: schedule interval. <strong>Retention</strong>: how many restore points to keep. <strong>Export</strong>: secondary copy for 3-2-1 compliance.</span></span><span class="sec-count">{{len .Kasten.Policies}} total · {{len .Kasten.PolicyPresets}} presets</span></div>
  {{if .Kasten.Policies}}
  <div class="twrap tscroll">
    <table><thead><tr><th>Name</th><th>Status</th><th>Action</th><th>Frequency</th><th>Retention</th><th>Export Profile</th><th>Selector</th><th>Last Run</th><th>Last Status</th><th>Avg Duration</th></tr></thead>
    <tbody>{{range .Kasten.Policies}}<tr>
      <td class="mono" style="font-size:11px">{{.Name}}{{if .IsSystemPolicy}} <span class="pill off" style="font-size:9px">system</span>{{end}}</td>
      <td>{{if .Enabled}}<span class="pill ok">on</span>{{else}}<span class="pill off">paused</span>{{end}}</td>
      <td style="color:var(--blue)">{{.Action}}</td>
      <td class="mono muted" style="font-size:11px">{{.Frequency}}</td>
      <td><div class="ret">{{if .Retention.Hourly}}<span>{{.Retention.Hourly}}h</span>{{end}}{{if .Retention.Daily}}<span>{{.Retention.Daily}}d</span>{{end}}{{if .Retention.Weekly}}<span>{{.Retention.Weekly}}w</span>{{end}}{{if .Retention.Monthly}}<span>{{.Retention.Monthly}}m</span>{{end}}{{if .Retention.Yearly}}<span>{{.Retention.Yearly}}y</span>{{end}}</div></td>
      <td class="mono muted" style="font-size:11px">{{if .ExportProfiles}}{{join .ExportProfiles ", "}}{{else}}—{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .Selector}}</td>
      <td class="mono muted" style="font-size:11px">{{fmtDate .LastRunTime}}</td>
      <td>{{if .LastRunStatus}}<span class="pill {{statusClass .LastRunStatus}}">{{.LastRunStatus}}</span>{{else}}—{{end}}</td>
      <td class="mono muted">{{orDash .AvgRunDuration}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{else}}<div class="twrap"><div class="empty">No policies found</div></div>{{end}}
  {{if .Kasten.PolicyPresets}}
  <div style="margin-top:12px">
    <div style="font-size:12px;font-weight:600;color:var(--tm);margin-bottom:8px;text-transform:uppercase;letter-spacing:.5px">Policy Presets ({{len .Kasten.PolicyPresets}})</div>
    <div class="twrap tscroll">
      <table><thead><tr><th>Name</th><th>Action</th><th>Frequency</th><th>Retention</th></tr></thead>
      <tbody>{{range .Kasten.PolicyPresets}}<tr>
        <td class="mono">{{.Name}}</td>
        <td style="color:var(--blue)">{{.Action}}</td>
        <td class="mono muted">{{.Frequency}}</td>
        <td><div class="ret">{{if .Retention.Daily}}<span>{{.Retention.Daily}}d</span>{{end}}{{if .Retention.Weekly}}<span>{{.Retention.Weekly}}w</span>{{end}}{{if .Retention.Monthly}}<span>{{.Retention.Monthly}}m</span>{{end}}</div></td>
      </tr>{{end}}</tbody></table>
    </div>
  </div>{{end}}
</div>

<!-- ══ APPLICATIONS ══ -->
<div class="sec" id="applications">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(255,166,87,.1)">📦</div><h2>Applications</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Namespaces with workloads discovered by K10. Inspector shows <strong>user namespaces only</strong> — system namespaces (openshift-*, kube-*, kasten-io, longhorn-system, etc.) are excluded, which is why the count differs from the Kasten UI. <strong>Protected</strong>: at least one policy targets this namespace. <strong>Unprotected</strong>: no policy covers it &mdash; data loss risk.</span></span><span class="sec-count">{{.Kasten.Applications.Protected}} protected · {{.Kasten.Applications.Unprotected}} unprotected</span></div>
  {{if .Kasten.Applications.Apps}}
  <div class="twrap tscroll">
    <table><thead><tr><th>Application</th><th>Namespace</th><th>Protection</th><th>Policies</th><th>Last Backup</th></tr></thead>
    <tbody>{{range .Kasten.Applications.Apps}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="mono muted">{{.Namespace}}</td>
      <td>{{if .Protected}}<span class="pill ok">protected</span>{{else}}<span class="pill fail">unprotected</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{if .PolicyNames}}{{join .PolicyNames ", "}}{{else}}—{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{fmtDate .LastBackup}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{else}}<div class="twrap"><div class="empty">No applications discovered</div></div>{{end}}
  {{if .Kasten.Namespaces.Unprotected}}
  <div style="margin-top:12px">
    <div class="alert warn">⚠️ {{len .Kasten.Namespaces.Unprotected}} non-system namespace(s) have no backup coverage</div>
    <div class="twrap tscroll">
      <table><thead><tr><th>Unprotected Namespace</th><th>Labels</th></tr></thead>
      <tbody>{{range .Kasten.Namespaces.Unprotected}}<tr>
        <td class="mono">{{.Name}}</td>
        <td class="mono muted" style="font-size:11px">{{range $k,$v := .Labels}}{{$k}}={{$v}} {{end}}</td>
      </tr>{{end}}</tbody></table>
    </div>
  </div>{{end}}
</div>

<!-- ══ PROFILES ══ -->
<div class="sec" id="profiles">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(255,184,0,.1)">🗄️</div><h2>Location &amp; Infrastructure Profiles</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Storage destinations where K10 sends backup data. <strong>Immutability</strong>: object lock prevents deletion &mdash; critical for ransomware protection. <strong>Ready</strong>: K10 has validated credentials and connectivity.</span></span><span class="sec-count">{{len .Kasten.Profiles}} configured</span></div>
  {{if .Kasten.Profiles}}
  <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin:8px 0 8px">📁 Location Profiles</div>
  {{if locationProfiles .Kasten.Profiles}}
  <div class="twrap tscroll">
    <table><thead><tr><th>Name</th><th>Provider</th><th>Bucket / Endpoint</th><th>Region</th><th>Immutability</th><th>Status</th></tr></thead>
    <tbody>
    {{range (locationProfiles .Kasten.Profiles)}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="muted">{{orDash .Provider}}</td>
      <td class="mono muted" style="font-size:11px">{{if .Bucket}}{{.Bucket}}{{else if .Endpoint}}{{.Endpoint}}{{else}}—{{end}}</td>
      <td class="mono muted">{{orDash .Region}}</td>
      <td>{{if .Immutability}}<span class="pill ok">{{.ImmutabilityPeriod}}</span>{{else}}—{{end}}</td>
      <td>{{if .Ready}}<span class="pill ok">ready</span>{{else}}<span class="pill fail">not ready</span>{{end}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{else}}<div class="empty" style="margin-bottom:12px">No location profiles configured</div>{{end}}
  {{if infraProfiles .Kasten.Profiles}}
  <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin:16px 0 8px">🏗️ Infrastructure Profiles</div>
  <div class="twrap tscroll">
    <table><thead><tr><th>Name</th><th>Provider</th><th>Bucket / Endpoint</th><th>Region</th><th>Immutability</th><th>Status</th></tr></thead>
    <tbody>
    {{range (infraProfiles .Kasten.Profiles)}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="muted">{{orDash .Provider}}</td>
      <td class="mono muted" style="font-size:11px">{{if .Bucket}}{{.Bucket}}{{else if .Endpoint}}{{.Endpoint}}{{else}}—{{end}}</td>
      <td class="mono muted">{{orDash .Region}}</td>
      <td>{{if .Immutability}}<span class="pill ok">{{.ImmutabilityPeriod}}</span>{{else}}—{{end}}</td>
      <td>{{if .Ready}}<span class="pill ok">ready</span>{{else}}<span class="pill fail">not ready</span>{{end}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{end}}
  {{else}}<div class="twrap"><div class="empty">No profiles configured</div></div>{{end}}
</div>

<!-- ══ KUBEVIRT ══ -->
{{if .Kasten.KubeVirt.Enabled}}
<div class="sec" id="kubevirt">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(88,166,255,.1)">💻</div><h2>KubeVirt / OCP Virtualization</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Virtual Machines managed by KubeVirt or OpenShift Virtualization. K10 8.5+ protects VMs natively. <strong>Unprotected VMs</strong>: no policy covers their namespace &mdash; VM data is at risk.</span></span><span class="sec-count">{{.Kasten.KubeVirt.TotalVMs}} VMs · v{{.Kasten.KubeVirt.Version}}</span></div>
  <div class="sgrid" style="margin-bottom:14px">
    <div class="scard green"><div class="slabel">Protected VMs</div><div class="sval">{{.Kasten.KubeVirt.ProtectedVMs}}</div></div>
    <div class="scard red"><div class="slabel">Unprotected VMs</div><div class="sval">{{.Kasten.KubeVirt.UnprotectedVMs}}</div></div>
    <div class="scard blue"><div class="slabel">Total VMs</div><div class="sval">{{.Kasten.KubeVirt.TotalVMs}}</div></div>
  </div>
  <div class="twrap tscroll">
    <table><thead><tr><th>VM Name</th><th>Namespace</th><th>Status</th><th>Protected</th><th>Policy</th></tr></thead>
    <tbody>{{range .Kasten.KubeVirt.VMs}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="mono muted">{{.Namespace}}</td>
      <td><span class="pill {{statusClass .Status}}">{{.Status}}</span></td>
      <td>{{if .Protected}}<span class="pill ok">yes</span>{{else}}<span class="pill fail">no</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .Policy}}</td>
    </tr>{{end}}</tbody></table>
  </div>
</div>
{{end}}

<!-- ══ POLICY FREQUENCIES ══ -->
<div class="sec" id="policy-freq">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(188,140,255,.1)">🔁</div>
    <h2>Policy frequencies</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">How often each policy is scheduled to run. <strong>on-demand</strong>: manual trigger only &mdash; no automatic backups. Policies without a schedule will never protect data automatically.</span></span>
    <span class="sec-count">schedule breakdown</span>
  </div>
  <div style="display:grid;grid-template-columns:1fr 1fr;gap:14px">
    <div class="twrap">
      <table><thead><tr><th>Policy</th><th>Action</th><th>Frequency</th><th>Retention</th><th>Export</th></tr></thead>
      <tbody>{{range .Kasten.Policies}}{{if not .IsSystemPolicy}}<tr>
        <td class="mono" style="font-size:11px">{{.Name}}</td>
        <td style="color:var(--blue)">{{.Action}}</td>
        <td><span class="pill {{if eq .Frequency ""}}off{{else}}run{{end}}">{{if eq .Frequency ""}}—{{else}}{{.Frequency}}{{end}}</span></td>
        <td><div class="ret">{{if .Retention.Hourly}}<span>{{.Retention.Hourly}}h</span>{{end}}{{if .Retention.Daily}}<span>{{.Retention.Daily}}d</span>{{end}}{{if .Retention.Weekly}}<span>{{.Retention.Weekly}}w</span>{{end}}{{if .Retention.Monthly}}<span>{{.Retention.Monthly}}m</span>{{end}}</div></td>
        <td class="mono muted" style="font-size:11px">{{if .ExportProfiles}}<span class="pill ok">yes</span>{{else}}<span class="pill off">no</span>{{end}}</td>
      </tr>{{end}}{{end}}</tbody></table>
    </div>
    <div style="position:relative;height:180px;max-width:180px;margin:0 auto"><canvas id="freqChart" role="img" aria-label="Doughnut: policy frequency distribution">Policy frequency distribution.</canvas></div>
  </div>
</div>

<!-- ══ POLICY COVERAGE MATRIX ══ -->
<div class="sec" id="policy-coverage">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(63,185,80,.1)">🗺️</div>
    <h2>Policy coverage matrix</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Maps each user namespace to the policy protecting it. System namespaces (openshift-*, kube-*, etc.) are excluded — only workload namespaces are shown. <strong>Red rows</strong>: no active policy assigned. Use this table to identify gaps before a disaster occurs.</span></span>
    <span class="sec-count">namespace × policy mapping</span>
  </div>
  {{if .Kasten.CoverageMatrix}}
  <div class="twrap tscroll">
    <table><thead><tr><th>Namespace</th><th>Status</th><th>Policy</th><th>Frequency</th><th>Last backup</th></tr></thead>
    <tbody>{{range .Kasten.CoverageMatrix}}<tr>
      <td class="mono">{{.Namespace}}</td>
      <td>{{if .Protected}}<span class="pill ok">covered</span>{{else}}<span class="pill fail">unprotected</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{if .Policies}}{{join .Policies ", "}}{{else}}—{{end}}</td>
      <td class="mono muted">{{orDash .Frequency}}</td>
      <td class="mono muted" style="font-size:11px">{{fmtDate .LastBackup}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{else}}<div class="twrap"><div class="empty">No namespace data available</div></div>{{end}}
</div>

<script>
(function(){
  // Policy frequency chart
  var freqData = {};
  {{range .Kasten.Policies}}{{if not .IsSystemPolicy}}
  var f = "{{if eq .Frequency ""}}not set{{else}}{{.Frequency}}{{end}}";
  freqData[f] = (freqData[f]||0) + 1;
  {{end}}{{end}}
  var freqLabels = Object.keys(freqData);
  var freqVals = freqLabels.map(function(k){return freqData[k];});
  var freqColors = ["#58a6ff","#3fb950","#d29922","#bc8cff","#ffa657","#f85149","#888780"];
  if(freqLabels.length > 0 && document.getElementById("freqChart")) {
    new Chart(document.getElementById("freqChart"), {
      type: "doughnut",
      data: {
        labels: freqLabels,
        datasets: [{data: freqVals, backgroundColor: freqColors.slice(0,freqLabels.length), borderWidth:0, hoverOffset:4}]
      },
      options: {
        responsive:true, maintainAspectRatio:false, cutout:"55%",
        plugins:{legend:{position:"right",labels:{font:{size:11},color:"#8b949e",boxWidth:10}},
                 tooltip:{callbacks:{label:function(c){return " "+c.label+": "+c.raw+" polic"+(c.raw===1?"y":"ies");}}}}
      }
    });
  }
})();
</script>

</div><!-- /tab-protection -->

<div class="tabpanel" id="tab-recovery">

<!-- ══ RECOVERY READINESS ══ -->
<div class="sec" id="recovery-readiness">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(63,185,80,.1)">🎯</div><h2>Recovery Readiness Score</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">A composite 0&ndash;100 score estimating how recoverable this cluster is &mdash; based on protection coverage, exported copies, immutability, Kasten DR, and recent success rates. Target &ge; 75 (grade B) for production.</span></span></div>
  <div id="rrsLayoutRec" style="display:grid;grid-template-columns:200px 1fr;gap:28px;align-items:start">
    <!-- Score card -->
    <div style="background:var(--s2);border:1px solid var(--b);border-radius:12px;padding:20px 16px;text-align:center">
      <div style="font-size:11px;color:var(--tm);margin-bottom:8px">How recoverable is this cluster?</div>
      <div style="font-size:64px;font-weight:800;line-height:1;color:{{gradeColor .Kasten.RecoveryReadiness.Grade}}">{{.Kasten.RecoveryReadiness.Score}}</div>
      <div style="font-size:11px;color:var(--tm);margin:2px 0 12px">out of 100</div>
      <!-- Grade badge -->
      <div style="display:inline-block;background:{{gradeColor .Kasten.RecoveryReadiness.Grade}};color:#fff;font-size:22px;font-weight:800;padding:4px 20px;border-radius:8px;letter-spacing:1px">{{.Kasten.RecoveryReadiness.Grade}}</div>
      <!-- Grade scale -->
      <div style="display:flex;justify-content:space-around;margin-top:14px;padding-top:12px;border-top:1px solid var(--b)">
        {{$recGrade := .Kasten.RecoveryReadiness.Grade}}
        <div style="text-align:center;opacity:{{if eq $recGrade "F"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#f85149">F</div></div>
        <div style="text-align:center;opacity:{{if eq $recGrade "D"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#f85149">D</div></div>
        <div style="text-align:center;opacity:{{if eq $recGrade "C"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#ffa657">C</div></div>
        <div style="text-align:center;opacity:{{if eq $recGrade "B"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#58a6ff">B</div></div>
        <div style="text-align:center;opacity:{{if eq $recGrade "A"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#3fb950">A</div></div>
      </div>
      <div style="font-size:9px;color:var(--tm);margin-top:6px;line-height:1.5;text-align:left">
        <strong style="color:#3fb950">A</strong> &ge;90 Excellent &nbsp;
        <strong style="color:#58a6ff">B</strong> &ge;75 Good<br>
        <strong style="color:#ffa657">C</strong> &ge;60 Fair &nbsp;&nbsp;
        <strong style="color:#f85149">D</strong> &ge;40 Poor &nbsp;
        <strong style="color:#f85149">F</strong> &lt;40 Critical
      </div>
    </div>
    <div style="position:relative;min-height:200px">
      <canvas id="rrsChartRec" role="img" aria-label="Recovery readiness score breakdown" style="max-height:260px"></canvas>
    </div>
  </div>
  {{if .Kasten.RecoveryReadiness.Findings}}
  <div style="margin-top:14px">
    <div style="font-size:11px;font-weight:600;color:var(--tm);margin-bottom:8px">Gaps to address:</div>
    {{range .Kasten.RecoveryReadiness.Findings}}
    <div style="font-size:12px;color:var(--red);padding:4px 0;border-bottom:1px solid var(--b)">⚠ {{.}}</div>
    {{end}}
  </div>
  {{end}}
</div>

<!-- ══ DISASTER RECOVERY ══ -->
<div class="sec" id="dr">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(248,81,73,.1)">🔄</div><h2>Kasten Disaster Recovery</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Protects the K10 catalog itself &mdash; the database of all backup metadata. Without KDR, a cluster failure loses the ability to restore even if backup data exists in object storage.</span></span></div>
  {{if .Kasten.DR.Enabled}}
  <div class="twrap">
    <table><tbody>
      <tr><td>Status</td><td><span class="pill ok">Enabled</span></td></tr>
      <tr><td>Mode</td><td>{{orDash .Kasten.DR.Mode}}</td></tr>
      <tr><td>DR Policy</td><td class="mono">{{.Kasten.DR.BackupPolicy}}</td></tr>
      <tr><td>Export Profile</td><td class="mono muted">{{orDash .Kasten.DR.ExportProfile}}</td></tr>
      <tr><td>Last Run</td><td class="mono muted">{{fmtDate .Kasten.DR.LastRunTime}}</td></tr>
      <tr><td>Last Status</td><td>{{if .Kasten.DR.LastRunStatus}}<span class="pill {{statusClass .Kasten.DR.LastRunStatus}}">{{.Kasten.DR.LastRunStatus}}</span>{{else}}—{{end}}</td></tr>
    </tbody></table>
  </div>
  {{if .Kasten.MultiCluster.Clusters}}
  <div style="margin-top:12px">
    <div class="twrap tscroll">
      <table><thead><tr><th>Remote Cluster</th><th>URL</th><th>Status</th><th>Version</th></tr></thead>
      <tbody>{{range .Kasten.MultiCluster.Clusters}}<tr>
        <td class="mono">{{.Name}}</td>
        <td class="mono muted" style="font-size:11px">{{.URL}}</td>
        <td><span class="pill {{statusClass .Status}}">{{.Status}}</span></td>
        <td class="mono muted">{{orDash .Version}}</td>
      </tr>{{end}}</tbody></table>
    </div>
  </div>{{end}}
  {{else}}
  <div class="alert warn">⚠️ No Kasten DR policy configured — the K10 catalog is not being backed up externally</div>
  {{end}}
</div>

<!-- ══ RESTORE POINTS ══ -->
<div class="sec" id="restorepoints">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(63,185,80,.1)">💾</div><h2>Restore Points</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Saved backup copies for application recovery. Each point has a <strong>snapshot</strong> (local) and optionally an <strong>exported</strong> copy in object storage. <strong>Orphaned</strong>: no matching namespace &mdash; safe to delete to reclaim storage.</span></span><span class="sec-count">{{.Kasten.RestorePoints.Total}} total · {{.Kasten.RestorePoints.Orphaned}} orphaned</span></div>
  {{if .Kasten.RestorePoints.ByApp}}
  <div style="background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px;margin-bottom:14px">
    <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:10px">Restore points per application</div>
    <div id="rpChartWrap" style="position:relative;height:180px"><canvas id="rpChart" role="img" aria-label="Restore points by application"></canvas></div>
  </div>
  {{end}}
  <div class="two">
    {{if .Kasten.RestorePoints.ByApp}}
    <div class="twrap tscroll">
      <table><thead><tr><th>Application</th><th>Restore Points</th></tr></thead>
      <tbody>{{range $app,$count := .Kasten.RestorePoints.ByApp}}<tr>
        <td class="mono">{{$app}}</td>
        <td style="color:var(--green);font-family:'IBM Plex Mono',monospace;font-weight:600">{{$count}}</td>
      </tr>{{end}}</tbody></table>
    </div>{{end}}
    {{if .Kasten.RestorePoints.ByPolicy}}
    <div class="twrap tscroll">
      <table><thead><tr><th>Policy</th><th>Restore Points</th></tr></thead>
      <tbody>{{range $pol,$count := .Kasten.RestorePoints.ByPolicy}}<tr>
        <td class="mono">{{$pol}}</td>
        <td style="color:var(--blue);font-family:'IBM Plex Mono',monospace;font-weight:600">{{$count}}</td>
      </tr>{{end}}</tbody></table>
    </div>{{end}}
  </div>
  {{if .Kasten.RestorePoints.Details}}
  <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin:16px 0 8px">Restore Point Detail</div>
  <div class="no-print" style="display:flex;gap:6px;margin-bottom:10px;align-items:center;flex-wrap:wrap">
    <span style="font-size:11px;color:var(--tm)">Filter:</span>
    <button class="job-filter-btn" data-rpddays="-1" onclick="filterRPDetail(-1)">Today</button>
    <button class="job-filter-btn active" data-rpddays="0" onclick="filterRPDetail(0)">All time</button>
    <button class="job-filter-btn" data-rpddays="7" onclick="filterRPDetail(7)">Last 7 days</button>
    <button class="job-filter-btn" data-rpddays="30" onclick="filterRPDetail(30)">Last 30 days</button>
    <button class="job-filter-btn" data-rpddays="90" onclick="filterRPDetail(90)">Last 90 days</button>
    <span id="rpd-count" style="font-size:11px;color:var(--tm);margin-left:auto"></span>
  </div>
  <div class="twrap tscroll">
    <table><thead><tr><th>Name</th><th>Application</th><th>Created</th><th>Policy</th></tr></thead>
    <tbody>{{range .Kasten.RestorePoints.Details}}<tr class="rpd-row" data-rpddate="{{.CreatedAt}}">
      <td class="mono" style="font-size:11px">{{.Name}}</td>
      <td class="mono muted">{{orDash .AppName}}</td>
      <td class="mono muted" style="font-size:11px;white-space:nowrap">{{formatTimeShort .CreatedAt}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .Policy}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{end}}
  {{if gt .Kasten.RestorePoints.Orphaned 0}}
  <div class="alert warn" style="margin-top:12px">⚠️ {{.Kasten.RestorePoints.Orphaned}} orphaned restore point(s) — consuming storage without a matching application</div>
  {{end}}
</div>

<!-- ══ APPLICATION RISK MATRIX ══ -->
<div class="sec" id="app-risk-matrix">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(248,81,73,.1)">📊</div><h2>Application Risk Matrix</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Per-application recovery risk. <strong>RPO now</strong>: age of the newest restore point (data you'd lose today). <strong>Est. RTO</strong>: rough time to recover. <strong>Export</strong>/<strong>Immutable</strong>: whether a secondary, tamper-proof copy exists. Use it to prioritize which apps to harden first.</span></span></div>
  {{if .Kasten.AppRiskMatrix}}
  <div class="twrap tscroll">
    <table style="width:100%;border-collapse:collapse;font-size:12px">
      <thead><tr style="border-bottom:1px solid var(--b)">
        <th style="text-align:left;padding:6px 8px;color:var(--tm);font-weight:500">App</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Risk</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">RPO now</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Est. RTO</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Export</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Immutable</th>
        <th style="text-align:left;padding:6px 8px;color:var(--tm);font-weight:500">Notes</th>
      </tr></thead>
      <tbody>
      {{range .Kasten.AppRiskMatrix}}<tr style="border-bottom:1px solid var(--b)">
        <td style="padding:6px 8px;font-family:monospace">{{.Namespace}}</td>
        <td style="padding:6px 8px;text-align:center">{{riskIcon .RiskLevel}}</td>
        <td style="padding:6px 8px;text-align:center;color:{{if gt .RPOHours 168.0}}var(--red){{else if gt .RPOHours 24.0}}var(--yellow){{else}}var(--green){{end}}">
          {{if gt .RPOHours 0.0}}{{printf "%.0fh" .RPOHours}}{{else}}—{{end}}</td>
        <td style="padding:6px 8px;text-align:center;color:var(--tm)">
          {{if gt .RTOMinutes 0.0}}{{printf "~%.0fm" .RTOMinutes}}{{else}}—{{end}}</td>
        <td style="padding:6px 8px;text-align:center">{{if .HasExport}}✅{{else}}❌{{end}}</td>
        <td style="padding:6px 8px;text-align:center">{{if .HasImmutable}}✅{{else}}❌{{end}}</td>
        <td style="padding:6px 8px;color:var(--tm);font-size:11px">{{range .RiskReasons}}{{.}} {{end}}</td>
      </tr>{{end}}
      </tbody>
    </table>
  </div>
  {{else}}
  <div class="alert" style="margin-top:4px">No application risk data available — run against a cluster with protected applications.</div>
  {{end}}
</div>

<script>
(function(){
  // Restore points chart (Recovery tab)
  var rpData = JSON.parse('{{rpByAppJSON .Kasten.RestorePoints.ByApp}}');
  var rpLabels = Object.keys(rpData);
  var rpVals = rpLabels.map(function(k){return rpData[k];});
  if(rpLabels.length === 0 && document.getElementById("rpChartWrap")) {
    document.getElementById("rpChartWrap").innerHTML =
      '<div style="height:160px;display:flex;align-items:center;justify-content:center;color:#8b949e;font-size:12px;text-align:center;padding:0 20px">No restore points yet &mdash; run a policy manually to create the first backup</div>';
  } else if(rpLabels.length > 0 && document.getElementById("rpChart")) {
    var rpH = Math.max(rpLabels.length * 36 + 60, 160);
    document.getElementById("rpChartWrap").style.height = rpH + "px";
    new Chart(document.getElementById("rpChart"), {
      type: "bar",
      data: {
        labels: rpLabels,
        datasets: [{label:"Restore points", data:rpVals, backgroundColor:"#3fb950", borderRadius:4, barThickness:18}]
      },
      options: {
        indexAxis:"y", responsive:true, maintainAspectRatio:false,
        scales:{x:{grid:{color:"rgba(139,148,158,0.12)"},border:{display:false}},
                y:{grid:{display:false},ticks:{font:{size:10},color:"#8b949e"}}},
        plugins:{legend:{display:false}}
      }
    });
  }

  // Recovery Readiness Score chart (Recovery tab)
  var rrsEl = document.getElementById("rrsChartRec");
  if(rrsEl){
    var rrs = JSON.parse('{{rrsJSON .Kasten.RecoveryReadiness}}');
    if(rrs && rrs.labels){
      var earned = rrs.earned;
      var gaps = rrs.max.map(function(m,i){return m - earned[i];});
      new Chart(rrsEl, {
        type: "bar",
        data: {
          labels: rrs.labels,
          datasets: [
            {label:"Earned", data:earned, backgroundColor:"#3fb950", borderWidth:0, borderRadius:3},
            {label:"Gap",    data:gaps,   backgroundColor:"rgba(248,81,73,0.25)", borderWidth:0, borderRadius:3}
          ]
        },
        options: {
          indexAxis:"y", responsive:true, maintainAspectRatio:false,
          scales:{
            x:{stacked:true, max:25, grid:{color:"rgba(139,148,158,0.12)"}, ticks:{stepSize:5}},
            y:{stacked:true, grid:{display:false}, ticks:{font:{size:10}}}
          },
          plugins:{legend:{display:false},tooltip:{callbacks:{
            label:function(c){
              if(c.datasetIndex===0) return " Earned: "+c.raw;
              return " Gap: "+c.raw;
            }
          }}}
        }
      });
    }
  }
})();
</script>

</div><!-- /tab-recovery -->

<div class="tabpanel" id="tab-operations">

<!-- ══ JOBS ══ -->
<div class="sec" id="jobs">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(88,166,255,.1)">⚡</div><h2>Recent Jobs</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box"><strong>run</strong>: orchestration job that triggers backup + export together. <strong>backup</strong>: creates a local snapshot. <strong>export</strong>: copies data to a location profile. <strong>Skipped</strong>: nothing changed since last run. Policy/App may be empty on run-type actions &mdash; this is normal in K10 8.x.</span></span><span class="sec-count">{{len .Kasten.Jobs}} collected</span></div>
  <div class="no-print" style="display:flex;gap:8px;align-items:center;margin-bottom:8px;flex-wrap:wrap">
    <span style="font-size:11px;color:var(--tm)">Filter:</span>
    <button class="job-filter-btn" data-days="-1" onclick="filterJobsToday()">Today</button>
    <button class="job-filter-btn" data-days="0" onclick="filterJobs(0)">All time</button>
    <button class="job-filter-btn active" data-days="7" onclick="filterJobs(7)">Last 7 days</button>
    <button class="job-filter-btn" data-days="30" onclick="filterJobs(30)">Last 30 days</button>
    <button class="job-filter-btn" data-days="90" onclick="filterJobs(90)">Last 90 days</button>
    <select id="job-status-filter" onchange="applyCurrentFilter()" style="margin-left:4px;padding:3px 8px;font-size:11px;background:var(--s2);border:1px solid var(--b);border-radius:4px;color:var(--t)">
      <option value="">All statuses</option>
      <option value="Complete">Complete</option>
      <option value="Failed">Failed</option>
      <option value="Skipped">Skipped</option>
      <option value="Running">Running</option>
    </select>
    <select id="job-policy-filter" onchange="applyCurrentFilter()" style="padding:3px 8px;font-size:11px;background:var(--s2);border:1px solid var(--b);border-radius:4px;color:var(--t)" title="Filter by policy name">
      <option value="">All policies</option>
    </select>
  </div>
  <div class="no-print" style="display:flex;gap:8px;align-items:center;margin-bottom:12px;flex-wrap:wrap">
    <span style="font-size:11px;color:var(--tm)">Custom range:</span>
    <input type="date" id="job-date-from" style="padding:3px 8px;font-size:11px;background:var(--s2);border:1px solid var(--b);border-radius:4px;color:var(--t)" onchange="filterJobsRange()">
    <span style="font-size:11px;color:var(--tm)">→</span>
    <input type="date" id="job-date-to" style="padding:3px 8px;font-size:11px;background:var(--s2);border:1px solid var(--b);border-radius:4px;color:var(--t)" onchange="filterJobsRange()">
    <button class="job-filter-btn" onclick="clearJobRange()" style="font-size:10px">✕ Clear</button>
    <button class="job-pdf-btn" onclick="exportJobsPDF()" title="Print / save the filtered jobs as PDF">⤓ Export PDF</button>
    <span id="job-count-label" style="font-size:11px;color:var(--tm);margin-left:auto"></span>
  </div>
  <div id="print-filter-summary" class="print-only"></div>
{{if .Kasten.Jobs}}
  <div class="twrap tscroll">
    <table><thead><tr><th>Action</th><th>Policy</th><th>Application</th><th>Status</th><th>Start</th><th>Duration</th><th>Error</th></tr></thead>
    <tbody>{{range .Kasten.Jobs}}<tr class="job-row" data-start="{{.StartTime}}" data-status="{{.Status}}" data-policy="{{.PolicyName}}">
      <td style="color:var(--blue)">{{.Action}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .PolicyName}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .AppName}}</td>
      <td><span class="pill {{statusClass .Status}}">{{.Status}}</span></td>
      <td class="mono muted" style="font-size:11px">{{fmtDate .StartTime}}</td>
      <td class="mono muted">{{orDash .Duration}}</td>
      <td style="font-size:11px;color:var(--red);max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{orDash .Error}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{else}}<div class="twrap"><div class="empty">No jobs found</div></div>{{end}}
</div>

<!-- ══ JOB EXECUTION TREND ══ -->
{{if .Kasten.Jobs}}
<div class="sec" id="job-trend">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(88,166,255,.1)">📈</div><h2>Job Execution Trend</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Stacked view of job outcomes over time (Complete / Failed / Skipped). Switch the granularity between day, week and month. Reflects all collected jobs, independent of the Recent Jobs filters above.</span></span><span class="sec-count" id="jobtrend-count"></span></div>
  <div class="no-print" style="display:flex;gap:6px;margin-bottom:10px;align-items:center">
    <span style="font-size:11px;color:var(--tm)">Granularity:</span>
    <button class="job-filter-btn active" data-gran="day" onclick="setJobTrendGran('day')">Day</button>
    <button class="job-filter-btn" data-gran="week" onclick="setJobTrendGran('week')">Week</button>
    <button class="job-filter-btn" data-gran="month" onclick="setJobTrendGran('month')">Month</button>
  </div>
  <div style="position:relative;height:240px"><canvas id="jobTrendChart" role="img" aria-label="Stacked bar: job execution outcomes over time">Job execution outcomes over time.</canvas></div>
</div>
{{end}}

<!-- ══ K10 REPORTS ══ -->
{{if .Kasten.K10Reports}}
<div class="sec" id="k10-reports">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(88,166,255,.1)">📋</div>
    <h2>K10 generated reports</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Reports automatically generated by Kasten K10 (daily/weekly/monthly). Each report contains a point-in-time snapshot of applications, actions, storage, and license status. The most recent report is used to enrich the license and storage sections of this tool.</span></span>
    <span class="sec-count" id="rpt-count">{{len .Kasten.K10Reports}} reports</span>
  </div>
  <div style="display:flex;gap:6px;margin-bottom:10px;align-items:center">
    <span style="font-size:11px;color:var(--tm)">Show:</span>
    <button class="job-filter-btn" data-rptdays="-1" onclick="filterReports(-1)">Today</button>
    <button class="job-filter-btn" data-rptdays="7" onclick="filterReports(7)">Last 7 days</button>
    <button class="job-filter-btn" data-rptdays="30" onclick="filterReports(30)">Last 30 days</button>
    <button class="job-filter-btn" data-rptdays="90" onclick="filterReports(90)">Last 90 days</button>
    <button class="job-filter-btn active" data-rptdays="0" onclick="filterReports(0)">All time</button>
  </div>
  <div class="twrap tscroll">
    <table><thead><tr><th>Report Name</th><th>Generated</th><th>Period</th><th>Apps</th><th>Actions</th><th>Completed</th><th>Failed</th><th>Snapshot</th><th>Export</th><th>Dedup</th></tr></thead>
    <tbody>{{range .Kasten.K10Reports}}<tr class="rpt-row" data-rptdate="{{.GeneratedAt}}">
      <td class="mono" style="font-size:11px">{{.Name}}</td>
      <td class="mono muted" style="font-size:11px">{{fmtDate .GeneratedAt}}</td>
      <td class="muted">{{orDash .Period}}</td>
      <td>{{.Stats.Apps.Total}}</td>
      <td>{{.Stats.Actions.Total}}</td>
      <td style="color:var(--green)">{{.Stats.Actions.Completed}}</td>
      <td style="color:{{if gt .Stats.Actions.Failed 0}}var(--red){{else}}var(--tm){{end}}">{{.Stats.Actions.Failed}}</td>
      <td class="mono muted">{{storageHuman .Stats.Storage.SnapshotSizeBytes}} ({{.Stats.Storage.SnapshotCount}})</td>
      <td class="mono muted">{{storageHuman .Stats.Storage.ExportSizeBytes}}</td>
      <td class="mono muted">{{if gt .Stats.Storage.DedupeRatio 0.0}}{{printf "%.1fx" .Stats.Storage.DedupeRatio}}{{else}}—{{end}}</td>
    </tr>{{end}}</tbody></table>
  </div>
</div>
{{end}}

<!-- ══ ACTIONS SUMMARY ══ -->
<div class="sec" id="actions-summary">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(255,166,87,.1)">⚡</div>
    <h2>Actions summary</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Breakdown of all collected actions by type and outcome. <strong>run</strong>: policy-level orchestration jobs (each triggers backup + optional export). <strong>backup</strong>: individual namespace snapshot actions. <strong>export</strong>: data copies to a location profile. <strong>restore</strong>: recovery actions. Counts cover the last 200 collected jobs. <strong>Skipped</strong> means nothing changed since the last run.</span></span>
  </div>
  <div style="display:grid;grid-template-columns:1fr 1fr;gap:14px">
    <div class="twrap">
      <table><thead><tr><th>Status</th><th>Count</th><th>%</th></tr></thead>
      <tbody>
        <tr><td><span class="pill ok">Complete</span></td><td class="mono">{{.Kasten.JobSummary.Completed}}</td><td class="mono muted" id="jspct-complete">—</td></tr>
        <tr><td><span class="pill fail">Failed</span></td><td class="mono">{{.Kasten.JobSummary.Failed}}</td><td class="mono muted" id="jspct-failed">—</td></tr>
        <tr><td><span class="pill off">Skipped</span></td><td class="mono">{{.Kasten.JobSummary.Skipped}}</td><td class="mono muted" id="jspct-skipped">—</td></tr>
        <tr><td><span class="pill off">Cancelled</span></td><td class="mono">{{.Kasten.JobSummary.Cancelled}}</td><td class="mono muted" id="jspct-cancelled">—</td></tr>
        <tr style="border-top:2px solid var(--b)"><td><strong>Total</strong></td><td class="mono"><strong>{{.Kasten.JobSummary.Total}}</strong></td><td></td></tr>
      </tbody></table>
    </div>
    <div style="background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px">
      <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:10px">By action type</div>
      <div style="position:relative;height:180px"><canvas id="actionTypeChart" role="img" aria-label="Actions by type">Actions by type.</canvas></div>
    </div>
  </div>
</div>



</div><!-- /tab-operations -->

<div class="tabpanel" id="tab-storage">

<!-- ══ STORAGE OVERVIEW ══ -->
<div class="sec" id="storage-overview">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(255,184,0,.1)">🗃️</div>
    <h2>Storage Overview</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box"><strong>Snapshot Storage</strong>: local snapshots on the cluster (consumed from PVC space). <strong>Export Storage</strong>: data sent to location profiles. <strong>Dedup Ratio</strong>: how much the exported data is deduplicated — 1.1x means 10% savings. <strong>Live Storage</strong>: total PVC capacity across all user namespaces.</span></span>
    <span class="sec-count" style="color:var(--tm)" title="Data sourced from K10 system reports — run k10-system-reports-policy for complete storage metrics">from K10 system reports</span>
  </div>

  {{/* ── Storage report age banner ── */}}
  {{if lt .Kasten.Storage.ReportAgeDays 0}}
  <div style="margin-bottom:16px;padding:12px 16px;border-radius:8px;border-left:4px solid var(--red);background:rgba(248,81,73,0.08);font-size:12px">
    <strong style="color:var(--red)">⚠ No storage metrics available</strong><br>
    <span style="color:var(--tm)">The <code>k10-system-reports-policy</code> has never been executed on this cluster.
    Snapshot and export storage data cannot be retrieved. Only live PVC storage is shown below.<br>
    <strong>Action:</strong> Ask the customer to run the k10-system-reports-policy to populate storage metrics.</span>
  </div>
  {{else if gt .Kasten.Storage.ReportAgeDays 30}}
  <div style="margin-bottom:16px;padding:12px 16px;border-radius:8px;border-left:4px solid var(--red);background:rgba(248,81,73,0.08);font-size:12px">
    <strong style="color:var(--red)">⚠ Storage data is {{.Kasten.Storage.ReportAgeDays}} days old</strong><br>
    <span style="color:var(--tm)">Last K10 report: <strong>{{fmtDate .Kasten.Storage.ReportDate}}</strong>.
    Snapshot and export metrics may be significantly outdated.<br>
    <strong>Action:</strong> Run the k10-system-reports-policy to refresh storage data.</span>
  </div>
  {{else if gt .Kasten.Storage.ReportAgeDays 7}}
  <div style="margin-bottom:16px;padding:12px 16px;border-radius:8px;border-left:4px solid var(--yellow);background:rgba(210,153,34,0.08);font-size:12px">
    <strong style="color:var(--yellow)">⚡ Storage data is {{.Kasten.Storage.ReportAgeDays}} days old</strong><br>
    <span style="color:var(--tm)">Last K10 report: <strong>{{fmtDate .Kasten.Storage.ReportDate}}</strong>.
    Consider running k10-system-reports-policy for fresher metrics.</span>
  </div>
  {{else}}
  <div style="margin-bottom:12px;font-size:11px;color:var(--tm)">
    📅 Storage data from K10 report generated <strong>{{fmtDate .Kasten.Storage.ReportDate}}</strong> ({{.Kasten.Storage.ReportAgeDays}} day(s) ago)
  </div>
  {{end}}

  <div class="sgrid">
    <div class="scard blue"><div class="slabel">Snapshot Storage</div><div class="sval" style="font-size:20px" id="stor-snap-size">—</div><div class="ssub" id="stor-snap-count">— snapshots</div></div>
    <div class="scard orange"><div class="slabel">Export Storage</div><div class="sval" style="font-size:20px" id="stor-exp-size">—</div><div class="ssub" id="stor-exp-dedup">dedup —</div></div>
    <div class="scard green"><div class="slabel">Live Storage</div><div class="sval" style="font-size:20px" id="stor-live-size">—</div><div class="ssub" id="stor-live-vols">— volumes</div></div>
    <div class="scard purple"><div class="slabel">Total PVCs</div><div class="sval">{{.Kasten.PVCs.Total}}</div><div class="ssub">{{.Kasten.PVCs.Bound}} bound · {{.Kasten.PVCs.Pending}} pending · {{.Kasten.PVCs.Lost}} lost</div></div>
  </div>

  {{if .Kasten.Storage.ServicesDisk}}
  <div style="margin-top:16px">
    <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:10px">K10 Services Disk Usage</div>
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:10px">
      {{range .Kasten.Storage.ServicesDisk}}
      <div style="background:var(--s1);border:1px solid var(--b);border-radius:8px;padding:14px">
        <div style="font-size:11px;font-weight:600;color:var(--t);margin-bottom:8px">{{.Name}}</div>
        <div style="font-size:11px;color:var(--tm);margin-bottom:4px">{{.FreeHuman}} free of {{.TotalHuman}}</div>
        {{if gt .TotalBytes 0}}
        <div style="background:var(--b);border-radius:3px;height:5px;overflow:hidden">
          <div style="height:100%;border-radius:3px;background:{{if lt .FreePercent 20.0}}var(--red){{else if lt .FreePercent 40.0}}var(--yellow){{else}}var(--green){{end}};width:{{printf "%.0f" .FreePercent}}%"></div>
        </div>
        <div style="font-size:10px;color:var(--tm);margin-top:3px">{{printf "%.0f" .FreePercent}}% free</div>
        {{end}}
      </div>
      {{end}}
    </div>
  </div>
  {{end}}
</div>

<!-- ══ CATALOG ══ -->
{{if .Kasten.Catalog.SizeHuman}}
<div class="sec" id="catalog">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(255,184,0,.1)">📚</div><h2>Catalog</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">The K10 catalog is the internal database of all backup metadata (restore points, actions, policies). If it runs low on space, K10 upgrades and new backups can fail &mdash; keep &ge;50% free (BP-24).</span></span></div>
  <div class="igrid">
    <div class="icard"><div class="icard-label">Catalog Size</div><div class="icard-val">{{.Kasten.Catalog.SizeHuman}}</div></div>
    {{if .Kasten.Catalog.FreeHuman}}<div class="icard"><div class="icard-label">Free Space</div><div class="icard-val" style="color:{{if .Kasten.Catalog.LowSpaceAlert}}var(--red){{else}}var(--green){{end}}">{{.Kasten.Catalog.FreeHuman}} ({{pct .Kasten.Catalog.FreePercent}})</div></div>{{end}}
    <div class="icard"><div class="icard-label">Storage Class</div><div class="icard-val">{{orDash .Kasten.Catalog.StorageClass}}</div></div>
    <div class="icard"><div class="icard-label">Low Space Alert</div><div class="icard-val">{{if .Kasten.Catalog.LowSpaceAlert}}<span style="color:var(--red)">YES</span>{{else}}<span style="color:var(--green)">No</span>{{end}}</div></div>
  </div>
</div>
{{end}}

<!-- ══ STORAGE CHARTS ══ -->
<div class="sec" id="storage-charts">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(63,185,80,.1)">📊</div>
    <h2>Storage breakdown</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box"><strong>Storage type breakdown</strong>: visual split of snapshot (local K10 copies), export (object storage copies), and live PVC data — populated from the K10 system report. {{if .Kasten.Storage.ExportByApp}}<strong>Export by application</strong>: which apps consume the most export storage.{{end}}</span></span>
  </div>
  <div style="display:grid;grid-template-columns:{{if .Kasten.Storage.ExportByApp}}1fr 1fr{{else}}1fr{{end}};gap:14px">
    <div class="stat-chart-card">
      <div class="stat-chart-title">Storage type breakdown</div>
      <div style="position:relative;height:200px"><canvas id="storTypeChart" role="img" aria-label="Storage breakdown by type">Storage type breakdown.</canvas></div>
    </div>
    {{if .Kasten.Storage.ExportByApp}}
    <div class="stat-chart-card">
      <div class="stat-chart-title">Export storage by application</div>
      <div id="storAppWrap" style="position:relative;height:200px"><canvas id="storAppChart" role="img" aria-label="Export storage by application">Export by application.</canvas></div>
    </div>
    {{end}}
  </div>
</div>

<!-- ══ PVC STATUS ══ -->
<div class="sec" id="pvc-status">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(255,184,0,.1)">💿</div>
    <h2>PVC status</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Persistent Volume Claims in user namespaces. <strong>Bound</strong>: healthy and attached. <strong>Pending</strong>: waiting for a volume &mdash; possible storage issue. <strong>Lost</strong>: backing volume deleted &mdash; data may be unrecoverable.</span></span>
    <span class="sec-count">{{.Kasten.PVCs.Total}} volumes · {{printf "%.1f" .Kasten.PVCs.TotalSizeGB}} GB total</span>
  </div>
  {{if .Kasten.PVCs.Items}}
  <div style="display:grid;grid-template-columns:1fr 1fr;gap:14px;margin-bottom:14px">
    <div style="background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px">
      <div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:10px">PVC health</div>
      <div style="position:relative;height:160px"><canvas id="pvcChart" role="img" aria-label="PVC status breakdown">PVC status breakdown.</canvas></div>
    </div>
    <div style="background:var(--s2);border:1px solid var(--b);border-radius:10px;padding:14px;display:grid;grid-template-columns:1fr 1fr;gap:10px;align-content:start">
      <div><div style="font-size:10px;color:var(--tm);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px">Total</div><div style="font-size:24px;font-weight:700;font-family:'IBM Plex Mono',monospace;color:var(--blue)">{{.Kasten.PVCs.Total}}</div></div>
      <div><div style="font-size:10px;color:var(--tm);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px">Bound</div><div style="font-size:24px;font-weight:700;font-family:'IBM Plex Mono',monospace;color:var(--green)">{{.Kasten.PVCs.Bound}}</div></div>
      <div><div style="font-size:10px;color:var(--tm);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px">Pending</div><div style="font-size:24px;font-weight:700;font-family:'IBM Plex Mono',monospace;color:var(--yellow)">{{.Kasten.PVCs.Pending}}</div></div>
      <div><div style="font-size:10px;color:var(--tm);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px">Lost</div><div style="font-size:24px;font-weight:700;font-family:'IBM Plex Mono',monospace;color:var(--red)">{{.Kasten.PVCs.Lost}}</div></div>
      <div style="grid-column:1/-1;border-top:1px solid var(--b);padding-top:10px"><div style="font-size:10px;color:var(--tm);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px">Total capacity</div><div style="font-size:20px;font-weight:700;font-family:'IBM Plex Mono',monospace;color:var(--purple)">{{printf "%.1f" .Kasten.PVCs.TotalSizeGB}} GB</div></div>
    </div>
  </div>
  <div class="twrap tscroll">
    <table><thead><tr><th>PVC</th><th>Namespace</th><th>Status</th><th>Size</th><th>Storage Class</th><th>Access</th></tr></thead>
    <tbody>{{range .Kasten.PVCs.Items}}<tr>
      <td class="mono" style="font-size:11px">{{.Name}}</td>
      <td class="mono muted">{{.Namespace}}</td>
      <td><span class="pill {{statusClass .Status}}">{{.Status}}</span></td>
      <td class="mono muted">{{printf "%.1f" .CapacityGB}} GB</td>
      <td class="mono muted" style="font-size:11px">{{orDash .StorageClass}}</td>
      <td class="mono muted">{{orDash .AccessModes}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{else}}<div class="twrap"><div class="empty">No PVCs found in user namespaces</div></div>{{end}}
</div>

<script>
(function(){
  // Storage data from Go template
  var snapBytes = {{.Kasten.Storage.SnapshotSizeBytes}};
  var expBytes  = {{.Kasten.Storage.ExportSizeBytes}};
  var liveBytes = {{.Kasten.Storage.LiveSizeBytes}};
  var snapCount = {{.Kasten.Storage.SnapshotCount}};
  var expDedup  = {{.Kasten.Storage.DedupeRatio}};  {{/* raw float — printf returns a string that html/template quotes, breaking expDedup.toFixed() */}}
  var liveVols  = {{.Kasten.Storage.LiveVolumeCount}};

  function humanBytes(b) {
    if(!b || b===0) return "0 B";
    var units=["B","KB","MB","GB","TB"], i=0;
    while(b>=1024&&i<units.length-1){b/=1024;i++;}
    return b.toFixed(1)+" "+units[i];
  }

  // Fill KPI cards
  document.getElementById("stor-snap-size").textContent = humanBytes(snapBytes);
  document.getElementById("stor-snap-count").textContent = snapCount + " snapshot" + (snapCount===1?"":"s");
  document.getElementById("stor-exp-size").textContent = humanBytes(expBytes);
  document.getElementById("stor-exp-dedup").textContent = expDedup > 0 ? "dedup " + expDedup.toFixed(1) + "x" : "dedup n/a";
  document.getElementById("stor-live-size").textContent = humanBytes(liveBytes);
  document.getElementById("stor-live-vols").textContent = liveVols + " volume" + (liveVols===1?"":"s");

  var grid = "rgba(139,148,158,0.12)";

  // Storage type donut
  if(document.getElementById("storTypeChart") && (snapBytes+expBytes+liveBytes) > 0) {
    new Chart(document.getElementById("storTypeChart"), {
      type: "doughnut",
      data: {
        labels: ["Snapshot", "Export", "Live volumes"],
        datasets: [{
          data: [snapBytes, expBytes, liveBytes],
          backgroundColor: ["#58a6ff","#ffa657","#3fb950"],
          borderWidth: 0, hoverOffset: 4
        }]
      },
      options: {
        responsive:true, maintainAspectRatio:false, cutout:"58%",
        plugins:{
          legend:{position:"right",labels:{font:{size:11},color:"#8b949e",boxWidth:10}},
          tooltip:{callbacks:{label:function(c){return " "+c.label+": "+humanBytes(c.raw);}}}
        }
      }
    });
  }

  // Export by app — from K10 report (placeholder if no data)
  var expByApp = JSON.parse('{{expByAppJSON .Kasten.Storage.ExportByApp}}');
  var appLabels = Object.keys(expByApp);
  var appVals = appLabels.map(function(k){return expByApp[k];});
  if(appLabels.length > 0 && document.getElementById("storAppChart")) {
    var h = Math.max(appLabels.length * 36 + 60, 160);
    document.getElementById("storAppWrap").style.height = h+"px";
    new Chart(document.getElementById("storAppChart"), {
      type: "bar",
      data: {
        labels: appLabels,
        datasets: [{label:"Export", data:appVals, backgroundColor:"#ffa657", borderRadius:3, barThickness:16}]
      },
      options: {
        indexAxis:"y", responsive:true, maintainAspectRatio:false,
        scales:{
          x:{grid:{color:grid},border:{display:false},
             ticks:{callback:function(v){return humanBytes(v);}}},
          y:{grid:{display:false},ticks:{font:{size:10},color:"#8b949e"}}
        },
        plugins:{legend:{display:false},
                 tooltip:{callbacks:{label:function(c){return " "+humanBytes(c.raw);}}}}
      }
    });
  } else if(document.getElementById("storAppWrap")) {
    document.getElementById("storAppWrap").innerHTML =
      '<div style="height:160px;display:flex;align-items:center;justify-content:center;color:#8b949e;font-size:12px;text-align:center;padding:0 20px">Export-by-application data available after K10 generates a report</div>';
  }

  // PVC chart
  var pvcBound = {{.Kasten.PVCs.Bound}};
  var pvcPending = {{.Kasten.PVCs.Pending}};
  var pvcLost = {{.Kasten.PVCs.Lost}};
  if((pvcBound+pvcPending+pvcLost) > 0 && document.getElementById("pvcChart")) {
    new Chart(document.getElementById("pvcChart"), {
      type: "doughnut",
      data: {
        labels: ["Bound","Pending","Lost"],
        datasets: [{data:[pvcBound,pvcPending,pvcLost], backgroundColor:["#3fb950","#d29922","#f85149"], borderWidth:0, hoverOffset:4}]
      },
      options: {
        responsive:true, maintainAspectRatio:false, cutout:"60%",
        plugins:{legend:{position:"right",labels:{font:{size:11},color:"#8b949e",boxWidth:10}}}
      }
    });
  } else if(document.getElementById("pvcChart")) {
    document.getElementById("pvcChart").parentElement.innerHTML =
      '<div style="height:160px;display:flex;align-items:center;justify-content:center;color:#8b949e;font-size:12px">No PVCs found in user namespaces</div>';
  }

  // Actions by type donut
  var actByType = JSON.parse('{{actionsByTypeJSON .Kasten.JobSummary.ByAction}}');
  var actLabels = Object.keys(actByType);
  var actVals = actLabels.map(function(k){return actByType[k];});
  // Sanity check: labels should be action names (run/backup/export/restore), not numbers
  var validAct = actLabels.length > 0 && actLabels.length <= 10 &&
    actLabels.every(function(l){return isNaN(parseInt(l));});
  if(validAct && document.getElementById("actionTypeChart")) {
    new Chart(document.getElementById("actionTypeChart"), {
      type: "doughnut",
      data: {
        labels: actLabels,
        datasets: [{
          data: actVals,
          backgroundColor: ["#58a6ff","#3fb950","#ffa657","#bc8cff","#f85149","#888780"],
          borderWidth:0, hoverOffset:4
        }]
      },
      options: {
        responsive:true, maintainAspectRatio:false, cutout:"55%",
        plugins:{
          legend:{position:"right",labels:{font:{size:11},color:"#8b949e",boxWidth:12,
            padding:6,generateLabels:function(chart){
              return chart.data.labels.map(function(label,i){
                return {text:label+" ("+chart.data.datasets[0].data[i]+")",
                        fillStyle:chart.data.datasets[0].backgroundColor[i],
                        strokeStyle:chart.data.datasets[0].backgroundColor[i],
                        lineWidth:0,index:i};
              });
            }
          }},
          tooltip:{callbacks:{label:function(c){return " "+c.label+": "+c.raw+" actions";}}}
        }
      }
    });
  } else if(document.getElementById("actionTypeChart")) {
    // Fallback: show text summary if chart data is invalid
    var el = document.getElementById("actionTypeChart").parentElement;
    var total = Object.values(actByType).reduce(function(a,b){return a+b;},0);
    var html = Object.keys(actByType).map(function(k){
      return "<div style=\"padding:4px 0;font-size:12px\"><span style=\"color:var(--blue)\">"+k+"</span>: "+actByType[k]+"</div>";
    }).join("");
    el.innerHTML = html || "<div style=\"color:var(--tm);font-size:12px\">No action data</div>";
  }

  // Job status percentages
  var total = {{.Kasten.JobSummary.Total}};
  if(total > 0) {
    var stats = {
      "complete": {{.Kasten.JobSummary.Completed}},
      "failed":   {{.Kasten.JobSummary.Failed}},
      "skipped":  {{.Kasten.JobSummary.Skipped}},
      "cancelled":{{.Kasten.JobSummary.Cancelled}}
    };
    Object.keys(stats).forEach(function(k){
      var el = document.getElementById("jspct-"+k);
      if(el) el.textContent = Math.round(stats[k]/total*100)+"%";
    });
  }
})();
</script>

<!-- ══ STORAGECLASS / VSC INVENTORY ══ -->
{{if or .Kasten.StorageClasses .Kasten.VolumeSnapshotClasses}}
<div class="sec" id="sc-vsc-inventory">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(63,185,80,.1)">💿</div><h2>StorageClass &amp; VolumeSnapshotClass Inventory</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">CSI cross-check: each StorageClass using a CSI provisioner must have a matching VolumeSnapshotClass for Kasten to use CSI snapshots. Missing VSC means PVCs on that class need a Kanister Blueprint or Generic Volume Backup.</span></span></div>
  {{if .Kasten.CSIWarnings}}
  <div style="margin-bottom:12px">{{range .Kasten.CSIWarnings}}<div class="pill warn" style="margin-bottom:4px;display:block;font-size:11px;border-radius:4px;padding:4px 8px">⚠️ {{.}}</div>{{end}}</div>
  {{end}}
  {{if .Kasten.StorageClasses}}
  <p style="font-size:12px;font-weight:600;margin:8px 0 4px;color:var(--t2)">STORAGECLASSES</p>
  <div class="twrap">
    <table><thead><tr><th>Name</th><th>Provisioner</th><th>Default</th><th>Expandable</th><th>Reclaim</th><th>VSC</th></tr></thead>
    <tbody>{{range .Kasten.StorageClasses}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="mono muted" style="font-size:11px">{{.Provisioner}}</td>
      <td style="text-align:center">{{if .IsDefault}}<span class="pill ok">yes</span>{{end}}</td>
      <td style="text-align:center">{{if .Expandable}}<span class="pill ok">yes</span>{{else}}<span class="pill muted">no</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .ReclaimPolicy}}</td>
      <td style="text-align:center">{{if .HasVSC}}<span class="pill ok">✅</span>{{else if .Provisioner}}<span class="pill warn">⚠️</span>{{else}}<span class="muted">—</span>{{end}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{end}}
  {{if .Kasten.VolumeSnapshotClasses}}
  <p style="font-size:12px;font-weight:600;margin:16px 0 4px;color:var(--t2)">VOLUMESNAPSHOTCLASSES</p>
  <div class="twrap">
    <table><thead><tr><th>Name</th><th>Driver</th><th>Default</th><th>Deletion Policy</th></tr></thead>
    <tbody>{{range .Kasten.VolumeSnapshotClasses}}<tr>
      <td class="mono">{{.Name}}</td>
      <td class="mono muted" style="font-size:11px">{{.Driver}}</td>
      <td style="text-align:center">{{if .IsDefault}}<span class="pill ok">yes</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .DeletionPolicy}}</td>
    </tr>{{end}}</tbody></table>
  </div>
  {{end}}
</div>
{{end}}
</div><!-- /tab-storage -->

<div class="tabpanel" id="tab-config">

<!-- ══ BLUEPRINTS & TRANSFORMS ══ -->
<div class="sec" id="blueprints">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(255,166,87,.1)">🔧</div><h2>Kanister Blueprints &amp; TransformSets</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box"><strong>Blueprints</strong>: custom backup/restore logic for specific applications (databases, stateful apps). <strong>TransformSets</strong>: rules that modify Kubernetes resources during restore (namespace, image registry, storage class).</span></span></div>
  <div style="display:grid;grid-template-columns:2fr 1fr;gap:20px">
    <div>
      <div style="font-size:12px;font-weight:600;color:var(--tm);margin-bottom:8px;text-transform:uppercase;letter-spacing:.5px">Blueprints ({{len .Kasten.Blueprints}})</div>
      {{if .Kasten.Blueprints}}
      <div class="twrap tscroll">
        <table><thead><tr><th>Name</th><th>Namespace</th><th>Actions</th></tr></thead>
        <tbody>{{range .Kasten.Blueprints}}<tr>
          <td class="mono">{{.Name}}</td>
          <td class="mono muted">{{.Namespace}}</td>
          <td class="mono muted" style="font-size:11px">{{join .Actions ", "}}</td>
        </tr>{{end}}</tbody></table>
      </div>
      {{else}}<div class="twrap"><div class="empty">No blueprints found</div></div>{{end}}
    </div>
    <div>
      <div style="font-size:12px;font-weight:600;color:var(--tm);margin-bottom:8px;text-transform:uppercase;letter-spacing:.5px">TransformSets ({{len .Kasten.TransformSets}})</div>
      {{if .Kasten.TransformSets}}
      <div class="twrap tscroll">
        <table><thead><tr><th>Name</th><th>Transforms</th></tr></thead>
        <tbody>{{range .Kasten.TransformSets}}<tr>
          <td class="mono">{{.Name}}</td>
          <td>{{.Transforms}}</td>
        </tr>{{end}}</tbody></table>
      </div>
      {{else}}<div class="twrap"><div class="empty">No TransformSets found</div></div>{{end}}
    </div>
  </div>
  {{if .Kasten.Bindings}}
  <div style="margin-top:12px">
    <div style="font-size:12px;font-weight:600;color:var(--tm);margin-bottom:8px;text-transform:uppercase;letter-spacing:.5px">Blueprint Bindings ({{len .Kasten.Bindings}})</div>
    <div class="twrap tscroll">
      <table><thead><tr><th>Name</th><th>Blueprint</th><th>Subject</th></tr></thead>
      <tbody>{{range .Kasten.Bindings}}<tr>
        <td class="mono">{{.Name}}</td>
        <td class="mono muted">{{.Blueprint}}</td>
        <td class="mono muted" style="font-size:11px">{{.Subject}}</td>
      </tr>{{end}}</tbody></table>
    </div>
  </div>{{end}}
</div>


</div><!-- /tab-config -->
<!-- tab-statistics injected by init() -->

<div class="tabpanel" id="tab-diagnostics">


<!-- ══ FAILED ACTIONS TOP-5 ══ -->
{{if .Kasten.RecentFailures}}
<div class="sec" id="recent-failures">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(248,81,73,.1)">🔴</div><h2>Recent Failures</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">The 5 most recent failed actions across BackupAction, ExportAction, and RestoreAction. The error shown is the deepest cause in the K10 error chain. <strong>Policy may be empty</strong> for actions triggered by a DR or system RunAction — this is normal in K10 8.x.</span></span><span class="sec-count">{{len .Kasten.RecentFailures}} failures</span></div>
  <div class="twrap">
    <table><thead><tr><th>Kind</th><th>App / Namespace</th><th>Policy</th><th>When</th><th>Error</th></tr></thead>
    <tbody>{{range .Kasten.RecentFailures}}<tr>
      <td><span class="pill warn" style="font-size:10px">{{.Kind}}</span></td>
      <td class="mono">{{orDash .AppName}}</td>
      <td class="muted mono" style="font-size:11px">{{orDash .PolicyName}}</td>
      <td class="mono muted" style="font-size:11px;white-space:nowrap">{{formatTimeShort .StartTime}}</td>
      <td style="font-size:11px;color:var(--red);max-width:320px;word-break:break-word">{{orDash .Error}}</td>
    </tr>{{end}}</tbody></table>
  </div>
</div>
{{end}}

<!-- ══ FAILURES BY POLICY ══ -->
{{if .Kasten.FailuresByPolicy}}
<div class="sec" id="failures-by-policy">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(248,81,73,.1)">📉</div><h2>Failures by Policy</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Failed job runs aggregated per policy, ordered by failure count. Use it to spot which policy generates the most failures and its most recent error.</span></span><span class="sec-count">{{len .Kasten.FailuresByPolicy}} policies</span></div>
  <div class="twrap">
    <table><thead><tr><th>Policy</th><th>Failed</th><th>Last Failure</th><th>Last Error</th></tr></thead>
    <tbody>{{range .Kasten.FailuresByPolicy}}<tr>
      <td class="mono" style="font-size:11px">{{.PolicyName}}</td>
      <td style="text-align:center"><span class="pill warn">{{.FailedCount}}</span></td>
      <td class="mono muted" style="font-size:11px;white-space:nowrap">{{formatTimeShort .LastFailure}}</td>
      <td style="font-size:11px;color:var(--red);max-width:320px;word-break:break-word">{{orDash .LastError}}</td>
    </tr>{{end}}</tbody></table>
  </div>
</div>
{{end}}

<!-- ══ STUCK ACTIONS ══ -->
{{if .Kasten.LongRunningActions}}
<div class="sec" id="long-running-actions">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(255,184,0,.1)">⏳</div><h2>Long-running Actions</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Actions in Running state for more than 24 hours. Almost always a hung Kanister job or a kubectl exec that never returned. Investigate the action pod logs.</span></span><span class="sec-count" style="color:var(--yellow)">{{len .Kasten.LongRunningActions}} stuck</span></div>
  <div class="twrap">
    <table><thead><tr><th>Kind</th><th>Name</th><th>App</th><th>Policy</th><th>Started</th><th>Running For</th></tr></thead>
    <tbody>{{range .Kasten.LongRunningActions}}<tr>
      <td><span class="pill warn" style="font-size:10px">{{.Kind}}</span></td>
      <td class="mono" style="font-size:11px">{{.Name}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .AppName}}</td>
      <td class="mono muted" style="font-size:11px">{{orDash .PolicyName}}</td>
      <td class="mono muted" style="font-size:11px;white-space:nowrap">{{formatTimeShort .StartTime}}</td>
      <td><span class="pill warn">{{.RunningFor}}</span></td>
    </tr>{{end}}</tbody></table>
  </div>
</div>
{{end}}

<!-- ══ NAMESPACE PROTECTION STATUS ══ -->
{{if .Kasten.BackupRecency}}
<div class="sec" id="backup-recency">
  <div class="sec-hdr"><div class="sec-icon" style="background:rgba(31,111,235,.1)">🗂️</div><h2>Backup Recency per Namespace</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Per-namespace view of last successful backup, export, and restore. <strong>Drift</strong> means a protected namespace has not had a successful backup in more than 7 days — it is covered by a policy but may not be recoverable.</span></span><span class="sec-count">{{len .Kasten.BackupRecency}} namespaces</span></div>
  <div class="twrap">
    <table><thead><tr><th>Namespace</th><th>Protected</th><th>Last Backup</th><th>Days Since</th><th>Last Export</th><th>Drift</th></tr></thead>
    <tbody>{{range .Kasten.BackupRecency}}<tr>
      <td class="mono">{{.Namespace}}</td>
      <td style="text-align:center">{{if .Protected}}<span class="pill ok">yes</span>{{else}}<span class="pill muted">no</span>{{end}}</td>
      <td class="mono muted" style="font-size:11px;white-space:nowrap">{{formatTimeShort .LastBackup}}</td>
      <td class="mono" style="font-size:11px;text-align:center;{{if and .Drift .Protected}}color:var(--red){{end}}">{{if .DaysSinceBackup}}{{.DaysSinceBackup}}d{{else}}—{{end}}</td>
      <td class="mono muted" style="font-size:11px;white-space:nowrap">{{formatTimeShort .LastExport}}</td>
      <td style="text-align:center">{{if and .Drift .Protected}}<span class="pill warn">⚠️ drift</span>{{end}}</td>
    </tr>{{end}}</tbody></table>
  </div>
</div>
{{end}}




</div><!-- /tab-diagnostics -->

<!-- Footer -->
<div class="footer">
  <div class="footer-l">Kasten Inspector v{{.ToolVersion}} · Veeam Kasten · <span style="color:var(--tm);font-style:italic">Independent tool — not an official Veeam product. Use at your own risk. See DISCLAIMER.md.</span></div>
  <div class="footer-r">{{generatedAt}} · {{.Cluster.Name}}</div>
</div>

</div>

<script>
(function(){
  var btns = document.querySelectorAll('.tablink');
  var panels = document.querySelectorAll('.tabpanel');
  btns.forEach(function(btn){
    btn.addEventListener('click', function(){
      var target = this.getAttribute('data-tab');
      btns.forEach(function(b){ b.classList.remove('active'); });
      panels.forEach(function(p){ p.classList.remove('active'); });
      this.classList.add('active');
      var panel = document.getElementById(target);
      if(panel) {
        panel.classList.add('active');
        // Charts created while their tab was hidden (display:none) may have
        // measured a zero-size canvas. Now that the panel is visible, force any
        // Chart.js instances inside it to recompute their size and redraw.
        if(window.Chart && Chart.getChart) {
          panel.querySelectorAll('canvas').forEach(function(cv){
            var ch = Chart.getChart(cv);
            if(ch) { ch.resize(); }
          });
        }
      }
      window.scrollTo(0,0);
      // Re-apply job filter when switching to Operations tab (preserves state)
      if(target === 'tab-operations' && typeof applyCurrentFilter === 'function') {
        applyCurrentFilter();
      }
    });
  });
})();
</script>
<script>
// ── Job filter (unified state: time + status + policy are independent) ─────────
// mode: 'preset' (uses days), 'today', or 'range' (uses from/to).
// status and policy are read live from their selects on every apply, so
// changing one filter never resets the others.
var jobFilter = { mode: 'preset', days: 7, from: null, to: null };

function fmtDay(ms){
  var d = new Date(ms);
  return d.toISOString().slice(0,10);
}

function applyCurrentFilter() {
  var statusFilter = (document.getElementById('job-status-filter')||{}).value || '';
  var policyFilter = (document.getElementById('job-policy-filter')||{}).value || '';
  var now = Date.now();
  var shown = 0, total = 0;
  // Precompute the active time window [lo, hi].
  var lo = 0, hi = now;
  if(jobFilter.mode === 'range') {
    lo = jobFilter.from ? new Date(jobFilter.from).getTime() : 0;
    hi = jobFilter.to   ? new Date(jobFilter.to).getTime() + 86399999 : now;
  } else if(jobFilter.mode === 'today') {
    var ts = new Date(); ts.setHours(0,0,0,0); lo = ts.getTime();
  } else { // preset
    lo = jobFilter.days > 0 ? now - jobFilter.days * 86400000 : 0;
  }
  document.querySelectorAll('tr.job-row').forEach(function(row){
    total++;
    var start  = row.getAttribute('data-start')  || '';
    var status = row.getAttribute('data-status') || '';
    var policy = row.getAttribute('data-policy') || '';
    var t = 0;
    if(start) { try { t = new Date(start.replace(' ', 'T')).getTime(); } catch(e) { t = 0; } }
    var inTime   = (lo === 0 && jobFilter.mode === 'preset') ? true : (t > 0 && t >= lo && t <= hi);
    var inStatus = statusFilter === '' || status === statusFilter;
    var inPolicy = policyFilter === '' || policy === policyFilter;
    var show = inTime && inStatus && inPolicy;
    row.style.display = show ? '' : 'none';
    if(show) shown++;
  });
  var lbl = document.getElementById('job-count-label');
  if(lbl) lbl.textContent = shown + ' of ' + total + ' jobs';
  updatePrintSummary(shown, total, statusFilter, policyFilter, lo, hi);
}

function updatePrintSummary(shown, total, status, policy, lo, hi) {
  var el = document.getElementById('print-filter-summary');
  if(!el) return;
  var parts = [];
  if(jobFilter.mode === 'range')      parts.push('Range: ' + (jobFilter.from||'…') + ' → ' + (jobFilter.to||'…'));
  else if(jobFilter.mode === 'today') parts.push('Range: today');
  else if(jobFilter.days > 0)         parts.push('Last ' + jobFilter.days + ' days');
  else                                parts.push('All time');
  parts.push('Status: ' + (status || 'all'));
  parts.push('Policy: ' + (policy || 'all'));
  el.textContent = 'Recent Jobs — ' + parts.join('  ·  ') + '  ·  ' + shown + ' of ' + total + ' jobs';
}

function filterJobs(days) {
  jobFilter = { mode: 'preset', days: days, from: null, to: null };
  var df = document.getElementById('job-date-from'); if(df) df.value = '';
  var dt = document.getElementById('job-date-to');   if(dt) dt.value = '';
  document.querySelectorAll('.job-filter-btn[data-days]').forEach(function(b){
    b.classList.toggle('active', parseInt(b.getAttribute('data-days'))===days && days !== -1);
  });
  applyCurrentFilter();
}

function filterJobsToday() {
  jobFilter = { mode: 'today', days: 0, from: null, to: null };
  var df = document.getElementById('job-date-from'); if(df) df.value = '';
  var dt = document.getElementById('job-date-to');   if(dt) dt.value = '';
  document.querySelectorAll('.job-filter-btn[data-days]').forEach(function(b){
    b.classList.toggle('active', b.getAttribute('data-days') === '-1');
  });
  applyCurrentFilter();
}

function filterJobsRange() {
  var from = (document.getElementById('job-date-from')||{}).value || '';
  var to   = (document.getElementById('job-date-to')||{}).value   || '';
  if(!from && !to) { filterJobs(jobFilter.days > 0 ? jobFilter.days : 7); return; }
  jobFilter = { mode: 'range', days: 0, from: from || null, to: to || null };
  document.querySelectorAll('.job-filter-btn[data-days]').forEach(function(b){ b.classList.remove('active'); });
  applyCurrentFilter();
}

function clearJobRange() {
  var df = document.getElementById('job-date-from'); if(df) df.value = '';
  var dt = document.getElementById('job-date-to');   if(dt) dt.value = '';
  filterJobs(7);
}

function exportJobsPDF() {
  applyCurrentFilter();
  window.print();
}

// Populate the policy dropdown from the distinct policy names present in the table.
function populatePolicyFilter() {
  var sel = document.getElementById('job-policy-filter');
  if(!sel) return;
  var seen = {};
  document.querySelectorAll('tr.job-row').forEach(function(row){
    var p = (row.getAttribute('data-policy') || '').trim();
    if(p) seen[p] = true;
  });
  Object.keys(seen).sort().forEach(function(p){
    var o = document.createElement('option');
    o.value = p; o.textContent = p;
    sel.appendChild(o);
  });
}

// Init count on load
window.addEventListener('DOMContentLoaded', function(){
  populatePolicyFilter();
  filterJobs(7);
  filterReports(0);
});

// ── K10 Reports time filter ───────────────────────────────────────────────────
function filterReports(days) {
  document.querySelectorAll('[data-rptdays]').forEach(function(b){
    b.classList.toggle('active', parseInt(b.getAttribute('data-rptdays'))===days);
  });
  var now = Date.now();
  var todayStart = new Date(); todayStart.setHours(0,0,0,0);
  var shown = 0, total = 0;
  document.querySelectorAll('tr.rpt-row').forEach(function(row){
    total++;
    var d = row.getAttribute('data-rptdate') || '';
    var t = d ? new Date(d.replace(' ', 'T')).getTime() : 0;
    var show;
    if(days === -1) {
      show = t >= todayStart.getTime();
    } else {
      var cutoff = days > 0 ? now - days * 86400000 : 0;
      show = cutoff === 0 || (t > 0 && t >= cutoff);
    }
    row.style.display = show ? '' : 'none';
    if(show) shown++;
  });
  var lbl = document.getElementById('rpt-count');
  if(lbl) lbl.textContent = shown + ' of ' + total + ' reports';
}

// ── Restore Point Detail time filter ───────────────────────────────────────────
function filterRPDetail(days) {
  document.querySelectorAll('[data-rpddays]').forEach(function(b){
    b.classList.toggle('active', parseInt(b.getAttribute('data-rpddays'))===days);
  });
  var now = Date.now();
  var todayStart = new Date(); todayStart.setHours(0,0,0,0);
  var shown = 0, total = 0;
  document.querySelectorAll('tr.rpd-row').forEach(function(row){
    total++;
    var d = row.getAttribute('data-rpddate') || '';
    var t = d ? new Date(d.replace(' ', 'T')).getTime() : 0;
    var show;
    if(days === -1) {
      show = t >= todayStart.getTime();
    } else {
      var cutoff = days > 0 ? now - days * 86400000 : 0;
      show = cutoff === 0 || (t > 0 && t >= cutoff);
    }
    row.style.display = show ? '' : 'none';
    if(show) shown++;
  });
  var lbl = document.getElementById('rpd-count');
  if(lbl) lbl.textContent = shown + ' of ' + total + ' points';
}

// ── Job Execution Trend (client-side bucketing: day / week / month) ─────────────
var jobTrendChart = null;
var jobTrendGran = 'day';

function jtPad(n){ return (n<10?'0':'')+n; }
function jtBucketStart(d, gran){
  if(gran === 'day'){ var x=new Date(d); x.setHours(0,0,0,0); return x.getTime(); }
  if(gran === 'week'){ var x=new Date(d); var dow=(x.getDay()+6)%7; x.setHours(0,0,0,0); x.setDate(x.getDate()-dow); return x.getTime(); }
  return new Date(d.getFullYear(), d.getMonth(), 1).getTime();
}
function jtLabel(ts, gran){
  var d=new Date(ts);
  var mon=["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
  if(gran === 'month') return mon[d.getMonth()]+" "+d.getFullYear();
  return jtPad(d.getDate())+" "+mon[d.getMonth()];
}
function jobTrendBuckets(gran){
  var map = {};
  document.querySelectorAll('tr.job-row').forEach(function(row){
    var start = row.getAttribute('data-start') || '';
    var status = row.getAttribute('data-status') || '';
    if(!start) return;
    var d = new Date(start.replace(' ', 'T'));
    if(isNaN(d.getTime())) return;
    var key = String(jtBucketStart(d, gran));
    var b = map[key] || (map[key] = {c:0, f:0, s:0});
    if(status === 'Complete' || status === 'Success') b.c++;
    else if(status === 'Failed' || status === 'Error') b.f++;
    else if(status === 'Skipped') b.s++;
  });
  return map;
}
function renderJobTrend(){
  var el = document.getElementById('jobTrendChart');
  if(!el || typeof Chart === 'undefined') return;
  var map = jobTrendBuckets(jobTrendGran);
  var keys = Object.keys(map).sort(function(a,b){ return parseInt(a)-parseInt(b); });
  var labels = keys.map(function(k){ return jtLabel(parseInt(k), jobTrendGran); });
  var c=[], f=[], s=[];
  keys.forEach(function(k){ c.push(map[k].c); f.push(map[k].f); s.push(map[k].s); });
  var data = { labels: labels.length ? labels : ["No data"], datasets:[
    {label:"Complete", data:c, backgroundColor:"#3fb950", stack:"s"},
    {label:"Failed",   data:f, backgroundColor:"#f85149", stack:"s"},
    {label:"Skipped",  data:s, backgroundColor:"#888780", stack:"s"}
  ]};
  if(jobTrendChart){ jobTrendChart.data = data; jobTrendChart.update(); }
  else {
    jobTrendChart = new Chart(el, {type:"bar", data:data, options:{responsive:true, maintainAspectRatio:false,
      scales:{
        x:{stacked:true, grid:{display:false}, ticks:{autoSkip:true, maxRotation:45, font:{size:10}}},
        y:{stacked:true, grid:{color:"rgba(255,255,255,.06)"}, border:{display:false}, ticks:{precision:0}}
      },
      plugins:{
        legend:{display:true, position:"bottom", labels:{font:{size:10}, boxWidth:10, color:"#8b949e", padding:8}},
        tooltip:{callbacks:{ label:function(ctx){ return " "+ctx.dataset.label+": "+ctx.raw+" job(s)"; } }}
      }
    }});
  }
  var lbl = document.getElementById('jobtrend-count');
  if(lbl) lbl.textContent = keys.length + ' ' + jobTrendGran + (keys.length===1?'':'s');
}
function setJobTrendGran(g){
  jobTrendGran = g;
  document.querySelectorAll('.job-filter-btn[data-gran]').forEach(function(b){
    b.classList.toggle('active', b.getAttribute('data-gran') === g);
  });
  renderJobTrend();
}
window.addEventListener('DOMContentLoaded', function(){
  filterRPDetail(0);
  renderJobTrend();
});
</script>
</body>
</html>`

func init() {
	htmlTmpl = injectStatistics(htmlTmpl)
}

func injectStatistics(tmpl string) string {
	// 1. Inject CSS before closing </style>
	tmpl = replaceOnce(tmpl, `</style>`, statCSS+`</style>`)

	// 2. Fill the statistics tab panel placeholder
	tmpl = replaceOnce(tmpl,
		"<!-- tab-statistics injected by init() -->",
		"<div class=\"tabpanel\" id=\"tab-statistics\">\n"+statSection+"\n</div>")

	return tmpl
}

func replaceOnce(s, old, new string) string {
	idx := strings.Index(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

var statCSS = `
/* ── Statistics section ── */
.stat-kpi-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(148px,1fr));gap:10px;margin-bottom:16px}
.stat-kpi{background:var(--s2);border:1px solid var(--b);border-radius:8px;padding:14px 16px}
.stat-kpi-label{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:6px}
.stat-kpi-val{font-size:26px;font-weight:700;font-family:'IBM Plex Mono',monospace;line-height:1}
.stat-kpi-sub{font-size:11px;color:var(--tm);margin-top:4px}
.stat-charts-3{display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px;margin-bottom:12px}
.stat-charts-2{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px}
.stat-chart-card{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px}
.stat-chart-full{background:var(--s1);border:1px solid var(--b);border-radius:10px;padding:16px;margin-bottom:12px}
.stat-chart-title{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--tm);margin-bottom:10px}
.stat-legend{display:flex;flex-wrap:wrap;gap:12px;margin-bottom:8px;font-size:11px;color:var(--tm)}
.stat-leg{display:flex;align-items:center;gap:5px}
.stat-leg-dot{width:9px;height:9px;border-radius:2px;flex-shrink:0}
.stat-score-row{margin:7px 0}
.stat-score-labels{display:flex;justify-content:space-between;font-size:11px;color:var(--tm);margin-bottom:3px}
.stat-score-bg{background:var(--b);border-radius:3px;height:5px;overflow:hidden}
.stat-score-fill{height:100%;border-radius:3px}
.stat-readiness{margin-top:12px;padding:10px 12px;background:var(--s2);border-radius:8px;border:1px solid var(--b)}
.stat-readiness-label{font-size:10px;color:var(--tm)}
.stat-readiness-val{font-size:22px;font-weight:700;font-family:'IBM Plex Mono',monospace;margin-top:2px}
.stat-readiness-sub{font-size:10px;color:var(--tm);margin-top:2px}
.stat-actions{display:flex;flex-direction:column;gap:7px;margin-bottom:12px}
.stat-action{display:flex;align-items:flex-start;gap:10px;padding:11px 14px;background:var(--s1);border:1px solid var(--b);border-radius:8px}
.stat-action-icon{font-size:15px;min-width:20px;margin-top:1px}
.stat-action-text{flex:1;font-size:12px;line-height:1.5}
.stat-action-pri{font-size:10px;font-weight:600;padding:2px 7px;border-radius:20px;white-space:nowrap;flex-shrink:0;margin-top:1px}
.stat-pri-critical{background:rgba(248,81,73,.15);color:#f85149;border:1px solid rgba(248,81,73,.3)}
.stat-pri-high{background:rgba(210,153,34,.15);color:#d29922;border:1px solid rgba(210,153,34,.3)}
.stat-pri-medium{background:rgba(88,166,255,.15);color:#58a6ff;border:1px solid rgba(88,166,255,.3)}
.stat-pri-low{background:rgba(139,148,158,.1);color:var(--tm);border:1px solid rgba(139,148,158,.2)}
.stat-ns-tags{display:flex;flex-wrap:wrap;gap:5px;margin-top:6px}
.stat-ns-tag{font-family:'IBM Plex Mono',monospace;font-size:10px;padding:2px 7px;border-radius:4px;background:rgba(248,81,73,.1);color:#f85149;border:1px solid rgba(248,81,73,.25)}
.stat-bp-grid{display:grid;grid-template-columns:1fr 1fr;gap:7px}
.stat-bp-row{display:flex;align-items:flex-start;gap:8px;padding:10px 12px;background:var(--s1);border-radius:7px;border-left:3px solid transparent}
.stat-bp-row.ok{border-left-color:#3fb950}
.stat-bp-row.warn{border-left-color:#d29922}
.stat-bp-row.fail{border-left-color:#f85149}
.stat-bp-id{font-family:'IBM Plex Mono',monospace;font-size:10px;color:var(--tm);min-width:38px;padding-top:2px}
.stat-bp-body{flex:1}
.stat-bp-name{font-size:11px;font-weight:600;color:var(--t)}
.stat-bp-detail{font-size:11px;color:var(--tm);margin-top:2px}
.stat-bp-badge{font-size:10px;font-weight:600;padding:2px 7px;border-radius:20px;flex-shrink:0}
.stat-bp-badge.ok{background:rgba(63,185,80,.12);color:#3fb950}
.stat-bp-badge.warn{background:rgba(210,153,34,.12);color:#d29922}
.stat-bp-badge.fail{background:rgba(248,81,73,.12);color:#f85149}
@media(max-width:900px){.stat-charts-3{grid-template-columns:1fr 1fr}.stat-charts-2{grid-template-columns:1fr}.stat-bp-grid{grid-template-columns:1fr}}
@media(max-width:600px){.stat-charts-3{grid-template-columns:1fr}}
`

var statSection = `<!-- ══ STATISTICS ══ -->
<div class="sec" id="statistics">
  <div class="sec-hdr">
    <div class="sec-icon" style="background:rgba(88,166,255,.1)">📈</div>
    <h2>Statistics &amp; QBR Dashboard</h2><span class="tip-wrap"><span class="tip-btn" tabindex="0">?</span><span class="tip-box">Aggregated metrics and charts for a Quarterly Business Review. Use <strong>Actions Required</strong> as your customer meeting agenda &mdash; items are sorted by business impact.</span></span>
    <span class="sec-count">data protection posture · quarterly business review</span>
  </div>

  <!-- KPI row -->
  <div class="stat-kpi-grid">
    <div class="stat-kpi">
      <div class="stat-kpi-label">Namespace coverage</div>
      <div class="stat-kpi-val" id="s-ns-pct" style="color:#f85149">—</div>
      <div class="stat-kpi-sub" id="s-ns-sub">— user namespaces</div>
    </div>
    <div class="stat-kpi">
      <div class="stat-kpi-label">Job success rate</div>
      <div class="stat-kpi-val" id="s-job-pct" style="color:#d29922">—</div>
      <div class="stat-kpi-sub" id="s-job-sub">— / — complete</div>
    </div>
    <div class="stat-kpi">
      <div class="stat-kpi-label">Active policies</div>
      <div class="stat-kpi-val" id="s-pol-val" style="color:#58a6ff">—</div>
      <div class="stat-kpi-sub" id="s-pol-sub">user-defined</div>
    </div>
    <div class="stat-kpi">
      <div class="stat-kpi-label">Location profiles</div>
      <div class="stat-kpi-val" id="s-prof-val" style="color:#58a6ff">—</div>
      <div class="stat-kpi-sub" id="s-prof-sub">— immutable</div>
    </div>
    <div class="stat-kpi">
      <div class="stat-kpi-label">Restore points</div>
      <div class="stat-kpi-val" id="s-rp-val" style="color:#3fb950">—</div>
      <div class="stat-kpi-sub" id="s-rp-sub">— orphaned</div>
    </div>
    <div class="stat-kpi">
      <div class="stat-kpi-label">BP readiness</div>
      <div class="stat-kpi-val" id="s-bp-pct" style="color:#d29922">—</div>
      <div class="stat-kpi-sub" id="s-bp-sub">— / 11 checks passed</div>
    </div>
  </div>

  <!-- Row 1: 3 charts -->
  <div class="stat-charts-3">
    <div class="stat-chart-card">
      <div class="stat-chart-title">Namespace protection</div>
      <div class="stat-legend" id="s-ns-legend"></div>
      <div style="position:relative;height:160px"><canvas id="sc1" role="img" aria-label="Donut: namespace protection breakdown">Namespace protection breakdown.</canvas></div>
    </div>
    <div class="stat-chart-card">
      <div class="stat-chart-title">Job outcome breakdown</div>
      <div class="stat-legend" id="s-job-legend"></div>
      <div style="position:relative;height:160px"><canvas id="sc2" role="img" aria-label="Donut: job outcome breakdown">Job outcome breakdown.</canvas></div>
    </div>
    <div class="stat-chart-card">
      <div class="stat-chart-title">Best practices score</div>
      <div id="s-bp-bars"></div>
      <div class="stat-readiness">
        <div class="stat-readiness-label">Overall readiness</div>
        <div class="stat-readiness-val" id="s-readiness-val" style="color:#d29922">—</div>
        <div class="stat-readiness-sub">target ≥ 80% for production</div>
      </div>
    </div>
  </div>

  <!-- Row 2: full-width job trend -->
  <div class="stat-chart-full">
    <div class="stat-chart-title">Job history — monthly trend</div>
    <div style="font-size:10px;color:var(--tm);margin:-6px 0 10px">Number of K10 jobs per month, grouped by outcome (green=complete, red=failed, grey=skipped). Hover a bar for details. Run Inspector monthly to build history.</div>
    <div class="stat-legend">
      <span class="stat-leg"><span class="stat-leg-dot" style="background:#3fb950"></span>Complete</span>
      <span class="stat-leg"><span class="stat-leg-dot" style="background:#f85149"></span>Failed</span>
      <span class="stat-leg"><span class="stat-leg-dot" style="background:#888780"></span>Skipped</span>
    </div>
    <div style="position:relative;height:200px"><canvas id="sc3" role="img" aria-label="Stacked bar: monthly job outcomes">Monthly job outcomes.</canvas></div>
  </div>

  <!-- Row 3: radar + limiters -->
  <div class="stat-charts-2">
    <div class="stat-chart-card">
      <div class="stat-chart-title">Security posture</div>
      <div class="stat-legend">
        <span class="stat-leg"><span class="stat-leg-dot" style="background:#d29922"></span>Current</span>
        <span class="stat-leg"><span class="stat-leg-dot" style="background:#3fb950;opacity:.5"></span>Target</span>
      </div>
      <div style="position:relative;height:220px"><canvas id="sc4" role="img" aria-label="Radar: security posture across 6 dimensions">Security posture radar.</canvas></div>
    </div>
    <div class="stat-chart-card">
      <div class="stat-chart-title">K10 concurrency limiters</div>
      <div style="font-size:10px;color:var(--tm);margin:-6px 0 8px;line-height:1.4">Max simultaneous operations K10 runs in parallel — limits prevent cluster overload</div>
      <div id="s-limiters-table" style="font-size:11px"></div>
    </div>
  </div>

  <!-- Actions required -->
  <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin:16px 0 10px">Actions required — quarterly review</div>
  <div class="stat-actions" id="s-actions"></div>

  <!-- BP detail -->
  <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin:16px 0 10px">Best practices detail</div>
  <div class="stat-bp-grid" id="s-bp-detail"></div>

  <!-- Recovery Readiness Score -->
  <div style="margin-top:28px">
    <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:14px">Recovery Readiness Score</div>
    <div id="rrsLayout" style="display:grid;grid-template-columns:200px 1fr;gap:28px;align-items:start">
      <!-- Score card -->
      <div style="background:var(--s2);border:1px solid var(--b);border-radius:12px;padding:20px 16px;text-align:center">
        <div style="font-size:11px;color:var(--tm);margin-bottom:8px">How recoverable is this cluster?</div>
        <div style="font-size:64px;font-weight:800;line-height:1;color:{{gradeColor .Kasten.RecoveryReadiness.Grade}}">{{.Kasten.RecoveryReadiness.Score}}</div>
        <div style="font-size:11px;color:var(--tm);margin:2px 0 12px">out of 100</div>
        <!-- Grade badge -->
        <div style="display:inline-block;background:{{gradeColor .Kasten.RecoveryReadiness.Grade}};color:#fff;font-size:22px;font-weight:800;padding:4px 20px;border-radius:8px;letter-spacing:1px">{{.Kasten.RecoveryReadiness.Grade}}</div>
        <!-- Grade scale -->
        <div style="display:flex;justify-content:space-around;margin-top:14px;padding-top:12px;border-top:1px solid var(--b)">
          {{$grade := .Kasten.RecoveryReadiness.Grade}}
          <div style="text-align:center;opacity:{{if eq $grade "F"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#f85149">F</div></div>
          <div style="text-align:center;opacity:{{if eq $grade "D"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#f85149">D</div></div>
          <div style="text-align:center;opacity:{{if eq $grade "C"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#ffa657">C</div></div>
          <div style="text-align:center;opacity:{{if eq $grade "B"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#58a6ff">B</div></div>
          <div style="text-align:center;opacity:{{if eq $grade "A"}}1{{else}}0.25{{end}}"><div style="font-size:14px;font-weight:700;color:#3fb950">A</div></div>
        </div>
        <div style="font-size:9px;color:var(--tm);margin-top:6px;line-height:1.5;text-align:left">
          <strong style="color:#3fb950">A</strong> ≥90 Excellent &nbsp;
          <strong style="color:#58a6ff">B</strong> ≥75 Good<br>
          <strong style="color:#ffa657">C</strong> ≥60 Fair &nbsp;&nbsp;
          <strong style="color:#f85149">D</strong> ≥40 Poor &nbsp;
          <strong style="color:#f85149">F</strong> &lt;40 Critical
        </div>
      </div>
      <div style="position:relative;min-height:200px">
        <canvas id="rrsChart" role="img" aria-label="Recovery readiness score breakdown" style="max-height:260px"></canvas>
      </div>
    </div>
    {{if .Kasten.RecoveryReadiness.Findings}}
    <div style="margin-top:14px">
      <div style="font-size:11px;font-weight:600;color:var(--tm);margin-bottom:8px">Gaps to address:</div>
      {{range .Kasten.RecoveryReadiness.Findings}}
      <div style="font-size:12px;color:var(--red);padding:4px 0;border-bottom:1px solid var(--b)">⚠ {{.}}</div>
      {{end}}
    </div>
    {{end}}
  </div>

  <!-- Weekly SLA Trend -->
  <div style="margin-top:28px">
    <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:14px">Backup success rate — weekly trend</div>
    {{if and .Kasten.WeeklySLATrend (gt (len .Kasten.WeeklySLATrend) 2)}}
    <div style="position:relative;height:180px">
      <canvas id="slaChart" role="img" aria-label="Weekly backup success rate trend"></canvas>
    </div>
    {{else}}
    <div style="padding:24px;text-align:center;color:var(--tm);font-size:12px;background:var(--s2);border-radius:8px;border:1px dashed var(--b)">
      📅 Insufficient historical data — run Inspector weekly to build a trend.<br>
      {{if .Kasten.WeeklySLATrend}}<span style="font-size:11px;margin-top:4px;display:inline-block">Current data: {{len .Kasten.WeeklySLATrend}} week(s)</span>{{end}}
    </div>
    {{end}}
  </div>

  <!-- App Risk Matrix -->
  {{if .Kasten.AppRiskMatrix}}
  <div style="margin-top:28px">
    <div style="font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--tm);margin-bottom:14px">Application risk matrix</div>
    <table style="width:100%;border-collapse:collapse;font-size:12px">
      <thead><tr style="border-bottom:1px solid var(--b)">
        <th style="text-align:left;padding:6px 8px;color:var(--tm);font-weight:500">App</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Risk</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">RPO now</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Est. RTO</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Export</th>
        <th style="padding:6px 8px;color:var(--tm);font-weight:500">Immutable</th>
        <th style="text-align:left;padding:6px 8px;color:var(--tm);font-weight:500">Notes</th>
      </tr></thead>
      <tbody>
      {{range .Kasten.AppRiskMatrix}}<tr style="border-bottom:1px solid var(--b)">
        <td style="padding:6px 8px;font-family:monospace">{{.Namespace}}</td>
        <td style="padding:6px 8px;text-align:center">{{riskIcon .RiskLevel}}</td>
        <td style="padding:6px 8px;text-align:center;color:{{if gt .RPOHours 168.0}}var(--red){{else if gt .RPOHours 24.0}}var(--yellow){{else}}var(--green){{end}}">
          {{if gt .RPOHours 0.0}}{{printf "%.0fh" .RPOHours}}{{else}}—{{end}}</td>
        <td style="padding:6px 8px;text-align:center;color:var(--tm)">
          {{if gt .RTOMinutes 0.0}}{{printf "~%.0fm" .RTOMinutes}}{{else}}—{{end}}</td>
        <td style="padding:6px 8px;text-align:center">{{if .HasExport}}✅{{else}}❌{{end}}</td>
        <td style="padding:6px 8px;text-align:center">{{if .HasImmutable}}✅{{else}}❌{{end}}</td>
        <td style="padding:6px 8px;color:var(--tm);font-size:11px">{{range .RiskReasons}}{{.}} {{end}}</td>
      </tr>{{end}}
      </tbody>
    </table>
  </div>
  {{end}}
</div>

<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.js"></script>
<script>
(function(){
  // ── Data from Go template ─────────────────────────────────────
  var D = {
    namespaces: {
      total: {{.Cluster.NamespaceCount}},
      unprotected: {{len .Kasten.Namespaces.Unprotected}},
      excluded: {{len .Kasten.Namespaces.Excluded}},
      unprotectedNames: [{{range .Kasten.Namespaces.Unprotected}}"{{.Name}}",{{end}}]
    },
    jobs: {
      total: {{len .Kasten.Jobs}},
      complete: 0, failed: 0, skipped: 0, other: 0,
      byMonth: {}
    },
    policies: {
      total: {{len .Kasten.Policies}},
      userPolicies: 0, withExport: 0
    },
    profiles: {
      total: {{len .Kasten.Profiles}},
      immutable: 0
    },
    restorePoints: {
      total: {{.Kasten.RestorePoints.Total}},
      orphaned: {{.Kasten.RestorePoints.Orphaned}}
    },
    bp: {
      total: {{.Kasten.BestPractices.TotalChecks}},
      passed: {{.Kasten.BestPractices.Passed}},
      warnings: {{.Kasten.BestPractices.Warnings}},
      critical: {{.Kasten.BestPractices.Critical}},
      checks: [{{range .Kasten.BestPractices.Checks}}{id:"{{.ID}}",name:{{jsStr .Name}},status:"{{.Status}}",detail:{{jsStr .Detail}}},{{end}}]
    },
    security: {
      auth: "{{.Kasten.Security.AuthMethod}}",
      encEnabled: {{if .Kasten.Security.Encryption.Enabled}}true{{else}}false{{end}},
      fips: {{if .Kasten.HelmConfig.FIPSMode}}true{{else}}false{{end}},
      audit: {{if .Kasten.HelmConfig.AuditLogging}}true{{else}}false{{end}},
      netpol: {{if .Kasten.HelmConfig.NetworkPolicies}}true{{else}}false{{end}},
      immutable: false
    },
    limiters: {
      "CSI snapshots/cluster":10,
      "Vol restores/cluster":10,
      "Snapshot exports/cluster":10,
      "Generic vol backups":10,
      "Image copies/cluster":10,
      "VM snapshots/cluster":1
    }
  };

  // Enrich from template data
  {{range .Kasten.Jobs}}
  (function(){
    var s = "{{.Status}}";
    var t = "{{.StartTime}}";
    if(s==="Complete"||s==="Success") D.jobs.complete++;
    else if(s==="Failed"||s==="Error") D.jobs.failed++;
    else if(s==="Skipped") D.jobs.skipped++;
    else D.jobs.other++;
    if(t && t.length>=7){
      var m=t.substring(0,7);
      if(!D.jobs.byMonth[m]) D.jobs.byMonth[m]={c:0,f:0,s:0};
      if(s==="Complete"||s==="Success") D.jobs.byMonth[m].c++;
      else if(s==="Failed"||s==="Error") D.jobs.byMonth[m].f++;
      else if(s==="Skipped") D.jobs.byMonth[m].s++;
    }
  })();
  {{end}}

  {{range .Kasten.Policies}}
  if(!{{if .IsSystemPolicy}}true{{else}}false{{end}}) D.policies.userPolicies++;
  {{end}}

  {{range .Kasten.Profiles}}
  if({{if .Immutability}}true{{else}}false{{end}}) { D.profiles.immutable++; D.security.immutable=true; }
  {{end}}

  {{range .Kasten.HelmConfig.Values}}{{end}}
  // Parse K10 limiter values from helm config
  var kvals = JSON.parse('{{helmLimiters .Kasten.HelmConfig.Values}}');
  if(kvals) D.limiters = kvals;

  // ── KPI fills ────────────────────────────────────────────────
  var userNS = D.namespaces.total - D.namespaces.excluded;
  var protectedNS = userNS - D.namespaces.unprotected;
  var nsPct = userNS > 0 ? Math.round(protectedNS/userNS*100) : 0;
  var jobPct = D.jobs.total > 0 ? Math.round(D.jobs.complete/D.jobs.total*100) : 0;
  var bpPct = D.bp.total > 0 ? Math.round(D.bp.passed/D.bp.total*100) : 0;

  function colorFor(v){ return v>=80?"#3fb950":v>=50?"#d29922":"#f85149"; }

  document.getElementById("s-ns-pct").textContent = nsPct+"%";
  document.getElementById("s-ns-pct").style.color = colorFor(nsPct);
  document.getElementById("s-ns-sub").textContent = protectedNS+" / "+userNS+" user NS";

  document.getElementById("s-job-pct").textContent = jobPct+"%";
  document.getElementById("s-job-pct").style.color = colorFor(jobPct);
  document.getElementById("s-job-sub").textContent = D.jobs.complete+" / "+D.jobs.total+" complete";

  document.getElementById("s-pol-val").textContent = D.policies.userPolicies;
  document.getElementById("s-pol-sub").textContent = "of "+D.policies.total+" total";

  document.getElementById("s-prof-val").textContent = D.profiles.total;
  document.getElementById("s-prof-sub").textContent = D.profiles.immutable+" immutable";

  document.getElementById("s-rp-val").textContent = D.restorePoints.total;
  document.getElementById("s-rp-sub").textContent = D.restorePoints.orphaned+" orphaned";

  document.getElementById("s-bp-pct").textContent = bpPct+"%";
  document.getElementById("s-bp-pct").style.color = colorFor(bpPct);
  document.getElementById("s-bp-sub").textContent = D.bp.passed+" / "+D.bp.total+" passed";

  document.getElementById("s-readiness-val").textContent = bpPct+"%";
  document.getElementById("s-readiness-val").style.color = colorFor(bpPct);

  // ── BP score bars ─────────────────────────────────────────────
  var barsHTML = [
    {label:"Passed", val:D.bp.passed, color:"#3fb950"},
    {label:"Warnings", val:D.bp.warnings, color:"#d29922"},
    {label:"Critical", val:D.bp.critical, color:"#f85149"}
  ].map(function(r){
    var pct = Math.round(r.val/D.bp.total*100);
    return '<div class="stat-score-row">'+
      '<div class="stat-score-labels"><span>'+r.label+'</span><span style="color:'+r.color+';font-weight:600">'+r.val+' / '+D.bp.total+'</span></div>'+
      '<div class="stat-score-bg"><div class="stat-score-fill" style="width:'+pct+'%;background:'+r.color+'"></div></div>'+
      '</div>';
  }).join('');
  document.getElementById("s-bp-bars").innerHTML = barsHTML;

  // ── NS legend ────────────────────────────────────────────────
  document.getElementById("s-ns-legend").innerHTML =
    '<span class="stat-leg"><span class="stat-leg-dot" style="background:#f85149"></span>Unprotected ('+D.namespaces.unprotected+')</span>'+
    '<span class="stat-leg"><span class="stat-leg-dot" style="background:#3fb950"></span>Protected ('+protectedNS+')</span>'+
    '<span class="stat-leg"><span class="stat-leg-dot" style="background:#888780"></span>System ('+D.namespaces.excluded+')</span>';

  document.getElementById("s-job-legend").innerHTML =
    '<span class="stat-leg"><span class="stat-leg-dot" style="background:#3fb950"></span>Complete ('+D.jobs.complete+')</span>'+
    '<span class="stat-leg"><span class="stat-leg-dot" style="background:#f85149"></span>Failed ('+D.jobs.failed+')</span>'+
    '<span class="stat-leg"><span class="stat-leg-dot" style="background:#888780"></span>Skipped ('+D.jobs.skipped+')</span>';

  // ── Actions ───────────────────────────────────────────────────
  var actions = [];
  var authBad = !D.security.auth || D.security.auth.indexOf("None")>=0 || D.security.auth.indexOf("Passthrough")>=0;
  if(authBad) actions.push({icon:"ti-shield-x",pri:"critical",color:"#f85149",text:"<strong>Enable authentication.</strong> The K10 dashboard has no auth configured — anyone with network access can manage backups. Configure OIDC, LDAP, or OpenShift OAuth."});
  if(!D.security.encEnabled) actions.push({icon:"ti-lock-open",pri:"critical",color:"#f85149",text:"<strong>Enable encryption at rest.</strong> No encryption provider is configured. Set up AWS KMS, Azure Key Vault, or HashiCorp Vault to protect backup data."});
  if(D.namespaces.unprotected>0){
    var tags = D.namespaces.unprotectedNames.map(function(n){return '<span class="stat-ns-tag">'+n+'</span>';}).join('');
    actions.push({icon:"ti-apps",pri:"high",color:"#d29922",text:"<strong>Create policies for "+D.namespaces.unprotected+" unprotected namespaces.</strong><div class='stat-ns-tags'>"+tags+"</div>"});
  }
  if(D.profiles.immutable===0) actions.push({icon:"ti-lock",pri:"high",color:"#d29922",text:"<strong>Enable object lock on at least one profile.</strong> No immutable storage configured. This is the primary defence against ransomware-based backup deletion."});
  var noLimits = {{noResourceLimits .Kasten.Resources.Deployments}};
  if(noLimits>0) actions.push({icon:"ti-cpu",pri:"medium",color:"#58a6ff",text:"<strong>Set resource limits on "+noLimits+" K10 containers.</strong> Missing CPU/memory limits can cause OOM kills and cluster instability under backup load."});
  var neverRun = 0;
  {{range .Kasten.Policies}}if(!{{if .IsSystemPolicy}}true{{else}}false{{end}} && "{{.LastRunTime}}"==="") neverRun++;{{end}}
  if(neverRun>0) actions.push({icon:"ti-player-play",pri:"low",color:"#888780",text:"<strong>Run "+neverRun+" policies that have never executed.</strong> Trigger a manual run to validate configuration and generate the first restore points."});

  var priClass = {critical:"stat-pri-critical",high:"stat-pri-high",medium:"stat-pri-medium",low:"stat-pri-low"};
  document.getElementById("s-actions").innerHTML = actions.map(function(a){
    return '<div class="stat-action">'+
      '<i class="action-icon ti '+a.icon+'" style="color:'+a.color+';font-size:15px;min-width:20px;margin-top:2px" aria-hidden="true"></i>'+
      '<div class="stat-action-text">'+a.text+'</div>'+
      '<span class="stat-action-pri '+priClass[a.pri]+'">'+a.pri+'</span>'+
      '</div>';
  }).join('');

  // ── BP detail ────────────────────────────────────────────────
  document.getElementById("s-bp-detail").innerHTML = D.bp.checks.map(function(c){
    var cls = c.status==="pass"?"ok":c.status==="warning"?"warn":"fail";
    return '<div class="stat-bp-row '+cls+'">'+
      '<div class="stat-bp-id">'+c.id+'</div>'+
      '<div class="stat-bp-body"><div class="stat-bp-name">'+c.name+'</div><div class="stat-bp-detail">'+c.detail+'</div></div>'+
      '<span class="stat-bp-badge '+cls+'">'+c.status+'</span>'+
      '</div>';
  }).join('');

  // ── Charts ───────────────────────────────────────────────────
  Chart.defaults.font.family = "'IBM Plex Sans', sans-serif";
  Chart.defaults.font.size = 11;
  Chart.defaults.color = "#8b949e";
  var grid = "rgba(139,148,158,0.12)";

  new Chart(document.getElementById("sc1"),{type:"doughnut",data:{
    labels:["Unprotected","Protected","System"],
    datasets:[{data:[D.namespaces.unprotected,protectedNS,D.namespaces.excluded],
      backgroundColor:["#f85149","#3fb950","#888780"],borderWidth:0,hoverOffset:4}]
  },options:{responsive:true,maintainAspectRatio:false,cutout:"62%",
    plugins:{legend:{display:false},tooltip:{callbacks:{label:function(c){return " "+c.label+": "+c.raw;}}}}}});

  new Chart(document.getElementById("sc2"),{type:"doughnut",data:{
    labels:["Complete","Failed","Skipped"],
    datasets:[{data:[D.jobs.complete,D.jobs.failed,D.jobs.skipped],
      backgroundColor:["#3fb950","#f85149","#888780"],borderWidth:0,hoverOffset:4}]
  },options:{responsive:true,maintainAspectRatio:false,cutout:"58%",
    plugins:{legend:{display:false},tooltip:{callbacks:{label:function(c){
      return " "+c.label+": "+c.raw+" ("+Math.round(c.raw/D.jobs.total*100)+"%)";
    }}}}}});

  // Monthly trend
  var months = Object.keys(D.jobs.byMonth).sort();
  var mc=[],mf=[],ms=[];
  months.forEach(function(m){mc.push(D.jobs.byMonth[m].c||0);mf.push(D.jobs.byMonth[m].f||0);ms.push(D.jobs.byMonth[m].s||0);});
  var labels3 = months.map(function(m){
    var parts=m.split("-");
    var names=["","Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
    return names[parseInt(parts[1])]+" "+parts[0];
  });
  new Chart(document.getElementById("sc3"),{type:"bar",data:{
    labels:labels3.length?labels3:["No data"],
    datasets:[
      {label:"Complete",data:mc,backgroundColor:"#3fb950",stack:"s"},
      {label:"Failed",  data:mf,backgroundColor:"#f85149",stack:"s"},
      {label:"Skipped", data:ms,backgroundColor:"#888780",stack:"s"}
    ]
  },options:{responsive:true,maintainAspectRatio:false,
    scales:{
      x:{stacked:true,grid:{display:false},ticks:{autoSkip:false,maxRotation:45,font:{size:10}}},
      y:{stacked:true,grid:{color:grid},border:{display:false},ticks:{stepSize:5}}
    },
    plugins:{
      legend:{display:true,position:"bottom",labels:{font:{size:10},boxWidth:10,color:"#8b949e",padding:8}},
      tooltip:{callbacks:{
        title:function(c){return "Month: "+c[0].label;},
        label:function(c){return " "+c.dataset.label+": "+c.raw+" job(s)";}
      }}
    }
  }});

  // Security radar
  var authScore = authBad?0:100;
  var encScore = D.security.encEnabled?100:0;
  var immScore = D.profiles.immutable>0?100:0;
  var netScore = D.security.netpol?100:0;
  var audScore = D.security.audit?100:0;
  var fipsScore = D.security.fips?100:0;
  new Chart(document.getElementById("sc4"),{type:"radar",data:{
    labels:["Auth","Encryption","Immutability","Net policies","Audit log","FIPS"],
    datasets:[
      {label:"Current",data:[authScore,encScore,immScore,netScore,audScore,fipsScore],
       backgroundColor:"rgba(210,153,34,.2)",borderColor:"#d29922",borderWidth:2,
       pointBackgroundColor:"#d29922",pointRadius:4,pointHoverRadius:6},
      {label:"Target", data:[100,100,80,80,60,40],
       backgroundColor:"rgba(63,185,80,.08)",borderColor:"#3fb950",borderWidth:1.5,
       borderDash:[5,3],pointRadius:0}
    ]
  },options:{responsive:true,maintainAspectRatio:false,
    scales:{r:{
      min:0,max:100,
      ticks:{display:false,stepSize:25},
      grid:{color:grid},
      angleLines:{color:grid},
      pointLabels:{font:{size:10},color:"#8b949e"}
    }},
    plugins:{
      legend:{display:true,position:"bottom",labels:{font:{size:10},boxWidth:10,color:"#8b949e",padding:6}},
      tooltip:{callbacks:{
        label:function(c){
          var tips=["Current posture","Target posture"];
          return " "+tips[c.datasetIndex]+": "+c.raw+"%";
        }
      }}
    }
  }});

  // Limiters — compact table
  (function(){
    var el = document.getElementById("s-limiters-table");
    if(!el) return;
    var lim = D.limiters;
    // Guard: if helmLimiters returned a string or null, show defaults
    if(typeof lim !== 'object' || lim === null || Array.isArray(lim)) {
      lim = {
        "CSI snapshots / cluster": 10,
        "Volume restores / cluster": 10,
        "Snapshot exports / cluster": 10,
        "Generic volume backups": 10,
        "Image copies / cluster": 10,
        "VM snapshots / cluster": 1
      };
    }
    var keys = Object.keys(lim);
    if(keys.length === 0) {
      el.innerHTML = '<div style="color:#8b949e;font-size:11px;padding:8px 0">No limiter configuration found — K10 defaults apply.</div>';
      return;
    }
    var rows = keys.map(function(k){
      var v = lim[k];
      return '<div style="display:flex;justify-content:space-between;align-items:center;' +
        'padding:6px 0;border-bottom:1px solid rgba(139,148,158,0.1);">' +
        '<span style="color:#8b949e;font-size:11px">'+k+'</span>' +
        '<span style="font-family:\'IBM Plex Mono\',monospace;font-size:13px;font-weight:600;color:#58a6ff">'+v+'</span>' +
        '</div>';
    });
    el.innerHTML = rows.join('');
  })();

  // ── Recovery Readiness Score chart ───────────────────────────
  (function(){
    var rrsEl = document.getElementById("rrsChart");
    if(!rrsEl) return;
    var rrs = JSON.parse('{{rrsJSON .Kasten.RecoveryReadiness}}');
    if(!rrs || !rrs.labels) return;
    var earned = rrs.earned;
    var gaps = rrs.max.map(function(m,i){return m - earned[i];});
    new Chart(rrsEl, {
      type: "bar",
      data: {
        labels: rrs.labels,
        datasets: [
          {label:"Earned", data:earned, backgroundColor:"#3fb950", borderWidth:0, borderRadius:3},
          {label:"Gap",    data:gaps,   backgroundColor:"rgba(248,81,73,0.25)", borderWidth:0, borderRadius:3}
        ]
      },
      options: {
        indexAxis:"y", responsive:true, maintainAspectRatio:false,
        scales:{
          x:{stacked:true, max:25, grid:{color:grid}, ticks:{stepSize:5}},
          y:{stacked:true, grid:{display:false}, ticks:{font:{size:10}}}
        },
        plugins:{legend:{display:false},tooltip:{callbacks:{
          label:function(c){
            if(c.datasetIndex===0) return " Earned: "+c.raw;
            return " Gap: "+c.raw;
          }
        }}}
      }
    });
  })();

  // ── Weekly SLA trend chart ────────────────────────────────────
  (function(){
    var slaEl = document.getElementById("slaChart");
    if(!slaEl) return;
    var sla = JSON.parse('{{weeklySLAJSON .Kasten.WeeklySLATrend}}');
    if(!sla || !sla.labels || sla.labels.length === 0) return;
    new Chart(slaEl, {
      type: "line",
      data: {
        labels: sla.labels,
        datasets: [
          {
            label: "Success rate %",
            data: sla.rates,
            borderColor: "#58a6ff",
            backgroundColor: "rgba(88,166,255,0.12)",
            fill: true,
            tension: 0.3,
            pointRadius: 4,
            pointHoverRadius: 6,
            spanGaps: false
          }
        ]
      },
      options: {
        responsive:true, maintainAspectRatio:false,
        scales:{
          y:{min:0, max:100, grid:{color:grid},
             ticks:{callback:function(v){return v+"%";}}},
          x:{grid:{display:false}}
        },
        plugins:{
          legend:{display:false},
          tooltip:{callbacks:{
            label:function(c){
              return c.raw !== null ? " "+c.raw+"%" : " no data";
            }
          }}
        }
      }
    });
  })();

})();
</script>`
