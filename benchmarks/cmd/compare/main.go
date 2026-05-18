// compare runs the benchmark suite against DumboDB and MongoDB and emits a
// side-by-side comparison table (text) and/or CSV.
//
// The runner manages its own containers: by default it pulls
// dolthub/dumbodb:latest and mongo:8.0, starts both, waits for readiness,
// runs the benchmarks, then tears the containers down (unless -f is given).
// Pass -dumbodb-src to opt into building DumboDB from a local source tree
// instead (useful for benching unreleased commits).
//
// Usage:
//
//	go run ./benchmarks/cmd/compare                    # all benchmarks, teardown after
//	go run ./benchmarks/cmd/compare -bench '^BenchmarkUpdateMany$' -f  # investigate one op
//
// The comparator intentionally does NOT verify functional parity — that is the
// existing tests/ harness's job. It only measures elapsed time per operation.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	benchRegex    = flag.String("bench", "^Benchmark", "-run pattern for benchmarks (go test -bench)")
	benchTime     = flag.String("benchtime", "2s", "-benchtime value passed to go test")
	count         = flag.Int("count", 1, "-count value passed to go test (repetitions per benchmark)")
	csvOut        = flag.String("csv", "", "if set, write CSV results to this path")
	benchPkg      = flag.String("pkg", "./benchmarks", "Go package containing the benchmarks")
	verbose       = flag.Bool("v", false, "stream go test output to stderr as it runs")
	keepAlive     = flag.Bool("f", false, "keep containers running after benchmarks complete (for investigation)")
	dumboImage    = flag.String("dumbodb-image", envOr("DUMBODB_IMAGE", "dolthub/dumbodb:latest"), "Docker image tag to pull and run as DumboDB. Ignored when -dumbodb-src is set.")
	dumboSrc      = flag.String("dumbodb-src", envOr("DUMBODB_SRC", ""), "if set, build DumboDB from this source directory (using benchmarks/Dockerfile.dumbodb) instead of pulling -dumbodb-image")
	noContainers  = flag.Bool("no-containers", false, "skip container management entirely (expects servers already reachable at the default ports)")
	healthTimeout = flag.Duration("health-timeout", 60*time.Second, "how long to wait for each container to accept connections")
	testTimeout   = flag.Duration("test-timeout", 10*time.Minute, "-timeout value passed to go test (caps the entire bench run; large-N benchmarks exceed the 10m default while seeding)")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// result is one (benchmark, target) data point.
type result struct {
	Name    string
	Target  string
	URI     string
	N       int // iterations
	NsPerOp float64
}

// combined is what we emit as CSV: one row per benchmark with both sides.
type combined struct {
	Name      string
	DumboDBNs *float64
	MongoNs   *float64
	PctChange *float64 // (dumbodb - mongodb) / mongodb * 100
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	if !*noContainers {
		if err := setupContainers(ctx); err != nil {
			return fmt.Errorf("container setup: %w", err)
		}
		// Teardown iff we're not keeping containers alive. Always runs even if
		// benchmarks fail partway — a stopped container from a crashed run is
		// much less confusing than leftover daemons.
		if !*keepAlive {
			defer stopContainer(context.Background(), dumboContainer)
			defer stopContainer(context.Background(), mongoContainer)
		}
	}

	dumboResults, dumboErr := runTargetBench(ctx, "dumbodb", dumboContainer)
	mongoResults, mongoErr := runTargetBench(ctx, "mongodb", mongoContainer)
	if dumboErr != nil || mongoErr != nil {
		return errors.Join(dumboErr, mongoErr)
	}

	rows := merge(dumboResults, mongoResults)
	printTable(os.Stdout, rows)

	if *csvOut != "" {
		if err := writeCSV(*csvOut, rows); err != nil {
			return err
		}
	}

	if *keepAlive && !*noContainers {
		printKeepAliveBanner(os.Stdout)
	}
	return nil
}

// setupContainers brings both target servers up and waits for them to accept
// connections. It short-circuits when containers are already running so the
// investigation flow (`-f`, then re-run) stays fast.
func setupContainers(ctx context.Context) error {
	if err := ensureMongoImage(ctx); err != nil {
		return fmt.Errorf("pull %s: %w", mongoContainer.image, err)
	}
	if err := prepareDumboImage(ctx); err != nil {
		return err
	}
	if err := startContainer(ctx, mongoContainer); err != nil {
		return err
	}
	if err := startContainer(ctx, dumboContainer); err != nil {
		return err
	}
	if err := waitHealthy(ctx, mongoContainer, *healthTimeout); err != nil {
		return err
	}
	if err := waitHealthy(ctx, dumboContainer, *healthTimeout); err != nil {
		return err
	}
	return nil
}

// prepareDumboImage selects the DumboDB image based on the flags and ensures
// it is available locally. When -dumbodb-src is set, build dumbodb-bench:local
// from that source tree. Otherwise pull -dumbodb-image from the registry.
// In both cases dumboContainer is mutated to point at the chosen image and the
// matching host-to-container port mapping for that image.
func prepareDumboImage(ctx context.Context) error {
	if *dumboSrc != "" {
		dumboContainer.image = "dumbodb-bench:local"
		dumboContainer.runArgs = []string{"-p", "127.0.0.1:27018:27018"}
		if err := buildDumboImage(ctx, *dumboSrc); err != nil {
			return fmt.Errorf("build %s: %w", dumboContainer.image, err)
		}
		return nil
	}
	dumboContainer.image = *dumboImage
	dumboContainer.runArgs = []string{"-p", "127.0.0.1:27018:27017"}
	if err := ensureDumboImage(ctx); err != nil {
		return fmt.Errorf("pull %s: %w", dumboContainer.image, err)
	}
	return nil
}

func writeCSV(path string, rows []combined) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"name", "dumbodb_ns_per_op", "mongodb_ns_per_op", "percent_change"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, r := range rows {
		if err := w.Write([]string{r.Name, fmtNs(r.DumboDBNs), fmtNs(r.MongoNs), fmtPct(r.PctChange)}); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}

// fmtNs formats an optional ns/op value as a fixed-point string, empty when nil.
func fmtNs(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

// fmtPct formats an optional percent-change value, empty when nil.
func fmtPct(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

func printKeepAliveBanner(w *os.File) {
	fmt.Fprintln(w, "Servers still running (-f):")
	fmt.Fprintf(w, "  DumboDB: %s\n", dumboContainer.hostURI)
	fmt.Fprintf(w, "  MongoDB: %s\n", mongoContainer.hostURI)
	fmt.Fprintf(w, "Clean up: docker stop %s %s && docker rm %s %s\n",
		dumboContainer.name, mongoContainer.name,
		dumboContainer.name, mongoContainer.name)
}

// runTargetBench verifies the target container is still running (and accepts
// connections) before handing off to runBench. The early check exists because
// an opaque "dial tcp ... connection refused" bubbling up from inside `go
// test` is much harder to triage than an explicit "container X is not running
// anymore" from the runner — which is exactly the failure mode pa-c8s
// reported against the first cut of this runner.
//
// When -no-containers is set, we trust the user and skip the verification.
func runTargetBench(ctx context.Context, label string, c container) ([]result, error) {
	if !*noContainers {
		if state := containerState(ctx, c.name); state != "running" {
			return nil, fmt.Errorf(
				"%s container %q is %q, not running — %s",
				label, c.name, stateOrMissing(state),
				"check `docker logs "+c.name+"` for why it exited")
		}
		// Re-ping: container may be alive but mongod/dumbodb crashed inside it.
		// Short timeout — if the server died the TCP refusal is immediate.
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		client, err := mongo.Connect(pingCtx, options.Client().ApplyURI(c.hostURI))
		if err == nil {
			err = client.Ping(pingCtx, nil)
			_ = client.Disconnect(context.Background())
		}
		if err != nil {
			return nil, fmt.Errorf(
				"%s container %q is running but not reachable at %s: %w — check `docker logs %s`",
				label, c.name, c.hostURI, err, c.name)
		}
	}
	return runBench(label, c.hostURI)
}

func stateOrMissing(s string) string {
	if s == "" {
		return "missing"
	}
	return s
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
		"-timeout", testTimeout.String(),
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
//
//	BenchmarkInsertOne-8            123    4567 ns/op    ...
//
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
			pct := ((*c.DumboDBNs - *c.MongoNs) / *c.MongoNs) * 100
			c.PctChange = &pct
		}
		rows = append(rows, *c)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func printTable(w *os.File, rows []combined) {
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	fmt.Fprintln(tw, "test_name\tdumbodb_latency\tmongodb_latency\tpercent_change")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			benchLabel(r.Name),
			fmtMs(r.DumboDBNs),
			fmtMs(r.MongoNs),
			fmtPctChange(r.DumboDBNs, r.MongoNs),
		)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

// benchLabel strips the "Benchmark" prefix and converts to snake_case.
func benchLabel(name string) string {
	name = strings.TrimPrefix(name, "Benchmark")
	// Insert underscore before uppercase runs: "FindOne" -> "Find_One"
	var b strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(name[i-1])
			if prev >= 'a' && prev <= 'z' {
				b.WriteRune('_')
			}
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func fmtMs(ns *float64) string {
	if ns == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *ns/1e6)
}

func fmtPctChange(dumboNs, mongoNs *float64) string {
	if dumboNs == nil || mongoNs == nil || *mongoNs == 0 {
		return "-"
	}
	pct := ((*dumboNs - *mongoNs) / *mongoNs) * 100
	return fmt.Sprintf("%.1f", pct)
}
