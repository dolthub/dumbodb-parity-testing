// perfgate compares two DumboDB builds (base and head) on the benchmark suite
// and exits non-zero when head is slower than base on any benchmark.
//
// Both builds run on the same machine. Each round starts both servers on fresh
// data directories and runs every benchmark of the shard against each, base and
// head alternating which goes first. Iteration counts are calibrated once, on
// base, so every sample of a benchmark does the same work. A benchmark
// regresses when head's median is more than -threshold slower and a one-sided
// Mann-Whitney test gives p < -alpha; flagged benchmarks are re-sampled
// -confirm-samples more times before the verdict.
//
// Usage (from the repository root):
//
//	go run ./benchmarks/cmd/perfgate -base-bin /tmp/dumbodb-base -head-bin /tmp/dumbodb-head \
//	    -data-root /mnt/perf -shard 0 -shards 4
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	baseBin        = flag.String("base-bin", "", "DumboDB binary built from the base commit")
	headBin        = flag.String("head-bin", "", "DumboDB binary built from the head commit")
	dataRoot       = flag.String("data-root", "", "directory for server data (use a memory filesystem)")
	benchPkg       = flag.String("pkg", "./benchmarks", "Go package containing the benchmarks")
	benchFilter    = flag.String("bench", "^Benchmark", "regexp selecting benchmarks before sharding")
	shard          = flag.Int("shard", 0, "index of this shard")
	shards         = flag.Int("shards", 1, "number of shards")
	samples        = flag.Int("samples", 5, "samples per build per benchmark")
	confirmSamples = flag.Int("confirm-samples", 5, "extra samples per build for benchmarks that look regressed")
	calibrate      = flag.Duration("calibrate", 300*time.Millisecond, "target timed duration per sample, used to pick iteration counts")
	threshold      = flag.Float64("threshold", 0.10, "fractional median slowdown that counts as a regression")
	alpha          = flag.Float64("alpha", 0.05, "one-sided Mann-Whitney significance level")
	basePort       = flag.Int("base-port", 27401, "port for the base server")
	headPort       = flag.Int("head-port", 27402, "port for the head server")
	healthTimeout  = flag.Duration("health-timeout", 60*time.Second, "how long to wait for a server to accept connections")
	benchTimeout   = flag.Duration("bench-timeout", 10*time.Minute, "timeout for one benchmark run")
	summaryPath    = flag.String("summary", "", "append a markdown report to this file (e.g. $GITHUB_STEP_SUMMARY)")
	jsonPath       = flag.String("json", "", "write per-benchmark samples and verdicts to this file")
	verbose        = flag.Bool("v", false, "log every benchmark run")
)

type verdict string

const (
	verdictOK         verdict = "ok"
	verdictRegression verdict = "REGRESSION"
	verdictFaster     verdict = "faster"
)

type benchResult struct {
	Name       string    `json:"name"`
	Iterations int       `json:"iterations"`
	Base       []float64 `json:"base_ns_per_op"`
	Head       []float64 `json:"head_ns_per_op"`
	BaseMedian float64   `json:"base_median"`
	HeadMedian float64   `json:"head_median"`
	Delta      float64   `json:"delta"`
	PSlower    float64   `json:"p_slower"`
	PFaster    float64   `json:"p_faster"`
	Verdict    verdict   `json:"verdict"`
}

func main() {
	flag.Parse()
	regressed, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "perfgate: %v\n", err)
		os.Exit(2)
	}
	if regressed {
		os.Exit(1)
	}
}

func run() (bool, error) {
	if *baseBin == "" || *headBin == "" || *dataRoot == "" {
		return false, errors.New("-base-bin, -head-bin, and -data-root are required")
	}
	if *shard < 0 || *shard >= *shards {
		return false, fmt.Errorf("-shard %d out of range for -shards %d", *shard, *shards)
	}
	start := time.Now()

	testBin := filepath.Join(*dataRoot, "bench.test")
	if out, err := exec.Command("go", "test", "-c", "-o", testBin, *benchPkg).CombinedOutput(); err != nil {
		return false, fmt.Errorf("compile benchmarks: %w\n%s", err, out)
	}
	all, err := listBenchmarks(testBin)
	if err != nil {
		return false, err
	}
	names := assignShard(all, *shard, *shards)
	logf("shard %d/%d: %d of %d benchmarks", *shard, *shards, len(names), len(all))

	iterations, err := calibrateIterations(testBin, names)
	if err != nil {
		return false, err
	}
	names = names[:0]
	for name := range iterations {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make(map[string]*benchResult, len(names))
	for _, name := range names {
		results[name] = &benchResult{Name: name, Iterations: iterations[name]}
	}

	round := 1
	if err := sampleRounds(testBin, names, results, &round, *samples); err != nil {
		return false, err
	}
	var suspects []string
	for _, name := range names {
		if judge(results[name]) == verdictRegression {
			suspects = append(suspects, name)
		}
	}
	if len(suspects) > 0 && *confirmSamples > 0 {
		logf("re-sampling %d suspected regressions: %s", len(suspects), strings.Join(suspects, ", "))
		if err := sampleRounds(testBin, suspects, results, &round, *confirmSamples); err != nil {
			return false, err
		}
	}

	ordered := make([]*benchResult, 0, len(names))
	regressed := false
	for _, name := range names {
		r := results[name]
		r.Verdict = judge(r)
		regressed = regressed || r.Verdict == verdictRegression
		ordered = append(ordered, r)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, rj := ordered[i].Verdict == verdictRegression, ordered[j].Verdict == verdictRegression
		if ri != rj {
			return ri
		}
		return ordered[i].Delta > ordered[j].Delta
	})

	report := markdownReport(ordered, time.Since(start))
	fmt.Print(report)
	if *summaryPath != "" {
		f, err := os.OpenFile(*summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return false, err
		}
		_, werr := f.WriteString(report)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return false, werr
		}
	}
	if *jsonPath != "" {
		data, err := json.MarshalIndent(ordered, "", "  ")
		if err != nil {
			return false, err
		}
		if err := os.WriteFile(*jsonPath, data, 0o644); err != nil {
			return false, err
		}
	}
	return regressed, nil
}

func judge(r *benchResult) verdict {
	r.BaseMedian, r.HeadMedian = median(r.Base), median(r.Head)
	if r.BaseMedian > 0 {
		r.Delta = r.HeadMedian/r.BaseMedian - 1
	}
	r.PSlower = slowerPValue(r.Base, r.Head)
	r.PFaster = slowerPValue(r.Head, r.Base)
	switch {
	case r.Delta > *threshold && r.PSlower < *alpha:
		return verdictRegression
	case r.Delta < -*threshold && r.PFaster < *alpha:
		return verdictFaster
	default:
		return verdictOK
	}
}

func listBenchmarks(testBin string) ([]string, error) {
	out, err := exec.Command(testBin, "-test.list", *benchFilter).Output()
	if err != nil {
		return nil, fmt.Errorf("list benchmarks: %w", err)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Benchmark") {
			names = append(names, strings.TrimSpace(line))
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no benchmarks match %q", *benchFilter)
	}
	return names, nil
}

// benchWeight estimates relative cost; seeding dominates the scaled variants.
func benchWeight(name string) int {
	switch {
	case strings.Contains(name, "50K"):
		return 6
	case strings.Contains(name, "10K"):
		return 2
	default:
		return 1
	}
}

func assignShard(names []string, index, count int) []string {
	sorted := append([]string(nil), names...)
	sort.Slice(sorted, func(i, j int) bool {
		wi, wj := benchWeight(sorted[i]), benchWeight(sorted[j])
		if wi != wj {
			return wi > wj
		}
		return sorted[i] < sorted[j]
	})
	load := make([]int, count)
	var mine []string
	for _, name := range sorted {
		lightest := 0
		for s := 1; s < count; s++ {
			if load[s] < load[lightest] {
				lightest = s
			}
		}
		load[lightest] += benchWeight(name)
		if lightest == index {
			mine = append(mine, name)
		}
	}
	sort.Strings(mine)
	return mine
}

func calibrateIterations(testBin string, names []string) (map[string]int, error) {
	srv, err := startServer("base", *baseBin, *basePort, roundDataDir(*dataRoot, "base", 0))
	if err != nil {
		return nil, err
	}
	defer srv.stop()

	iterations := make(map[string]int, len(names))
	for _, name := range names {
		res, skipped, err := runBenchmark(testBin, name, srv, calibrate.String())
		if err != nil {
			return nil, err
		}
		if skipped {
			logf("%s: skipped by the benchmark", name)
			continue
		}
		perOp := time.Duration(res.nsPerOp)
		n := 1
		if perOp > 0 {
			n = int(*calibrate / perOp)
		}
		iterations[name] = max(n, 1)
		logf("%s: %d iterations (%.3f ms/op)", name, iterations[name], res.nsPerOp/1e6)
	}
	return iterations, nil
}

func sampleRounds(testBin string, names []string, results map[string]*benchResult, round *int, count int) error {
	for i := 0; i < count; i++ {
		if err := sampleRound(testBin, names, results, *round); err != nil {
			return err
		}
		*round++
	}
	return nil
}

func sampleRound(testBin string, names []string, results map[string]*benchResult, round int) error {
	base, err := startServer("base", *baseBin, *basePort, roundDataDir(*dataRoot, "base", round))
	if err != nil {
		return err
	}
	defer base.stop()
	head, err := startServer("head", *headBin, *headPort, roundDataDir(*dataRoot, "head", round))
	if err != nil {
		return err
	}
	defer head.stop()

	order := []*server{base, head}
	if round%2 == 0 {
		order = []*server{head, base}
	}
	for _, name := range names {
		r := results[name]
		benchtime := fmt.Sprintf("%dx", r.Iterations)
		for _, srv := range order {
			res, skipped, err := runBenchmark(testBin, name, srv, benchtime)
			if err != nil {
				return err
			}
			if skipped {
				return fmt.Errorf("%s skipped on %s after calibrating", name, srv.label)
			}
			if srv == base {
				r.Base = append(r.Base, res.nsPerOp)
			} else {
				r.Head = append(r.Head, res.nsPerOp)
			}
		}
	}
	logf("round %d done", round)
	return nil
}

type runResult struct {
	iterations int
	nsPerOp    float64
}

var resultLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([0-9.]+) ns/op`)

func runBenchmark(testBin, name string, srv *server, benchtime string) (runResult, bool, error) {
	if !srv.alive() {
		return runResult{}, false, fmt.Errorf("%s server is not running; log: %s", srv.label, srv.logPath)
	}
	cmd := exec.Command(testBin,
		"-test.run", "^$",
		"-test.bench", "^"+regexp.QuoteMeta(name)+"$",
		"-test.benchtime", benchtime,
		"-test.timeout", benchTimeout.String(),
		"-bench.target-uri", srv.uri(),
		"-bench.target-name", srv.label,
	)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	started := time.Now()
	err := cmd.Run()
	if err != nil {
		return runResult{}, false, fmt.Errorf("%s on %s: %w\n%s", name, srv.label, err, out.String())
	}

	var res runResult
	found := false
	scanner := bufio.NewScanner(bytes.NewReader(out.Bytes()))
	for scanner.Scan() {
		m := resultLine.FindStringSubmatch(scanner.Text())
		if m == nil || m[1] != name {
			continue
		}
		res.iterations, _ = strconv.Atoi(m[2])
		res.nsPerOp, _ = strconv.ParseFloat(m[3], 64)
		found = true
	}
	if !found {
		return runResult{}, true, nil
	}
	if *verbose {
		logf("%s %s %s: %.3f ms/op (%s)", name, srv.label, benchtime, res.nsPerOp/1e6, time.Since(started).Round(time.Millisecond))
	}
	return res, false, nil
}

func markdownReport(results []*benchResult, elapsed time.Duration) string {
	var b strings.Builder
	regressions := 0
	for _, r := range results {
		if r.Verdict == verdictRegression {
			regressions++
		}
	}
	fmt.Fprintf(&b, "## Performance: shard %d/%d\n\n", *shard+1, *shards)
	if regressions > 0 {
		fmt.Fprintf(&b, "**%d regression(s)**: head median more than %.0f%% slower than base with p < %.2f.\n\n", regressions, *threshold*100, *alpha)
	} else {
		fmt.Fprintf(&b, "No regressions (threshold %.0f%%, p < %.2f).\n\n", *threshold*100, *alpha)
	}
	fmt.Fprintf(&b, "| Benchmark | Base (ms/op) | Head (ms/op) | Change | p | Samples | Verdict |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|---|\n")
	for _, r := range results {
		p := r.PSlower
		if r.Delta < 0 {
			p = r.PFaster
		}
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %+.1f%% | %.3f | %d | %s |\n",
			strings.TrimPrefix(r.Name, "Benchmark"), r.BaseMedian/1e6, r.HeadMedian/1e6, r.Delta*100, p, len(r.Head), r.Verdict)
	}
	fmt.Fprintf(&b, "\n%d benchmarks in %s.\n\n", len(results), elapsed.Round(time.Second))
	return b.String()
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{time.Now().UTC().Format("15:04:05")}, args...)...)
}
