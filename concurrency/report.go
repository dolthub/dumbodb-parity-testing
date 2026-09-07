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
	"time"
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
}

func (r LifecycleResult) Passed() bool {
	return !r.Truncated && r.Ledger.ClientErrors == 0 && r.Ledger.Validate() == nil && ChecksPassed(r.Checks)
}

func (r LifecycleResult) OperationsPerSecond() float64 {
	elapsed := r.FinishedAt.Sub(r.StartedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(r.Ledger.Attempts) / elapsed
}

func (r LifecycleResult) Summary() string {
	return fmt.Sprintf(
		"%s %s scenario=%s attempts=%d matched=%d noMatch=%d errors=%d opsPerSecond=%.1f passed=%t",
		r.Product,
		r.Version,
		r.Scenario,
		r.Ledger.Attempts,
		r.Ledger.Matched,
		r.Ledger.NoMatch,
		r.Ledger.CommandErrors+r.Ledger.ClientErrors,
		r.OperationsPerSecond(),
		r.Passed(),
	)
}

func (r LifecycleResult) WriteJSON(writer io.Writer) error {
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

func elapsedMilliseconds(start, finish time.Time) int64 {
	return finish.Sub(start).Milliseconds()
}
