package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type report struct {
	Subject  string
	TextBody string
	HTMLBody string
	CSVName  string
	CSV      []byte
}

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

// prevChange is DumboDB's fractional change in ns/op since the previous run.
func prevChange(r combined, prev map[string]float64) (float64, bool) {
	p := prev[r.Name]
	if p <= 0 || r.DumboDBNs == nil || *r.DumboDBNs <= 0 {
		return 0, false
	}
	return *r.DumboDBNs/p - 1, true
}

func multiplierValue(r combined) float64 {
	if r.Multiplier == nil {
		return 0
	}
	return *r.Multiplier
}

func buildReport(info runInfo, rows []combined, prev map[string]float64, prevDate string, runErr error, csvData []byte) report {
	s := summarize(rows)
	var subject string
	if runErr != nil {
		subject = fmt.Sprintf("%s %s: FAILED", *emailSubject, info.DumboDBVersion)
	} else {
		subject = fmt.Sprintf("%s %s: median %.2fx vs MongoDB", *emailSubject, info.DumboDBVersion, s.median)
	}

	sorted := append([]combined(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return multiplierValue(sorted[i]) > multiplierValue(sorted[j]) })

	var moved []string
	for _, r := range sorted {
		if c, ok := prevChange(r, prev); ok && math.Abs(c) > *changeThreshold {
			moved = append(moved, fmt.Sprintf("%s %+.0f%%", strings.TrimPrefix(r.Name, "Benchmark"), c*100))
		}
	}

	var t strings.Builder
	fmt.Fprintf(&t, "%s\n\nDumboDB %s, MongoDB %s, host %s, run %s\n", subject, info.DumboDBVersion, info.MongoDBVersion, info.Host, info.Date)
	if runErr != nil {
		fmt.Fprintf(&t, "\nERROR: %v\n", runErr)
	}
	fmt.Fprintf(&t, "\n%d benchmarks compared: median %.2fx, geometric mean %.2fx, %d faster than MongoDB, %d over 2x.\n",
		s.compared, s.median, s.geomean, s.faster, s.over2x)
	if prevDate != "" {
		fmt.Fprintf(&t, "DumboDB changes over %.0f%% since %s: %d\n", *changeThreshold*100, prevDate, len(moved))
		for _, m := range moved {
			fmt.Fprintf(&t, "  %s\n", m)
		}
	}
	fmt.Fprintf(&t, "\n%-48s %12s %12s %10s %10s\n", "benchmark", "dumbodb ms", "mongodb ms", "multiplier", "vs prev")
	for _, r := range sorted {
		fmt.Fprintf(&t, "%-48s %12s %12s %10s %10s\n", strings.TrimPrefix(r.Name, "Benchmark"),
			fmtMs3(r.DumboDBNs), fmtMs3(r.MongoNs), fmtMultiplierStdout(r.DumboDBNs, r.MongoNs), changeText(r, prev))
	}

	var h strings.Builder
	h.WriteString(`<html><body style="font-family:-apple-system,Helvetica,Arial,sans-serif;font-size:14px">`)
	fmt.Fprintf(&h, "<h2>%s</h2>", html.EscapeString(subject))
	fmt.Fprintf(&h, "<p>DumboDB <b>%s</b>, MongoDB <b>%s</b>, host %s, run %s UTC. Writes are journaled on both (<code>j:true</code>).</p>",
		html.EscapeString(info.DumboDBVersion), html.EscapeString(info.MongoDBVersion), html.EscapeString(info.Host), html.EscapeString(info.Date))
	if runErr != nil {
		fmt.Fprintf(&h, `<pre style="background:#fee;padding:8px;white-space:pre-wrap">%s</pre>`, html.EscapeString(runErr.Error()))
	}
	fmt.Fprintf(&h, "<p>%d benchmarks compared: median <b>%.2fx</b>, geometric mean %.2fx, %d faster than MongoDB, %d over 2x.</p>",
		s.compared, s.median, s.geomean, s.faster, s.over2x)
	if prevDate != "" {
		fmt.Fprintf(&h, "<p>DumboDB changes over %.0f%% since %s: <b>%d</b>, highlighted below.</p>", *changeThreshold*100, html.EscapeString(prevDate), len(moved))
	}
	h.WriteString(`<table cellpadding="4" cellspacing="0" style="border-collapse:collapse;font-size:13px">`)
	h.WriteString(`<tr style="background:#eee;text-align:right"><th style="text-align:left">Benchmark</th><th>DumboDB ms/op</th><th>MongoDB ms/op</th><th>Multiplier</th><th>DumboDB vs prev</th></tr>`)
	for _, r := range sorted {
		style := ""
		if c, ok := prevChange(r, prev); ok && c > *changeThreshold {
			style = "background:#fdd"
		} else if ok && c < -*changeThreshold {
			style = "background:#dfd"
		}
		fmt.Fprintf(&h, `<tr style="text-align:right;border-bottom:1px solid #eee"><td style="text-align:left">%s</td><td>%s</td><td>%s</td><td>%s</td><td style="%s">%s</td></tr>`,
			html.EscapeString(strings.TrimPrefix(r.Name, "Benchmark")), fmtMs3(r.DumboDBNs), fmtMs3(r.MongoNs), fmtMultiplierStdout(r.DumboDBNs, r.MongoNs), style, changeText(r, prev))
	}
	h.WriteString("</table><p>The CSV is attached; each row carries the run columns so runs stack in one sheet.</p></body></html>")

	return report{
		Subject:  subject,
		TextBody: t.String(),
		HTMLBody: h.String(),
		CSVName:  "dumbodb-compare-" + strings.NewReplacer(" ", "_", ":", "").Replace(info.Date) + ".csv",
		CSV:      csvData,
	}
}

func fmtMs3(ns *float64) string {
	if ns == nil {
		return "-"
	}
	return fmt.Sprintf("%.3f", *ns/1e6)
}

func changeText(r combined, prev map[string]float64) string {
	c, ok := prevChange(r, prev)
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%+.1f%%", c*100)
}

type notifier func(ctx context.Context, r report)

func buildNotifiers() []notifier {
	var out []notifier
	if *reportDir != "" {
		out = append(out, fileNotifier(*reportDir))
	}
	if *alertCmd != "" {
		out = append(out, cmdNotifier(*alertCmd))
	}
	if *emailFrom != "" && *emailTo != "" {
		out = append(out, sesNotifier(*emailFrom, *emailTo, *emailRegion))
	}
	return out
}

func fileNotifier(dir string) notifier {
	return func(_ context.Context, r report) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("report-dir: %v", err)
			return
		}
		for name, data := range map[string][]byte{
			"report-latest.txt":  []byte(r.TextBody),
			"report-latest.html": []byte(r.HTMLBody),
			r.CSVName:            r.CSV,
		} {
			if len(data) == 0 {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				log.Printf("report-dir write %s: %v", name, err)
			}
		}
		log.Printf("report written to %s", dir)
	}
}

func cmdNotifier(cmdLine string) notifier {
	return func(ctx context.Context, r report) {
		cmd := exec.CommandContext(ctx, "sh", "-c", cmdLine)
		cmd.Env = append(cmd.Environ(), "ALERT_SUBJECT="+r.Subject)
		cmd.Stdin = strings.NewReader(r.TextBody)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("alert-cmd failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
}

func sesNotifier(from, toCSV, region string) notifier {
	var to []string
	for _, p := range strings.Split(toCSV, ",") {
		if v := strings.TrimSpace(p); v != "" {
			to = append(to, v)
		}
	}
	return func(ctx context.Context, r report) {
		var opts []func(*config.LoadOptions) error
		if region != "" {
			opts = append(opts, config.WithRegion(region))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			log.Printf("ses config: %v", err)
			return
		}
		raw := buildRawEmail(from, strings.Join(to, ", "), r)
		_, err = sesv2.NewFromConfig(cfg).SendEmail(ctx, &sesv2.SendEmailInput{
			FromEmailAddress: aws.String(from),
			Destination:      &types.Destination{ToAddresses: to},
			Content:          &types.EmailContent{Raw: &types.RawMessage{Data: raw}},
		})
		if err != nil {
			log.Printf("ses send: %v", err)
			return
		}
		log.Printf("report emailed to %s", strings.Join(to, ", "))
	}
}

func buildRawEmail(from, to string, r report) []byte {
	const boundary = "mixed-boundary-dumbodb-compare"
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", r.Subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(r.HTMLBody)
	b.WriteString("\r\n")

	if len(r.CSV) > 0 {
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: text/csv; name=\"%s\"\r\n", r.CSVName)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", r.CSVName)
		encoded := base64.StdEncoding.EncodeToString(r.CSV)
		for i := 0; i < len(encoded); i += 76 {
			b.WriteString(encoded[i:min(i+76, len(encoded))])
			b.WriteString("\r\n")
		}
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}
