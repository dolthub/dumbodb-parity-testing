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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const fieldDivergentMatrixPrefix = "field-divergent-matrix-"
const documentTouchedMatrixPrefix = "document-touched-matrix-"
const documentDivergentMatrixPrefix = "document-divergent-matrix-"

type matrixChangeKind string

const (
	matrixNoChange    matrixChangeKind = "none"
	matrixSetAOne     matrixChangeKind = "set-a-one"
	matrixSetATwo     matrixChangeKind = "set-a-two"
	matrixSetAOneBOne matrixChangeKind = "set-a-one-b-one"
	matrixInsertAOne  matrixChangeKind = "insert-a-one"
	matrixInsertATwo  matrixChangeKind = "insert-a-two"
	matrixDelete      matrixChangeKind = "delete"
)

type mergeMatrixRow struct {
	Name             string
	Base             bson.M
	FeatureChange    matrixChangeKind
	MainChange       matrixChangeKind
	ExpectConflict   bool
	ExpectedDocument bson.M
}

var fieldDivergentMatrixRows = []mergeMatrixRow{
	{Name: "one-sided", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixNoChange, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "disjoint-fields", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetBOne, ExpectedDocument: matrixDocument(1, 1)},
	{Name: "same-field-same-value", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetAOne, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "same-value-plus-disjoint", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOneBOne, MainChange: matrixSetAOne, ExpectedDocument: matrixDocument(1, 1)},
	{Name: "same-field-different-values", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetATwo, ExpectConflict: true, ExpectedDocument: matrixDocument(2, 0)},
	{Name: "modify-delete", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixDelete, ExpectConflict: true},
	{Name: "add-add-identical", FeatureChange: matrixInsertAOne, MainChange: matrixInsertAOne, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "add-add-different", FeatureChange: matrixInsertAOne, MainChange: matrixInsertATwo, ExpectConflict: true, ExpectedDocument: matrixDocument(2, 0)},
	{Name: "both-delete", Base: matrixDocument(0, 0), FeatureChange: matrixDelete, MainChange: matrixDelete},
}

var documentTouchedMatrixRows = []mergeMatrixRow{
	{Name: "one-sided", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixNoChange, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "disjoint-fields", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetBOne, ExpectConflict: true, ExpectedDocument: matrixDocument(0, 1)},
	{Name: "same-field-same-value", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetAOne, ExpectConflict: true, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "same-value-plus-disjoint", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOneBOne, MainChange: matrixSetAOne, ExpectConflict: true, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "same-field-different-values", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetATwo, ExpectConflict: true, ExpectedDocument: matrixDocument(2, 0)},
	{Name: "modify-delete", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixDelete, ExpectConflict: true},
	{Name: "add-add-identical", FeatureChange: matrixInsertAOne, MainChange: matrixInsertAOne, ExpectConflict: true, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "add-add-different", FeatureChange: matrixInsertAOne, MainChange: matrixInsertATwo, ExpectConflict: true, ExpectedDocument: matrixDocument(2, 0)},
	{Name: "both-delete", Base: matrixDocument(0, 0), FeatureChange: matrixDelete, MainChange: matrixDelete, ExpectConflict: true},
}

var documentDivergentMatrixRows = []mergeMatrixRow{
	{Name: "one-sided", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixNoChange, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "disjoint-fields", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetBOne, ExpectConflict: true, ExpectedDocument: matrixDocument(0, 1)},
	{Name: "same-field-same-value", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetAOne, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "same-value-plus-disjoint", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOneBOne, MainChange: matrixSetAOne, ExpectConflict: true, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "same-field-different-values", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixSetATwo, ExpectConflict: true, ExpectedDocument: matrixDocument(2, 0)},
	{Name: "modify-delete", Base: matrixDocument(0, 0), FeatureChange: matrixSetAOne, MainChange: matrixDelete, ExpectConflict: true},
	{Name: "add-add-identical", FeatureChange: matrixInsertAOne, MainChange: matrixInsertAOne, ExpectedDocument: matrixDocument(1, 0)},
	{Name: "add-add-different", FeatureChange: matrixInsertAOne, MainChange: matrixInsertATwo, ExpectConflict: true, ExpectedDocument: matrixDocument(2, 0)},
	{Name: "both-delete", Base: matrixDocument(0, 0), FeatureChange: matrixDelete, MainChange: matrixDelete},
}

const matrixSetBOne matrixChangeKind = "set-b-one"

type mergeMatrixDefinition struct {
	Mode   string
	Prefix string
	Rows   []mergeMatrixRow
}

var mergeMatrixDefinitions = []mergeMatrixDefinition{
	{
		Mode:   MergeModeDocumentTouched,
		Prefix: documentTouchedMatrixPrefix,
		Rows:   documentTouchedMatrixRows,
	},
	{
		Mode:   MergeModeDocumentDivergent,
		Prefix: documentDivergentMatrixPrefix,
		Rows:   documentDivergentMatrixRows,
	},
	{
		Mode:   MergeModeFieldDivergent,
		Prefix: fieldDivergentMatrixPrefix,
		Rows:   fieldDivergentMatrixRows,
	},
}

type mergeMatrixScenario struct {
	definition    mergeMatrixDefinition
	row           mergeMatrixRow
	payload       string
	mu            sync.Mutex
	main          BranchCollection
	mergeResponse bson.M
	mergeErr      error
}

func newMergeMatrixScenario(name, payload, mergeMode string) (Scenario, bool, error) {
	definition, matched := mergeMatrixDefinitionForName(name)
	if !matched {
		return nil, false, nil
	}
	if mergeMode == "" {
		return nil, true, fmt.Errorf("matrix scenario %q requires merge mode %q", name, definition.Mode)
	}
	if mergeMode != definition.Mode {
		return nil, true, fmt.Errorf("matrix scenario %q requires merge mode %q, got %q", name, definition.Mode, mergeMode)
	}
	rowName := strings.TrimPrefix(name, definition.Prefix)
	for _, row := range definition.Rows {
		if row.Name == rowName {
			return &mergeMatrixScenario{definition: definition, row: row, payload: payload}, true, nil
		}
	}
	return nil, true, fmt.Errorf("unknown %s matrix row %q", definition.Mode, rowName)
}

func mergeMatrixDefinitionForName(name string) (mergeMatrixDefinition, bool) {
	for _, definition := range mergeMatrixDefinitions {
		if strings.HasPrefix(name, definition.Prefix) {
			return definition, true
		}
	}
	return mergeMatrixDefinition{}, false
}

func (s *mergeMatrixScenario) Name() string {
	return s.definition.Prefix + s.row.Name
}

func (s *mergeMatrixScenario) Setup(ctx context.Context, collection Collection) error {
	branchCollection, ok := collection.(BranchCollection)
	if !ok {
		return fmt.Errorf("%s matrix requires branch-capable collection", s.definition.Mode)
	}
	database := branchCollection.DatabaseName()
	if s.row.Base != nil {
		if err := branchCollection.InsertOne(ctx, matrixDocumentWithPayload(s.row.Base, s.payload)); err != nil {
			return err
		}
	}
	if err := runSuccessfulCommand(ctx, branchCollection, database, bson.D{
		{Key: "doltCommit", Value: int32(1)},
		{Key: "message", Value: "matrix baseline"},
		{Key: "author", Value: "concurrency-harness"},
	}); err != nil {
		return err
	}
	mainDatabase := database + "@main"
	if err := runSuccessfulCommand(ctx, branchCollection, mainDatabase, bson.D{
		{Key: "doltBranch", Value: int32(1)},
		{Key: "action", Value: "add"},
		{Key: "branch", Value: "feature"},
	}); err != nil {
		return err
	}
	feature := branchCollection.AtDatabase(database + "@feature")
	main := branchCollection.AtDatabase(mainDatabase)
	if err := applyMatrixChange(ctx, feature, s.row.FeatureChange, s.payload); err != nil {
		return err
	}
	if err := commitMatrixChange(ctx, feature, s.row.FeatureChange, "feature change"); err != nil {
		return err
	}
	if err := applyMatrixChange(ctx, main, s.row.MainChange, s.payload); err != nil {
		return err
	}
	if err := commitMatrixChange(ctx, main, s.row.MainChange, "main change"); err != nil {
		return err
	}
	s.main = main
	return nil
}

func (s *mergeMatrixScenario) Execute(ctx context.Context, _ Collection, _ int, _ int64) Outcome {
	response, err := s.main.RunCommand(ctx, s.main.DatabaseName(), bson.D{
		{Key: "doltMerge", Value: int32(1)},
		{Key: "mergeIn", Value: "feature"},
	})
	s.mu.Lock()
	s.mergeResponse = response
	s.mergeErr = err
	s.mu.Unlock()
	if commandSucceeded(response) {
		return Outcome{Kind: OutcomeMatched, Modified: true}
	}
	return Outcome{Kind: OutcomeRejected, Err: err}
}

func (s *mergeMatrixScenario) Verify(ctx context.Context, _ Collection, _ LedgerSnapshot) ([]Check, error) {
	s.mu.Lock()
	response := s.mergeResponse
	mergeErr := s.mergeErr
	s.mu.Unlock()
	conflicted := !commandSucceeded(response)
	document, absent, err := readMatrixDocument(ctx, s.main)
	if err != nil {
		return nil, err
	}
	statePassed := false
	expectedDocument := matrixDocumentWithPayload(s.row.ExpectedDocument, s.payload)
	if expectedDocument == nil {
		statePassed = absent
	} else {
		statePassed = !absent && reflect.DeepEqual(document, expectedDocument)
	}
	conflictCount := 0
	if conflicted {
		conflicts, commandErr := s.main.RunCommand(ctx, s.main.DatabaseName(), bson.D{{Key: "doltConflicts", Value: int32(1)}})
		if commandErr != nil {
			return nil, commandErr
		}
		if entries, ok := conflicts["conflicts"].(bson.A); ok {
			conflictCount = len(entries)
		}
	}
	return []Check{
		{
			Name:   "mergeVerdictMatchesMode",
			Passed: conflicted == s.row.ExpectConflict,
			Detail: fmt.Sprintf("mode=%s row=%s conflict=%t expected=%t error=%v", s.definition.Mode, s.row.Name, conflicted, s.row.ExpectConflict, mergeErr),
		},
		{
			Name:   "finalStateMatchesMode",
			Passed: statePassed,
			Detail: fmt.Sprintf("mode=%s row=%s absent=%t document=%v expected=%v", s.definition.Mode, s.row.Name, absent, document, expectedDocument),
		},
		{
			Name:   "expectedConflictIsRecorded",
			Passed: !s.row.ExpectConflict || conflictCount > 0,
			Detail: fmt.Sprintf("row=%s conflicts=%d", s.row.Name, conflictCount),
		},
	}, nil
}

func matrixDocument(a, b int64) bson.M {
	return bson.M{"_id": "matrix", "a": a, "b": b}
}

func matrixDocumentWithPayload(document bson.M, payload string) bson.M {
	if document == nil {
		return nil
	}
	withPayload := make(bson.M, len(document)+1)
	for key, value := range document {
		withPayload[key] = value
	}
	withPayload["payload"] = payload
	return withPayload
}

func applyMatrixChange(ctx context.Context, collection BranchCollection, change matrixChangeKind, payload string) error {
	switch change {
	case matrixNoChange:
		return nil
	case matrixSetAOne:
		return updateMatrixFields(ctx, collection, bson.M{"a": int64(1)})
	case matrixSetATwo:
		return updateMatrixFields(ctx, collection, bson.M{"a": int64(2)})
	case matrixSetBOne:
		return updateMatrixFields(ctx, collection, bson.M{"b": int64(1)})
	case matrixSetAOneBOne:
		return updateMatrixFields(ctx, collection, bson.M{"a": int64(1), "b": int64(1)})
	case matrixInsertAOne:
		return collection.InsertOne(ctx, matrixDocumentWithPayload(matrixDocument(1, 0), payload))
	case matrixInsertATwo:
		return collection.InsertOne(ctx, matrixDocumentWithPayload(matrixDocument(2, 0), payload))
	case matrixDelete:
		result, err := collection.DeleteOne(ctx, bson.M{"_id": "matrix"})
		if err == nil && result.Matched != 1 {
			return fmt.Errorf("delete matched %d documents", result.Matched)
		}
		return err
	default:
		return fmt.Errorf("unknown matrix change %q", change)
	}
}

func updateMatrixFields(ctx context.Context, collection BranchCollection, fields bson.M) error {
	result, err := collection.UpdateOne(ctx, bson.M{"_id": "matrix"}, bson.M{"$set": fields})
	if err == nil && result.Matched != 1 {
		return fmt.Errorf("update matched %d documents", result.Matched)
	}
	return err
}

func commitMatrixChange(ctx context.Context, collection BranchCollection, change matrixChangeKind, message string) error {
	if change == matrixNoChange {
		return nil
	}
	return runSuccessfulCommand(ctx, collection, collection.DatabaseName(), bson.D{
		{Key: "doltCommit", Value: int32(1)},
		{Key: "message", Value: message},
		{Key: "author", Value: "concurrency-harness"},
	})
}

func runSuccessfulCommand(ctx context.Context, collection BranchCollection, database string, command interface{}) error {
	response, err := collection.RunCommand(ctx, database, command)
	if err != nil {
		return err
	}
	if !commandSucceeded(response) {
		return fmt.Errorf("command failed: %v", response)
	}
	return nil
}

func commandSucceeded(response bson.M) bool {
	switch ok := response["ok"].(type) {
	case float64:
		return ok == 1
	case int32:
		return ok == 1
	case int64:
		return ok == 1
	default:
		return false
	}
}

func readMatrixDocument(ctx context.Context, collection BranchCollection) (bson.M, bool, error) {
	var document bson.M
	err := collection.FindOne(ctx, bson.M{"_id": "matrix"}, &document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, true, nil
	}
	return document, false, err
}
