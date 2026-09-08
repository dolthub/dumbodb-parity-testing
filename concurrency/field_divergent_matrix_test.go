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

import "testing"

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
		scenario, err := NewScenario(fieldDivergentMatrixPrefix+row.Name, 1, 0)
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
	if _, err := NewScenario(fieldDivergentMatrixPrefix+"unknown", 1, 0); err == nil {
		t.Fatal("expected unknown matrix row to fail")
	}
}
