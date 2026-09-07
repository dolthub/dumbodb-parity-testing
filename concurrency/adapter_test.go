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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeTarget struct {
	collection   Collection
	dropped      atomic.Bool
	disconnected atomic.Bool
}

func (t *fakeTarget) Ping(context.Context) error {
	return nil
}

func (t *fakeTarget) Identity(context.Context) (serverInfo, error) {
	return serverInfo{Product: "fake", Version: "1"}, nil
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
	executed   atomic.Int64
	setup      atomic.Bool
	verified   atomic.Bool
	setupDelay time.Duration
}

type invalidOutcomeScenario struct {
	countingScenario
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
	return Outcome{Kind: OutcomeClientError, Err: ctx.Err()}
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
	if _, err := RunWithTarget(context.Background(), cfg, scenario, target); err == nil {
		t.Fatal("expected invalid outcome to fail the run")
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
	s.executed.Add(1)
	return Outcome{Kind: OutcomeMatched}
}

func (s *countingScenario) Verify(context.Context, Collection, LedgerSnapshot) ([]Check, error) {
	s.verified.Store(true)
	return []Check{{Name: "verified", Passed: true}}, nil
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
