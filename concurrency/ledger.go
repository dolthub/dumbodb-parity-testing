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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type OutcomeKind string

const (
	OutcomeMatched      OutcomeKind = "matched"
	OutcomeNoMatch      OutcomeKind = "noMatch"
	OutcomeCommandError OutcomeKind = "commandError"
	OutcomeClientError  OutcomeKind = "clientError"
)

type Outcome struct {
	Kind     OutcomeKind
	Modified bool
	Err      error
	Sequence int64
	Worker   int
	CAS      *CASOperation
}

type CASOperation struct {
	ObservedGeneration int64
	ProposedGeneration int64
}

type LedgerSnapshot struct {
	Attempts      int64
	Matched       int64
	NoMatch       int64
	Modified      int64
	CommandErrors int64
	ClientErrors  int64
	Latency       LatencySnapshot
	ErrorSamples  []string
	CAS           CASSnapshot
}

func (s LedgerSnapshot) Validate() error {
	terminal := s.Matched + s.NoMatch + s.CommandErrors + s.ClientErrors
	if s.Attempts != terminal {
		return fmt.Errorf("attempts %d != terminal outcomes %d", s.Attempts, terminal)
	}
	if s.Modified < 0 || s.Modified > s.Matched {
		return fmt.Errorf("modified %d outside matched range [0,%d]", s.Modified, s.Matched)
	}
	return nil
}

type Ledger struct {
	attempts      atomic.Int64
	matched       atomic.Int64
	noMatch       atomic.Int64
	modified      atomic.Int64
	commandErrors atomic.Int64
	clientErrors  atomic.Int64
	latency       latencyLedger
	samplesMu     sync.Mutex
	errorSamples  []string
	causal        causalLedger
}

const maxErrorSamples = 20

func (l *Ledger) Record(outcome Outcome, latency time.Duration) error {
	switch outcome.Kind {
	case OutcomeMatched:
		l.matched.Add(1)
		if outcome.Modified {
			l.modified.Add(1)
		}
	case OutcomeNoMatch:
		if outcome.Modified {
			return errors.New("no-match outcome cannot be modified")
		}
		l.noMatch.Add(1)
	case OutcomeCommandError:
		if outcome.Modified {
			return errors.New("command-error outcome cannot be modified")
		}
		l.commandErrors.Add(1)
	case OutcomeClientError:
		if outcome.Modified {
			return errors.New("client-error outcome cannot be modified")
		}
		l.clientErrors.Add(1)
	default:
		return fmt.Errorf("unknown outcome kind %q", outcome.Kind)
	}
	l.attempts.Add(1)
	l.latency.record(latency)
	if outcome.CAS != nil && outcome.Kind == OutcomeMatched {
		l.causal.record(outcome.Sequence, *outcome.CAS)
	}
	if outcome.Err != nil {
		l.samplesMu.Lock()
		if len(l.errorSamples) < maxErrorSamples {
			l.errorSamples = append(l.errorSamples, outcome.Err.Error())
		}
		l.samplesMu.Unlock()
	}
	return nil
}

func (l *Ledger) Snapshot() LedgerSnapshot {
	snapshot := LedgerSnapshot{
		Attempts:      l.attempts.Load(),
		Matched:       l.matched.Load(),
		NoMatch:       l.noMatch.Load(),
		Modified:      l.modified.Load(),
		CommandErrors: l.commandErrors.Load(),
		ClientErrors:  l.clientErrors.Load(),
		Latency:       l.latency.snapshot(),
		CAS:           l.causal.snapshot(),
	}
	l.samplesMu.Lock()
	snapshot.ErrorSamples = append([]string(nil), l.errorSamples...)
	l.samplesMu.Unlock()
	return snapshot
}

type CASSnapshot struct {
	MatchedEdges     int64
	DuplicateMatches int64
	InvalidEdges     int64
	HighestObserved  int64
	matchedBits      []uint64
}

func (s CASSnapshot) OperationMatched(sequence int64) bool {
	if sequence <= 0 {
		return false
	}
	index := sequence / 64
	if index >= int64(len(s.matchedBits)) {
		return false
	}
	return s.matchedBits[index]&(uint64(1)<<uint(sequence%64)) != 0
}

type causalLedger struct {
	mu              sync.Mutex
	observedCounts  []uint8
	matchedBits     []uint64
	matchedEdges    int64
	duplicates      int64
	invalidEdges    int64
	highestObserved int64
}

func (l *causalLedger) record(sequence int64, operation CASOperation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.matchedEdges++
	if operation.ObservedGeneration < 0 ||
		operation.ProposedGeneration != operation.ObservedGeneration+1 {
		l.invalidEdges++
		return
	}
	observed := int(operation.ObservedGeneration)
	if observed >= len(l.observedCounts) {
		l.observedCounts = append(l.observedCounts, make([]uint8, observed-len(l.observedCounts)+1)...)
	}
	if l.observedCounts[observed] > 0 {
		l.duplicates++
	}
	if l.observedCounts[observed] < 255 {
		l.observedCounts[observed]++
	}
	if operation.ObservedGeneration > l.highestObserved {
		l.highestObserved = operation.ObservedGeneration
	}
	if sequence > 0 {
		index := int(sequence / 64)
		if index >= len(l.matchedBits) {
			l.matchedBits = append(l.matchedBits, make([]uint64, index-len(l.matchedBits)+1)...)
		}
		l.matchedBits[index] |= uint64(1) << uint(sequence%64)
	}
}

func (l *causalLedger) snapshot() CASSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return CASSnapshot{
		MatchedEdges:     l.matchedEdges,
		DuplicateMatches: l.duplicates,
		InvalidEdges:     l.invalidEdges,
		HighestObserved:  l.highestObserved,
		matchedBits:      append([]uint64(nil), l.matchedBits...),
	}
}

type LatencySnapshot struct {
	LessThan100us int64
	LessThan1ms   int64
	LessThan10ms  int64
	LessThan100ms int64
	LessThan1s    int64
	AtLeast1s     int64
}

type latencyLedger struct {
	buckets [6]atomic.Int64
}

func (l *latencyLedger) record(duration time.Duration) {
	index := 5
	limits := [...]time.Duration{
		100 * time.Microsecond,
		time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		time.Second,
	}
	for i, limit := range limits {
		if duration < limit {
			index = i
			break
		}
	}
	l.buckets[index].Add(1)
}

func (l *latencyLedger) snapshot() LatencySnapshot {
	return LatencySnapshot{
		LessThan100us: l.buckets[0].Load(),
		LessThan1ms:   l.buckets[1].Load(),
		LessThan10ms:  l.buckets[2].Load(),
		LessThan100ms: l.buckets[3].Load(),
		LessThan1s:    l.buckets[4].Load(),
		AtLeast1s:     l.buckets[5].Load(),
	}
}

func UpdateOutcome(result WriteResult, err error) Outcome {
	if err != nil {
		var commandError mongo.CommandError
		var writeException mongo.WriteException
		if errors.As(err, &commandError) || errors.As(err, &writeException) {
			return Outcome{Kind: OutcomeCommandError, Err: err}
		}
		return Outcome{Kind: OutcomeClientError, Err: err}
	}
	if result.Matched == 0 {
		return Outcome{Kind: OutcomeNoMatch}
	}
	return Outcome{Kind: OutcomeMatched, Modified: result.Modified > 0}
}
