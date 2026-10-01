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
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type transactionTestCollection struct {
	documents     []bson.M
	pending       []bson.M
	inTransaction bool
}

func (c *transactionTestCollection) InsertOne(_ context.Context, document interface{}) error {
	encoded, err := bson.Marshal(document)
	if err != nil {
		return err
	}
	var decoded bson.M
	if err := bson.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	if c.inTransaction {
		c.pending = append(c.pending, decoded)
	} else {
		c.documents = append(c.documents, decoded)
	}
	return nil
}

func (c *transactionTestCollection) FindOne(context.Context, interface{}, interface{}) error {
	return nil
}

func (c *transactionTestCollection) UpdateOne(context.Context, interface{}, interface{}) (WriteResult, error) {
	return WriteResult{}, nil
}

func (c *transactionTestCollection) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	c.inTransaction = true
	c.pending = nil
	err := fn(ctx)
	c.inTransaction = false
	if err == nil {
		c.documents = append(c.documents, c.pending...)
	}
	c.pending = nil
	return err
}

func (c *transactionTestCollection) VisitAll(_ context.Context, _ interface{}, visit func(bson.M) error) error {
	for _, document := range c.documents {
		if err := visit(document); err != nil {
			return err
		}
	}
	return nil
}

func TestTransactionCommitRaceExecutesAndVerifiesCommittedBatch(t *testing.T) {
	scenario := &transactionCommitRaceScenario{}
	collection := &transactionTestCollection{}
	if err := scenario.Setup(context.Background(), collection); err != nil {
		t.Fatal(err)
	}
	outcome := scenario.Execute(context.Background(), collection, 2, 1)
	if outcome.Kind != OutcomeMatched || !outcome.Modified {
		t.Fatalf("outcome = %+v", outcome)
	}
	ledger := &Ledger{}
	if err := ledger.Record(outcome, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	checks, err := scenario.Verify(context.Background(), collection, ledger.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !ChecksPassed(checks) {
		t.Fatalf("committed batch failed: %+v", checks)
	}
}

func TestTransactionCommitRaceOracleRejectsPartialBatch(t *testing.T) {
	scenario := &transactionCommitRaceScenario{}
	scenario.record(1, OutcomeMatched)
	collection := &transactionTestCollection{}
	for member := 0; member < transactionBatchSize-1; member++ {
		collection.documents = append(collection.documents, transactionTestDocument(1, member))
	}
	checks, err := scenario.Verify(context.Background(), collection, LedgerSnapshot{Attempts: 1, Matched: 1, Modified: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ChecksPassed(checks) {
		t.Fatalf("partial batch passed: %+v", checks)
	}
}

func TestTransactionCommitRaceOracleRejectsStoredFailedBatch(t *testing.T) {
	scenario := &transactionCommitRaceScenario{}
	scenario.record(1, OutcomeRejected)
	collection := &transactionTestCollection{}
	for member := 0; member < transactionBatchSize; member++ {
		collection.documents = append(collection.documents, transactionTestDocument(1, member))
	}
	checks, err := scenario.Verify(context.Background(), collection, LedgerSnapshot{Attempts: 1, Rejected: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ChecksPassed(checks) {
		t.Fatalf("stored failed batch passed: %+v", checks)
	}
}

func transactionTestDocument(sequence int64, member int) bson.M {
	return bson.M{
		"_id":       transactionDocumentID(sequence, member),
		"scenario":  "txn-commit-race",
		"operation": sequence,
		"member":    int32(member),
		"worker":    int32(0),
	}
}
