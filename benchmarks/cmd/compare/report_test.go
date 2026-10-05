package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
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

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.csv")

	prev, date, err := previousRun(path)
	if err != nil || prev != nil || date != "" {
		t.Fatalf("missing history: got %v %q %v", prev, date, err)
	}

	first := runInfo{Date: "2026-10-05 06:00", DumboDBVersion: "v1", MongoDBVersion: "v8.0.28", Host: "h"}
	second := runInfo{Date: "2026-10-06 06:00", DumboDBVersion: "v2", MongoDBVersion: "v8.0.28", Host: "h"}
	if err := appendHistory(path, historyRecords(first, merge(
		results("dumbodb", map[string]float64{"BenchmarkA": 100, "BenchmarkB": 200}),
		results("mongodb", map[string]float64{"BenchmarkA": 50, "BenchmarkB": 100})))); err != nil {
		t.Fatal(err)
	}
	if err := appendHistory(path, historyRecords(second, merge(
		results("dumbodb", map[string]float64{"BenchmarkA": 120}),
		results("mongodb", map[string]float64{"BenchmarkA": 50})))); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if n := strings.Count(string(data), "date,dumbodb_version"); n != 1 {
		t.Fatalf("header written %d times:\n%s", n, data)
	}

	prev, date, err = previousRun(path)
	if err != nil {
		t.Fatal(err)
	}
	if date != second.Date || len(prev) != 1 || prev["BenchmarkA"] != 120 {
		t.Fatalf("previous run = %q %v, want %q with only BenchmarkA=120", date, prev, second.Date)
	}
}

func TestHistoryRecordsOneSided(t *testing.T) {
	rows := merge(results("dumbodb", map[string]float64{"BenchmarkA": 10, "BenchmarkB": 30}),
		results("mongodb", map[string]float64{"BenchmarkA": 5}))
	rec := historyRecords(runInfo{}, rows)
	if rec[0][7] != "2.00" || rec[1][6] != "" || rec[1][7] != "" {
		t.Fatalf("records = %v", rec)
	}
}

func TestReport(t *testing.T) {
	rows := merge(
		results("dumbodb", map[string]float64{"BenchmarkA": 2e6, "BenchmarkB": 1e6}),
		results("mongodb", map[string]float64{"BenchmarkA": 1e6, "BenchmarkB": 2e6}))
	prev := map[string]float64{"BenchmarkA": 1e6}
	info := runInfo{Date: "2026-10-06 06:00", DumboDBVersion: "v0.7.1-30", MongoDBVersion: "v8.0.28", Host: "h"}

	r := buildReport(info, rows, prev, "2026-10-05 06:00", nil, []byte("csv"))
	if !strings.Contains(r.Subject, "v0.7.1-30") || !strings.Contains(r.Subject, "median 1.25x") {
		t.Fatalf("subject %q", r.Subject)
	}
	if !strings.Contains(r.HTMLBody, "+100.0%") || !strings.Contains(r.HTMLBody, "#fdd") {
		t.Fatal("doubled DumboDB time is not highlighted")
	}

	failed := buildReport(info, nil, nil, "", errors.New("mongod exited"), nil)
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
