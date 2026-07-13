package report

// pptx.go — PowerPoint QBR generator (pure Go, zero dependencies)
// Uses Veeam brand assets and XML templates from official QBR template.

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	kasten "github.com/veeam/kasten-inspector/pkg/kasten"
)

// PPTXOptions holds QBR presentation options.
type PPTXOptions struct {
	Customer    string
	TAM         string
	MeetingDate string
	LogoPath    string
}

// WritePPTX generates a QBR PowerPoint file from report data.
func WritePPTX(outputPath string, d *Data, opts PPTXOptions) error {
	if opts.TAM == "" {
		opts.TAM = "<TAM name>"
	}
	if opts.MeetingDate == "" {
		opts.MeetingDate = d.GeneratedAt.Format("January 2006")
	}
	if opts.Customer == "" {
		opts.Customer = d.Cluster.Name
	}
	b := &pptxBuilder{d: d, opts: opts}
	return b.write(outputPath)
}

type pptxBuilder struct {
	d    *Data
	opts PPTXOptions
}

func emu(inches float64) int64 { return int64(inches * 914400) }

// Brand colors — Veeam green palette on dark backgrounds
const (
	cDark   = "003D2B" // very dark green (replaces teal dark)
	cTeal   = "00B336" // Veeam medium green (header bars)
	cGreen  = "00D15F" // Veeam primary green (accents)
	cWhite  = "FFFFFF"
	cLight  = "F0FAF4" // very light green tint (content bg)
	cLGray  = "D4EDE0" // light green-gray (borders)
	cGray   = "64748B" // neutral gray (secondary text)
	cText   = "1A2F1E" // very dark green-black (primary text)
	cMuted  = "66C48A" // muted green (metadata text)
	cRed    = "D92B2B"
	cYellow = "F5A623"
	cNavy   = "002D1C" // darkest green
)

// Aliases for compatibility
const (
	cVGreen  = cGreen
	cVDark   = cDark
	cVText   = cText
	cVGray   = cGray
	cVLight  = cLight
	cVWhite  = cWhite
	cVRed    = cRed
	cVYellow = cYellow
)

// ── Write ──────────────────────────────────────────────────────────────────────

func (b *pptxBuilder) write(path string) error {
	slides, slideImages := b.buildSlides()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	add := func(name, content string) {
		w, _ := zw.Create(name)
		w.Write([]byte(content))
	}
	// Static XML parts
	add("[Content_Types].xml", b.contentTypes(len(slides)))
	add("_rels/.rels", xmlRootRels)
	add("ppt/presProps.xml", xmlPresProps)
	add("ppt/tableStyles.xml", xmlTableStyles)
	add("ppt/viewProps.xml", xmlViewProps)
	add("ppt/theme/theme1.xml", xmlVeeamTheme)
	add("ppt/slideLayouts/slideLayout1.xml", xmlSlideLayout)
	add("ppt/slideLayouts/_rels/slideLayout1.xml.rels", xmlSlideLayoutRels)
	add("ppt/slideMasters/slideMaster1.xml", xmlSlideMaster)
	add("ppt/slideMasters/_rels/slideMaster1.xml.rels", xmlSlideMasterRels)
	add("ppt/presentation.xml", b.presentationXML(len(slides)))
	add("ppt/_rels/presentation.xml.rels", b.presRels(len(slides)))
	add("docProps/core.xml", b.coreXML())
	add("docProps/app.xml", b.appXML(slides))

	// Slides
	for i, sl := range slides {
		n := i + 1
		_ = slideImages[i] // unused now
		add(fmt.Sprintf("ppt/slides/slide%d.xml", n), b.wrapSlide(sl))
		add(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n), xmlSlideRels)
	}
	return nil
}

// ── Presentation boilerplate ───────────────────────────────────────────────────

func (b *pptxBuilder) contentTypes(n int) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n")
	sb.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	for _, line := range []string{
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`,
		`<Default Extension="xml" ContentType="application/xml"/>`,
		`<Default Extension="jpeg" ContentType="image/jpeg"/>`,
		`<Default Extension="svg" ContentType="image/svg+xml"/>`,
		`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`,
		`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`,
		`<Override PartName="/ppt/presProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"/>`,
		`<Override PartName="/ppt/viewProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml"/>`,
		`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`,
		`<Override PartName="/ppt/tableStyles.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml"/>`,
		`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`,
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`,
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>`,
	} {
		sb.WriteString(line)
	}
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, i))
	}
	sb.WriteString(`</Types>`)
	return sb.String()
}

func (b *pptxBuilder) presentationXML(n int) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n")
	sb.WriteString(`<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">`)
	sb.WriteString(`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst><p:sldIdLst>`)
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, 255+i, i+1))
	}
	sb.WriteString(`</p:sldIdLst><p:sldSz cx="9144000" cy="5143500"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`)
	return sb.String()
}

func (b *pptxBuilder) presRels(n int) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n")
	sb.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	sb.WriteString(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>`)
	for i := 1; i <= n; i++ {
		sb.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i+1, i))
	}
	sb.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/presProps" Target="presProps.xml"/>`, n+2))
	sb.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/viewProps" Target="viewProps.xml"/>`, n+3))
	sb.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>`, n+4))
	sb.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/tableStyles" Target="tableStyles.xml"/>`, n+5))
	sb.WriteString(`</Relationships>`)
	return sb.String()
}

func (b *pptxBuilder) coreXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:title>Kasten K10 QBR ` + xmlEsc(b.opts.Customer) + `</dc:title>` +
		`<dc:creator>` + b.opts.TAM + `</dc:creator>` +
		`<cp:revision>1</cp:revision></cp:coreProperties>`
}

func (b *pptxBuilder) appXML(slides []string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">` +
		`<Application>Kasten K10 Inspector</Application>` +
		fmt.Sprintf(`<Slides>%d</Slides>`, len(slides)) +
		`</Properties>`
}

// ── Slide wrapper ──────────────────────────────────────────────────────────────

func (b *pptxBuilder) wrapSlide(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree>` +
		`<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="9144000" cy="5143500"/>` +
		`<a:chOff x="0" y="0"/><a:chExt cx="9144000" cy="5143500"/></a:xfrm></p:grpSpPr>` +
		body +
		`</p:spTree></p:cSld></p:sld>`
}

// ── Shape helpers ──────────────────────────────────────────────────────────────

var spID int

func nid() int { spID++; return spID + 100 }

// rect draws a solid color rectangle
func rect(x, y, w, h float64, fill string) string {
	id := nid()
	return fmt.Sprintf(
		`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="r%d"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr/></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
			`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`+
			`<a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr></p:sp>`,
		id, id, emu(x), emu(y), emu(w), emu(h), fill)
}

// gradRect draws a rectangle with the dark green background (cover slides)
func gradRect(x, y, w, h float64) string {
	return rect(x, y, w, h, cDark)
}

// bgImage - unused, kept for compatibility
func bgImage() string { return "" }

// logoImage - unused, kept for compatibility
func logoImage(x, y, w, h float64) string { return "" }

// txb creates a text box
func txb(x, y, w, h float64, text, color string, size float64, bold bool, align string) string {
	id := nid()
	bStr := "0"
	if bold {
		bStr = "1"
	}
	if align == "" {
		align = "l"
	}
	return fmt.Sprintf(
		`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="t%d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
			`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>`+
			`<p:txBody><a:bodyPr wrap="square"><a:normAutofit/></a:bodyPr><a:lstStyle/>`+
			`<a:p><a:pPr algn="%s"/><a:r>`+
			`<a:rPr lang="en-US" sz="%d" b="%s" dirty="0"><a:solidFill><a:srgbClr val="%s"/></a:solidFill>`+
			`<a:latin typeface="Calibri"/></a:rPr>`+
			`<a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`,
		id, id, emu(x), emu(y), emu(w), emu(h),
		align, int(size*100), bStr, color, xmlEsc(text))
}

// veeamFooter adds the standard Veeam footer: copyright + logo
func (b *pptxBuilder) veeamFooter() string {
	footerLeft := "Kasten Inspector v" + b.d.ToolVersion + " · Veeam Kasten"
	if b.opts.Customer != "" {
		footerLeft = "Customer: " + b.opts.Customer + "  ·  Cluster: " + b.d.Cluster.Name
	}
	return txb(0.3, 5.32, 7, 0.22, footerLeft, cGray, 8, false, "l")
}

// contentHeader adds the white slide title
func contentHeader(title string) string {
	return rect(0, 0, 10, 0.72, cTeal) +
		txb(0.4, 0, 8, 0.72, title, cWhite, 22, true, "l")
}

func contentHeaderSub(title, sub string) string {
	return rect(0, 0, 10, 0.72, cTeal) +
		txb(0.4, 0, 8, 0.72, title, cWhite, 22, true, "l") +
		txb(0.4, 0, 9.2, 0.72, sub, cMuted, 11, false, "r")
}

// kpiCard draws a KPI card with white bg, colored top, label, value, sub
func kpiCard(x, y, w, h float64, label, value, sub, accent string) string {
	return rect(x, y, w, h, cVWhite) +
		rect(x, y, w, 0.055, accent) +
		txb(x+0.12, y+0.1, w-0.2, 0.28, label, cVGray, 9, false, "l") +
		txb(x+0.12, y+0.38, w-0.2, 0.62, value, cVText, 26, true, "l") +
		txb(x+0.12, y+1.0, w-0.2, 0.32, sub, cVGray, 9, false, "l")
}

// actionRow draws a prioritized action row
func actionRow(y float64, priority, text, color string) string {
	return rect(0.3, y, 9.4, 0.58, cVWhite) +
		rect(0.3, y, 0.07, 0.58, color) +
		txb(0.45, y+0.04, 1.2, 0.22, priority, color, 8, true, "l") +
		txb(0.45, y+0.27, 8.9, 0.26, text, cVText, 11, false, "l")
}

// ── Slide builders ─────────────────────────────────────────────────────────────

func (b *pptxBuilder) buildSlides() ([]string, []bool) {
	spID = 0
	k := b.d.Kasten
	cl := b.d.Cluster
	o := b.opts

	apps := k.Applications
	comp := k.Compliance
	bp := k.BestPractices
	stor := k.Storage
	dr := k.DR
	sec := k.Security
	js := k.JobSummary
	ns := k.Namespaces

	covPct := int(math.Round(comp.ProtectionCoverage))
	sucRate := int(math.Round(comp.SuccessRate7d))

	pctColor := func(v int) string {
		if v >= 80 {
			return cVGreen
		} else if v >= 50 {
			return cVYellow
		}
		return cVRed
	}

	rrs := k.RecoveryReadiness
	rrsColor := func(score int) string {
		switch {
		case score >= 75: return cVGreen
		case score >= 50: return cVYellow
		default: return cVRed
		}
	}

	slides := []string{
		b.slide1Cover(o.Customer, o.MeetingDate, o.TAM, cl.Name, k.Version, cl.KubernetesVersion),
		b.slide2Exec(o.Customer, o.MeetingDate,
			fmt.Sprintf("%d%%", covPct), pctColor(covPct), fmt.Sprintf("%d of %d namespaces", apps.Protected, apps.Total),
			fmt.Sprintf("%d%%", sucRate), pctColor(sucRate), fmt.Sprintf("%d complete / %d failed", js.Completed, js.Failed),
			fmt.Sprintf("%d/%d", bp.Passed, bp.TotalChecks), boolColor(bp.Critical == 0), fmt.Sprintf("%d critical · %d warnings", bp.Critical, bp.Warnings),
			fmt.Sprintf("%d/100  %s", rrs.Score, rrs.Grade), rrsColor(rrs.Score), fmt.Sprintf("%d gap(s) to close", len(rrs.Findings)),
			k.Version, cl.Name, cl.Platform, k.MultiCluster.Mode, sec.AuthMethod, boolStr(dr.Enabled, "Enabled", "Not configured")),
		b.slide3Coverage(apps.Total, apps.Protected, apps.Unprotected, ns.Unprotected),
		b.slide4Policies(k.Policies),
		b.slide5Jobs(js, k.Jobs, sucRate, k.WeeklySLATrend),
		b.slide5bDaily(k.Jobs),
		b.slide6BP(bp),
		b.slide7Storage(stor, k.Profiles),
		b.slideRRS(rrs),
		b.slideRiskMatrix(k.AppRiskMatrix),
		b.slide8Actions(sec, ns, k.Profiles, dr, bp, rrs),
		b.slide9NextSteps(o.Customer, o.MeetingDate, o.TAM, b.d.ToolVersion, rrs),
	}

	// Which slides use the content background image (rId2)
	hasBg := []bool{false, true, true, true, true, true, true, true, true, true, true, false}

	return slides, hasBg
}

// ── SLIDE 1: Cover ─────────────────────────────────────────────────────────────

func (b *pptxBuilder) slide1Cover(customer, date, tam, cluster, k10ver, k8sver string) string {
	spID = 0
	return rect(0, 0, 10, 5.625, cDark) +
		rect(0, 0, 0.18, 5.625, cGreen) +
		txb(7.5, 0.25, 2.2, 0.5, "veeam|Kasten", cWhite, 14, true, "r") +
		txb(0.45, 1.1, 6, 0.5, "Kasten K10", cGreen, 14, true, "l") +
		txb(0.45, 1.65, 8, 0.9, "Quarterly Business Review", cWhite, 36, true, "l") +
		rect(0.45, 2.9, 3.5, 0.055, cGreen) +
		txb(0.45, 3.05, 7, 0.55, customer, cWhite, 22, false, "l") +
		txb(0.45, 4.35, 7, 0.35, date+"  ·  Prepared by "+tam, cMuted, 11, false, "l") +
		txb(0.45, 5.05, 9, 0.3, "Cluster: "+cluster+"  ·  K10 "+k10ver+"  ·  "+k8sver, "4A9060", 9, false, "l")
}

// ── SLIDE 2: Executive Summary ─────────────────────────────────────────────────

func (b *pptxBuilder) slide2Exec(customer, date,
	covVal, covClr, covSub,
	jobVal, jobClr, jobSub,
	bpVal, bpClr, bpSub,
	rrsVal, rrsClr, rrsSub,
	k10ver, clName, platform, mcMode, auth, drStatus string) string {
	spID = 0
	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeaderSub("Executive Summary", customer+"  ·  "+date)

	type kpi struct{ label, val, sub, color string }
	kpis := []kpi{
		{"Protection Coverage", covVal, covSub, covClr},
		{"Job Success Rate (7d)", jobVal, jobSub, jobClr},
		{"Best Practices", bpVal, bpSub, bpClr},
		{"Recovery Readiness", rrsVal, rrsSub, rrsClr},
	}
	for i, k := range kpis {
		out += kpiCard(0.25+float64(i)*2.38, 1.0, 2.2, 1.55, k.label, k.val, k.sub, k.color)
	}

	type row struct{ label, val string }
	rows := []row{
		{"K10 Version", k10ver}, {"Cluster", clName}, {"Platform", platform},
		{"Multi-cluster", mcMode}, {"Authentication", auth}, {"DR (KDR)", drStatus},
	}
	for i, r := range rows {
		x := 0.3 + float64(i%3)*3.15
		y := 3.2 + float64(i/3)*0.55
		out += txb(x, y, 1.3, 0.4, r.label+":", cVGray, 10, false, "l")
		out += txb(x+1.35, y, 1.7, 0.4, r.val, cVText, 10, true, "l")
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE 3: Coverage ──────────────────────────────────────────────────────────

func (b *pptxBuilder) slide3Coverage(total, protected, unprotected int, unprotNS interface{}) string {
	spID = 0
	pct := 0
	if total > 0 {
		pct = int(math.Round(float64(protected) / float64(total) * 100))
	}
	pctClr := cVRed
	if pct >= 80 {
		pctClr = cVGreen
	} else if pct >= 50 {
		pctClr = cVYellow
	}

	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeader("Protection Coverage")

	// Big percentage
	out += rect(0.5, 0.88, 4.0, 3.9, cVWhite)
	out += txb(0.5, 1.5, 4.0, 1.4, fmt.Sprintf("%d%%", pct), pctClr, 72, true, "ctr")
	out += txb(0.5, 2.95, 4.0, 0.45, "Protection Coverage", cVGray, 13, false, "ctr")
	out += txb(0.5, 3.42, 4.0, 0.35, fmt.Sprintf("%d of %d namespaces", protected, total), cVGray, 11, false, "ctr")

	// Stats
	out += txb(5.0, 0.9, 4.7, 0.35, "Namespace breakdown", cVText, 13, true, "l")
	type stat struct {
		val   int
		label string
		color string
	}
	for i, s := range []stat{{total, "Total namespaces", "505861"}, {protected, "Protected", cVGreen}, {unprotected, "Unprotected", cVRed}} {
		y := 1.35 + float64(i)*0.65
		out += rect(5.0, y, 0.5, 0.48, s.color)
		out += txb(5.0, y, 0.5, 0.48, fmt.Sprint(s.val), cVWhite, 18, true, "ctr")
		out += txb(5.6, y+0.08, 4.0, 0.32, s.label, cVText, 12, false, "l")
	}

	// Unprotected list
	raw, _ := json.Marshal(unprotNS)
	var nsList []map[string]interface{}
	json.Unmarshal(raw, &nsList)
	if len(nsList) > 0 {
		out += txb(5.0, 3.4, 4.7, 0.3, "Unprotected namespaces:", cVRed, 11, true, "l")
		lines := []string{}
		for _, n := range nsList {
			if len(lines) >= 6 {
				break
			}
			if name, ok := n["name"].(string); ok {
				lines = append(lines, "  "+name)
			}
		}
		out += multiLineTxb(5.1, 3.75, 4.6, 1.4, lines, 11, cVText)
	}

	out += b.veeamFooter()
	return out
}

// multiLineTxb creates a text box with multiple lines
func multiLineTxb(x, y, w, h float64, lines []string, size float64, color string) string {
	id := nid()
	var paras strings.Builder
	for _, line := range lines {
		paras.WriteString(fmt.Sprintf(
			`<a:p><a:r><a:rPr lang="en-US" sz="%d" dirty="0"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="Calibri"/></a:rPr><a:t>%s</a:t></a:r></a:p>`,
			int(size*100), color, xmlEsc(line)))
	}
	return fmt.Sprintf(
		`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="ml%d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
			`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>`+
			`<p:txBody><a:bodyPr wrap="square"><a:normAutofit/></a:bodyPr><a:lstStyle/>%s</p:txBody></p:sp>`,
		id, id, emu(x), emu(y), emu(w), emu(h), paras.String())
}

// ── SLIDE 4: Policies ──────────────────────────────────────────────────────────

func (b *pptxBuilder) slide4Policies(policies interface{}) string {
	spID = 0
	raw, _ := json.Marshal(policies)
	var pols []map[string]interface{}
	json.Unmarshal(raw, &pols)

	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeaderSub("Backup Policies", fmt.Sprintf("%d policies configured", len(pols)))

	// Header row
	cols := []struct {
		label string
		x, w  float64
	}{
		{"Policy Name", 0.3, 3.2}, {"Action", 3.55, 1.1},
		{"Frequency", 4.7, 1.5}, {"Export Profile", 6.25, 2.0}, {"Status", 8.3, 1.2},
	}
	for _, c := range cols {
		out += rect(c.x, 0.88, c.w-0.05, 0.35, cTeal)
		out += txb(c.x+0.08, 0.9, c.w-0.15, 0.31, c.label, cVWhite, 10, true, "l")
	}

	for i, pol := range pols {
		if i >= 9 {
			break
		}
		y := 1.27 + float64(i)*0.41
		bg := cVWhite
		if i%2 == 1 {
			bg = cLight
		}
		out += rect(0.3, y, 9.4, 0.37, bg)

		name := strVal(pol, "name")
		action := strVal(pol, "action")
		freq := strVal(pol, "frequency")
		if freq == "" {
			freq = "on-demand"
		}
		expProfs := []string{}
		if ep, ok := pol["exportProfiles"].([]interface{}); ok {
			for _, p := range ep {
				if s, ok := p.(string); ok {
					expProfs = append(expProfs, s)
				}
			}
		}
		exportStr := strings.Join(expProfs, ", ")
		if exportStr == "" {
			exportStr = "—"
		}
		enabled := pol["enabled"] == true
		status, statusClr := "Active", cVGreen
		if !enabled {
			status, statusClr = "Paused", cVYellow
		}

		out += txb(0.38, y+0.05, 3.1, 0.28, name, cVText, 10, false, "l")
		out += txb(3.63, y+0.05, 1.0, 0.28, action, cVGreen, 10, false, "l")
		out += txb(4.78, y+0.05, 1.4, 0.28, freq, cVText, 10, false, "l")
		out += txb(6.33, y+0.05, 1.9, 0.28, exportStr, cVText, 10, false, "l")
		out += txb(8.38, y+0.05, 1.1, 0.28, status, statusClr, 10, true, "l")
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE 5: Jobs ──────────────────────────────────────────────────────────────

func (b *pptxBuilder) slide5Jobs(js interface{}, jobs interface{}, sucRate int, weeklyTrend interface{}) string {
	spID = 0
	raw, _ := json.Marshal(js)
	var jsum map[string]interface{}
	json.Unmarshal(raw, &jsum)

	total := intVal(jsum, "total")
	completed := intVal(jsum, "completed")
	failed := intVal(jsum, "failed")
	skipped := intVal(jsum, "skipped")

	// Adaptive granularity: daily when the collected span is short enough to be
	// readable (≤16 days), otherwise monthly. Fixes the "single column" case where
	// all jobs fell in one calendar month. (Weekly is available in the HTML report.)
	dayB := jobBucketsBy(jobs, 10)
	gran := "Monthly"
	var labels []string
	var vals [][3]int
	if len(dayB) > 0 && len(dayB) <= 16 {
		gran = "Daily"
		for _, k := range sortedKeys(dayB) {
			labels = append(labels, dayLabel(k))
			vals = append(vals, dayB[k])
		}
	} else {
		monthB := jobBucketsBy(jobs, 7)
		keys := sortedKeys(monthB)
		if len(keys) > 6 {
			keys = keys[len(keys)-6:]
		}
		for _, k := range keys {
			labels = append(labels, monthLabel(k))
			vals = append(vals, monthB[k])
		}
	}

	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeaderSub("Job History & Success Rate", gran+" trend")

	chartX, chartY, chartH := 0.35, 0.95, 3.3
	chartW := 6.1
	out += trendBars(labels, vals, chartX, chartY, chartW, chartH)

	// Legend
	out += rect(0.4, chartY+chartH+0.42, 0.18, 0.14, cVGreen) + txb(0.63, chartY+chartH+0.4, 1.0, 0.2, "Complete", cVText, 9, false, "l")
	out += rect(1.7, chartY+chartH+0.42, 0.18, 0.14, cVRed) + txb(1.93, chartY+chartH+0.4, 0.8, 0.2, "Failed", cVText, 9, false, "l")
	out += rect(2.75, chartY+chartH+0.42, 0.18, 0.14, "ADACAF") + txb(2.98, chartY+chartH+0.4, 0.8, 0.2, "Skipped", cVText, 9, false, "l")

	rateClr := cVGreen
	if sucRate < 70 {
		rateClr = cVRed
	}
	failClr := cVGray
	if failed > 0 {
		failClr = cVRed
	}

	type stat struct{ label, val, color string }
	for i, s := range []stat{
		{"Total jobs", fmt.Sprint(total), cVText},
		{"Completed", fmt.Sprint(completed), cVGreen},
		{"Failed", fmt.Sprint(failed), failClr},
		{"Skipped", fmt.Sprint(skipped), cVGray},
		{"Success rate (7d)", fmt.Sprintf("%d%%", sucRate), rateClr},
	} {
		y := 1.2 + float64(i)*0.72
		out += rect(6.8, y, 2.9, 0.62, cVWhite)
		out += txb(6.95, y+0.04, 2.6, 0.25, s.label, cVGray, 9, false, "l")
		out += txb(6.95, y+0.3, 2.6, 0.28, s.val, s.color, 16, true, "l")
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE 6: Best Practices ────────────────────────────────────────────────────

func (b *pptxBuilder) slide6BP(bpData interface{}) string {
	spID = 0
	raw, _ := json.Marshal(bpData)
	var bp map[string]interface{}
	json.Unmarshal(raw, &bp)

	passed := intVal(bp, "passed")
	total := intVal(bp, "totalChecks")
	critical := intVal(bp, "critical")
	if total == 0 {
		total = 11
	}
	barColor := cVGreen
	if critical > 0 {
		barColor = cVRed
	}
	scoreW := 9.4 * float64(passed) / float64(total)

	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeaderSub("Best Practices Assessment", fmt.Sprintf("%d/%d passing", passed, total)) +
		rect(0.3, 1.22, 9.4, 0.12, "E8E8E8") +
		rect(0.3, 1.22, scoreW, 0.12, barColor)

	// Show all BP checks in a 3-column grid
	// Status badge colors: green=pass, yellow=warning, red=critical
	checks, _ := bp["checks"].([]interface{})
	for i, cr := range checks {
		cm, ok := cr.(map[string]interface{})
		if !ok {
			continue
		}
		col := i % 3
		row := i / 3
		x := 0.25 + float64(col)*3.2
		y := 1.42 + float64(row)*0.7

		status := strVal(cm, "status")
		dotClr := cVGreen
		if status == "warning" {
			dotClr = cVYellow
		} else if status == "critical" || status == "fail" {
			dotClr = cVRed
		}

		out += rect(x, y, 3.08, 0.62, cVWhite)
		out += rect(x, y, 0.05, 0.62, dotClr)
		out += txb(x+0.12, y+0.04, 0.55, 0.2, strVal(cm, "id"), cVGray, 8, false, "l")
		out += rect(x+2.75, y+0.18, 0.2, 0.2, dotClr)
		out += txb(x+0.12, y+0.27, 2.58, 0.28, strVal(cm, "name"), cVText, 9, false, "l")
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE 7: Storage ───────────────────────────────────────────────────────────

func (b *pptxBuilder) slide7Storage(storData interface{}, profiles interface{}) string {
	spID = 0
	raw, _ := json.Marshal(storData)
	var stor map[string]interface{}
	json.Unmarshal(raw, &stor)

	snapSize := strVal(stor, "snapshotSizeHuman")
	snapCount := intVal(stor, "snapshotCount")
	expSize := strVal(stor, "exportSizeHuman")
	dedup := floatVal(stor, "deduplicationRatio")
	liveSize := strVal(stor, "liveSizeHuman")
	liveVols := intVal(stor, "liveVolumeCount")
	if snapSize == "" {
		snapSize = "—"
	}
	if expSize == "" {
		expSize = "—"
	}
	if liveSize == "" {
		liveSize = "—"
	}

	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeader("Storage Overview")

	type kpi struct{ label, val, sub string }
	for i, k := range []kpi{
		{"Snapshot Storage", snapSize, fmt.Sprintf("%d snapshot(s)", snapCount)},
		{"Export Storage", expSize, fmt.Sprintf("Dedup %.2fx", dedup)},
		{"Live PVC Storage", liveSize, fmt.Sprintf("%d volumes", liveVols)},
	} {
		out += kpiCard(0.3+float64(i)*3.2, 0.88, 3.0, 1.5, k.label, k.val, k.sub, cGreen)
	}

	// Services disk
	svcs, _ := stor["servicesDiskUsage"].([]interface{})
	if len(svcs) > 0 {
		out += txb(0.3, 2.55, 5, 0.32, "K10 Services Disk Usage", cTeal, 13, true, "l")
		for i, sr := range svcs {
			if i >= 3 {
				break
			}
			sm, ok := sr.(map[string]interface{})
			if !ok {
				continue
			}
			x := 0.3 + float64(i)*3.2
			freeP := floatVal(sm, "freePercent")
			barClr := cVGreen
			if freeP < 40 {
				barClr = cVYellow
			}
			if freeP < 20 {
				barClr = cVRed
			}
			out += txb(x, 2.95, 3.0, 0.28, strVal(sm, "name"), cVText, 11, true, "l")
			out += txb(x, 3.24, 3.0, 0.25, fmt.Sprintf("%s free of %s (%.0f%% free)", strVal(sm, "freeHuman"), strVal(sm, "totalHuman"), freeP), cVGray, 9, false, "l")
			out += rect(x, 3.52, 2.8, 0.14, cLGray)
			if freeP > 0 {
				out += rect(x, 3.52, 2.8*freeP/100, 0.14, barClr)
			}
		}
	}

	// Profiles
	rawP, _ := json.Marshal(profiles)
	var profList []map[string]interface{}
	json.Unmarshal(rawP, &profList)
	out += txb(0.3, 3.82, 5, 0.32, "Location Profiles", cTeal, 13, true, "l")
	for i, p := range profList {
		if i >= 4 {
			break
		}
		x := 0.3 + float64(i)*2.4
		provider := strVal(p, "provider")
		if provider == "" {
			provider = "S3"
		}
		immut := p["immutabilityEnabled"] == true
		immutStr, immutClr := "no lock", cVGray
		if immut {
			immutStr = "[lock] "+strVal(p, "immutabilityPeriod")
			immutClr = "007C5A"
		}
		out += txb(x, 4.22, 2.2, 0.25, strVal(p, "name"), cVText, 10, true, "l")
		out += txb(x, 4.48, 2.2, 0.22, provider+" · "+immutStr, immutClr, 9, false, "l")
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE 8: Actions ───────────────────────────────────────────────────────────


// ── SLIDE RRS: Recovery Readiness Score ───────────────────────────────────────

func (b *pptxBuilder) slideRRS(rrs kasten.RecoveryReadinessScore) string {
	spID = 0
	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeader("Recovery Readiness Score")

	// Big score + grade
	scoreClr := cVGreen
	if rrs.Score < 75 {
		scoreClr = cVYellow
	}
	if rrs.Score < 50 {
		scoreClr = cVRed
	}
	out += rect(0.3, 0.85, 2.8, 4.0, cVWhite)
	out += txb(0.3, 1.4, 2.8, 1.4, fmt.Sprintf("%d", rrs.Score), scoreClr, 72, true, "ctr")
	out += txb(0.3, 2.85, 2.8, 0.6, "Grade: "+rrs.Grade, scoreClr, 24, true, "ctr")
	out += txb(0.3, 3.48, 2.8, 0.3, "out of 100", cVGray, 11, false, "ctr")

	// Component breakdown bars
	order := []string{
		"Protection coverage", "Backup recency", "Offsite export",
		"Immutability", "Disaster recovery", "Authentication",
		"Encryption", "Restore test",
	}
	out += txb(3.4, 0.88, 6.0, 0.28, "Score breakdown", cVText, 11, true, "l")
	for i, name := range order {
		earned := rrs.Components[name]
		max := rrs.MaxComponents[name]
		if max == 0 {
			continue
		}
		y := 1.22 + float64(i)*0.47
		out += txb(3.4, y, 3.2, 0.22, name, cVGray, 9, false, "l")
		// Bar background
		out += rect(3.4, y+0.24, 4.0, 0.15, "E8E8E8")
		// Bar filled
		barW := 4.0 * float64(earned) / float64(max)
		barClr := cVGreen
		if earned == 0 {
			barClr = cVRed
		} else if earned < max {
			barClr = cVYellow
		}
		if barW > 0 {
			out += rect(3.4, y+0.24, barW, 0.15, barClr)
		}
		out += txb(7.45, y+0.22, 0.6, 0.2, fmt.Sprintf("%d/%d", earned, max), cVGray, 8, false, "l")
	}

	// Findings
	if len(rrs.Findings) > 0 {
		out += txb(3.4, 4.92, 6.3, 0.22, fmt.Sprintf("⚠ %d gap(s) — see Actions Required slide", len(rrs.Findings)), cVRed, 9, false, "l")
	} else {
		out += txb(3.4, 4.92, 6.3, 0.22, "✓ All components at full score", cVGreen, 9, false, "l")
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE RISK MATRIX: Application Risk ───────────────────────────────────────

func (b *pptxBuilder) slideRiskMatrix(matrix []kasten.AppRisk) string {
	spID = 0
	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeader("Application Risk Matrix")

	if len(matrix) == 0 {
		out += txb(0.5, 2.5, 9, 0.4, "No application data available.", cVGray, 14, false, "ctr")
		out += b.veeamFooter()
		return out
	}

	// Table header
	headerY := 0.88
	headerCols := []struct{ x, w float64; label string }{
		{0.3, 2.2, "Application"},
		{2.6, 0.7, "Risk"},
		{3.4, 0.9, "RPO now"},
		{4.4, 0.9, "Est. RTO"},
		{5.4, 0.8, "Export"},
		{6.3, 0.85, "Immutable"},
		{7.2, 2.5, "Top concern"},
	}
	for _, col := range headerCols {
		out += txb(col.x, headerY, col.w, 0.25, col.label, cVGray, 9, true, "l")
	}
	out += rect(0.3, headerY+0.27, 9.4, 0.02, "ADACAF")

	maxRows := 8
	if len(matrix) < maxRows {
		maxRows = len(matrix)
	}
	for i := 0; i < maxRows; i++ {
		app := matrix[i]
		y := 1.25 + float64(i)*0.47

		// Row background alternating
		if i%2 == 0 {
			out += rect(0.28, y-0.04, 9.44, 0.44, "F7F7F7")
		}

		riskClr := cVGreen
		riskIcon := "●"
		switch app.RiskLevel {
		case "red":
			riskClr = cVRed
			riskIcon = "●"
		case "yellow":
			riskClr = cVYellow
			riskIcon = "●"
		}

		out += txb(0.3, y, 2.2, 0.35, app.Namespace, cVText, 11, false, "l")
		out += txb(2.6, y, 0.7, 0.35, riskIcon, riskClr, 14, true, "ctr")

		rpoStr := "—"
		if app.RPOHours > 0 {
			rpoStr = fmt.Sprintf("%.0fh", app.RPOHours)
		}
		rtoStr := "—"
		if app.RTOMinutes > 0 {
			rtoStr = fmt.Sprintf("~%.0fm", app.RTOMinutes)
		}
		exportStr := boolStr(app.HasExport, "✓", "✗")
		exportClr := boolColor(app.HasExport)
		immutStr := boolStr(app.HasImmutable, "✓", "✗")
		immutClr := boolColor(app.HasImmutable)

		out += txb(3.4, y, 0.9, 0.35, rpoStr, cVText, 11, false, "ctr")
		out += txb(4.4, y, 0.9, 0.35, rtoStr, cVGray, 11, false, "ctr")
		out += txb(5.4, y, 0.8, 0.35, exportStr, exportClr, 12, true, "ctr")
		out += txb(6.3, y, 0.85, 0.35, immutStr, immutClr, 12, true, "ctr")

		concern := ""
		if len(app.RiskReasons) > 0 {
			concern = app.RiskReasons[0]
			if len(concern) > 50 {
				concern = concern[:47] + "..."
			}
		}
		out += txb(7.2, y, 2.5, 0.35, concern, cVGray, 9, false, "l")
	}

	if len(matrix) > maxRows {
		out += txb(0.3, 5.0, 9.4, 0.25,
			fmt.Sprintf("+ %d more applications — see full HTML report", len(matrix)-maxRows),
			cVGray, 9, false, "l")
	}

	out += b.veeamFooter()
	return out
}

func (b *pptxBuilder) slide8Actions(sec, ns, profiles, dr, bp interface{}, rrs kasten.RecoveryReadinessScore) string {
	spID = 0
	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeader("Actions Required")

	rawSec, _ := json.Marshal(sec)
	var secData map[string]interface{}
	json.Unmarshal(rawSec, &secData)

	rawNS, _ := json.Marshal(ns)
	var nsData map[string]interface{}
	json.Unmarshal(rawNS, &nsData)

	rawP, _ := json.Marshal(profiles)
	var profList []map[string]interface{}
	json.Unmarshal(rawP, &profList)

	rawDR, _ := json.Marshal(dr)
	var drData map[string]interface{}
	json.Unmarshal(rawDR, &drData)

	rawBP, _ := json.Marshal(bp)
	var bpData map[string]interface{}
	json.Unmarshal(rawBP, &bpData)

	type action struct{ pri, text, color string }
	var actions []action

	auth := strVal(secData, "authMethod")
	encEnabled := false
	if enc, ok := secData["encryption"].(map[string]interface{}); ok {
		encEnabled = enc["enabled"] == true
	}
	hasImmut := false
	for _, p := range profList {
		if p["immutabilityEnabled"] == true {
			hasImmut = true
		}
	}
	drEnabled := drData["enabled"] == true

	if auth == "" || strings.Contains(auth, "None") || strings.Contains(auth, "Passthrough") {
		actions = append(actions, action{"Critical", "Configure authentication (OIDC/LDAP) — dashboard is currently open", cVRed})
	}
	if !encEnabled {
		actions = append(actions, action{"Critical", "Enable encryption at rest (AWS KMS, Azure Key Vault, or K10 Passphrase)", cVRed})
	}
	unprotected, _ := nsData["unprotected"].([]interface{})
	if len(unprotected) > 0 {
		names := []string{}
		for _, u := range unprotected {
			if um, ok := u.(map[string]interface{}); ok {
				if n := strVal(um, "name"); n != "" {
					names = append(names, n)
				}
			}
		}
		actions = append(actions, action{"High", fmt.Sprintf("Create backup policies for %d unprotected namespace(s): %s", len(unprotected), strings.Join(names, ", ")), cVYellow})
	}
	if !hasImmut {
		actions = append(actions, action{"High", "Enable object lock (immutability) on at least one location profile", cVYellow})
	}
	if !drEnabled {
		actions = append(actions, action{"High", "Configure Kasten DR (KDR) to protect the K10 catalog", cVYellow})
	}

	alreadyCovered := map[string]bool{"BP-01": true, "BP-02": true, "BP-06": true}
	if checks, ok := bpData["checks"].([]interface{}); ok {
		for _, cr := range checks {
			if len(actions) >= 7 {
				break
			}
			cm, ok := cr.(map[string]interface{})
			if !ok {
				continue
			}
			id := strVal(cm, "id")
			if alreadyCovered[id] {
				continue
			}
			status := strVal(cm, "status")
			if status == "critical" || status == "warning" {
				pri, clr := "Medium", cVYellow
				if status == "critical" {
					pri, clr = "Critical", cVRed
				}
				actions = append(actions, action{pri, id + ": " + strVal(cm, "detail"), clr})
			}
		}
	}

	// Add RRS findings not already covered
	for _, finding := range rrs.Findings {
		if len(actions) >= 7 {
			break
		}
		// Avoid duplicating items already in actions
		already := false
		for _, a := range actions {
			if strings.Contains(a.text, finding[:min(len(finding), 30)]) {
				already = true
				break
			}
		}
		if !already {
			actions = append(actions, action{"High", finding, cVYellow})
		}
	}

	if len(actions) == 0 {
		out += txb(0.5, 2.5, 9, 0.6, "No critical actions required — environment is in good health.", cVGreen, 18, true, "ctr")
	} else {
		for i, a := range actions {
			if i >= 7 {
				break
			}
			out += actionRow(0.88+float64(i)*0.63, a.pri, a.text, a.color)
		}
	}

	out += b.veeamFooter()
	return out
}

// ── SLIDE 9: Next Steps ────────────────────────────────────────────────────────

func (b *pptxBuilder) slide9NextSteps(customer, date, tam, ver string, rrs kasten.RecoveryReadinessScore) string {
	spID = 0
	out := rect(0, 0, 10, 5.625, cDark) +
		rect(0, 0, 0.18, 5.625, cGreen) +
		txb(0.5, 0.7, 9, 0.7, "Next Steps", cWhite, 32, true, "l") +
		txb(0.5, 1.38, 9, 0.35, customer+"  ·  "+date, cMuted, 12, false, "l")

	// Build steps from top RRS findings (max 3), then add schedule slot
	steps := []string{}
	for i, f := range rrs.Findings {
		if i >= 3 {
			break
		}
		steps = append(steps, f)
	}
	// Fill with placeholders if fewer than 3 findings
	for len(steps) < 3 {
		steps = append(steps, "[ Add agreed action item here ]")
	}
	steps = append(steps, "[ Schedule next QBR: _____________ ]")
	for i, s := range steps {
		y := 2.65 + float64(i)*0.6
		out += rect(0.5, y+0.05, 0.28, 0.28, cGreen)
		out += txb(0.5, y+0.05, 0.28, 0.28, fmt.Sprint(i+1), cVWhite, 12, true, "ctr")
		out += txb(0.88, y+0.07, 8.5, 0.3, s, cWhite, 13, false, "l")
	}
	out += txb(0.3, 5.2, 9, 0.25, "Prepared by "+tam+"  ·  Kasten K10 Inspector v"+ver, "4A9060", 9, false, "l")
	return out
}

// ── Static XML (from PowerPoint-validated reference) ──────────────────────────

const xmlRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
	`</Relationships>`

const xmlPresProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:presentationPr xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:extLst>` +
	`<p:ext uri="{E76CE94A-603C-4142-B9EB-6D1370010A27}"><p14:discardImageEditData xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main" val="0"/></p:ext>` +
	`<p:ext uri="{D31A062A-798A-4329-ABDD-BBA856620510}"><p14:defaultImageDpi xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main" val="220"/></p:ext>` +
	`</p:extLst></p:presentationPr>`

const xmlTableStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<a:tblStyleLst xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`

const xmlViewProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:viewPr xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:normalViewPr><p:restoredLeft sz="15611"/><p:restoredTop sz="94690"/></p:normalViewPr>` +
	`<p:slideViewPr><p:cSldViewPr snapToGrid="0"><p:cViewPr varScale="1">` +
	`<p:scale><a:sx n="100" d="100"/><a:sy n="100" d="100"/></p:scale><p:origin x="0" y="0"/>` +
	`</p:cViewPr></p:cSldViewPr></p:slideViewPr>` +
	`<p:notesTextViewPr><p:cViewPr><p:scale><a:sx n="1" d="1"/><a:sy n="1" d="1"/></p:scale><p:origin x="0" y="0"/></p:cViewPr></p:notesTextViewPr>` +
	`<p:gridSpacing cx="76200" cy="76200"/></p:viewPr>`

const xmlVeeamTheme = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Veeam">` +
	`<a:themeElements><a:clrScheme name="Veeam">` +
	`<a:dk1><a:srgbClr val="505861"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1>` +
	`<a:dk2><a:srgbClr val="ADACAF"/></a:dk2><a:lt2><a:srgbClr val="EFEFEF"/></a:lt2>` +
	`<a:accent1><a:srgbClr val="000000"/></a:accent1><a:accent2><a:srgbClr val="00D15F"/></a:accent2>` +
	`<a:accent3><a:srgbClr val="3700FF"/></a:accent3><a:accent4><a:srgbClr val="01B0FE"/></a:accent4>` +
	`<a:accent5><a:srgbClr val="FFD836"/></a:accent5><a:accent6><a:srgbClr val="FE8A25"/></a:accent6>` +
	`<a:hlink><a:srgbClr val="00D15F"/></a:hlink><a:folHlink><a:srgbClr val="007C5A"/></a:folHlink>` +
	`</a:clrScheme>` +
	`<a:fontScheme name="Veeam"><a:majorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
	`<a:fmtScheme name="Office">` +
	`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
	`<a:lnStyleLst><a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
	`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
	`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst>` +
	`</a:fmtScheme></a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`

const xmlSlideLayout = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="9144000" cy="5143500"/><a:chOff x="0" y="0"/><a:chExt cx="9144000" cy="5143500"/></a:xfrm></p:grpSpPr>` +
	`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`

const xmlSlideLayoutRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>` +
	`</Relationships>`

const xmlSlideMaster = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
	`<p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg>` +
	`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="9144000" cy="5143500"/><a:chOff x="0" y="0"/><a:chExt cx="9144000" cy="5143500"/></a:xfrm></p:grpSpPr>` +
	`</p:spTree></p:cSld>` +
	`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
	`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>` +
	`<p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`

const xmlSlideMasterRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/>` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
	`</Relationships>`

// Slide rels without image
const xmlSlideRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
	`</Relationships>`

// Slide rels with content background image (rId2) and logo (rId3)
const xmlSlideRelsWithBg = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/content_bg.jpeg"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/logo_small.svg"/>` +
	`</Relationships>`

// Cover/closing slide rels - gradient bg + logo
const xmlSlideRelsCover = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/logo_small.svg"/>` +
	`</Relationships>`

// ── Utilities ──────────────────────────────────────────────────────────────────

func min(a, b int) int { if a < b { return a }; return b }

func xmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

func boolColor(b bool) string {
	if b {
		return cVGreen
	}
	return cVRed
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if v != nil {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func intVal(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}

func floatVal(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if n, ok := v.(float64); ok {
			return n
		}
	}
	return 0
}

func sortedKeys(m map[string][3]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func monthLabel(m string) string {
	mon := map[string]string{
		"01": "Jan", "02": "Feb", "03": "Mar", "04": "Apr",
		"05": "May", "06": "Jun", "07": "Jul", "08": "Aug",
		"09": "Sep", "10": "Oct", "11": "Nov", "12": "Dec",
	}
	if len(m) < 7 {
		return m
	}
	if label, ok := mon[m[5:7]]; ok {
		return label + " " + m[2:4]
	}
	return m
}

// ── Job trend helpers (string-key bucketing; no time/sort imports) ─────────────

// jobBucketsBy groups jobs by a prefix of their startTime: keyLen=10 → daily
// ("2026-07-07"), keyLen=7 → monthly ("2026-07"). Returns {complete,failed,skipped}.
func jobBucketsBy(jobs interface{}, keyLen int) map[string][3]int {
	rawJobs, _ := json.Marshal(jobs)
	var jobList []map[string]interface{}
	json.Unmarshal(rawJobs, &jobList)
	m := map[string][3]int{}
	for _, j := range jobList {
		st := strVal(j, "startTime")
		if len(st) < keyLen {
			continue
		}
		k := st[:keyLen]
		cur := m[k]
		switch strVal(j, "status") {
		case "Complete", "Success":
			cur[0]++
		case "Failed", "Error":
			cur[1]++
		case "Skipped":
			cur[2]++
		}
		m[k] = cur
	}
	return m
}

// dayLabel turns "2026-07-07" into "07 Jul".
func dayLabel(k string) string {
	if len(k) < 10 {
		return k
	}
	mon := map[string]string{
		"01": "Jan", "02": "Feb", "03": "Mar", "04": "Apr",
		"05": "May", "06": "Jun", "07": "Jul", "08": "Aug",
		"09": "Sep", "10": "Oct", "11": "Nov", "12": "Dec",
	}
	if label, ok := mon[k[5:7]]; ok {
		return k[8:10] + " " + label
	}
	return k
}

// trendBars renders a stacked (skipped/failed/complete) bar chart into the given
// rectangle, auto-sizing bar width to the number of buckets.
func trendBars(labels []string, vals [][3]int, x, y, w, h float64) string {
	n := len(labels)
	if n == 0 {
		return txb(x, y+h/2-0.15, w, 0.3, "No job history in range", cVGray, 11, false, "ctr")
	}
	maxVal := 1
	for _, v := range vals {
		if t := v[0] + v[1] + v[2]; t > maxVal {
			maxVal = t
		}
	}
	slot := w / float64(n)
	barW := slot * 0.6
	if barW > 0.7 {
		barW = 0.7
	}
	out := ""
	for i, v := range vals {
		cx := x + float64(i)*slot + (slot-barW)/2
		totalH := float64(v[0]+v[1]+v[2]) / float64(maxVal) * h
		base := y + h - totalH
		if v[2] > 0 {
			hh := float64(v[2]) / float64(maxVal) * h
			out += rect(cx, base, barW, hh, "ADACAF")
			base += hh
		}
		if v[1] > 0 {
			hh := float64(v[1]) / float64(maxVal) * h
			out += rect(cx, base, barW, hh, cVRed)
			base += hh
		}
		if v[0] > 0 {
			hh := float64(v[0]) / float64(maxVal) * h
			out += rect(cx, base, barW, hh, cVGreen)
		}
		// Label every bar when few, every other when crowded, to avoid overlap.
		if n <= 16 || i%2 == 0 {
			out += txb(x+float64(i)*slot-0.05, y+h+0.06, slot+0.1, 0.22, labels[i], cVGray, 7, false, "ctr")
		}
	}
	return out
}

// ── SLIDE 5b: Daily job history (recent detail) ────────────────────────────────

func (b *pptxBuilder) slide5bDaily(jobs interface{}) string {
	spID = 0
	dayB := jobBucketsBy(jobs, 10)
	keys := sortedKeys(dayB)
	if len(keys) > 14 {
		keys = keys[len(keys)-14:]
	}
	var labels []string
	var vals [][3]int
	for _, k := range keys {
		labels = append(labels, dayLabel(k))
		vals = append(vals, dayB[k])
	}
	out := rect(0, 0, 10, 5.625, cLight) +
		contentHeaderSub("Job History — Daily Detail", "last 14 days with activity")
	out += trendBars(labels, vals, 0.35, 0.95, 9.3, 3.5)
	// Legend
	out += rect(0.4, 4.75, 0.18, 0.14, cVGreen) + txb(0.63, 4.73, 1.0, 0.2, "Complete", cVText, 9, false, "l")
	out += rect(1.7, 4.75, 0.18, 0.14, cVRed) + txb(1.93, 4.73, 0.8, 0.2, "Failed", cVText, 9, false, "l")
	out += rect(2.75, 4.75, 0.18, 0.14, "ADACAF") + txb(2.98, 4.73, 0.8, 0.2, "Skipped", cVText, 9, false, "l")
	out += b.veeamFooter()
	return out
}
