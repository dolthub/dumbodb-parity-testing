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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
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
