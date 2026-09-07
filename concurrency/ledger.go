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
	"sync/atomic"

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
}

type LedgerSnapshot struct {
	Attempts      int64
	Matched       int64
	NoMatch       int64
	Modified      int64
	CommandErrors int64
	ClientErrors  int64
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
}

func (l *Ledger) Record(outcome Outcome) error {
	l.attempts.Add(1)
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
	return nil
}

func (l *Ledger) Snapshot() LedgerSnapshot {
	return LedgerSnapshot{
		Attempts:      l.attempts.Load(),
		Matched:       l.matched.Load(),
		NoMatch:       l.noMatch.Load(),
		Modified:      l.modified.Load(),
		CommandErrors: l.commandErrors.Load(),
		ClientErrors:  l.clientErrors.Load(),
	}
}

func UpdateOutcome(result *mongo.UpdateResult, err error) Outcome {
	if err != nil {
		var commandError mongo.CommandError
		var writeException mongo.WriteException
		if errors.As(err, &commandError) || errors.As(err, &writeException) {
			return Outcome{Kind: OutcomeCommandError, Err: err}
		}
		return Outcome{Kind: OutcomeClientError, Err: err}
	}
	if result == nil || result.MatchedCount == 0 {
		return Outcome{Kind: OutcomeNoMatch}
	}
	return Outcome{Kind: OutcomeMatched, Modified: result.ModifiedCount > 0}
}
