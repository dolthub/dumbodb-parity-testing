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
	"math"
	"testing"
)

func TestWilsonRate(t *testing.T) {
	estimate := wilsonRate(50, 100)
	if estimate.Rate != 0.5 {
		t.Fatalf("rate = %f", estimate.Rate)
	}
	if math.Abs(estimate.Lower95-0.4038) > 0.001 || math.Abs(estimate.Upper95-0.5962) > 0.001 {
		t.Fatalf("interval = [%f,%f]", estimate.Lower95, estimate.Upper95)
	}
	if zero := wilsonRate(0, 0); zero.Rate != 0 || zero.Lower95 != 0 || zero.Upper95 != 0 {
		t.Fatalf("zero sample = %+v", zero)
	}
	if none := wilsonRate(0, 100); none.Lower95 < 0 || none.Upper95 > 1 {
		t.Fatalf("bounded interval = %+v", none)
	}
}

func TestCalculateStatisticsUsesAllAttempts(t *testing.T) {
	statistics := calculateStatistics(LedgerSnapshot{
		Attempts:      100,
		Matched:       20,
		NoMatch:       70,
		CommandErrors: 5,
		ClientErrors:  5,
	})
	if statistics.MatchRate.Rate != 0.2 || statistics.NoMatchRate.Rate != 0.7 {
		t.Fatalf("unexpected statistics: %+v", statistics)
	}
}
