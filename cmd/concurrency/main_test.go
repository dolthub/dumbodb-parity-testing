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

package main

import (
	"errors"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/concurrency"
)

func TestResultExitCodeDistinguishesVerdicts(t *testing.T) {
	tests := []struct {
		name   string
		result concurrency.LifecycleResult
		err    error
		want   int
	}{
		{name: "pass", result: concurrency.LifecycleResult{}, want: 0},
		{name: "inconclusive", result: concurrency.LifecycleResult{Truncated: true}, want: 3},
		{name: "failed", result: concurrency.LifecycleResult{Checks: []concurrency.Check{{Passed: false}}}, want: 1},
		{name: "runner error", result: concurrency.LifecycleResult{}, err: errors.New("runner"), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resultExitCode(test.result, test.err); got != test.want {
				t.Fatalf("exit code=%d, want %d", got, test.want)
			}
		})
	}
}
