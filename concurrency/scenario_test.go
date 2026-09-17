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

type counterVersionCollection struct {
	version int64
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

func (c counterVersionCollection) InsertOne(context.Context, interface{}) error {
	return nil
}

func (c counterVersionCollection) UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

func (c counterVersionCollection) FindOne(_ context.Context, _ interface{}, result interface{}) error {
	document := result.(*counterDocument)
	document.Version = c.version
	return nil
}

func TestNewScenario(t *testing.T) {
	names := []string{"cas", "uuid-cas", "blind-inc", "disjoint-set", "same-set", "identical-set", "divergent-cas"}
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
	if _, err := NewScenario("disjoint-set", -1, 0); err == nil {
		t.Fatal("expected negative workers to fail")
	}
	if _, err := NewScenario("cas", 1, -1); err == nil {
		t.Fatal("expected negative payload to fail")
	}
}

func TestWholeDocumentScenariosRequireDocumentDivergent(t *testing.T) {
	for _, name := range []string{"whole-document-convergent", "whole-document-divergent"} {
		for _, mode := range []string{"", MergeModeDocumentTouched, MergeModeFieldTouched, MergeModeFieldDivergent} {
			if _, err := NewScenarioForConfig(Config{Scenario: name, Workers: 2, MergeMode: mode}); err == nil {
				t.Fatalf("scenario %s accepted mode %q", name, mode)
			}
		}
		scenario, err := NewScenarioForConfig(Config{Scenario: name, Workers: 2, MergeMode: MergeModeDocumentDivergent})
		if err != nil {
			t.Fatal(err)
		}
		if scenario.Name() != name {
			t.Fatalf("scenario name = %q, want %q", scenario.Name(), name)
		}
	}
}

func TestWholeDocumentOracleSeparatesConvergenceFromDivergence(t *testing.T) {
	ledger := &Ledger{}
	for sequence := int64(1); sequence <= 2; sequence++ {
		if err := ledger.Record(Outcome{
			Kind:     OutcomeMatched,
			Modified: true,
			Sequence: sequence,
			CAS: &CASOperation{
				ObservedGeneration: 0,
				ProposedGeneration: 1,
			},
		}, 0); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := ledger.Snapshot()
	convergent := &wholeDocumentCASScenario{name: "whole-document-convergent"}
	checks, err := convergent.Verify(context.Background(), finalDocumentCollection{document: bson.M{
		"_id": "whole-document", "generation": int64(1), "state": int64(1), "payload": "",
	}}, snapshot)
	if err != nil || !ChecksPassed(checks) {
		t.Fatalf("convergent duplicate matches failed: checks=%+v err=%v", checks, err)
	}

	divergent := &wholeDocumentCASScenario{name: "whole-document-divergent", divergent: true}
	checks, err = divergent.Verify(context.Background(), finalDocumentCollection{document: bson.M{
		"_id": "whole-document", "generation": int64(1), "state": int64(1), "payload": "", "worker_0": int64(1),
	}}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if ChecksPassed(checks) {
		t.Fatalf("divergent duplicate matches passed: %+v", checks)
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
	if len(snapshot.CAS.Findings) != 1 || snapshot.CAS.Findings[0].Reason != "duplicateMatch" ||
		snapshot.CAS.Findings[0].FirstSequence != 1 || snapshot.CAS.Findings[0].CompetingSequence != 2 {
		t.Fatalf("missing duplicate evidence: %+v", snapshot.CAS.Findings)
	}
	if snapshot.CAS.ObservedGenerationSlots != 1 || snapshot.CAS.TrackerBytes == 0 {
		t.Fatalf("missing tracker memory evidence: %+v", snapshot.CAS)
	}
}

func TestDocumentTouchedCASRequiresNoMatchInsteadOfRejection(t *testing.T) {
	passing := LedgerSnapshot{Attempts: 2, Matched: 1, NoMatch: 1, Modified: 1}
	if !ChecksPassed(strictCASClientOutcomeChecks(passing)) {
		t.Fatalf("clean no-match failed: %+v", strictCASClientOutcomeChecks(passing))
	}

	rejected := LedgerSnapshot{Attempts: 2, Matched: 1, Rejected: 1, Modified: 1}
	if ChecksPassed(strictCASClientOutcomeChecks(rejected)) {
		t.Fatal("client-visible CAS rejection passed")
	}

	missing := LedgerSnapshot{Attempts: 2, Matched: 1, Modified: 1}
	if ChecksPassed(strictCASClientOutcomeChecks(missing)) {
		t.Fatal("unaccounted conclusive CAS attempt passed")
	}
}

func TestFieldDivergentCounterAllowsConvergentMatches(t *testing.T) {
	ledger := &Ledger{}
	for sequence := int64(1); sequence <= 2; sequence++ {
		if err := ledger.Record(Outcome{
			Kind:     OutcomeMatched,
			Modified: true,
			Sequence: sequence,
			CAS: &CASOperation{
				ObservedGeneration: 0,
				ProposedGeneration: 1,
			},
		}, 0); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := ledger.Snapshot()
	if !ChecksPassed(fieldDivergentCounterChecks(1, snapshot)) {
		t.Fatalf("fieldDivergent rejected convergent matches: %+v", snapshot)
	}
	if ChecksPassed(counterChecks(1, snapshot)) {
		t.Fatal("strict CAS oracle accepted convergent duplicate matches")
	}
}

func TestFieldDivergentBlindIncrementAllowsCoalescing(t *testing.T) {
	checks := fieldDivergentBlindIncrementChecks(1, LedgerSnapshot{
		Attempts: 2,
		Matched:  2,
		Modified: 2,
	})
	if !ChecksPassed(checks) {
		t.Fatalf("fieldDivergent rejected coalesced increments: %+v", checks)
	}
}

func TestCounterChecksSkipUnknownConservationButRetainHardChecks(t *testing.T) {
	checks := counterChecks(7, LedgerSnapshot{Attempts: 1, Indeterminate: 1})
	if !checks[0].Skipped || !checks[len(checks)-1].Skipped {
		t.Fatalf("indeterminate conservation checks were evaluable: %+v", checks)
	}
	if !ChecksPassed(checks) {
		t.Fatalf("skipped conservation rendered as failure: %+v", checks)
	}
	checks[2] = Check{Name: "oneMatchPerObservedGeneration", Passed: false}
	if ChecksPassed(checks) {
		t.Fatalf("hard CAS failure was hidden by indeterminacy: %+v", checks)
	}
}

func TestBlindIncrementRejectsNoMatch(t *testing.T) {
	scenario := &blindIncrementScenario{}
	checks, err := scenario.Verify(context.Background(), counterVersionCollection{version: 2}, LedgerSnapshot{
		Attempts: 3,
		Matched:  2,
		Modified: 2,
		NoMatch:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		if check.Name == "allAcknowledgedWritesMatched" {
			if check.Passed || check.Skipped {
				t.Fatalf("no-match accounting check did not fail: %+v", check)
			}
			return
		}
	}
	t.Fatal("allAcknowledgedWritesMatched check is missing")
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
	ledger := LedgerSnapshot{Attempts: 3, Matched: 1, Rejected: 1, Indeterminate: 1}
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
	if !disjointChecks[1].Skipped {
		t.Fatalf("indeterminate final-state check was not skipped: %+v", disjointChecks)
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
