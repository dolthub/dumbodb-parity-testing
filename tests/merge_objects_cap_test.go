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
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMergeObjectsCap: DumboDB deliberately limits the document a
// $mergeObjects accumulator keeps to 100MB, which mongod does not limit; a
// value is counted while its key holds it. A merge just over the limit
// succeeds on mongod and fails on DumboDB with ExceededMemoryLimit; one just
// under it, and one whose values overwrite a single key, succeed on both.
func TestMergeObjectsCap(t *testing.T) {
	const valueBytes = 8_500_000
	seed := func(n int) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			big := strings.Repeat("x", valueBytes)
			for i := 0; i < n; i++ {
				doc := bson.D{{Key: "_id", Value: int32(i)}, {Key: "k", Value: fmt.Sprintf("k%d", i)}, {Key: "s", Value: big}}
				if _, err := col.InsertOne(ctx, doc); err != nil {
					return err
				}
			}
			return nil
		}
	}
	merge := func(key interface{}) bson.A {
		return bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
				{Key: "m", Value: bson.D{{Key: "$mergeObjects", Value: bson.D{{Key: "$arrayToObject", Value: bson.A{bson.A{bson.A{key, "$s"}}}}}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "n", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$objectToArray", Value: "$m"}}}}}}}},
		}
	}
	run := func(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var out []bson.M
			err = cursor.All(ctx, &out)
			return out, err
		}
	}

	harness.PairTest(t, harness.TestCase{
		Name:    "MergeObjectsCap_over",
		Support: harness.DumboDBDeviates,
		Setup:   seed(13),
		Run:     run(merge("$k")),
		MongoExpect: func(t *testing.T, res interface{}, err error) {
			if err != nil || fmt.Sprint(res) != "[map[n:13]]" {
				t.Errorf("mongo: want [map[n:13]], got %v (err %v)", res, err)
			}
		},
		DumboExpect: func(t *testing.T, res interface{}, err error) {
			var cmdErr mongo.CommandError
			if !errors.As(err, &cmdErr) || cmdErr.Code != 146 || !strings.Contains(cmdErr.Message, "mongod applies none") {
				t.Errorf("dumbodb: want ExceededMemoryLimit naming the deviation, got %v (err %v)", res, err)
			}
		},
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "MergeObjectsCap_under",
		Support: harness.DumboDBFull,
		Setup:   seed(11),
		Run:     run(merge("$k")),
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "MergeObjectsCap_overwritten",
		Support: harness.DumboDBFull,
		Setup:   seed(13),
		Run:     run(merge("same")),
	})
}
