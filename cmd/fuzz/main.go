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

// fuzz sends randomly generated hinted reads and writes to MongoDB and
// DumboDB until -duration passes, and reports every command whose results
// differ, grouped by shape, each group with a minimized repro.
//
// Servers come from -mongo-uri/-dumbodb-uri, or local binaries
// (-mongod-bin/-dumbodb-bin), or else Docker containers it starts and stops.
//
// Exit status: 0 no divergences, 1 divergences, 2 the run itself failed
// (setup, or a server stopped answering).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/fuzz"
	"github.com/dolthub/dumbodb-parity-testing/targets"
)

var (
	duration       = flag.Duration("duration", 2*time.Hour, "stop starting new cases after this long")
	maxCases       = flag.Int("cases", 0, "stop after this many cases; zero uses -duration only")
	firstSeed      = flag.Uint64("seed", 0, "seed of the first case, then one more per case; zero picks one from the clock")
	modeFlag       = flag.String("mode", "both", "reads, writes, or both (alternating by seed)")
	readSteps      = flag.Int("read-steps", 40, "filters per reads case; each runs as find, find with limit, count, aggregate and distinct")
	writeSteps     = flag.Int("write-steps", 60, "hinted single-document writes per writes case")
	ignoreOrder    = flag.Bool("ignore-order", false, "do not fail the run on order-only differences (they are still recorded)")
	minimize       = flag.Bool("minimize", true, "shrink the dataset of the first finding of each shape")
	commandTimeout = flag.Duration("command-timeout", 30*time.Second, "per-command timeout; a timeout counts as a server failure")
	outDir         = flag.String("out", "", "directory for findings.jsonl, repro/ and summary.txt; default fuzz-results/<UTC time>")
	database       = flag.String("database", "fuzz_differential", "database the run drops and uses on both servers")
	keepData       = flag.Bool("keep-data", false, "keep the run database at the end")
	replay         = flag.String("replay", "", "replay a repro file against both servers instead of fuzzing")

	mongoURI      = flag.String("mongo-uri", "", "use the MongoDB server at this URI (requires -dumbodb-uri)")
	dumboURI      = flag.String("dumbodb-uri", "", "use the DumboDB server at this URI (requires -mongo-uri)")
	dumboBin      = flag.String("dumbodb-bin", "", "run this dumbodb binary locally (requires -mongod-bin)")
	mongodBin     = flag.String("mongod-bin", "", "run this mongod binary locally (requires -dumbodb-bin)")
	workDir       = flag.String("work-dir", "", "with -dumbodb-bin/-mongod-bin, directory for server data and logs; default <out>/servers (a temporary directory for -replay); data is removed at exit, logs are kept")
	dumboImage    = flag.String("dumbodb-image", "dolthub/dumbodb:latest", "Docker image to run as DumboDB; ignored with -dumbodb-src")
	dumboSrc      = flag.String("dumbodb-src", "", "build the DumboDB image from this dumbodb source directory")
	mongoImage    = flag.String("mongo-image", "mongo:8.0.28", "Docker image to run as MongoDB")
	healthTimeout = flag.Duration("health-timeout", 60*time.Second, "how long to wait for each server to accept connections")

	reportDir   = flag.String("report-dir", "", "write the report (txt, html, repros) to this directory")
	alertCmd    = flag.String("alert-cmd", "", "shell command run with the text report on stdin and the subject in ALERT_SUBJECT")
	emailFrom   = flag.String("email-from", "", "SES verified From address for the report")
	emailTo     = flag.String("email-to", "", "report recipient(s), comma-separated")
	emailRegion = flag.String("email-region", "", "AWS region for SES; default chain region when blank")
)

func main() {
	flag.Parse()
	if *outDir == "" {
		*outDir = filepath.Join("fuzz-results", time.Now().UTC().Format("20060102-150405"))
	}
	code, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	os.Exit(code)
}

// servers are the two running targets and how to stop them.
type servers struct {
	mongoURI, dumboURI string
	local              []*targets.LocalServer
	stop               func()
}

// diagnose explains a server failure with the log tail of a local server
// that exited.
func (s servers) diagnose() string {
	for _, srv := range s.local {
		if !srv.Alive() {
			return fmt.Sprintf("%s exited; log %s ends:\n%s", srv.Label, srv.LogPath, targets.LogTail(srv.LogPath, 30))
		}
	}
	return ""
}

func startServers(ctx context.Context) (servers, error) {
	switch {
	case *mongoURI != "" || *dumboURI != "":
		if *mongoURI == "" || *dumboURI == "" {
			return servers{}, errors.New("-mongo-uri and -dumbodb-uri must be set together")
		}
		return servers{mongoURI: *mongoURI, dumboURI: *dumboURI, stop: func() {}}, nil
	case *dumboBin != "" || *mongodBin != "":
		if *dumboBin == "" || *mongodBin == "" {
			return servers{}, errors.New("-dumbodb-bin and -mongod-bin must be set together")
		}
		return startLocalServers()
	}
	return startContainers(ctx)
}

func startLocalServers() (servers, error) {
	dir := *workDir
	cleanup := func() {}
	if dir == "" && *replay == "" {
		dir = filepath.Join(*outDir, "servers")
	}
	if dir == "" {
		tmp, err := os.MkdirTemp("", "dumbodb-fuzz-")
		if err != nil {
			return servers{}, err
		}
		dir, cleanup = tmp, func() { _ = os.RemoveAll(tmp) }
	}
	mongoSrv, err := targets.StartLocal("mongodb", filepath.Join(dir, "mongodb-data"), targets.MongodCommand(*mongodBin), *healthTimeout)
	if err != nil {
		cleanup()
		return servers{}, err
	}
	dumboSrv, err := targets.StartLocal("dumbodb", filepath.Join(dir, "dumbodb-data"), targets.DumboDBCommand(*dumboBin), *healthTimeout)
	if err != nil {
		mongoSrv.Stop()
		cleanup()
		return servers{}, err
	}
	return servers{
		mongoURI: mongoSrv.URI(),
		dumboURI: dumboSrv.URI(),
		local:    []*targets.LocalServer{mongoSrv, dumboSrv},
		stop: func() {
			dumboSrv.Stop()
			mongoSrv.Stop()
			cleanup()
		},
	}, nil
}

// startContainers uses its own names and ports, so a kept-alive compare run
// (mongodb-bench, dumbodb-bench on 27017/27018) is left alone.
func startContainers(ctx context.Context) (servers, error) {
	mongoC := targets.Container{
		Name: "mongodb-fuzz", Image: *mongoImage, HostURI: "mongodb://127.0.0.1:27037",
		RunArgs: []string{"-p", "127.0.0.1:27037:27017"},
	}
	dumboC := targets.Container{
		Name: "dumbodb-fuzz", Image: *dumboImage, HostURI: "mongodb://127.0.0.1:27038",
		RunArgs: []string{"-p", "127.0.0.1:27038:27017"},
	}
	if err := targets.EnsureImage(ctx, mongoC.Image); err != nil {
		return servers{}, err
	}
	if *dumboSrc != "" {
		// The source-built image listens on 27018 (benchmarks/Dockerfile.dumbodb).
		dumboC.Image, dumboC.RunArgs = "dumbodb-fuzz:local", []string{"-p", "127.0.0.1:27038:27018"}
		if err := targets.BuildDumboDBImage(ctx, dumboC.Image, *dumboSrc); err != nil {
			return servers{}, err
		}
	} else if err := targets.EnsureImage(ctx, dumboC.Image); err != nil {
		return servers{}, err
	}
	stop := func() {
		dumboC.Stop(context.Background())
		mongoC.Stop(context.Background())
	}
	for _, c := range []targets.Container{mongoC, dumboC} {
		if err := c.Fresh(ctx, *healthTimeout); err != nil {
			stop()
			return servers{}, err
		}
	}
	return servers{mongoURI: mongoC.HostURI, dumboURI: dumboC.HostURI, stop: stop}, nil
}

func connect(ctx context.Context, uri string) (*mongo.Client, error) {
	return mongo.Connect(ctx, options.Client().ApplyURI(uri))
}

func serverVersion(ctx context.Context, db *mongo.Database) string {
	var info struct {
		Version string `bson:"version"`
	}
	if err := db.RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&info); err != nil {
		return "unknown"
	}
	return info.Version
}

func modeFor(seed uint64) (fuzz.Mode, int, error) {
	switch *modeFlag {
	case "reads":
		return fuzz.Reads, *readSteps, nil
	case "writes":
		return fuzz.Writes, *writeSteps, nil
	case "both":
		if seed%2 == 0 {
			return fuzz.Reads, *readSteps, nil
		}
		return fuzz.Writes, *writeSteps, nil
	}
	return "", 0, fmt.Errorf("-mode must be reads, writes or both, not %q", *modeFlag)
}

func run() (int, error) {
	if _, _, err := modeFor(0); err != nil {
		return 2, err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv, err := startServers(ctx)
	if err != nil {
		return 2, err
	}
	defer srv.stop()
	mongoClient, err := connect(ctx, srv.mongoURI)
	if err != nil {
		return 2, err
	}
	defer func() { _ = mongoClient.Disconnect(context.Background()) }()
	dumboClient, err := connect(ctx, srv.dumboURI)
	if err != nil {
		return 2, err
	}
	defer func() { _ = dumboClient.Disconnect(context.Background()) }()
	mongoDB, dumboDB := mongoClient.Database(*database), dumboClient.Database(*database)
	for _, db := range []*mongo.Database{mongoDB, dumboDB} {
		if err := db.Drop(ctx); err != nil {
			return 2, fmt.Errorf("drop %s: %w", *database, err)
		}
	}
	if !*keepData {
		defer func() {
			_ = mongoDB.Drop(context.Background())
			_ = dumboDB.Drop(context.Background())
		}()
	}
	runner := &fuzz.Runner{Mongo: mongoDB, DumboDB: dumboDB, CommandTimeout: *commandTimeout, Minimize: *minimize}

	if *replay != "" {
		cmds, err := fuzz.ReadRepro(*replay)
		if err != nil {
			return 2, err
		}
		if n := runner.Replay(ctx, cmds, os.Stdout); n > 0 {
			fmt.Printf("%d of %d commands diverge\n", n, len(cmds))
			return 1, nil
		}
		fmt.Printf("all %d commands match\n", len(cmds))
		return 0, nil
	}
	return fuzzRun(ctx, runner, srv, mongoDB, dumboDB)
}

func fuzzRun(ctx context.Context, runner *fuzz.Runner, srv servers, mongoDB, dumboDB *mongo.Database) (int, error) {
	started := time.Now().UTC()
	dir := *outDir
	rec, err := fuzz.NewRecorder(dir, os.Stderr)
	if err != nil {
		return 2, err
	}
	defer rec.Close()
	runner.Record, runner.IsNew = rec.Record, rec.IsNew

	seed := *firstSeed
	if seed == 0 {
		seed = uint64(started.UnixNano())
	}
	summary := fuzz.RunSummary{
		Date:           started.Format("2006-01-02 15:04"),
		Host:           fuzz.Hostname(),
		DumboDBVersion: serverVersion(ctx, dumboDB),
		MongoDBVersion: serverVersion(ctx, mongoDB),
		FirstSeed:      seed,
		IgnoreOrder:    *ignoreOrder,
		OutDir:         dir,
	}
	fmt.Fprintf(os.Stderr, "==> fuzzing DumboDB %s against MongoDB %s from seed %d for %s; output in %s\n",
		summary.DumboDBVersion, summary.MongoDBVersion, seed, *duration, dir)

	runCtx, stopRun := context.WithTimeout(ctx, *duration)
	defer stopRun()
	for ; *maxCases == 0 || summary.Cases < *maxCases; seed++ {
		if runCtx.Err() != nil {
			break
		}
		mode, steps, _ := modeFor(seed)
		c := fuzz.Generate(seed, mode, steps)
		err := runner.RunCase(runCtx, c)
		if runCtx.Err() != nil {
			break
		}
		summary.Cases++
		summary.Commands += len(c.Steps)
		if errors.Is(err, fuzz.ErrServerFailure) {
			summary.Err = fmt.Errorf("seed %d: %w. %s", seed, err, srv.diagnose())
			break
		}
		fmt.Fprintf(os.Stderr, "case %d seed %d %s: %d commands; %d shapes so far\n",
			summary.Cases, seed, mode, len(c.Steps), len(rec.Groups()))
	}
	summary.Elapsed = time.Since(started)
	if err := rec.WriteErr(); err != nil && summary.Err == nil {
		summary.Err = fmt.Errorf("writing findings: %w", err)
	}

	groups := rec.Groups()
	report := fuzz.BuildReport(summary, groups)
	fmt.Print(report.TextBody)
	if err := os.WriteFile(filepath.Join(dir, "summary.txt"), []byte(report.TextBody), 0o644); err != nil && summary.Err == nil {
		summary.Err = err
	}
	notifyCtx, cancelNotify := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelNotify()
	for _, notify := range (targets.NotifyConfig{
		ReportDir: *reportDir, AlertCmd: *alertCmd,
		EmailFrom: *emailFrom, EmailTo: *emailTo, EmailRegion: *emailRegion,
	}).Notifiers() {
		notify(notifyCtx, report)
	}

	if summary.Err != nil {
		return 2, summary.Err
	}
	for _, g := range groups {
		if summary.Failing(g) {
			return 1, nil
		}
	}
	return 0, nil
}
