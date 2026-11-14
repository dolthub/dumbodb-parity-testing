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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestBucketFoldedSize: $bucket's folded boundaries and default must fit in
// the $switch mongod builds from them, which it refuses once every field has
// been checked, but before the groupBy and output expressions parse.
func TestBucketFoldedSize(t *testing.T) {
	const big = `{"$range":[0,1000000]}`
	const big1 = `{"$range":[1,1000001]}`
	const mid = `{"$range":[0,500000]}`
	const mid1 = `{"$range":[1,500001]}`
	cases := map[string]string{
		"two_mid":          `{"groupBy":"$x","boundaries":[` + mid + `,` + mid1 + `]}`,
		"big_default":      `{"groupBy":"$x","boundaries":[0,1],"default":{"$range":[0,1400000]}}`,
		"fits_default":     `{"groupBy":"$x","boundaries":[0,1],"default":{"$range":[0,300000]}}`,
		"one_big":          `{"groupBy":"$x","boundaries":[` + big + `]}`,
		"big_then_string":  `{"groupBy":"$x","boundaries":[` + mid + `,"a"]}`,
		"later_unsorted":   `{"groupBy":"$x","boundaries":[` + mid + `,` + mid1 + `,{"$range":[0,1]}]}`,
		"later_fold_error": `{"groupBy":"$x","boundaries":[` + mid + `,` + mid1 + `,{"$divide":[1,0]}]}`,
		"unknown_after":    `{"groupBy":"$x","boundaries":[` + mid + `,` + mid1 + `],"zz":1}`,
		"default_in_range": `{"groupBy":"$x","boundaries":[` + big + `,` + big1 + `],"default":{"$range":[0,1000001]}}`,
		"bad_group_by":     `{"groupBy":"$a..b","boundaries":[` + mid + `,` + mid1 + `]}`,
		"bad_output":       `{"groupBy":"$x","boundaries":[` + mid + `,` + mid1 + `],"output":{"a":{"$bogus":1}}}`,
	}
	for name, spec := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"$bucket":`+spec+`}`), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "BucketFoldedSize_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "x", Value: -1}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{s, bson.D{{Key: "$project", Value: bson.D{{Key: "isArray", Value: bson.D{{Key: "$isArray", Value: "$_id"}}}}}}})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
