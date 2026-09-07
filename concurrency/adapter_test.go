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
	executed atomic.Int64
	setup    atomic.Bool
	verified atomic.Bool
}

type invalidOutcomeScenario struct {
	countingScenario
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
	s.setup.Store(true)
	return nil
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
	if !scenario.setup.Load() || !scenario.verified.Load() {
		t.Fatal("scenario lifecycle was not completed")
	}
	if !target.dropped.Load() || !target.disconnected.Load() {
		t.Fatal("target lifecycle was not completed")
	}
}
