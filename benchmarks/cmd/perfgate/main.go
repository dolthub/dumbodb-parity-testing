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
	"context"
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
	"syscall"
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
	benchTimeout   = flag.Duration("bench-timeout", 3*time.Minute, "kill a benchmark run after this long and count it as a failure on that build")
	summaryPath    = flag.String("summary", "", "append a markdown report to this file (e.g. $GITHUB_STEP_SUMMARY)")
	jsonPath       = flag.String("json", "", "write per-benchmark samples and verdicts to this file")
	verbose        = flag.Bool("v", false, "log every benchmark run")
)

type verdict string

const (
	verdictOK         verdict = "ok"
	verdictRegression verdict = "REGRESSION"
	verdictFaster     verdict = "faster"
	verdictHeadFailed verdict = "HEAD FAILED"
	// verdictBaseFailed means only base failed: head fixed the benchmark.
	verdictBaseFailed verdict = "base failed"
)

func (v verdict) failsGate() bool { return v == verdictRegression || v == verdictHeadFailed }

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
	BaseError  string    `json:"base_error,omitempty"`
	HeadError  string    `json:"head_error,omitempty"`
	Verdict    verdict   `json:"verdict"`
}

func (r *benchResult) failed() bool { return r.BaseError != "" || r.HeadError != "" }

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

	results, err := calibrateIterations(testBin, names)
	if err != nil {
		return false, err
	}
	names = names[:0]
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)

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
		regressed = regressed || r.Verdict.failsGate()
		ordered = append(ordered, r)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, rj := ordered[i].Verdict.failsGate(), ordered[j].Verdict.failsGate()
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
	switch {
	case r.HeadError != "":
		return verdictHeadFailed
	case r.BaseError != "":
		return verdictBaseFailed
	}
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

// target is one build's server, restarted on demand if it dies mid-run.
type target struct {
	label, bin string
	port       int
	srv        *server
}

func (t *target) ensure(round int) error {
	if t.srv != nil && t.srv.alive() {
		return nil
	}
	if t.srv != nil {
		logf("%s server is not running; restarting (log: %s)", t.label, t.srv.logPath)
		t.srv.stop()
	}
	srv, err := startServer(t.label, t.bin, t.port, roundDataDir(*dataRoot, t.label, round))
	t.srv = srv
	return err
}

func (t *target) stop() {
	if t.srv != nil {
		t.srv.stop()
	}
}

// run returns a benchmark failure in benchErr; a server that cannot be
// started is fatal and returned in err.
func (t *target) run(testBin, name, benchtime string, round int) (res runResult, skipped bool, benchErr, err error) {
	if err := t.ensure(round); err != nil {
		return runResult{}, false, nil, err
	}
	res, skipped, benchErr = runBenchmark(testBin, name, t.srv, benchtime)
	if errors.Is(benchErr, errBenchTimeout) {
		// The server may still be executing the abandoned work; replace it.
		t.srv.stop()
	}
	return res, skipped, benchErr, nil
}

func newTargets() (base, head *target) {
	return &target{label: "base", bin: *baseBin, port: *basePort}, &target{label: "head", bin: *headBin, port: *headPort}
}

// calibrateIterations picks each benchmark's iteration count on base, or on
// head when base fails. Benchmarks that skip themselves are dropped.
func calibrateIterations(testBin string, names []string) (map[string]*benchResult, error) {
	base, head := newTargets()
	defer base.stop()
	defer head.stop()

	results := make(map[string]*benchResult, len(names))
	for _, name := range names {
		r := &benchResult{Name: name}
		res, skipped, benchErr, err := base.run(testBin, name, calibrate.String(), 0)
		if err != nil {
			return nil, err
		}
		if benchErr != nil {
			r.BaseError = errorSummary(benchErr)
			logf("%s: failed on base: %s", name, r.BaseError)
			if res, skipped, benchErr, err = head.run(testBin, name, calibrate.String(), 0); err != nil {
				return nil, err
			}
			if benchErr != nil {
				r.HeadError = errorSummary(benchErr)
				logf("%s: failed on head: %s", name, r.HeadError)
			}
		}
		if skipped {
			logf("%s: skipped by the benchmark", name)
			continue
		}
		results[name] = r
		if r.failed() {
			continue
		}
		n := 1
		if perOp := time.Duration(res.nsPerOp); perOp > 0 {
			n = int(*calibrate / perOp)
		}
		r.Iterations = max(n, 1)
		logf("%s: %d iterations (%.3f ms/op)", name, r.Iterations, res.nsPerOp/1e6)
	}
	return results, nil
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

// sampleRound runs each benchmark once per build on fresh servers. A
// benchmark that fails records the error for that build and is not run again.
func sampleRound(testBin string, names []string, results map[string]*benchResult, round int) error {
	base, head := newTargets()
	defer base.stop()
	defer head.stop()
	if err := base.ensure(round); err != nil {
		return err
	}
	if err := head.ensure(round); err != nil {
		return err
	}

	order := []*target{base, head}
	if round%2 == 0 {
		order = []*target{head, base}
	}
	for _, name := range names {
		r := results[name]
		if r.failed() {
			continue
		}
		benchtime := fmt.Sprintf("%dx", r.Iterations)
		for _, t := range order {
			res, skipped, benchErr, err := t.run(testBin, name, benchtime, round)
			if err != nil {
				return err
			}
			if benchErr == nil && skipped {
				benchErr = errors.New("skipped after calibrating")
			}
			if benchErr != nil {
				msg := errorSummary(benchErr)
				logf("%s: failed on %s: %s", name, t.label, msg)
				if t == base {
					r.BaseError = msg
				} else {
					r.HeadError = msg
				}
				break
			}
			if t == base {
				r.Base = append(r.Base, res.nsPerOp)
			} else {
				r.Head = append(r.Head, res.nsPerOp)
			}
		}
	}
	logf("round %d done", round)
	return nil
}

// errorSummary keeps the informative tail of a benchmark failure.
func errorSummary(err error) string {
	var keep []string
	for _, line := range strings.Split(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "FAIL" || strings.HasPrefix(line, "exit status") || strings.HasPrefix(line, "goos:") ||
			strings.HasPrefix(line, "goarch:") || strings.HasPrefix(line, "pkg:") || strings.HasPrefix(line, "cpu:") {
			continue
		}
		keep = append(keep, line)
	}
	if len(keep) > 3 {
		keep = keep[len(keep)-3:]
	}
	msg := strings.Join(keep, " | ")
	if len(msg) > 400 {
		msg = msg[:400] + "..."
	}
	return msg
}

var errBenchTimeout = errors.New("timed out")

type runResult struct {
	iterations int
	nsPerOp    float64
}

var resultLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([0-9.]+) ns/op`)

func runBenchmark(testBin, name string, srv *server, benchtime string) (runResult, bool, error) {
	if !srv.alive() {
		return runResult{}, false, fmt.Errorf("%s server is not running; log: %s", srv.label, srv.logPath)
	}
	// The test binary's -test.timeout does not cover benchmarks, so the
	// deadline is enforced here.
	ctx, cancel := context.WithTimeout(context.Background(), *benchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBin,
		"-test.run", "^$",
		"-test.bench", "^"+regexp.QuoteMeta(name)+"$",
		"-test.benchtime", benchtime,
		"-bench.target-uri", srv.uri(),
		"-bench.target-name", srv.label,
	)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 10 * time.Second
	started := time.Now()
	err := cmd.Run()
	if ctx.Err() != nil {
		return runResult{}, false, fmt.Errorf("%s on %s: %w after %s", name, srv.label, errBenchTimeout, *benchTimeout)
	}
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
	counts := map[verdict]int{}
	for _, r := range results {
		counts[r.Verdict]++
	}
	fmt.Fprintf(&b, "## Performance: shard %d/%d\n\n", *shard+1, *shards)
	if n := counts[verdictRegression]; n > 0 {
		fmt.Fprintf(&b, "**%d regression(s)**: head median more than %.0f%% slower than base with p < %.2f.\n\n", n, *threshold*100, *alpha)
	} else {
		fmt.Fprintf(&b, "No regressions (threshold %.0f%%, p < %.2f).\n\n", *threshold*100, *alpha)
	}
	if n := counts[verdictHeadFailed]; n > 0 {
		fmt.Fprintf(&b, "**%d benchmark(s) fail on head.**\n\n", n)
	}
	if n := counts[verdictBaseFailed]; n > 0 {
		fmt.Fprintf(&b, "%d benchmark(s) fail on base but pass on head; they are not compared.\n\n", n)
	}
	fmt.Fprintf(&b, "| Benchmark | Base (ms/op) | Head (ms/op) | Change | p | Samples | Verdict |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|---|\n")
	for _, r := range results {
		name := strings.TrimPrefix(r.Name, "Benchmark")
		if r.failed() {
			fmt.Fprintf(&b, "| %s | - | - | - | - | - | %s |\n", name, r.Verdict)
			continue
		}
		p := r.PSlower
		if r.Delta < 0 {
			p = r.PFaster
		}
		fmt.Fprintf(&b, "| %s | %.3f | %.3f | %+.1f%% | %.3f | %d | %s |\n",
			name, r.BaseMedian/1e6, r.HeadMedian/1e6, r.Delta*100, p, len(r.Head), r.Verdict)
	}
	var failures []string
	for _, r := range results {
		if r.HeadError != "" {
			failures = append(failures, fmt.Sprintf("- `%s` on head: `%s`", r.Name, r.HeadError))
		}
		if r.BaseError != "" {
			failures = append(failures, fmt.Sprintf("- `%s` on base: `%s`", r.Name, r.BaseError))
		}
	}
	if len(failures) > 0 {
		fmt.Fprintf(&b, "\n### Failures\n\n%s\n", strings.Join(failures, "\n"))
	}
	fmt.Fprintf(&b, "\n%d benchmarks in %s.\n\n", len(results), elapsed.Round(time.Second))
	return b.String()
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{time.Now().UTC().Format("15:04:05")}, args...)...)
}
