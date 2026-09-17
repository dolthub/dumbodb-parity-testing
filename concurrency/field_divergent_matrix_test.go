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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestFieldDivergentMatrixCoversNineDistinctRows(t *testing.T) {
	if len(fieldDivergentMatrixRows) != 9 {
		t.Fatalf("matrix rows = %d, want 9", len(fieldDivergentMatrixRows))
	}
	seen := make(map[string]bool, len(fieldDivergentMatrixRows))
	conflicts := 0
	for _, row := range fieldDivergentMatrixRows {
		if seen[row.Name] {
			t.Fatalf("duplicate matrix row %q", row.Name)
		}
		seen[row.Name] = true
		if row.ExpectConflict {
			conflicts++
		}
		scenario, err := NewScenarioForConfig(Config{
			Scenario:   fieldDivergentMatrixPrefix + row.Name,
			Workers:    1,
			MergeMode:  MergeModeFieldDivergent,
			Operations: 1,
		})
		if err != nil {
			t.Fatalf("construct %s: %v", row.Name, err)
		}
		if scenario.Name() != fieldDivergentMatrixPrefix+row.Name {
			t.Fatalf("scenario name = %q", scenario.Name())
		}
	}
	if conflicts != 3 {
		t.Fatalf("conflict rows = %d, want 3", conflicts)
	}
}

func TestFieldDivergentMatrixRejectsUnknownRow(t *testing.T) {
	if _, err := NewScenarioForConfig(Config{
		Scenario:  fieldDivergentMatrixPrefix + "unknown",
		Workers:   1,
		MergeMode: MergeModeFieldDivergent,
	}); err == nil {
		t.Fatal("expected unknown matrix row to fail")
	}
}

func TestMergeMatrixRequiresItsConfiguredMode(t *testing.T) {
	name := fieldDivergentMatrixPrefix + "one-sided"
	for _, mergeMode := range []string{"", MergeModeDocumentTouched, MergeModeFieldTouched, MergeModeDocumentDivergent} {
		_, err := NewScenarioForConfig(Config{Scenario: name, Workers: 1, MergeMode: mergeMode})
		if err == nil {
			t.Fatalf("merge mode %q unexpectedly accepted", mergeMode)
		}
	}
	scenario, err := NewScenarioForConfig(Config{Scenario: name, Workers: 1, MergeMode: MergeModeFieldDivergent})
	if err != nil {
		t.Fatal(err)
	}
	if scenario.Name() != name {
		t.Fatalf("scenario name = %q, want %q", scenario.Name(), name)
	}
}

func TestMergeMatrixEngineAcceptsEveryModeDefinition(t *testing.T) {
	row := mergeMatrixRow{Name: "one-sided", ExpectedDocument: matrixDocument(1, 0)}
	for _, mode := range []string{
		MergeModeDocumentTouched,
		MergeModeFieldTouched,
		MergeModeFieldDivergent,
		MergeModeDocumentDivergent,
	} {
		definition := mergeMatrixDefinition{Mode: mode, Prefix: mode + "-matrix-", Rows: []mergeMatrixRow{row}}
		scenario := &mergeMatrixScenario{definition: definition, row: row}
		if scenario.Name() != definition.Prefix+row.Name {
			t.Fatalf("mode %s scenario name = %q", mode, scenario.Name())
		}
	}
}

func TestMatrixDocumentCarriesPayloadWithoutMutatingOracle(t *testing.T) {
	original := bson.M{"_id": "matrix", "a": int64(1)}
	withPayload := matrixDocumentWithPayload(original, "payload")
	if withPayload["payload"] != "payload" {
		t.Fatalf("payload = %v", withPayload["payload"])
	}
	if _, exists := original["payload"]; exists {
		t.Fatal("matrix oracle document was mutated")
	}
}
