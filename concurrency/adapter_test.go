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

package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type fakeTarget struct {
	collection   Collection
	dropped      atomic.Bool
	disconnected atomic.Bool
}

func (t *fakeTarget) Ping(context.Context) error {
	return nil
}

func (t *fakeTarget) Identity(context.Context) (ServerInfo, error) {
	return ServerInfo{Product: "fake", Version: "1", Revision: "test"}, nil
}

func TestProductFromBuildInfo(t *testing.T) {
	if got := productFromBuildInfo(bson.M{"storageEngines": primitive.A{"wiredTiger"}}); got != "MongoDB" {
		t.Fatalf("MongoDB product=%q", got)
	}
	if got := productFromBuildInfo(bson.M{"storageEngines": primitive.A{"dolt"}}); got != "DumboDB" {
		t.Fatalf("DumboDB product=%q", got)
	}
}

func (t *fakeTarget) Collection(string, string) Collection {
	return t.collection
}

func (t *fakeTarget) DropDatabase(context.Context, string) error {
	t.dropped.Store(true)
	return nil
}

func (t *fakeTarget) Disconnect(context.Context) error {
	t.disconnected.Store(true)
	return nil
}

type unusedCollection struct{}

func (unusedCollection) InsertOne(context.Context, interface{}) error {
	return nil
}

func (unusedCollection) FindOne(context.Context, interface{}, interface{}) error {
	return nil
}

func (unusedCollection) UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

type countingScenario struct {
	executed          atomic.Int64
	active            atomic.Int64
	setup             atomic.Bool
	verified          atomic.Bool
	verifiedQuiescent atomic.Bool
	setupDelay        time.Duration
}

type failingLifecycleScenario struct {
	setupErr  error
	verifyErr error
	executed  atomic.Int64
}

func (s *failingLifecycleScenario) Name() string {
	return "failing-lifecycle"
}

func (s *failingLifecycleScenario) Setup(context.Context, Collection) error {
	return s.setupErr
}

func (s *failingLifecycleScenario) Execute(context.Context, Collection, int, int64) Outcome {
	s.executed.Add(1)
	return Outcome{Kind: OutcomeMatched}
}

func (s *failingLifecycleScenario) Verify(context.Context, Collection, LedgerSnapshot) ([]Check, error) {
	return nil, s.verifyErr
}

type invalidOutcomeScenario struct {
	countingScenario
}

type absurdGenerationScenario struct {
	countingScenario
}

func (s *absurdGenerationScenario) Execute(context.Context, Collection, int, int64) Outcome {
	return Outcome{
		Kind:     OutcomeMatched,
		Modified: true,
		CAS: &CASOperation{
			ObservedGeneration: 1 << 40,
			ProposedGeneration: (1 << 40) + 1,
		},
	}
}

func (s *absurdGenerationScenario) Verify(_ context.Context, _ Collection, ledger LedgerSnapshot) ([]Check, error) {
	return []Check{{Name: "validEdges", Passed: ledger.CAS.InvalidEdges == 0}}, nil
}

type interruptedScenario struct {
	started           chan struct{}
	verifySawCanceled atomic.Bool
}

func (s *interruptedScenario) Name() string {
	return "interrupted"
}

func (s *interruptedScenario) Setup(context.Context, Collection) error {
	return nil
}

func (s *interruptedScenario) Execute(ctx context.Context, _ Collection, _ int, _ int64) Outcome {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
	<-ctx.Done()
	return Outcome{Kind: OutcomeIndeterminate, Err: ctx.Err()}
}

func (s *interruptedScenario) Verify(ctx context.Context, _ Collection, _ LedgerSnapshot) ([]Check, error) {
	s.verifySawCanceled.Store(ctx.Err() != nil)
	return []Check{{Name: "final state read", Passed: true}}, nil
}

func (s *invalidOutcomeScenario) Execute(context.Context, Collection, int, int64) Outcome {
	return Outcome{Kind: OutcomeNoMatch, Modified: true}
}

func (s *countingScenario) Name() string {
	return "counting"
}

func TestRunWithTargetFailsOnLedgerRecordError(t *testing.T) {
	target := &fakeTarget{collection: unusedCollection{}}
	scenario := &invalidOutcomeScenario{}
	cfg := Config{
		TargetURI:  "mongodb://unused",
		Operations: 100,
		Workers:    8,
		Database:   "test",
		Collection: "documents",
		Scenario:   "invalid",
	}
	result, err := RunWithTarget(context.Background(), cfg, scenario, target)
	if err == nil {
		t.Fatal("expected invalid outcome to fail the run")
	}
	if result.RunError == "" || result.Verdict() != VerdictFailed {
		t.Fatalf("runner error was not reported as failed: %+v", result)
	}
}

func TestRunWithTargetRejectsAbsurdObservedGenerationWithBoundedMemory(t *testing.T) {
	target := &fakeTarget{collection: unusedCollection{}}
	result, err := RunWithTarget(context.Background(), Config{
		TargetURI:  "mongodb://unused",
		Operations: 1,
		Workers:    1,
		Database:   "test",
		Collection: "documents",
		Scenario:   "absurd",
	}, &absurdGenerationScenario{}, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict() != VerdictFailed || result.Ledger.CAS.InvalidEdges != 1 {
		t.Fatalf("absurd generation was not a clean failure: %+v", result)
	}
	if result.Ledger.CAS.ObservedGenerationSlots != 0 || result.Ledger.CAS.TrackerBytes != 0 {
		t.Fatalf("absurd generation allocated dense tracking: %+v", result.Ledger.CAS)
	}
	if len(result.Ledger.CAS.Findings) != 1 || result.Ledger.CAS.Findings[0].ObservedGeneration != 1<<40 {
		t.Fatalf("missing bounded invalid-edge evidence: %+v", result.Ledger.CAS.Findings)
	}
	if err := result.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestReserveOperationDoesNotOvershoot(t *testing.T) {
	var issued atomic.Int64
	var workers sync.WaitGroup
	var reserved atomic.Int64
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				if _, ok := reserveOperation(&issued, 1000); !ok {
					return
				}
				reserved.Add(1)
			}
		}()
	}
	workers.Wait()
	if issued.Load() != 1000 || reserved.Load() != 1000 {
		t.Fatalf("issued=%d reserved=%d", issued.Load(), reserved.Load())
	}
}

func (s *countingScenario) Setup(context.Context, Collection) error {
	if s.setupDelay > 0 {
		time.Sleep(s.setupDelay)
	}
	s.setup.Store(true)
	return nil
}

func TestDurationStartsAfterSetup(t *testing.T) {
	target := &fakeTarget{collection: unusedCollection{}}
	scenario := &countingScenario{setupDelay: 20 * time.Millisecond}
	result, err := RunWithTarget(context.Background(), Config{
		TargetURI:  "mongodb://unused",
		Duration:   10 * time.Millisecond,
		Operations: 1,
		Workers:    1,
		Database:   "test",
		Collection: "documents",
		Scenario:   "counting",
	}, scenario, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Ledger.Attempts != 1 {
		t.Fatalf("attempts=%d, setup consumed workload duration", result.Ledger.Attempts)
	}
	if err := result.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func (s *countingScenario) Execute(context.Context, Collection, int, int64) Outcome {
	s.active.Add(1)
	defer s.active.Add(-1)
	s.executed.Add(1)
	return Outcome{Kind: OutcomeMatched}
}

func (s *countingScenario) Verify(context.Context, Collection, LedgerSnapshot) ([]Check, error) {
	s.verified.Store(true)
	s.verifiedQuiescent.Store(s.active.Load() == 0)
	return []Check{{Name: "verified", Passed: true}}, nil
}

func TestRunWithFixtureCreatesCollectionBeforeScenarioSetup(t *testing.T) {
	collection := unusedCollection{}
	target := &fakeTarget{}
	scenario := &countingScenario{}
	var created atomic.Bool
	fixture := FixtureFunc(func(_ context.Context, received Target, cfg Config) (Collection, error) {
		if received != target {
			t.Fatal("fixture received a different target")
		}
		if cfg.Database != "test" || cfg.Collection != "documents" {
			t.Fatalf("fixture received wrong config: %+v", cfg)
		}
		created.Store(true)
		return collection, nil
	})
	result, err := RunWithFixture(context.Background(), Config{
		TargetURI:  "mongodb://unused",
		Operations: 100,
		Workers:    8,
		Database:   "test",
		Collection: "documents",
		Scenario:   "counting",
	}, scenario, target, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Load() || !scenario.setup.Load() || !scenario.verified.Load() {
		t.Fatal("configured fixture lifecycle was not completed")
	}
	if !scenario.verifiedQuiescent.Load() {
		t.Fatal("verification ran before workers stopped")
	}
	if err := result.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestRunWithFixtureStopsBeforeWritesWhenCreationFails(t *testing.T) {
	target := &fakeTarget{}
	scenario := &countingScenario{}
	fixtureErr := errors.New("configured collection rejected")
	result, err := RunWithFixture(context.Background(), Config{
		TargetURI:  "mongodb://unused",
		Operations: 100,
		Workers:    8,
		Database:   "test",
		Collection: "documents",
		Scenario:   "counting",
	}, scenario, target, FixtureFunc(func(context.Context, Target, Config) (Collection, error) {
		return nil, fixtureErr
	}))
	if !errors.Is(err, fixtureErr) {
		t.Fatalf("run error = %v, want %v", err, fixtureErr)
	}
	if scenario.setup.Load() || scenario.executed.Load() != 0 || scenario.verified.Load() {
		t.Fatal("scenario ran after fixture creation failed")
	}
	if result.RunError == "" {
		t.Fatal("fixture failure was not retained in the result")
	}
	if err := result.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestCommonLifecycleSeparatesSetupAndVerificationFailures(t *testing.T) {
	cfg := Config{
		TargetURI:  "mongodb://unused",
		Operations: 10,
		Workers:    2,
		Database:   "test",
		Collection: "documents",
		Scenario:   "failing-lifecycle",
	}
	setupErr := errors.New("seed failed")
	setupScenario := &failingLifecycleScenario{setupErr: setupErr}
	setupResult, err := RunWithTarget(context.Background(), cfg, setupScenario, &fakeTarget{collection: unusedCollection{}})
	if !errors.Is(err, setupErr) {
		t.Fatalf("setup error = %v, want %v", err, setupErr)
	}
	if setupScenario.executed.Load() != 0 || !setupResult.WorkloadStartedAt.IsZero() {
		t.Fatal("workload started after scenario setup failed")
	}
	if err := setupResult.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	verifyErr := errors.New("final read failed")
	verifyScenario := &failingLifecycleScenario{verifyErr: verifyErr}
	verifyResult, err := RunWithTarget(context.Background(), cfg, verifyScenario, &fakeTarget{collection: unusedCollection{}})
	if !errors.Is(err, verifyErr) {
		t.Fatalf("verification error = %v, want %v", err, verifyErr)
	}
	if verifyScenario.executed.Load() != cfg.Operations {
		t.Fatalf("executed=%d, want %d", verifyScenario.executed.Load(), cfg.Operations)
	}
	if verifyResult.WorkloadFinishedAt.IsZero() || verifyResult.RunError == "" {
		t.Fatal("verification failure did not retain completed workload evidence")
	}
	if err := verifyResult.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestRunWithTargetUsesInjectedAdapter(t *testing.T) {
	target := &fakeTarget{collection: unusedCollection{}}
	scenario := &countingScenario{}
	cfg := Config{
		TargetURI:  "mongodb://unused",
		Operations: 1000,
		Workers:    8,
		Database:   "test",
		Collection: "documents",
		Scenario:   "counting",
	}
	result, err := RunWithTarget(context.Background(), cfg, scenario, target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Ledger.Attempts != 1000 || scenario.executed.Load() != 1000 {
		t.Fatalf("attempts=%d executed=%d", result.Ledger.Attempts, scenario.executed.Load())
	}
	if result.WorkloadStartedAt.IsZero() || result.WorkloadFinishedAt.Before(result.WorkloadStartedAt) {
		t.Fatalf("invalid workload window: %s to %s", result.WorkloadStartedAt, result.WorkloadFinishedAt)
	}
	if result.Config.LatencyScope != "update" {
		t.Fatalf("latency scope=%q", result.Config.LatencyScope)
	}
	if !scenario.setup.Load() || !scenario.verified.Load() {
		t.Fatal("scenario lifecycle was not completed")
	}
	if err := result.Finalize(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !target.dropped.Load() || !target.disconnected.Load() {
		t.Fatal("target lifecycle was not completed")
	}
}

func TestInterruptedRunVerifiesWithFreshContextAndPreservesEvidence(t *testing.T) {
	target := &fakeTarget{collection: unusedCollection{}}
	scenario := &interruptedScenario{started: make(chan struct{})}
	cfg := Config{
		TargetURI:  "mongodb://unused",
		Duration:   time.Minute,
		Workers:    1,
		Database:   "test",
		Collection: "documents",
		Scenario:   "interrupted",
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var result LifecycleResult
	var runErr error
	go func() {
		result, runErr = RunWithTarget(ctx, cfg, scenario, target)
		close(done)
	}()
	<-scenario.started
	cancel()
	<-done
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !result.Truncated || result.StopReason != context.Canceled.Error() {
		t.Fatalf("truncated=%t stopReason=%q", result.Truncated, result.StopReason)
	}
	if scenario.verifySawCanceled.Load() {
		t.Fatal("verification received canceled signal context")
	}
	if result.Passed() {
		t.Fatal("truncated run reported a conclusive pass")
	}
	if err := result.Finalize(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if target.dropped.Load() {
		t.Fatal("interrupted run database was dropped")
	}
	if !target.disconnected.Load() {
		t.Fatal("interrupted target was not disconnected")
	}
}
