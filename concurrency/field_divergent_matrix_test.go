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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type matrixVerificationCollection struct {
	document      bson.M
	absent        bool
	conflictCount int
}

func (c matrixVerificationCollection) InsertOne(context.Context, interface{}) error {
	return nil
}

func (c matrixVerificationCollection) FindOne(_ context.Context, _ interface{}, result interface{}) error {
	if c.absent {
		return mongo.ErrNoDocuments
	}
	document := result.(*bson.M)
	*document = c.document
	return nil
}

func (c matrixVerificationCollection) UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

func (c matrixVerificationCollection) DeleteOne(context.Context, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

func (c matrixVerificationCollection) AtDatabase(string) BranchCollection {
	return c
}

func (c matrixVerificationCollection) DatabaseName() string {
	return "matrix"
}

func (c matrixVerificationCollection) RunCommand(context.Context, string, interface{}) (bson.M, error) {
	conflicts := make(bson.A, c.conflictCount)
	return bson.M{"ok": float64(1), "conflicts": conflicts}, nil
}

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

func TestMergeMatrixDiscriminatesEveryMode(t *testing.T) {
	type expectation struct {
		mode              string
		disjointConflict  bool
		identicalConflict bool
	}
	for _, expected := range []expectation{
		{MergeModeDocumentTouched, true, true},
		{MergeModeFieldTouched, false, true},
		{MergeModeFieldDivergent, false, false},
		{MergeModeDocumentDivergent, true, false},
	} {
		t.Run(expected.mode, func(t *testing.T) {
			assertMatrixControl(t, expected.mode, "disjoint-fields", expected.disjointConflict, matrixDocument(0, 1))
			assertMatrixControl(t, expected.mode, "same-field-same-value", expected.identicalConflict, matrixDocument(1, 0))
		})
	}
}

func assertMatrixControl(t *testing.T, mode, rowName string, expectConflict bool, expectedDocument bson.M) {
	t.Helper()
	definition := mergeMatrixDefinition{Mode: mode, Prefix: mode + "-matrix-"}
	row := mergeMatrixRow{Name: rowName, ExpectConflict: expectConflict, ExpectedDocument: expectedDocument}
	conflictCount := 0
	response := bson.M{"ok": float64(1)}
	var mergeErr error
	if expectConflict {
		conflictCount = 1
		response = bson.M{"ok": float64(0)}
		mergeErr = errors.New("conflict")
	}
	scenario := &mergeMatrixScenario{
		definition:    definition,
		row:           row,
		main:          matrixVerificationCollection{document: matrixDocumentWithPayload(expectedDocument, ""), conflictCount: conflictCount},
		mergeResponse: response,
		mergeErr:      mergeErr,
	}
	checks, err := scenario.Verify(context.Background(), nil, LedgerSnapshot{})
	if err != nil || !ChecksPassed(checks) {
		t.Fatalf("matching control failed: checks=%+v err=%v", checks, err)
	}

	scenario.mergeResponse = bson.M{"ok": float64(1)}
	if !expectConflict {
		scenario.mergeResponse = bson.M{"ok": float64(0)}
		scenario.mergeErr = errors.New("conflict")
	}
	checks, err = scenario.Verify(context.Background(), nil, LedgerSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if ChecksPassed(checks) {
		t.Fatalf("wrong verdict passed: %+v", checks)
	}

	scenario.mergeResponse = response
	scenario.mergeErr = mergeErr
	scenario.main = matrixVerificationCollection{document: matrixDocument(99, 99), conflictCount: conflictCount}
	checks, err = scenario.Verify(context.Background(), nil, LedgerSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if ChecksPassed(checks) {
		t.Fatalf("corrupted final state passed: %+v", checks)
	}

	if expectConflict {
		scenario.main = matrixVerificationCollection{document: matrixDocumentWithPayload(expectedDocument, "")}
		checks, err = scenario.Verify(context.Background(), nil, LedgerSnapshot{})
		if err != nil {
			t.Fatal(err)
		}
		if ChecksPassed(checks) {
			t.Fatalf("missing conflict record passed: %+v", checks)
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
