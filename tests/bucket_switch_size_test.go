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

package tests

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

var bsonObjSizePattern = regexp.MustCompile(`BSONObj size: (\d+)`)

// TestBucketSwitchSize: $bucket fails with 10334 once an object of the
// $switch mongod builds for it is too large, innermost first: an object of
// one branch, the branches array, the $switch with its default, or the
// $group with its output fields. The reported object size is compared too.
func TestBucketSwitchSize(t *testing.T) {
	ints := func(n int) bson.A {
		a := make(bson.A, n)
		for i := range a {
			a[i] = int32(i)
		}
		return a
	}
	ranges := func(n int) bson.A {
		a := make(bson.A, n)
		for i := range a {
			a[i] = bson.D{{Key: "$range", Value: bson.A{int32(i), int32(i + 1000000)}}}
		}
		return a
	}
	literal := func(n int) bson.D {
		return bson.D{{Key: "s", Value: bson.D{{Key: "$max", Value: bson.D{{Key: "$literal", Value: strings.Repeat("a", n)}}}}}}
	}
	cases := map[string]bson.D{
		// The branches array: about 115 bytes for each branch.
		"ManyBoundaries": {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ints(200000)}},
		// The $group, without and with output.
		"DefaultFits":      {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ints(145730)}, {Key: "default", Value: strings.Repeat("d", 53)}},
		"DefaultGroupOver": {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ints(145730)}, {Key: "default", Value: strings.Repeat("d", 54)}},
		"OutputGroupOver":  {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ints(145730)}, {Key: "output", Value: literal(60)}},
		// One branch: its $gte array, then the branch itself.
		"LargeBoundary": {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ranges(8)}},
		"LargeBranch":   {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: bson.A{strings.Repeat("a", 9000000), "b"}}},
		// Checks mongod makes before it builds the $switch still come first.
		"LargeBoundaryDefaultInRange": {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ranges(8)},
			{Key: "default", Value: bson.D{{Key: "$range", Value: bson.A{int32(0), int32(5)}}}}},
		"LargeBoundaryNotConstant": {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: append(ranges(8), "$y")}},
		"LargeBoundaryNoGroupBy":   {{Key: "boundaries", Value: ranges(8)}},
		"LargeBoundaryUnknown":     {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: ranges(8)}, {Key: "zz", Value: int32(1)}},
		"LargeBoundaryLastFoldError": {{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: append(ranges(8),
			bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}})}},
	}
	for name, spec := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "BucketSwitchSize_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(0)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$bucket", Value: spec}}})
				if err != nil {
					res := errOutcome(err)
					if m := bsonObjSizePattern.FindStringSubmatch(err.Error()); m != nil {
						res["size"] = m[1]
					}
					return res, nil
				}
				return drainCursor(ctx, cursor, nil)
			},
		})
	}

	// Past the size limit, DumboDB folds no boundary given as an expression
	// but the last, so memory and time stay bounded; mongod folds them all
	// and reports an error among them that DumboDB does not see.
	unsorted := append(ranges(8), bson.D{{Key: "$range", Value: bson.A{int32(0), int32(1)}}})
	foldError := append(ranges(4), bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}})
	foldError = append(foldError, ranges(8)[4:]...)
	deviations := map[string]struct {
		spec      bson.D
		mongoCode int32
	}{
		"LargeBoundaryLastUnsorted":    {bson.D{{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: unsorted}}, 40194},
		"LargeBoundaryMiddleFoldError": {bson.D{{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: foldError}}, 2},
	}
	for name, tc := range deviations {
		harness.PairTest(t, harness.TestCase{
			Name:    "BucketSwitchSize_" + name,
			Support: harness.DumboDBDeviates,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$bucket", Value: tc.spec}}})
				return drainCursor(ctx, cursor, err)
			},
			MongoExpect: expectErrorCode("mongo", tc.mongoCode),
			DumboExpect: expectErrorCode("dumbodb", 10334),
		})
	}
}

func expectErrorCode(server string, code int32) func(*testing.T, interface{}, error) {
	return func(t *testing.T, got interface{}, err error) {
		t.Helper()
		res, _ := got.(bson.M)
		if err != nil || res == nil || res["code"] != code {
			t.Errorf("%s: got %v (%v), want error code %d", server, got, err, code)
		}
	}
}
