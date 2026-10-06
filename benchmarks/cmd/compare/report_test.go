package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func results(target string, nsByName map[string]float64) []result {
	var out []result
	for name, ns := range nsByName {
		out = append(out, result{Name: name, Target: target, NsPerOp: ns})
	}
	return out
}

func TestResultsCSVKeepsFormat(t *testing.T) {
	rows := merge(results("dumbodb", map[string]float64{"BenchmarkA": 10, "BenchmarkB": 30}),
		results("mongodb", map[string]float64{"BenchmarkA": 5}))
	var b bytes.Buffer
	if err := writeResultsCSV(&b, rows); err != nil {
		t.Fatal(err)
	}
	want := "name,dumbodb_ns_per_op,mongodb_ns_per_op,multiplier\nBenchmarkA,10.00,5.00,2.00\nBenchmarkB,30.00,,\n"
	if b.String() != want {
		t.Fatalf("csv =\n%s\nwant\n%s", b.String(), want)
	}
}

func TestReportShowsWorstOutliers(t *testing.T) {
	dumbo, mongo := map[string]float64{}, map[string]float64{}
	for i, m := range []float64{1, 9, 2, 8, 3, 7, 4, 6, 5} {
		name := fmt.Sprintf("BenchmarkM%d", i)
		dumbo[name], mongo[name] = m*1e6, 1e6
	}
	info := runInfo{Date: "2026-10-06 06:00", DumboDBVersion: "v0.7.1-30", MongoDBVersion: "v8.0.28", Host: "h"}
	r := buildReport(info, merge(results("dumbodb", dumbo), results("mongodb", mongo)), nil, []byte("csv"))

	if !strings.Contains(r.Subject, "v0.7.1-30") || !strings.Contains(r.Subject, "median 5.00x") {
		t.Fatalf("subject %q", r.Subject)
	}
	for _, want := range []string{"9.00x", "8.00x", "7.00x", "6.00x", "5.00x"} {
		if !strings.Contains(r.HTMLBody, want) {
			t.Fatalf("report lacks outlier %s", want)
		}
	}
	if strings.Contains(r.HTMLBody, "4.00x") {
		t.Fatal("report lists more than the 5 worst")
	}

	failed := buildReport(info, nil, errors.New("mongod exited"), nil)
	if !strings.Contains(failed.Subject, "FAILED") || !strings.Contains(failed.HTMLBody, "mongod exited") {
		t.Fatalf("failure report: %q", failed.Subject)
	}
}

func TestRawEmailAttachesCSV(t *testing.T) {
	r := report{Subject: "s", HTMLBody: "<p>x</p>", CSVName: "run.csv", CSV: []byte("a,b\n1,2\n")}
	raw := buildRawEmail("from@example.com", "to@example.com", r)
	for _, want := range []string{
		"multipart/mixed",
		`Content-Disposition: attachment; filename="run.csv"`,
		base64.StdEncoding.EncodeToString(r.CSV),
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("email lacks %q:\n%s", want, raw)
		}
	}
}
