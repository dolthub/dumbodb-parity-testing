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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"sync/atomic"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type counterDocument struct {
	Version int64
}

func makePayload(size int) string {
	return strings.Repeat("x", size)
}

func seedCounter(ctx context.Context, collection *mongo.Collection, payload string) error {
	_, err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "counter"},
		{Key: "version", Value: int64(0)},
		{Key: "payload", Value: payload},
	})
	return err
}

func readCounter(ctx context.Context, collection *mongo.Collection) (int64, error) {
	var document counterDocument
	err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "counter"}}).Decode(&document)
	return document.Version, err
}

type casScenario struct {
	payload string
}

type uuidCASDocument struct {
	Token         primitive.Binary
	Applied       int64
	LastOperation int64
}

type uuidCASScenario struct {
	payload string
}

func (s *uuidCASScenario) Name() string {
	return "uuid-cas"
}

func (s *uuidCASScenario) Setup(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "uuid-counter"},
		{Key: "token", Value: uuidToken(0)},
		{Key: "applied", Value: int64(0)},
		{Key: "lastOperation", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
	return err
}

func (s *uuidCASScenario) Execute(ctx context.Context, collection *mongo.Collection, _ int, sequence int64) Outcome {
	var observed uuidCASDocument
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "uuid-counter"}}).Decode(&observed); err != nil {
		return Outcome{Kind: OutcomeClientError, Err: err}
	}
	replacement := uuidToken(sequence)
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "uuid-counter"}, {Key: "token", Value: observed.Token}},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "token", Value: replacement},
				{Key: "lastOperation", Value: sequence},
			}},
			{Key: "$inc", Value: bson.D{{Key: "applied", Value: int64(1)}}},
		},
	)
	return UpdateOutcome(result, err)
}

func (s *uuidCASScenario) Verify(ctx context.Context, collection *mongo.Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document uuidCASDocument
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "uuid-counter"}}).Decode(&document); err != nil {
		return nil, err
	}
	validUUID := document.Token.Subtype == 4 && len(document.Token.Data) == 16
	expectedToken := uuidToken(document.LastOperation)
	tokenMatchesOperation := binaryTokensEqual(document.Token, expectedToken)
	operationWasIssued := document.LastOperation > 0 && document.LastOperation <= ledger.Attempts
	return []Check{
		{
			Name:   "storedAppliedEqualsMatched",
			Passed: document.Applied == ledger.Matched,
			Detail: fmt.Sprintf("applied=%d matched=%d", document.Applied, ledger.Matched),
		},
		{
			Name:   "everyMatchModified",
			Passed: ledger.Modified == ledger.Matched,
			Detail: fmt.Sprintf("modified=%d matched=%d", ledger.Modified, ledger.Matched),
		},
		{
			Name:   "finalTokenIsUUID",
			Passed: validUUID,
			Detail: fmt.Sprintf("subtype=%d bytes=%d", document.Token.Subtype, len(document.Token.Data)),
		},
		{
			Name:   "finalTokenMatchesIssuedOperation",
			Passed: tokenMatchesOperation && operationWasIssued && document.Applied <= ledger.Matched,
			Detail: fmt.Sprintf("lastOperation=%d attempts=%d", document.LastOperation, ledger.Attempts),
		},
	}, nil
}

func uuidToken(sequence int64) primitive.Binary {
	var input [8]byte
	binary.BigEndian.PutUint64(input[:], uint64(sequence))
	digest := sha256.Sum256(input[:])
	data := append([]byte(nil), digest[:16]...)
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	return primitive.Binary{Subtype: 4, Data: data}
}

func binaryTokensEqual(left, right primitive.Binary) bool {
	if left.Subtype != right.Subtype || len(left.Data) != len(right.Data) {
		return false
	}
	for i := range left.Data {
		if left.Data[i] != right.Data[i] {
			return false
		}
	}
	return true
}

func (s *casScenario) Name() string {
	return "cas"
}

func (s *casScenario) Setup(ctx context.Context, collection *mongo.Collection) error {
	return seedCounter(ctx, collection, s.payload)
}

func (s *casScenario) Execute(ctx context.Context, collection *mongo.Collection, _ int, _ int64) Outcome {
	version, err := readCounter(ctx, collection)
	if err != nil {
		return Outcome{Kind: OutcomeClientError, Err: err}
	}
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "counter"}, {Key: "version", Value: version}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}}},
	)
	return UpdateOutcome(result, err)
}

func (s *casScenario) Verify(ctx context.Context, collection *mongo.Collection, ledger LedgerSnapshot) ([]Check, error) {
	version, err := readCounter(ctx, collection)
	if err != nil {
		return nil, err
	}
	return counterChecks(version, ledger), nil
}

type blindIncrementScenario struct {
	payload string
}

func (s *blindIncrementScenario) Name() string {
	return "blind-inc"
}

func (s *blindIncrementScenario) Setup(ctx context.Context, collection *mongo.Collection) error {
	return seedCounter(ctx, collection, s.payload)
}

func (s *blindIncrementScenario) Execute(ctx context.Context, collection *mongo.Collection, _ int, _ int64) Outcome {
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "counter"}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}}},
	)
	return UpdateOutcome(result, err)
}

func (s *blindIncrementScenario) Verify(ctx context.Context, collection *mongo.Collection, ledger LedgerSnapshot) ([]Check, error) {
	version, err := readCounter(ctx, collection)
	if err != nil {
		return nil, err
	}
	return counterChecks(version, ledger), nil
}

func counterChecks(version int64, ledger LedgerSnapshot) []Check {
	return []Check{
		{
			Name:   "storedVersionEqualsMatched",
			Passed: version == ledger.Matched,
			Detail: fmt.Sprintf("version=%d matched=%d", version, ledger.Matched),
		},
		{
			Name:   "everyMatchModified",
			Passed: ledger.Modified == ledger.Matched,
			Detail: fmt.Sprintf("modified=%d matched=%d", ledger.Modified, ledger.Matched),
		},
	}
}

type disjointSetScenario struct {
	acknowledged []atomic.Int64
	payload      string
}

func newDisjointSetScenario(workers int, payload string) *disjointSetScenario {
	return &disjointSetScenario{acknowledged: make([]atomic.Int64, workers), payload: payload}
}

func (s *disjointSetScenario) Name() string {
	return "disjoint-set"
}

func (s *disjointSetScenario) Setup(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "fields"},
		{Key: "payload", Value: s.payload},
	})
	return err
}

func (s *disjointSetScenario) Execute(ctx context.Context, collection *mongo.Collection, worker int, sequence int64) Outcome {
	field := fmt.Sprintf("worker_%d", worker)
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "fields"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: sequence}}}},
	)
	outcome := UpdateOutcome(result, err)
	if outcome.Kind == OutcomeMatched {
		s.acknowledged[worker].Store(sequence)
	}
	return outcome
}

func (s *disjointSetScenario) Verify(ctx context.Context, collection *mongo.Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document bson.M
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "fields"}}).Decode(&document); err != nil {
		return nil, err
	}
	checks := []Check{{
		Name:   "allWritesMatched",
		Passed: ledger.Matched == ledger.Attempts,
		Detail: fmt.Sprintf("matched=%d attempts=%d", ledger.Matched, ledger.Attempts),
	}}
	for worker := range s.acknowledged {
		want := s.acknowledged[worker].Load()
		field := fmt.Sprintf("worker_%d", worker)
		got, ok := numericInt64(document[field])
		checks = append(checks, Check{
			Name:   field + "RetainsLastAcknowledgement",
			Passed: ok && got == want,
			Detail: fmt.Sprintf("stored=%d acknowledged=%d", got, want),
		})
	}
	return checks, nil
}

type sameFieldSetScenario struct {
	payload string
}

func (s *sameFieldSetScenario) Name() string {
	return "same-set"
}

func (s *sameFieldSetScenario) Setup(ctx context.Context, collection *mongo.Collection) error {
	_, err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "field"},
		{Key: "value", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
	return err
}

func (s *sameFieldSetScenario) Execute(ctx context.Context, collection *mongo.Collection, _ int, sequence int64) Outcome {
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "field"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "value", Value: sequence}}}},
	)
	return UpdateOutcome(result, err)
}

func (s *sameFieldSetScenario) Verify(ctx context.Context, collection *mongo.Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document struct {
		Value int64
	}
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "field"}}).Decode(&document); err != nil {
		return nil, err
	}
	return []Check{
		{
			Name:   "allWritesMatched",
			Passed: ledger.Matched == ledger.Attempts,
			Detail: fmt.Sprintf("matched=%d attempts=%d", ledger.Matched, ledger.Attempts),
		},
		{
			Name:   "finalValueWasIssued",
			Passed: document.Value > 0 && document.Value <= ledger.Attempts,
			Detail: fmt.Sprintf("value=%d attempts=%d", document.Value, ledger.Attempts),
		},
	}, nil
}

func numericInt64(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	default:
		return 0, false
	}
}
