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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type counterDocument struct {
	Version int64
}

func makePayload(size int) string {
	return strings.Repeat("x", size)
}

func seedCounter(ctx context.Context, collection Collection, payload string) error {
	err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "counter"},
		{Key: "version", Value: int64(0)},
		{Key: "payload", Value: payload},
	})
	return err
}

func readCounter(ctx context.Context, collection Collection) (int64, error) {
	var document counterDocument
	err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "counter"}}, &document)
	return document.Version, err
}

type casScenario struct {
	payload   string
	seed      int64
	maxDelay  time.Duration
	mergeMode string
}

type uuidCASDocument struct {
	Token         primitive.Binary
	Applied       int64
	LastOperation int64
}

type uuidCASScenario struct {
	payload   string
	seed      int64
	maxDelay  time.Duration
	mergeMode string
}

func (s *uuidCASScenario) Name() string {
	return "uuid-cas"
}

func (s *uuidCASScenario) Setup(ctx context.Context, collection Collection) error {
	err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "uuid-counter"},
		{Key: "token", Value: uuidToken(0)},
		{Key: "applied", Value: int64(0)},
		{Key: "lastOperation", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
	return err
}

func (s *uuidCASScenario) Execute(ctx context.Context, collection Collection, worker int, sequence int64) Outcome {
	var observed uuidCASDocument
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "uuid-counter"}}, &observed); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	if err := waitCASDelay(ctx, s.seed, sequence, s.maxDelay); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	replacement := uuidToken(sequence)
	result, err := collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: "uuid-counter"},
			{Key: "token", Value: observed.Token},
			{Key: "applied", Value: observed.Applied},
		},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "token", Value: replacement},
				{Key: "lastOperation", Value: sequence},
			}},
			{Key: "$inc", Value: bson.D{{Key: "applied", Value: int64(1)}}},
		},
	)
	outcome := UpdateOutcome(result, err)
	outcome.Sequence = sequence
	outcome.Worker = worker
	outcome.CAS = &CASOperation{
		ObservedGeneration: observed.Applied,
		ProposedGeneration: observed.Applied + 1,
	}
	return outcome
}

func (s *uuidCASScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document uuidCASDocument
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "uuid-counter"}}, &document); err != nil {
		return nil, err
	}
	validUUID := document.Token.Subtype == 4 && len(document.Token.Data) == 16
	expectedToken := uuidToken(document.LastOperation)
	tokenMatchesOperation := binaryTokensEqual(document.Token, expectedToken)
	operationWasIssued := document.LastOperation > 0 && document.LastOperation <= ledger.Attempts
	checks := []Check{
		{
			Name:    "storedAppliedEqualsMatched",
			Passed:  document.Applied == ledger.Matched,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("applied=%d matched=%d", document.Applied, ledger.Matched),
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
			Name: "finalTokenMatchesIssuedOperation",
			Passed: tokenMatchesOperation && operationWasIssued &&
				ledger.CAS.OperationMatched(document.LastOperation),
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("lastOperation=%d attempts=%d", document.LastOperation, ledger.Attempts),
		},
		{
			Name:   "oneMatchPerObservedGeneration",
			Passed: ledger.CAS.DuplicateMatches == 0,
			Detail: fmt.Sprintf("duplicateMatches=%d", ledger.CAS.DuplicateMatches),
		},
		{
			Name: "matchedEdgesFormCompleteChain",
			Passed: ledger.CAS.InvalidEdges == 0 &&
				ledger.CAS.MatchedEdges == ledger.Matched &&
				(document.Applied == 0 || ledger.CAS.HighestObserved == document.Applied-1),
			Skipped: ledger.Indeterminate > 0,
			Detail: fmt.Sprintf(
				"edges=%d invalid=%d highestObserved=%d applied=%d",
				ledger.CAS.MatchedEdges,
				ledger.CAS.InvalidEdges,
				ledger.CAS.HighestObserved,
				document.Applied,
			),
		},
	}
	if s.mergeMode == MergeModeDocumentTouched || s.mergeMode == MergeModeDocumentDivergent {
		checks = append(checks, strictCASClientOutcomeChecks(ledger)...)
	}
	return checks, nil
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

func (s *casScenario) Setup(ctx context.Context, collection Collection) error {
	return seedCounter(ctx, collection, s.payload)
}

func (s *casScenario) Execute(ctx context.Context, collection Collection, worker int, sequence int64) Outcome {
	version, err := readCounter(ctx, collection)
	if err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	if err := waitCASDelay(ctx, s.seed, sequence, s.maxDelay); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "counter"}, {Key: "version", Value: version}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}}},
	)
	outcome := UpdateOutcome(result, err)
	outcome.Sequence = sequence
	outcome.Worker = worker
	outcome.CAS = &CASOperation{ObservedGeneration: version, ProposedGeneration: version + 1}
	return outcome
}

func waitCASDelay(ctx context.Context, seed, sequence int64, maximum time.Duration) error {
	delay := deterministicDelay(seed, sequence, maximum)
	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func deterministicDelay(seed, sequence int64, maximum time.Duration) time.Duration {
	if maximum <= 0 {
		return 0
	}
	value := uint64(seed) ^ uint64(sequence)*0x9e3779b97f4a7c15
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	return time.Duration(value % (uint64(maximum) + 1))
}

func (s *casScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	version, err := readCounter(ctx, collection)
	if err != nil {
		return nil, err
	}
	if s.mergeMode == MergeModeFieldDivergent {
		return fieldDivergentCounterChecks(version, ledger), nil
	}
	if s.mergeMode == MergeModeDocumentDivergent {
		checks := fieldDivergentCounterChecks(version, ledger)
		return append(checks, strictCASClientOutcomeChecks(ledger)...), nil
	}
	checks := counterChecks(version, ledger)
	if s.mergeMode == MergeModeDocumentTouched {
		checks = append(checks, strictCASClientOutcomeChecks(ledger)...)
	}
	return checks, nil
}

type blindIncrementScenario struct {
	payload   string
	mergeMode string
}

func (s *blindIncrementScenario) Name() string {
	return "blind-inc"
}

func (s *blindIncrementScenario) Setup(ctx context.Context, collection Collection) error {
	return seedCounter(ctx, collection, s.payload)
}

func (s *blindIncrementScenario) Execute(ctx context.Context, collection Collection, _ int, _ int64) Outcome {
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "counter"}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "version", Value: int64(1)}}}},
	)
	return UpdateOutcome(result, err)
}

func (s *blindIncrementScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	version, err := readCounter(ctx, collection)
	if err != nil {
		return nil, err
	}
	if s.mergeMode == MergeModeFieldDivergent || s.mergeMode == MergeModeDocumentDivergent {
		return fieldDivergentBlindIncrementChecks(version, ledger), nil
	}
	return []Check{
		{
			Name:    "storedVersionEqualsMatched",
			Passed:  version == ledger.Matched,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("version=%d matched=%d", version, ledger.Matched),
		},
		{
			Name:   "everyMatchModified",
			Passed: ledger.Modified == ledger.Matched,
			Detail: fmt.Sprintf("modified=%d matched=%d", ledger.Modified, ledger.Matched),
		},
		{
			Name:   "allAcknowledgedWritesMatched",
			Passed: ledger.Matched == ledger.Attempts-ledger.Rejected-ledger.Indeterminate,
			Detail: fmt.Sprintf("matched=%d attempts=%d rejected=%d indeterminate=%d", ledger.Matched, ledger.Attempts, ledger.Rejected, ledger.Indeterminate),
		},
	}, nil
}

func fieldDivergentCounterChecks(version int64, ledger LedgerSnapshot) []Check {
	return []Check{
		{
			Name:    "storedVersionEqualsUniqueObservedChain",
			Passed:  version == ledger.CAS.UniqueObserved,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("version=%d uniqueObserved=%d matched=%d", version, ledger.CAS.UniqueObserved, ledger.Matched),
		},
		{
			Name:   "everyMatchModified",
			Passed: ledger.Modified == ledger.Matched,
			Detail: fmt.Sprintf("modified=%d matched=%d", ledger.Modified, ledger.Matched),
		},
		{
			Name:   "matchedEdgesAreSuccessive",
			Passed: ledger.CAS.InvalidEdges == 0 && ledger.CAS.MatchedEdges == ledger.Matched,
			Detail: fmt.Sprintf("edges=%d invalid=%d matched=%d", ledger.CAS.MatchedEdges, ledger.CAS.InvalidEdges, ledger.Matched),
		},
		{
			Name:    "observedGenerationsFormCompleteChain",
			Passed:  version == 0 || ledger.CAS.HighestObserved == version-1,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("highestObserved=%d version=%d", ledger.CAS.HighestObserved, version),
		},
	}
}

func fieldDivergentBlindIncrementChecks(version int64, ledger LedgerSnapshot) []Check {
	return []Check{
		{
			Name:    "storedVersionDoesNotExceedMatched",
			Passed:  version >= 0 && version <= ledger.Matched,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("version=%d matched=%d coalesced=%d", version, ledger.Matched, ledger.Matched-version),
		},
		{
			Name:   "everyMatchModified",
			Passed: ledger.Modified == ledger.Matched,
			Detail: fmt.Sprintf("modified=%d matched=%d", ledger.Modified, ledger.Matched),
		},
		{
			Name:   "allAcknowledgedWritesMatched",
			Passed: ledger.Matched == ledger.Attempts-ledger.Rejected-ledger.Indeterminate,
			Detail: fmt.Sprintf("matched=%d attempts=%d rejected=%d indeterminate=%d", ledger.Matched, ledger.Attempts, ledger.Rejected, ledger.Indeterminate),
		},
	}
}

func counterChecks(version int64, ledger LedgerSnapshot) []Check {
	return []Check{
		{
			Name:    "storedVersionEqualsMatched",
			Passed:  version == ledger.Matched,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("version=%d matched=%d", version, ledger.Matched),
		},
		{
			Name:   "everyMatchModified",
			Passed: ledger.Modified == ledger.Matched,
			Detail: fmt.Sprintf("modified=%d matched=%d", ledger.Modified, ledger.Matched),
		},
		{
			Name:   "oneMatchPerObservedGeneration",
			Passed: ledger.CAS.DuplicateMatches == 0,
			Detail: fmt.Sprintf("duplicateMatches=%d", ledger.CAS.DuplicateMatches),
		},
		{
			Name:   "matchedEdgesAreSuccessive",
			Passed: ledger.CAS.InvalidEdges == 0 && ledger.CAS.MatchedEdges == ledger.Matched,
			Detail: fmt.Sprintf("edges=%d invalid=%d matched=%d", ledger.CAS.MatchedEdges, ledger.CAS.InvalidEdges, ledger.Matched),
		},
		{
			Name:    "matchedEdgesFormCompleteChain",
			Passed:  version == 0 || ledger.CAS.HighestObserved == version-1,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("highestObserved=%d version=%d", ledger.CAS.HighestObserved, version),
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

func (s *disjointSetScenario) Setup(ctx context.Context, collection Collection) error {
	err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "fields"},
		{Key: "payload", Value: s.payload},
	})
	return err
}

func (s *disjointSetScenario) Execute(ctx context.Context, collection Collection, worker int, sequence int64) Outcome {
	field := fmt.Sprintf("worker_%d", worker)
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "fields"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: sequence}}}},
	)
	outcome := UpdateOutcome(result, err)
	if outcome.Kind == OutcomeMatched {
		// Each worker executes sequentially, so its latest acknowledgement wins this slot.
		s.acknowledged[worker].Store(sequence)
	}
	return outcome
}

func (s *disjointSetScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document bson.M
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "fields"}}, &document); err != nil {
		return nil, err
	}
	checks := []Check{{
		Name:   "allAcknowledgedWritesMatched",
		Passed: ledger.Matched == ledger.Attempts-ledger.Rejected-ledger.Indeterminate,
		Detail: fmt.Sprintf("matched=%d attempts=%d rejected=%d indeterminate=%d", ledger.Matched, ledger.Attempts, ledger.Rejected, ledger.Indeterminate),
	}}
	for worker := range s.acknowledged {
		want := s.acknowledged[worker].Load()
		field := fmt.Sprintf("worker_%d", worker)
		value, present := document[field]
		got, numeric := numericInt64(value)
		passed := !present
		if want > 0 {
			passed = present && numeric && got == want
		}
		checks = append(checks, Check{
			Name:    field + "RetainsLastAcknowledgement",
			Passed:  passed,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("present=%t stored=%d acknowledged=%d", present, got, want),
		})
	}
	return checks, nil
}

type sameFieldSetScenario struct {
	payload string
}

type identicalSetScenario struct {
	payload string
}

func (s *identicalSetScenario) Name() string {
	return "identical-set"
}

func (s *identicalSetScenario) Setup(ctx context.Context, collection Collection) error {
	return collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "identical-field"},
		{Key: "value", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
}

func (s *identicalSetScenario) Execute(ctx context.Context, collection Collection, _ int, _ int64) Outcome {
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "identical-field"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "value", Value: int64(1)}}}},
	)
	return UpdateOutcome(result, err)
}

func (s *identicalSetScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document struct {
		Value int64
	}
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "identical-field"}}, &document); err != nil {
		return nil, err
	}
	return []Check{
		{
			Name:   "allAcknowledgedWritesMatched",
			Passed: ledger.Matched == ledger.Attempts-ledger.Rejected-ledger.Indeterminate,
			Detail: fmt.Sprintf("matched=%d attempts=%d rejected=%d indeterminate=%d", ledger.Matched, ledger.Attempts, ledger.Rejected, ledger.Indeterminate),
		},
		{
			Name:   "convergentValueRetained",
			Passed: document.Value == 1,
			Detail: fmt.Sprintf("value=%d", document.Value),
		},
	}, nil
}

type divergentCASDocument struct {
	Generation int64
	Value      int64
}

type divergentCASScenario struct {
	payload   string
	seed      int64
	maxDelay  time.Duration
	mergeMode string
}

func (s *divergentCASScenario) Name() string {
	return "divergent-cas"
}

func (s *divergentCASScenario) Setup(ctx context.Context, collection Collection) error {
	return collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "divergent-cas"},
		{Key: "generation", Value: int64(0)},
		{Key: "value", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
}

func (s *divergentCASScenario) Execute(ctx context.Context, collection Collection, worker int, sequence int64) Outcome {
	var observed divergentCASDocument
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "divergent-cas"}}, &observed); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	if err := waitCASDelay(ctx, s.seed, sequence, s.maxDelay); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	result, err := collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: "divergent-cas"},
			{Key: "generation", Value: observed.Generation},
		},
		bson.D{
			{Key: "$set", Value: bson.D{{Key: "value", Value: sequence}}},
			{Key: "$inc", Value: bson.D{{Key: "generation", Value: int64(1)}}},
		},
	)
	outcome := UpdateOutcome(result, err)
	outcome.Sequence = sequence
	outcome.Worker = worker
	outcome.CAS = &CASOperation{ObservedGeneration: observed.Generation, ProposedGeneration: observed.Generation + 1}
	return outcome
}

func (s *divergentCASScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document divergentCASDocument
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "divergent-cas"}}, &document); err != nil {
		return nil, err
	}
	checks := counterChecks(document.Generation, ledger)
	if s.mergeMode == MergeModeDocumentTouched || s.mergeMode == MergeModeDocumentDivergent {
		checks = append(checks, strictCASClientOutcomeChecks(ledger)...)
	}
	checks = append(checks, Check{
		Name: "finalValueBelongsToAcceptedOperation",
		Passed: document.Value > 0 && document.Value <= ledger.Attempts &&
			ledger.CAS.OperationMatched(document.Value),
		Skipped: ledger.Indeterminate > 0,
		Detail:  fmt.Sprintf("value=%d attempts=%d", document.Value, ledger.Attempts),
	})
	return checks, nil
}

func strictCASClientOutcomeChecks(ledger LedgerSnapshot) []Check {
	return []Check{
		{
			Name:   "refusedCASReturnsNoMatch",
			Passed: ledger.Rejected == 0,
			Detail: fmt.Sprintf("rejected=%d noMatch=%d", ledger.Rejected, ledger.NoMatch),
		},
		{
			Name:   "everyConclusiveCASAttemptHasCommandResult",
			Passed: ledger.Matched+ledger.NoMatch == ledger.Attempts-ledger.Indeterminate,
			Detail: fmt.Sprintf("matched=%d noMatch=%d attempts=%d indeterminate=%d", ledger.Matched, ledger.NoMatch, ledger.Attempts, ledger.Indeterminate),
		},
	}
}

type wholeDocumentCASScenario struct {
	name      string
	payload   string
	seed      int64
	maxDelay  time.Duration
	divergent bool
}

type wholeDocumentCASState struct {
	Generation int64
}

func (s *wholeDocumentCASScenario) Name() string {
	return s.name
}

func (s *wholeDocumentCASScenario) Setup(ctx context.Context, collection Collection) error {
	return collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "whole-document"},
		{Key: "generation", Value: int64(0)},
		{Key: "state", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
}

func (s *wholeDocumentCASScenario) Execute(ctx context.Context, collection Collection, worker int, sequence int64) Outcome {
	var observed wholeDocumentCASState
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "whole-document"}}, &observed); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	if err := waitCASDelay(ctx, s.seed, sequence, s.maxDelay); err != nil {
		return Outcome{Kind: OutcomeIndeterminate, Err: err}
	}
	nextGeneration := observed.Generation + 1
	fields := bson.D{
		{Key: "generation", Value: nextGeneration},
		{Key: "state", Value: nextGeneration},
	}
	if s.divergent {
		fields = append(fields, bson.E{Key: fmt.Sprintf("worker_%d", worker), Value: sequence})
	}
	result, err := collection.UpdateOne(ctx,
		bson.D{
			{Key: "_id", Value: "whole-document"},
			{Key: "generation", Value: observed.Generation},
		},
		bson.D{{Key: "$set", Value: fields}},
	)
	outcome := UpdateOutcome(result, err)
	outcome.Sequence = sequence
	outcome.Worker = worker
	outcome.CAS = &CASOperation{ObservedGeneration: observed.Generation, ProposedGeneration: nextGeneration}
	return outcome
}

func (s *wholeDocumentCASScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document bson.M
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "whole-document"}}, &document); err != nil {
		return nil, err
	}
	generation, generationOK := numericInt64(document["generation"])
	state, stateOK := numericInt64(document["state"])
	checks := []Check{
		{
			Name:   "wholeDocumentShapeIsValid",
			Passed: generationOK && stateOK && state == generation && document["payload"] == s.payload,
			Detail: fmt.Sprintf("generation=%d state=%d payloadBytes=%d", generation, state, len(s.payload)),
		},
	}
	if s.divergent {
		checks = append(checks, counterChecks(generation, ledger)...)
		workerFields := 0
		invalidWorkerFields := 0
		for key, value := range document {
			if !strings.HasPrefix(key, "worker_") {
				continue
			}
			workerFields++
			sequence, numeric := numericInt64(value)
			if !numeric || sequence <= 0 || !ledger.CAS.OperationMatched(sequence) {
				invalidWorkerFields++
			}
		}
		checks = append(checks, Check{
			Name:    "workerFieldsBelongToMatchedOperations",
			Passed:  (ledger.Matched == 0 || workerFields > 0) && invalidWorkerFields == 0,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("workerFields=%d invalid=%d", workerFields, invalidWorkerFields),
		})
	} else {
		checks = append(checks, fieldDivergentCounterChecks(generation, ledger)...)
		unexpectedFields := 0
		for key := range document {
			switch key {
			case "_id", "generation", "state", "payload":
			default:
				unexpectedFields++
			}
		}
		checks = append(checks, Check{
			Name:   "convergentWholeDocumentIsExact",
			Passed: unexpectedFields == 0,
			Detail: fmt.Sprintf("unexpectedFields=%d", unexpectedFields),
		})
	}
	checks = append(checks, strictCASClientOutcomeChecks(ledger)...)
	return checks, nil
}

func (s *sameFieldSetScenario) Name() string {
	return "same-set"
}

func (s *sameFieldSetScenario) Setup(ctx context.Context, collection Collection) error {
	err := collection.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "field"},
		{Key: "value", Value: int64(0)},
		{Key: "payload", Value: s.payload},
	})
	return err
}

func (s *sameFieldSetScenario) Execute(ctx context.Context, collection Collection, _ int, sequence int64) Outcome {
	result, err := collection.UpdateOne(ctx,
		bson.D{{Key: "_id", Value: "field"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "value", Value: sequence}}}},
	)
	return UpdateOutcome(result, err)
}

func (s *sameFieldSetScenario) Verify(ctx context.Context, collection Collection, ledger LedgerSnapshot) ([]Check, error) {
	var document struct {
		Value int64
	}
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: "field"}}, &document); err != nil {
		return nil, err
	}
	return []Check{
		{
			Name:   "allAcknowledgedWritesMatched",
			Passed: ledger.Matched == ledger.Attempts-ledger.Rejected-ledger.Indeterminate,
			Detail: fmt.Sprintf("matched=%d attempts=%d rejected=%d indeterminate=%d", ledger.Matched, ledger.Attempts, ledger.Rejected, ledger.Indeterminate),
		},
		{
			Name:    "finalValueWasIssued",
			Passed:  document.Value > 0 && document.Value <= ledger.Attempts,
			Skipped: ledger.Indeterminate > 0,
			Detail:  fmt.Sprintf("value=%d attempts=%d", document.Value, ledger.Attempts),
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
