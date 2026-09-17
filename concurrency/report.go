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
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

type RunConfig struct {
	Target       string
	Duration     string
	Operations   int64
	Workers      int
	Seed         int64
	Database     string
	Collection   string
	PayloadBytes int
	CASDelay     string
	LatencyScope string
	MergeMode    string
}

type Verdict string

const (
	VerdictConclusivePass Verdict = "conclusivePass"
	VerdictInconclusive   Verdict = "inconclusive"
	VerdictFailed         Verdict = "failed"
)

func (r LifecycleResult) Verdict() Verdict {
	if r.RunError != "" || r.Ledger.Validate() != nil || len(r.Checks) == 0 || !ChecksPassed(r.Checks) {
		return VerdictFailed
	}
	if r.Truncated || r.Ledger.Indeterminate > 0 {
		return VerdictInconclusive
	}
	return VerdictConclusivePass
}

func (r LifecycleResult) Passed() bool {
	return r.Verdict() == VerdictConclusivePass
}

func (r LifecycleResult) OperationsPerSecond() float64 {
	elapsed := r.WorkloadFinishedAt.Sub(r.WorkloadStartedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(r.Ledger.Attempts) / elapsed
}

func (r LifecycleResult) Summary() string {
	return fmt.Sprintf(
		"%s %s scenario=%s attempts=%d matched=%d noMatch=%d rejected=%d indeterminate=%d opsPerSecond=%.1f verdict=%s",
		r.Product,
		r.Version,
		r.Scenario,
		r.Ledger.Attempts,
		r.Ledger.Matched,
		r.Ledger.NoMatch,
		r.Ledger.Rejected,
		r.Ledger.Indeterminate,
		r.OperationsPerSecond(),
		r.Verdict(),
	)
}

func (r LifecycleResult) WriteJSON(writer io.Writer) error {
	r.VerdictValue = r.Verdict()
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

func sanitizedTarget(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "invalid"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	return parsed.String()
}
