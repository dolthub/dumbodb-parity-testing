package main

import (
	"fmt"
	"html"
	"math"
	"sort"
	"strings"

	"github.com/dolthub/dumbodb-parity-testing/targets"
)

type runInfo struct {
	Date           string
	DumboDBVersion string
	MongoDBVersion string
	Host           string
}

const worstOutliers = 5

type stats struct {
	compared, faster, over2x int
	median, geomean          float64
}

func summarize(rows []combined) stats {
	var mults []float64
	var s stats
	logSum := 0.0
	for _, r := range rows {
		if r.Multiplier == nil || *r.Multiplier <= 0 {
			continue
		}
		m := *r.Multiplier
		mults = append(mults, m)
		logSum += math.Log(m)
		if m < 1 {
			s.faster++
		}
		if m > 2 {
			s.over2x++
		}
	}
	s.compared = len(mults)
	if s.compared == 0 {
		return s
	}
	sort.Float64s(mults)
	if s.compared%2 == 1 {
		s.median = mults[s.compared/2]
	} else {
		s.median = (mults[s.compared/2-1] + mults[s.compared/2]) / 2
	}
	s.geomean = math.Exp(logSum / float64(s.compared))
	return s
}

// worst returns up to n compared benchmarks with the highest multipliers.
func worst(rows []combined, n int) []combined {
	var compared []combined
	for _, r := range rows {
		if r.Multiplier != nil {
			compared = append(compared, r)
		}
	}
	sort.SliceStable(compared, func(i, j int) bool { return *compared[i].Multiplier > *compared[j].Multiplier })
	return compared[:min(n, len(compared))]
}

func buildReport(info runInfo, rows []combined, runErr error, csvData []byte) targets.Report {
	s := summarize(rows)
	subject := fmt.Sprintf("%s %s: median %.2fx vs MongoDB", *emailSubject, info.DumboDBVersion, s.median)
	if runErr != nil {
		subject = fmt.Sprintf("%s %s: FAILED", *emailSubject, info.DumboDBVersion)
	}
	header := fmt.Sprintf("DumboDB %s, MongoDB %s, host %s, run %s UTC.", info.DumboDBVersion, info.MongoDBVersion, info.Host, info.Date)
	summary := fmt.Sprintf("%d benchmarks compared: median %.2fx, geometric mean %.2fx, %d faster than MongoDB, %d over 2x.",
		s.compared, s.median, s.geomean, s.faster, s.over2x)
	outliers := worst(rows, worstOutliers)

	var t strings.Builder
	fmt.Fprintf(&t, "%s\n\n%s\n", subject, header)
	if runErr != nil {
		fmt.Fprintf(&t, "\nERROR: %v\n", runErr)
	}
	fmt.Fprintf(&t, "\n%s\n\nWorst %d:\n", summary, len(outliers))
	fmt.Fprintf(&t, "%-48s %12s %12s %10s\n", "benchmark", "dumbodb ms", "mongodb ms", "multiplier")
	for _, r := range outliers {
		fmt.Fprintf(&t, "%-48s %12s %12s %10s\n", strings.TrimPrefix(r.Name, "Benchmark"),
			fmtMs3(r.DumboDBNs), fmtMs3(r.MongoNs), fmtMultiplierStdout(r.DumboDBNs, r.MongoNs))
	}

	var h strings.Builder
	h.WriteString(`<html><body style="font-family:-apple-system,Helvetica,Arial,sans-serif;font-size:14px">`)
	fmt.Fprintf(&h, "<h2>%s</h2><p>%s</p>", html.EscapeString(subject), html.EscapeString(header))
	if runErr != nil {
		fmt.Fprintf(&h, `<pre style="background:#fee;padding:8px;white-space:pre-wrap">%s</pre>`, html.EscapeString(runErr.Error()))
	}
	fmt.Fprintf(&h, "<p>%s</p><h3>Worst %d</h3>", html.EscapeString(summary), len(outliers))
	h.WriteString(`<table cellpadding="4" cellspacing="0" style="border-collapse:collapse;font-size:13px">`)
	h.WriteString(`<tr style="background:#eee;text-align:right"><th style="text-align:left">Benchmark</th><th>DumboDB ms/op</th><th>MongoDB ms/op</th><th>Multiplier</th></tr>`)
	for _, r := range outliers {
		fmt.Fprintf(&h, `<tr style="text-align:right;border-bottom:1px solid #eee"><td style="text-align:left">%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(strings.TrimPrefix(r.Name, "Benchmark")), fmtMs3(r.DumboDBNs), fmtMs3(r.MongoNs), fmtMultiplierStdout(r.DumboDBNs, r.MongoNs))
	}
	fmt.Fprintf(&h, "</table><p>All %d results are in the attached CSV.</p></body></html>", len(rows))

	return targets.Report{
		Subject:        subject,
		TextBody:       t.String(),
		HTMLBody:       h.String(),
		AttachmentName: "dumbodb-compare-" + strings.NewReplacer(" ", "_", ":", "").Replace(info.Date) + ".csv",
		AttachmentType: "text/csv",
		Attachment:     csvData,
	}
}

func fmtMs3(ns *float64) string {
	if ns == nil {
		return "-"
	}
	return fmt.Sprintf("%.3f", *ns/1e6)
}
