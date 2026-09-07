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
	passing := counterChecks(3, LedgerSnapshot{Matched: 3, Modified: 3})
	if !ChecksPassed(passing) {
		t.Fatalf("expected checks to pass: %+v", passing)
	}

	wrongVersion := counterChecks(2, LedgerSnapshot{Matched: 3, Modified: 3})
	if ChecksPassed(wrongVersion) {
		t.Fatal("expected stored-version mismatch to fail")
	}

	wrongModified := counterChecks(3, LedgerSnapshot{Matched: 3, Modified: 2})
	if ChecksPassed(wrongModified) {
		t.Fatal("expected modification mismatch to fail")
	}
}
