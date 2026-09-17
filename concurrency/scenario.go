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
	"fmt"
	"time"
)

type Check struct {
	Name    string
	Passed  bool
	Skipped bool
	Detail  string
}

type Scenario interface {
	Name() string
	Setup(context.Context, Collection) error
	Execute(context.Context, Collection, int, int64) Outcome
	Verify(context.Context, Collection, LedgerSnapshot) ([]Check, error)
}

func NewScenario(name string, workers, payloadBytes int) (Scenario, error) {
	return newScenario(name, workers, payloadBytes, 0, 0, "")
}

func NewScenarioWithWorkload(name string, workers, payloadBytes int, seed int64, casDelay time.Duration) (Scenario, error) {
	return newScenario(name, workers, payloadBytes, seed, casDelay, "")
}

func NewScenarioForConfig(cfg Config) (Scenario, error) {
	return newScenario(cfg.Scenario, cfg.Workers, cfg.PayloadBytes, cfg.Seed, cfg.CASDelay, cfg.MergeMode)
}

func newScenario(name string, workers, payloadBytes int, seed int64, casDelay time.Duration, mergeMode string) (Scenario, error) {
	if workers <= 0 {
		return nil, fmt.Errorf("workers must be positive")
	}
	if payloadBytes < 0 {
		return nil, fmt.Errorf("payload bytes cannot be negative")
	}
	if casDelay < 0 {
		return nil, fmt.Errorf("CAS delay cannot be negative")
	}
	payload := makePayload(payloadBytes)
	if scenario, matched, err := newMergeMatrixScenario(name, payload, mergeMode); matched {
		return scenario, err
	}
	switch name {
	case "cas":
		return &casScenario{payload: payload, seed: seed, maxDelay: casDelay, mergeMode: mergeMode}, nil
	case "uuid-cas":
		return &uuidCASScenario{payload: payload, seed: seed, maxDelay: casDelay, mergeMode: mergeMode}, nil
	case "blind-inc":
		return &blindIncrementScenario{payload: payload, mergeMode: mergeMode}, nil
	case "disjoint-set":
		return newDisjointSetScenario(workers, payload), nil
	case "same-set":
		return &sameFieldSetScenario{payload: payload}, nil
	case "identical-set":
		return &identicalSetScenario{payload: payload}, nil
	case "divergent-cas":
		return &divergentCASScenario{payload: payload, seed: seed, maxDelay: casDelay, mergeMode: mergeMode}, nil
	default:
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
}

func ChecksPassed(checks []Check) bool {
	for _, check := range checks {
		if !check.Skipped && !check.Passed {
			return false
		}
	}
	return true
}
