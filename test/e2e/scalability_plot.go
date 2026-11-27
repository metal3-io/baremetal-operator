//go:build e2e
// +build e2e

package e2e

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Output file names (under artifactFolder/<specName>/) for the
// provisioning-time-vs-concurrent-host-count plot.
const (
	provisioningTimePlotSVG = "provisioning-time-vs-concurrent-hosts.svg"
	provisioningTimePlotCSV = "provisioning-time-vs-concurrent-hosts.csv"
)

// SVG layout for the provisioning-time plot, in pixels.
const (
	plotWidth        = 960
	plotHeight       = 600
	plotMarginLeft   = 80
	plotMarginRight  = 230 // room for the legend
	plotMarginTop    = 60
	plotMarginBottom = 70
	plotTargetTicks  = 5
	plotPointRadius  = 4
	plotMarkerRadius = 9

	plotYTickLabelGap = 8  // y-axis tick labels, left of the axis
	plotXTickLabelGap = 18 // x-axis tick labels, below the axis
	plotXAxisTitleGap = 48 // x-axis title, below the axis

	plotLegendGap     = 20 // legend, right of the plot area
	plotLegendTop     = 10 // first legend row, below the plot top
	plotLegendRowH    = 22
	plotLegendLineLen = 24
	plotLegendMarkerX = plotLegendLineLen / 2 // marker centred on the line sample
	plotLegendTextGap = 32                    // label, right of the legend's left edge
	plotLegendRingR   = plotMarkerRadius - 2  // timed-out ring, a bit smaller than on the chart
)

// plotSeries is one line on the provisioning-time plot.
type plotSeries struct {
	name  string
	color string
	value func(concurrentRoundResult) float64
}

var provisioningTimeSeries = []plotSeries{
	{"Avg per-host provision time", "#1f77b4", func(r concurrentRoundResult) float64 { return r.AvgProvisionDurationSec }},
	{"Max per-host provision time", "#ff7f0e", func(r concurrentRoundResult) float64 { return r.MaxProvisionDurationSec }},
	{"Batch wall-clock time", "#2ca02c", func(r concurrentRoundResult) float64 { return r.BatchDurationSec }},
}

// writeProvisioningTimePlot renders results as an SVG line chart of
// provisioning time against concurrent host count, plus the raw series as
// CSV, into dir. degradedAtScale (0 for none) is the scale point at which the
// ramp stopped on a degradation signal; it and any timed-out rounds are
// highlighted on the chart.
func writeProvisioningTimePlot(dir string, results []concurrentRoundResult, degradedAtScale int) (svgPath, csvPath string, err error) {
	if err = os.MkdirAll(dir, filePerm755); err != nil {
		return "", "", fmt.Errorf("creating plot directory %s: %w", dir, err)
	}

	csvPath = filepath.Join(dir, provisioningTimePlotCSV)
	if err = writeProvisioningTimeCSV(csvPath, results, degradedAtScale); err != nil {
		return "", "", err
	}

	svgPath = filepath.Join(dir, provisioningTimePlotSVG)
	if err = os.WriteFile(svgPath, []byte(renderProvisioningTimeSVG(results, degradedAtScale)), filePerm600); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", svgPath, err)
	}
	return svgPath, csvPath, nil
}

func writeProvisioningTimeCSV(csvPath string, results []concurrentRoundResult, degradedAtScale int) error {
	f, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", csvPath, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	rows := [][]string{{
		"concurrentHosts", "avgProvisionDurationSec", "maxProvisionDurationSec",
		"batchDurationSec", "retryRate", "hostsWithRetries", "ironicMemoryMb",
		"timedOut", "degraded",
	}}
	for _, r := range results {
		rows = append(rows, []string{
			strconv.Itoa(r.Scale),
			formatPlotFloat(r.AvgProvisionDurationSec),
			formatPlotFloat(r.MaxProvisionDurationSec),
			formatPlotFloat(r.BatchDurationSec),
			formatPlotFloat(r.RetryRate),
			strconv.Itoa(r.HostsWithRetries),
			formatPlotFloat(r.IronicMemoryMB),
			strconv.FormatBool(r.TimedOut),
			strconv.FormatBool(r.Scale == degradedAtScale),
		})
	}
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("writing %s: %w", csvPath, err)
	}
	return nil
}

func formatPlotFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// midpoint returns the centre between a and b, for centring labels.
func midpoint(a, b float64) float64 {
	const half = 0.5
	return (a + b) * half
}

// niceAxis picks a "round" axis maximum and tick step (1, 2 or 5 times a
// power of ten) giving roughly plotTargetTicks ticks covering maxVal.
func niceAxis(maxVal, minStep float64) (axisMax, step float64) {
	if maxVal <= 0 {
		maxVal = 1
	}
	raw := maxVal / plotTargetTicks
	mag := math.Pow10(int(math.Floor(math.Log10(raw))))
	for _, m := range []float64{1, 2, 5, 10} {
		step = m * mag
		if step >= raw {
			break
		}
	}
	step = math.Max(step, minStep)
	return math.Ceil(maxVal/step) * step, step
}

// renderProvisioningTimeSVG draws the plot as a standalone SVG document
// (viewable in any browser), with no dependencies beyond the standard
// library.
func renderProvisioningTimeSVG(results []concurrentRoundResult, degradedAtScale int) string {
	maxX, maxY := 0.0, 0.0
	for _, r := range results {
		maxX = math.Max(maxX, float64(r.Scale))
		for _, s := range provisioningTimeSeries {
			maxY = math.Max(maxY, s.value(r))
		}
	}
	xMax, xStep := niceAxis(maxX, 1) // host counts are whole numbers
	yMax, yStep := niceAxis(maxY, 0)

	innerW := float64(plotWidth - plotMarginLeft - plotMarginRight)
	innerH := float64(plotHeight - plotMarginTop - plotMarginBottom)
	px := func(x float64) float64 { return plotMarginLeft + x/xMax*innerW }
	py := func(y float64) float64 { return plotMarginTop + innerH - y/yMax*innerH }
	left, right := px(0), px(xMax)
	top, bottom := py(yMax), py(0)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="sans-serif" font-size="12">`+"\n",
		plotWidth, plotHeight, plotWidth, plotHeight)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="white"/>`+"\n")
	fmt.Fprintf(&b, `<text x="%.1f" y="30" text-anchor="middle" font-size="16" font-weight="bold">Provisioning time vs. concurrent host count (Fake IPA)</text>`+"\n",
		midpoint(left, right))

	// Grid lines and tick labels.
	for i := 0; float64(i)*yStep <= yMax+yStep/2; i++ {
		v := float64(i) * yStep
		y := py(v)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e0e0e0"/>`+"\n", left, y, right, y)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="end" dominant-baseline="middle">%s</text>`+"\n",
			left-plotYTickLabelGap, y, strconv.FormatFloat(v, 'f', -1, 64))
	}
	for i := 0; float64(i)*xStep <= xMax+xStep/2; i++ {
		v := float64(i) * xStep
		x := px(v)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e0e0e0"/>`+"\n", x, top, x, bottom)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle">%s</text>`+"\n",
			x, bottom+plotXTickLabelGap, strconv.FormatFloat(v, 'f', -1, 64))
	}

	// Axes and axis titles.
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="black"/>`+"\n", left, bottom, right, bottom)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="black"/>`+"\n", left, top, left, bottom)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-size="14">Concurrent hosts</text>`+"\n",
		midpoint(left, right), bottom+plotXAxisTitleGap)
	fmt.Fprintf(&b, `<text x="20" y="%.1f" text-anchor="middle" font-size="14" transform="rotate(-90 20 %.1f)">Time (seconds)</text>`+"\n",
		midpoint(top, bottom), midpoint(top, bottom))

	// Highlight timed-out rounds and the degradation stopping point.
	for _, r := range results {
		if !r.TimedOut && r.Scale != degradedAtScale {
			continue
		}
		x := px(float64(r.Scale))
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#d62728" stroke-dasharray="4 4"/>`+"\n", x, top, x, bottom)
		for _, s := range provisioningTimeSeries {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%d" fill="none" stroke="#d62728" stroke-width="2"/>`+"\n",
				x, py(s.value(r)), plotMarkerRadius)
		}
	}

	// Data series.
	for _, s := range provisioningTimeSeries {
		points := make([]string, 0, len(results))
		for _, r := range results {
			points = append(points, fmt.Sprintf("%.1f,%.1f", px(float64(r.Scale)), py(s.value(r))))
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`+"\n", strings.Join(points, " "), s.color)
		for _, r := range results {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%d" fill="%s"><title>%d hosts: %.1fs</title></circle>`+"\n",
				px(float64(r.Scale)), py(s.value(r)), plotPointRadius, s.color, r.Scale, s.value(r))
		}
	}

	// Legend.
	legendX := right + plotLegendGap
	for i, s := range provisioningTimeSeries {
		y := top + plotLegendTop + float64(i)*plotLegendRowH
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="2"/>`+"\n", legendX, y, legendX+plotLegendLineLen, y, s.color)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%d" fill="%s"/>`+"\n", legendX+plotLegendMarkerX, y, plotPointRadius, s.color)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" dominant-baseline="middle">%s</text>`+"\n", legendX+plotLegendTextGap, y, s.name)
	}
	y := top + plotLegendTop + float64(len(provisioningTimeSeries))*plotLegendRowH
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%d" fill="none" stroke="#d62728" stroke-width="2"/>`+"\n", legendX+plotLegendMarkerX, y, plotLegendRingR)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" dominant-baseline="middle">Timed out / degraded</text>`+"\n", legendX+plotLegendTextGap, y)

	b.WriteString("</svg>\n")
	return b.String()
}
