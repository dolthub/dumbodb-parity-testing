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

import "math"

type RateEstimate struct {
	Count      int64
	SampleSize int64
	Rate       float64
	Lower95    float64
	Upper95    float64
}

type RunStatistics struct {
	MatchRate        RateEstimate
	NoMatchRate      RateEstimate
	CommandErrorRate RateEstimate
	ClientErrorRate  RateEstimate
}

func calculateStatistics(ledger LedgerSnapshot) RunStatistics {
	return RunStatistics{
		MatchRate:        wilsonRate(ledger.Matched, ledger.Attempts),
		NoMatchRate:      wilsonRate(ledger.NoMatch, ledger.Attempts),
		CommandErrorRate: wilsonRate(ledger.CommandErrors, ledger.Attempts),
		ClientErrorRate:  wilsonRate(ledger.ClientErrors, ledger.Attempts),
	}
}

func wilsonRate(count, sampleSize int64) RateEstimate {
	estimate := RateEstimate{Count: count, SampleSize: sampleSize}
	if sampleSize <= 0 {
		return estimate
	}
	const z = 1.959963984540054
	n := float64(sampleSize)
	p := float64(count) / n
	zSquared := z * z
	denominator := 1 + zSquared/n
	center := (p + zSquared/(2*n)) / denominator
	margin := z * math.Sqrt((p*(1-p)+zSquared/(4*n))/n) / denominator
	estimate.Rate = p
	estimate.Lower95 = math.Max(0, center-margin)
	estimate.Upper95 = math.Min(1, center+margin)
	return estimate
}
