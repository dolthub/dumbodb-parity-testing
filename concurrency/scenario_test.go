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
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type finalDocumentCollection struct {
	document bson.M
}

func TestDeterministicDelayUsesSeedAndSequence(t *testing.T) {
	maximum := 10 * time.Second
	first := deterministicDelay(7, 11, maximum)
	if first != deterministicDelay(7, 11, maximum) {
		t.Fatal("same seed and sequence produced different delays")
	}
	if first == deterministicDelay(8, 11, maximum) {
		t.Fatal("different seeds produced the same test delay")
	}
	if first < 0 || first > maximum {
		t.Fatalf("delay %s outside [0,%s]", first, maximum)
	}
	if deterministicDelay(7, 11, 0) != 0 {
		t.Fatal("disabled delay was nonzero")
	}
}

func (c finalDocumentCollection) InsertOne(context.Context, interface{}) error {
	return nil
}

func (c finalDocumentCollection) UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

func (c finalDocumentCollection) FindOne(_ context.Context, _ interface{}, result interface{}) error {
	destination := result.(*bson.M)
	*destination = c.document
	return nil
}

func TestNewScenario(t *testing.T) {
	names := []string{"cas", "uuid-cas", "blind-inc", "disjoint-set", "same-set"}
	for _, name := range names {
		scenario, err := NewScenario(name, 4, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if scenario.Name() != name {
			t.Fatalf("name = %q, want %q", scenario.Name(), name)
		}
	}
	if _, err := NewScenario("unknown", 4, 0); err == nil {
		t.Fatal("expected unknown scenario to fail")
	}
}

func TestUUIDTokenIsDeterministicUniqueSubtypeFour(t *testing.T) {
	first := uuidToken(1)
	again := uuidToken(1)
	second := uuidToken(2)
	if first.Subtype != 4 || len(first.Data) != 16 {
		t.Fatalf("invalid UUID representation: %+v", first)
	}
	if string(first.Data) != string(again.Data) {
		t.Fatal("same sequence produced different UUIDs")
	}
	if string(first.Data) == string(second.Data) {
		t.Fatal("different sequences produced the same UUID")
	}
	if !binaryTokensEqual(first, again) || binaryTokensEqual(first, second) {
		t.Fatal("binary UUID equality is incorrect")
	}
	if first.Data[6]&0xf0 != 0x40 || first.Data[8]&0xc0 != 0x80 {
		t.Fatalf("UUID version or variant bits are invalid: %x", first.Data)
	}
}

func TestCounterChecksPassAndFail(t *testing.T) {
	passing := counterChecks(3, LedgerSnapshot{
		Matched:  3,
		Modified: 3,
		CAS: CASSnapshot{
			MatchedEdges:    3,
			HighestObserved: 2,
		},
	})
	if !ChecksPassed(passing) {
		t.Fatalf("expected checks to pass: %+v", passing)
	}

	wrongVersion := counterChecks(2, LedgerSnapshot{
		Matched:  3,
		Modified: 3,
		CAS: CASSnapshot{
			MatchedEdges:    3,
			HighestObserved: 2,
		},
	})
	if ChecksPassed(wrongVersion) {
		t.Fatal("expected stored-version mismatch to fail")
	}

	wrongModified := counterChecks(3, LedgerSnapshot{
		Matched:  3,
		Modified: 2,
		CAS: CASSnapshot{
			MatchedEdges:    3,
			HighestObserved: 2,
		},
	})
	if ChecksPassed(wrongModified) {
		t.Fatal("expected modification mismatch to fail")
	}
}

func TestCounterChecksRejectDoubleMatch(t *testing.T) {
	ledger := &Ledger{}
	for sequence := int64(1); sequence <= 2; sequence++ {
		err := ledger.Record(Outcome{
			Kind:     OutcomeMatched,
			Modified: true,
			Sequence: sequence,
			CAS: &CASOperation{
				ObservedGeneration: 0,
				ProposedGeneration: 1,
			},
		}, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot := ledger.Snapshot()
	checks := counterChecks(2, snapshot)
	if ChecksPassed(checks) {
		t.Fatalf("double match passed oracle: %+v", checks)
	}
	if snapshot.CAS.DuplicateMatches != 1 {
		t.Fatalf("duplicate matches = %d, want 1", snapshot.CAS.DuplicateMatches)
	}
}

func TestDisjointSetWorkerWithoutMatchRequiresAbsentField(t *testing.T) {
	ledger := LedgerSnapshot{Attempts: 1, Matched: 1}
	scenario := newDisjointSetScenario(2, "")
	scenario.acknowledged[0].Store(1)
	checks, err := scenario.Verify(context.Background(), finalDocumentCollection{document: bson.M{
		"_id":      "fields",
		"worker_0": int64(1),
	}}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !ChecksPassed(checks) {
		t.Fatalf("absent unmatched worker field failed: %+v", checks)
	}

	checks, err = scenario.Verify(context.Background(), finalDocumentCollection{document: bson.M{
		"_id":      "fields",
		"worker_0": int64(1),
		"worker_1": int64(9),
	}}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if ChecksPassed(checks) {
		t.Fatalf("present unacknowledged worker field passed: %+v", checks)
	}
}

func TestSetScenariosSeparateIndeterminateOutcomesFromMatchAccounting(t *testing.T) {
	ledger := LedgerSnapshot{Attempts: 3, Matched: 1, CommandErrors: 1, ClientErrors: 1}
	disjoint := newDisjointSetScenario(1, "")
	disjoint.acknowledged[0].Store(1)
	disjointChecks, err := disjoint.Verify(context.Background(), finalDocumentCollection{document: bson.M{
		"worker_0": int64(1),
	}}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !ChecksPassed(disjointChecks) {
		t.Fatalf("disjoint accounting folded errors into failure: %+v", disjointChecks)
	}

	same := &sameFieldSetScenario{}
	sameChecks, err := same.Verify(context.Background(), finalStructCollection{value: 1}, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !ChecksPassed(sameChecks) {
		t.Fatalf("same-field accounting folded errors into failure: %+v", sameChecks)
	}
}

type finalStructCollection struct {
	value int64
}

func (c finalStructCollection) InsertOne(context.Context, interface{}) error {
	return nil
}

func (c finalStructCollection) UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

func (c finalStructCollection) FindOne(_ context.Context, _ interface{}, result interface{}) error {
	document := result.(*struct{ Value int64 })
	document.Value = c.value
	return nil
}
