// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fuzz

import (
	"fmt"
	"html"
	"os"
	"strings"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/targets"
)

// RunSummary describes a finished run for its report.
type RunSummary struct {
	Date           string
	Host           string
	DumboDBVersion string
	MongoDBVersion string
	FirstSeed      uint64
	Cases          int
	Commands       int
	Elapsed        time.Duration
	IgnoreOrder    bool
	OutDir         string
	Err            error
}

// maxReportedGroups bounds the report body and attachment; findings.jsonl in
// the output directory keeps everything.
const maxReportedGroups = 50

// Failing reports whether g counts against the run.
func (s RunSummary) Failing(g Group) bool {
	return !(s.IgnoreOrder && g.Class == OrderOnly)
}

func BuildReport(s RunSummary, groups []Group) targets.Report {
	counts := map[Class]int{}
	failing := 0
	for _, g := range groups {
		counts[g.Class] += g.Count
		if s.Failing(g) {
			failing++
		}
	}
	subject := fmt.Sprintf("dumbodb fuzz %s: %d divergent shapes", s.DumboDBVersion, failing)
	switch {
	case s.Err != nil:
		subject = fmt.Sprintf("dumbodb fuzz %s: FAILED", s.DumboDBVersion)
	case failing == 0:
		subject = fmt.Sprintf("dumbodb fuzz %s: no divergences", s.DumboDBVersion)
	}
	header := fmt.Sprintf("DumboDB %s, MongoDB %s, host %s, started %s UTC, ran %s.",
		s.DumboDBVersion, s.MongoDBVersion, s.Host, s.Date, s.Elapsed.Round(time.Second))
	summary := fmt.Sprintf("%d cases from seed %d, %d commands on each server. Divergent commands: %d mismatch, %d order-only, %d server failure. Output: %s",
		s.Cases, s.FirstSeed, s.Commands, counts[Mismatch], counts[OrderOnly], counts[ServerFailure], s.OutDir)
	if s.IgnoreOrder {
		summary += " Order-only differences do not fail the run (-ignore-order)."
	}
	shown := groups
	if len(shown) > maxReportedGroups {
		shown = shown[:maxReportedGroups]
	}

	var t, h, repros strings.Builder
	fmt.Fprintf(&t, "%s\n\n%s\n", subject, header)
	h.WriteString(`<html><body style="font-family:-apple-system,Helvetica,Arial,sans-serif;font-size:14px">`)
	fmt.Fprintf(&h, "<h2>%s</h2><p>%s</p>", html.EscapeString(subject), html.EscapeString(header))
	if s.Err != nil {
		fmt.Fprintf(&t, "\nERROR: %v\n", s.Err)
		fmt.Fprintf(&h, `<pre style="background:#fee;padding:8px;white-space:pre-wrap">%s</pre>`, html.EscapeString(s.Err.Error()))
	}
	fmt.Fprintf(&t, "\n%s\n", summary)
	fmt.Fprintf(&h, "<p>%s</p>", html.EscapeString(summary))
	if len(shown) > 0 {
		fmt.Fprintf(&t, "\nShapes, most frequent first (%d of %d):\n", len(shown), len(groups))
		fmt.Fprintf(&h, "<h3>Shapes, most frequent first (%d of %d)</h3>", len(shown), len(groups))
		h.WriteString(`<table cellpadding="4" cellspacing="0" style="border-collapse:collapse;font-size:13px">`)
		h.WriteString(`<tr style="background:#eee;text-align:left"><th>Count</th><th>Class</th><th>Shape</th><th>Repro</th></tr>`)
	}
	for _, g := range shown {
		fmt.Fprintf(&t, "%6d  %s\n        %s\n", g.Count, g.Signature, g.ReproPath)
		fmt.Fprintf(&h, `<tr style="border-bottom:1px solid #eee;vertical-align:top"><td>%d</td><td>%s</td><td><code>%s</code></td><td>%s</td></tr>`,
			g.Count, html.EscapeString(string(g.Class)), html.EscapeString(g.Signature), html.EscapeString(g.ReproPath))
		repros.WriteString(ReproText(g.First))
		repros.WriteString("\n")
	}
	if len(shown) > 0 {
		h.WriteString("</table><p>The first repro of each shape is attached.</p>")
	}
	h.WriteString("</body></html>")

	return targets.Report{
		Subject:        subject,
		TextBody:       t.String(),
		HTMLBody:       h.String(),
		AttachmentName: "dumbodb-fuzz-" + strings.NewReplacer(" ", "_", ":", "").Replace(s.Date) + "-repros.txt",
		AttachmentType: "text/plain",
		Attachment:     []byte(repros.String()),
	}
}

// Hostname is os.Hostname, or "" on error.
func Hostname() string {
	h, _ := os.Hostname()
	return h
}
