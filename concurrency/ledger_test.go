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
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestLedgerBalancesTerminalOutcomes(t *testing.T) {
	ledger := &Ledger{}
	outcomes := []Outcome{
		{Kind: OutcomeMatched, Modified: true},
		{Kind: OutcomeMatched},
		{Kind: OutcomeNoMatch},
		{Kind: OutcomeCommandError},
		{Kind: OutcomeClientError},
	}
	for _, outcome := range outcomes {
		if err := ledger.Record(outcome); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := ledger.Snapshot()
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	if snapshot.Attempts != 5 || snapshot.Matched != 2 || snapshot.Modified != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestLedgerRejectsInvalidOutcome(t *testing.T) {
	ledger := &Ledger{}
	if err := ledger.Record(Outcome{Kind: OutcomeNoMatch, Modified: true}); err == nil {
		t.Fatal("expected invalid outcome to fail")
	}
}

func TestLedgerSnapshotDetectsCorruption(t *testing.T) {
	tests := []LedgerSnapshot{
		{Attempts: 2, Matched: 1},
		{Attempts: 1, Matched: 1, Modified: 2},
	}
	for _, snapshot := range tests {
		if err := snapshot.Validate(); err == nil {
			t.Fatalf("expected corrupted snapshot to fail: %+v", snapshot)
		}
	}
}

func TestUpdateOutcome(t *testing.T) {
	tests := []struct {
		name string
		got  Outcome
		want OutcomeKind
	}{
		{name: "matched", got: UpdateOutcome(&mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil), want: OutcomeMatched},
		{name: "no match", got: UpdateOutcome(&mongo.UpdateResult{}, nil), want: OutcomeNoMatch},
		{name: "command", got: UpdateOutcome(nil, mongo.CommandError{Code: 1}), want: OutcomeCommandError},
		{name: "client", got: UpdateOutcome(nil, errors.New("network")), want: OutcomeClientError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got.Kind != test.want {
				t.Fatalf("kind = %q, want %q", test.got.Kind, test.want)
			}
		})
	}
}
