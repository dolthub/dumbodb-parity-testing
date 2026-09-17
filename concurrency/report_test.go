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
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReportSanitizesCredentials(t *testing.T) {
	got := sanitizedTarget("mongodb://alice:secret@localhost:27017/db?authSource=admin")
	if got != "mongodb://localhost:27017/db" {
		t.Fatalf("sanitized target = %q", got)
	}
}

func TestReportPassAndFailure(t *testing.T) {
	start := time.Now()
	result := LifecycleResult{
		Product:            "MongoDB",
		Version:            "8.0.28",
		Scenario:           "cas",
		StartedAt:          start,
		FinishedAt:         start.Add(time.Second),
		WorkloadStartedAt:  start.Add(100 * time.Millisecond),
		WorkloadFinishedAt: start.Add(1100 * time.Millisecond),
		Ledger:             LedgerSnapshot{Attempts: 1, Matched: 1, Modified: 1},
		Checks:             []Check{{Name: "stored", Passed: true}},
	}
	if !result.Passed() {
		t.Fatal("expected valid result to pass")
	}
	if result.Verdict() != VerdictConclusivePass {
		t.Fatalf("verdict=%s", result.Verdict())
	}
	if !strings.Contains(result.Summary(), "verdict=conclusivePass") {
		t.Fatalf("summary missing verdict: %s", result.Summary())
	}
	if result.OperationsPerSecond() != 1 {
		t.Fatalf("operations per second = %f", result.OperationsPerSecond())
	}
	var output bytes.Buffer
	if err := result.WriteJSON(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\"Scenario\": \"cas\"") {
		t.Fatalf("JSON report missing scenario: %s", output.String())
	}

	result.Checks[0].Passed = false
	if result.Verdict() != VerdictFailed {
		t.Fatalf("failed check verdict=%s", result.Verdict())
	}
	result.Checks[0] = Check{Name: "stored", Skipped: true}
	result.Ledger = LedgerSnapshot{Attempts: 1, Indeterminate: 1}
	if result.Verdict() != VerdictInconclusive {
		t.Fatalf("indeterminate verdict=%s", result.Verdict())
	}
	if !strings.Contains(result.Summary(), "verdict=inconclusive") {
		t.Fatalf("summary missing inconclusive verdict: %s", result.Summary())
	}
	output.Reset()
	if err := result.WriteJSON(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\"Verdict\": \"inconclusive\"") {
		t.Fatalf("JSON report missing verdict: %s", output.String())
	}
}

func TestReportWithoutChecksFails(t *testing.T) {
	result := LifecycleResult{Ledger: LedgerSnapshot{Attempts: 1, Matched: 1}}
	if result.Verdict() != VerdictFailed {
		t.Fatalf("empty evidence verdict = %s", result.Verdict())
	}
}

func TestLedgerBoundsErrorSamplesAndBucketsLatency(t *testing.T) {
	ledger := &Ledger{}
	for i := 0; i < maxErrorSamples+5; i++ {
		if err := ledger.Record(Outcome{Kind: OutcomeIndeterminate, Err: errors.New("failure")}, 2*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := ledger.Snapshot()
	if len(snapshot.ErrorSamples) != maxErrorSamples {
		t.Fatalf("error samples = %d, want %d", len(snapshot.ErrorSamples), maxErrorSamples)
	}
	if snapshot.Latency.LessThan10ms != maxErrorSamples+5 {
		t.Fatalf("latency bucket = %d", snapshot.Latency.LessThan10ms)
	}
}
