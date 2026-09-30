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
	"sync"

	"go.mongodb.org/mongo-driver/bson"
)

const transactionBatchSize = 2

type transactionCommitRaceScenario struct {
	mu       sync.Mutex
	outcomes []OutcomeKind
}

func (s *transactionCommitRaceScenario) Name() string {
	return "txn-commit-race"
}

func (s *transactionCommitRaceScenario) Setup(_ context.Context, collection Collection) error {
	if _, ok := collection.(TransactionalCollection); !ok {
		return fmt.Errorf("scenario %q requires transaction support", s.Name())
	}
	return nil
}

func (s *transactionCommitRaceScenario) Execute(ctx context.Context, collection Collection, worker int, sequence int64) Outcome {
	transactional := collection.(TransactionalCollection)
	err := transactional.WithTransaction(ctx, func(transactionCtx context.Context) error {
		for member := 0; member < transactionBatchSize; member++ {
			if err := collection.InsertOne(transactionCtx, bson.D{
				{Key: "_id", Value: transactionDocumentID(sequence, member)},
				{Key: "scenario", Value: s.Name()},
				{Key: "operation", Value: sequence},
				{Key: "member", Value: member},
				{Key: "worker", Value: worker},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	outcome := UpdateOutcome(WriteResult{Matched: 1, Modified: 1}, err)
	outcome.Sequence = sequence
	outcome.Worker = worker
	s.record(sequence, outcome.Kind)
	return outcome
}

func (s *transactionCommitRaceScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	transactional, ok := collection.(TransactionalCollection)
	if !ok {
		return nil, fmt.Errorf("scenario %q requires transaction support", s.Name())
	}
	storedByOperation := make([]int, ledger.Attempts+1)
	invalidDocuments := 0
	storedDocuments := int64(0)
	err := transactional.VisitAll(ctx, bson.D{{Key: "scenario", Value: s.Name()}}, func(document bson.M) error {
		storedDocuments++
		operation, operationOK := numericInt64(document["operation"])
		member, memberOK := numericInt64(document["member"])
		id, idOK := document["_id"].(string)
		if !operationOK || operation <= 0 || operation > ledger.Attempts ||
			!memberOK || member < 0 || member >= transactionBatchSize ||
			!idOK || id != transactionDocumentID(operation, int(member)) {
			invalidDocuments++
			return nil
		}
		storedByOperation[operation]++
		return nil
	})
	if err != nil {
		return nil, err
	}
	outcomes := s.snapshot(ledger.Attempts)
	partialBatches := 0
	missingAcknowledged := 0
	storedUnacknowledged := 0
	missingOutcomes := 0
	for sequence := int64(1); sequence <= ledger.Attempts; sequence++ {
		stored := storedByOperation[sequence]
		if stored != 0 && stored != transactionBatchSize {
			partialBatches++
		}
		switch outcomes[sequence] {
		case OutcomeMatched:
			if stored != transactionBatchSize {
				missingAcknowledged++
			}
		case OutcomeRejected, OutcomeIndeterminate:
			if stored != 0 {
				storedUnacknowledged++
			}
		default:
			missingOutcomes++
		}
	}
	expectedDocuments := ledger.Matched * transactionBatchSize
	return []Check{
		{
			Name:   "allTransactionsEventuallyCommit",
			Passed: ledger.Matched == ledger.Attempts && ledger.Rejected == 0 && ledger.Indeterminate == 0,
			Detail: fmt.Sprintf("matched=%d attempts=%d rejected=%d indeterminate=%d", ledger.Matched, ledger.Attempts, ledger.Rejected, ledger.Indeterminate),
		},
		{
			Name:   "transactionBatchesAreAtomic",
			Passed: partialBatches == 0 && invalidDocuments == 0,
			Detail: fmt.Sprintf("partialBatches=%d invalidDocuments=%d", partialBatches, invalidDocuments),
		},
		{
			Name:   "acknowledgedTransactionsAreDurable",
			Passed: missingAcknowledged == 0 && storedDocuments == expectedDocuments,
			Detail: fmt.Sprintf("missingBatches=%d storedDocuments=%d expectedDocuments=%d", missingAcknowledged, storedDocuments, expectedDocuments),
		},
		{
			Name:   "unacknowledgedTransactionsLeaveNoWrites",
			Passed: storedUnacknowledged == 0 && missingOutcomes == 0,
			Detail: fmt.Sprintf("storedBatches=%d missingOutcomes=%d", storedUnacknowledged, missingOutcomes),
		},
	}, nil
}

func (s *transactionCommitRaceScenario) record(sequence int64, outcome OutcomeKind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sequence >= int64(len(s.outcomes)) {
		s.outcomes = append(s.outcomes, make([]OutcomeKind, sequence-int64(len(s.outcomes))+1)...)
	}
	s.outcomes[sequence] = outcome
}

func (s *transactionCommitRaceScenario) snapshot(attempts int64) []OutcomeKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	outcomes := make([]OutcomeKind, attempts+1)
	copy(outcomes, s.outcomes)
	return outcomes
}

func transactionDocumentID(sequence int64, member int) string {
	return fmt.Sprintf("txn-%020d-%02d", sequence, member)
}
