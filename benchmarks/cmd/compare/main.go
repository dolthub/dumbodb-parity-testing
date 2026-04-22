// compare runs the benchmark suite twice — once against DumboDB, once against
// MongoDB — and emits a side-by-side comparison table (text) and/or JSON.
//
// Usage:
//
//	go run ./benchmarks/cmd/compare \
//	    -dumbodb-uri mongodb://localhost:27018 \
//	    -mongodb-uri mongodb://localhost:27017 \
//	    -bench '^Benchmark' \
//	    -benchtime 3s \
//	    -json results.json
//
// The comparator intentionally does NOT verify functional parity — that is the
// existing tests/ harness's job. It only measures elapsed time per operation.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

var (
	dumboDBURI = flag.String("dumbodb-uri", envOr("DUMBODB_URI", "mongodb://localhost:27018"), "DumboDB connection URI")
	mongoURI   = flag.String("mongodb-uri", envOr("MONGO_URI", "mongodb://localhost:27017"), "MongoDB connection URI")
	benchRegex = flag.String("bench", "^Benchmark", "-run pattern for benchmarks (go test -bench)")
	benchTime  = flag.String("benchtime", "2s", "-benchtime value passed to go test")
	count      = flag.Int("count", 1, "-count value passed to go test (repetitions per benchmark)")
	jsonOut    = flag.String("json", "", "if set, write JSON results to this path")
	benchPkg   = flag.String("pkg", "./benchmarks", "Go package containing the benchmarks")
	verbose    = flag.Bool("v", false, "stream go test output to stderr as it runs")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// result is one (benchmark, target) data point.
type result struct {
	Name   string  `json:"name"`
	Target string  `json:"target"`
	URI    string  `json:"uri"`
	N      int     `json:"n"`        // iterations
	NsPerOp float64 `json:"ns_per_op"`
}

// combined is what we emit as JSON: one row per benchmark with both sides.
type combined struct {
	Name        string   `json:"name"`
	DumboDBNs   *float64 `json:"dumbodb_ns_per_op,omitempty"`
	MongoNs     *float64 `json:"mongodb_ns_per_op,omitempty"`
	Ratio       *float64 `json:"ratio,omitempty"` // DumboDB / MongoDB; >1 means DumboDB is slower
}

func main() {
	flag.Parse()

	dumboResults, err := runBench("dumbodb", *dumboDBURI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dumbodb bench run failed: %v\n", err)
		os.Exit(1)
	}
	mongoResults, err := runBench("mongodb", *mongoURI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mongodb bench run failed: %v\n", err)
		os.Exit(1)
	}

	rows := merge(dumboResults, mongoResults)
	printTable(os.Stdout, rows)

	if *jsonOut != "" {
		f, err := os.Create(*jsonOut)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", *jsonOut, err)
			os.Exit(1)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintf(os.Stderr, "write json: %v\n", err)
			os.Exit(1)
		}
	}
}

// runBench invokes `go test -bench` against the given target and parses the
// output. It returns one result per benchmark; when -count > 1 the last run
// wins — Go's benchmark output is line-oriented so rolling up is trivial.
func runBench(label, uri string) ([]result, error) {
	cmd := exec.Command("go", "test",
		*benchPkg,
		"-run", "^$",
		"-bench", *benchRegex,
		"-benchtime", *benchTime,
		"-count", strconv.Itoa(*count),
		"-args",
		"-bench.target-uri", uri,
		"-bench.target-name", label,
	)

	fmt.Fprintf(os.Stderr, "==> running benchmarks against %s (%s)\n", label, uri)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if *verbose {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, stdout.String())
		return nil, fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
	}
	fmt.Fprint(os.Stderr, stdout.String())
	return parseBench(stdout.String(), label, uri), nil
}

// benchLine matches lines like:
//   BenchmarkInsertOne-8            123    4567 ns/op    ...
// We tolerate the optional -N suffix and only extract the name, iterations,
// and ns/op. Other columns (B/op, allocs/op) are ignored for now.
var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+(\d+(?:\.\d+)?)\s+ns/op`)

func parseBench(out, label, uri string) []result {
	var results []result
	for _, line := range strings.Split(out, "\n") {
		m := benchLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		ns, _ := strconv.ParseFloat(m[3], 64)
		results = append(results, result{
			Name:    m[1],
			Target:  label,
			URI:     uri,
			N:       n,
			NsPerOp: ns,
		})
	}
	return results
}

func merge(dumbo, mongo []result) []combined {
	byName := map[string]*combined{}
	ensure := func(name string) *combined {
		c, ok := byName[name]
		if !ok {
			c = &combined{Name: name}
			byName[name] = c
		}
		return c
	}
	for _, r := range dumbo {
		c := ensure(r.Name)
		v := r.NsPerOp
		c.DumboDBNs = &v
	}
	for _, r := range mongo {
		c := ensure(r.Name)
		v := r.NsPerOp
		c.MongoNs = &v
	}
	var rows []combined
	for _, c := range byName {
		if c.DumboDBNs != nil && c.MongoNs != nil && *c.MongoNs > 0 {
			ratio := *c.DumboDBNs / *c.MongoNs
			c.Ratio = &ratio
		}
		rows = append(rows, *c)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func printTable(w *os.File, rows []combined) {
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Benchmark\tDumboDB (ms/op)\tMongoDB (ms/op)\tRatio (Dumbo/Mongo)")
	fmt.Fprintln(tw, "---------\t---------------\t---------------\t-------------------")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			r.Name,
			fmtMs(r.DumboDBNs),
			fmtMs(r.MongoNs),
			fmtRatio(r.Ratio),
		)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

func fmtMs(ns *float64) string {
	if ns == nil {
		return "—"
	}
	return fmt.Sprintf("%.3f", *ns/1e6)
}

func fmtRatio(r *float64) string {
	if r == nil {
		return "—"
	}
	return fmt.Sprintf("%.2fx", *r)
}
