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

package harness

import (
	"errors"
	"math"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestCompareResponses_MillisTolerance covers the asymmetric millis
// tolerance: DumboDB may be faster than MongoDB without limit; DumboDB
// is allowed to be at most millisToleranceMS slower before the
// comparison flags the divergence.
func TestCompareResponses_MillisTolerance(t *testing.T) {
	base := func(millis int32) bson.D {
		return bson.D{
			{Key: "estimate", Value: false},
			{Key: "millis", Value: millis},
			{Key: "numObjects", Value: int32(5)},
			{Key: "ok", Value: float64(1)},
			{Key: "size", Value: int32(216)},
		}
	}

	cases := []struct {
		name        string
		mongoMillis int32
		dumboMillis int32
		wantResult  CompareResult
	}{
		{name: "dumbodb_faster_zero", mongoMillis: 1, dumboMillis: 0, wantResult: Match},
		{name: "dumbodb_much_faster", mongoMillis: 50, dumboMillis: 5, wantResult: Match},
		{name: "exact_match", mongoMillis: 3, dumboMillis: 3, wantResult: Match},
		{name: "dumbodb_slower_within_tolerance", mongoMillis: 10, dumboMillis: 15, wantResult: Match},
		{name: "dumbodb_slower_at_tolerance_boundary", mongoMillis: 10, dumboMillis: 15, wantResult: Match},
		{name: "dumbodb_slower_beyond_tolerance", mongoMillis: 10, dumboMillis: 16, wantResult: Diverge},
		{name: "dumbodb_much_slower", mongoMillis: 5, dumboMillis: 100, wantResult: Diverge},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareResponses(base(tc.mongoMillis), nil, base(tc.dumboMillis), nil)
			if got.Result != tc.wantResult {
				t.Errorf("mongo=%dms dumbo=%dms: result = %v, want %v (diff: %s)",
					tc.mongoMillis, tc.dumboMillis, got.Result, tc.wantResult, got.Diff)
			}
			if tc.wantResult == Diverge && !strings.Contains(got.Diff, "millis") {
				t.Errorf("expected diff to mention millis, got: %s", got.Diff)
			}
		})
	}
}

func TestCompareErrorsContract(t *testing.T) {
	commandError := func(message, name string, labels ...string) error {
		return mongo.CommandError{Code: 112, Name: name, Message: message, Labels: labels}
	}

	t.Run("message is informational", func(t *testing.T) {
		got := CompareResponses(nil, commandError("mongo wording", "WriteConflict"),
			nil, commandError("dumbodb wording", "WriteConflict"))
		if got.Result != Match || !strings.Contains(got.Diff, "informational") {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("different unstructured messages diverge", func(t *testing.T) {
		got := CompareResponses(nil, errors.New("context deadline exceeded"),
			nil, errors.New("decode failure"))
		if got.Result != Diverge || !strings.Contains(got.Diff, "message") {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("identical unstructured messages match", func(t *testing.T) {
		got := CompareResponses(nil, errors.New("context deadline exceeded"),
			nil, errors.New("context deadline exceeded"))
		if got.Result != Match {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("code name is required", func(t *testing.T) {
		got := CompareResponses(nil, commandError("same", "WriteConflict"),
			nil, commandError("same", "ErrorCode(112)"))
		if got.Result != Diverge || !strings.Contains(got.Diff, "codeName") {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("labels compare as a set", func(t *testing.T) {
		got := CompareResponses(nil,
			commandError("same", "WriteConflict", "TransientTransactionError", "RetryableWriteError"),
			nil, commandError("same", "WriteConflict", "RetryableWriteError", "TransientTransactionError"))
		if got.Result != Match {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("missing label diverges", func(t *testing.T) {
		got := CompareResponses(nil,
			commandError("same", "WriteConflict", "TransientTransactionError"),
			nil, commandError("same", "WriteConflict"))
		if got.Result != Diverge || !strings.Contains(got.Diff, "labels") {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("write exception labels", func(t *testing.T) {
		mongoErr := mongo.WriteException{
			WriteErrors: mongo.WriteErrors{{Code: 11000, Message: "mongo"}},
			Labels:      []string{"RetryableWriteError"},
		}
		dumboErr := mongo.WriteException{
			WriteErrors: mongo.WriteErrors{{Code: 11000, Message: "dumbodb"}},
			Labels:      []string{"RetryableWriteError"},
		}
		got := CompareResponses(nil, mongoErr, nil, dumboErr)
		if got.Result != Match {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("bulk write exception labels", func(t *testing.T) {
		mongoErr := mongo.BulkWriteException{
			WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Code: 11000}}},
			Labels:      []string{"RetryableWriteError", "TransientTransactionError"},
		}
		dumboErr := mongo.BulkWriteException{
			WriteErrors: []mongo.BulkWriteError{{WriteError: mongo.WriteError{Code: 11000}}},
			Labels:      []string{"TransientTransactionError", "RetryableWriteError"},
		}
		got := CompareResponses(nil, mongoErr, nil, dumboErr)
		if got.Result != Match {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("write concern error code and name", func(t *testing.T) {
		mongoErr := mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{
			Code: 100, Name: "UnsatisfiableWriteConcern", Message: "mongo wording",
		}}
		dumboErr := mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{
			Code: 100, Name: "UnsatisfiableWriteConcern", Message: "dumbodb wording",
		}}
		got := CompareResponses(nil, mongoErr, nil, dumboErr)
		if got.Result != Match {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})

	t.Run("one error only diverges", func(t *testing.T) {
		got := CompareResponses(nil, errors.New("failure"), nil, nil)
		if got.Result != Diverge {
			t.Fatalf("got result=%v diff=%q", got.Result, got.Diff)
		}
	})
}

// TestCompareResponses_MillisOnlyOnOneSide ensures that an asymmetric
// millis presence does not crash and does not incorrectly mask a real
// divergence elsewhere.
func TestCompareResponses_MillisOnlyOnOneSide(t *testing.T) {
	mongo := bson.D{{Key: "ok", Value: float64(1)}, {Key: "millis", Value: int32(3)}}
	dumbo := bson.D{{Key: "ok", Value: float64(1)}}
	got := CompareResponses(mongo, nil, dumbo, nil)
	if got.Result != Diverge {
		t.Errorf("missing millis on one side should diverge, got %v", got.Result)
	}
}

// TestCompareResponses_MillisToleranceDoesNotMaskOtherFields confirms
// the tolerance is local to the millis field: any other diverging field
// must still surface.
func TestCompareResponses_MillisToleranceDoesNotMaskOtherFields(t *testing.T) {
	mongo := bson.D{
		{Key: "millis", Value: int32(10)},
		{Key: "size", Value: int32(216)},
		{Key: "ok", Value: float64(1)},
	}
	dumbo := bson.D{
		{Key: "millis", Value: int32(12)}, // within tolerance
		{Key: "size", Value: int32(999)},  // diverges
		{Key: "ok", Value: float64(1)},
	}
	got := CompareResponses(mongo, nil, dumbo, nil)
	if got.Result != Diverge {
		t.Errorf("non-millis diverge must surface, got %v", got.Result)
	}
	if !strings.Contains(got.Diff, "216") || !strings.Contains(got.Diff, "999") {
		t.Errorf("diff should report the size mismatch, got: %s", got.Diff)
	}
}

// TestCompareResponses_NestedMillis exercises the recursive walk: when
// millis sits inside a nested sub-document, the tolerance still applies.
func TestCompareResponses_NestedMillis(t *testing.T) {
	mongo := bson.D{
		{Key: "stats", Value: bson.D{
			{Key: "millis", Value: int32(2)},
			{Key: "count", Value: int32(5)},
		}},
		{Key: "ok", Value: float64(1)},
	}
	dumbo := bson.D{
		{Key: "stats", Value: bson.D{
			{Key: "millis", Value: int32(4)}, // within tolerance
			{Key: "count", Value: int32(5)},
		}},
		{Key: "ok", Value: float64(1)},
	}
	got := CompareResponses(mongo, nil, dumbo, nil)
	if got.Result != Match {
		t.Errorf("nested millis within tolerance should match, got %v (diff: %s)", got.Result, got.Diff)
	}
}

func TestCompareResponses_ExactNumericTypesAndDates(t *testing.T) {
	tenth, fifth := 0.1, 0.2
	cases := []struct {
		name         string
		mongo, dumbo interface{}
		want         CompareResult
	}{
		{"int32_vs_int64", bson.D{{Key: "n", Value: int32(1)}}, bson.D{{Key: "n", Value: int64(1)}}, Diverge},
		{"int32_vs_double", bson.D{{Key: "n", Value: int32(1)}}, bson.D{{Key: "n", Value: float64(1)}}, Diverge},
		{"double_rounding", bson.D{{Key: "n", Value: 0.3}}, bson.D{{Key: "n", Value: tenth + fifth}}, Diverge},
		{"negative_zero", bson.D{{Key: "n", Value: math.Copysign(0, -1)}}, bson.D{{Key: "n", Value: float64(0)}}, Diverge},
		{"date_value", bson.D{{Key: "d", Value: primitive.DateTime(1000)}}, bson.D{{Key: "d", Value: primitive.DateTime(1001)}}, Diverge},
		{"same_int32", bson.D{{Key: "n", Value: int32(7)}}, bson.D{{Key: "n", Value: int32(7)}}, Match},
		{"same_date", bson.D{{Key: "d", Value: primitive.DateTime(1000)}}, bson.D{{Key: "d", Value: primitive.DateTime(1000)}}, Match},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareResponses(tc.mongo, nil, tc.dumbo, nil)
			if got.Result != tc.want {
				t.Errorf("got %v, want %v (diff: %s)", got.Result, tc.want, got.Diff)
			}
		})
	}
}
