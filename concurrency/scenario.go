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
)

type Check struct {
	Name   string
	Passed bool
	Detail string
}

type Scenario interface {
	Name() string
	Setup(context.Context, Collection) error
	Execute(context.Context, Collection, int, int64) Outcome
	Verify(context.Context, Collection, LedgerSnapshot) ([]Check, error)
}

func NewScenario(name string, workers, payloadBytes int) (Scenario, error) {
	payload := makePayload(payloadBytes)
	switch name {
	case "cas":
		return &casScenario{payload: payload}, nil
	case "uuid-cas":
		return &uuidCASScenario{payload: payload}, nil
	case "blind-inc":
		return &blindIncrementScenario{payload: payload}, nil
	case "disjoint-set":
		return newDisjointSetScenario(workers, payload), nil
	case "same-set":
		return &sameFieldSetScenario{payload: payload}, nil
	default:
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
}

func ChecksPassed(checks []Check) bool {
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}
