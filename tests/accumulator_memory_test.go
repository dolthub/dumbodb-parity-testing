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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// An accumulator that keeps values fails with ExceededMemoryLimit once they
// pass 100MB, however small the input documents are.
func TestAccumulatorMemoryLimit(t *testing.T) {
	big := bson.D{{Key: "$range", Value: bson.A{int32(0), int32(1000000)}}}
	bigFrom := bson.D{{Key: "$range", Value: bson.A{"$_id", int32(1000000)}}}
	group := func(acc bson.D) []bson.D {
		return []bson.D{{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "f", Value: acc}}}}}
	}
	cases := map[string][]bson.D{
		"Push":     group(bson.D{{Key: "$push", Value: big}}),
		"AddToSet": group(bson.D{{Key: "$addToSet", Value: bigFrom}}),
		"FirstN":   group(bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "input", Value: big}}}}),
		"LastNFew": group(bson.D{{Key: "$lastN", Value: bson.D{{Key: "n", Value: int32(3)}, {Key: "input", Value: big}}}}),
		"TopN":     group(bson.D{{Key: "$topN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "output", Value: big}, {Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}}}}),
		"MaxN":     group(bson.D{{Key: "$maxN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "input", Value: bigFrom}}}}),
		"BucketAutoPush": {{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$_id"}, {Key: "buckets", Value: int32(1)},
			{Key: "output", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$push", Value: big}}}}}}}}},
		"WindowPush": {{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$push", Value: big},
				{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "current"}}}}}}}}}}}},
	}
	for name, stages := range cases {
		stages := append(stages, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}})
		harness.PairTest(t, harness.TestCase{
			Name:    "AccumulatorMemoryLimit_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				var docs []interface{}
				for i := int32(1); i <= 14; i++ {
					docs = append(docs, bson.D{{Key: "_id", Value: i}})
				}
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
}

// mongod does not limit the memory $mergeObjects keeps.
func TestMergeObjectsMemoryUnlimited(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "MergeObjectsMemoryUnlimited",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			pad := strings.Repeat("x", 8<<20)
			for i := int32(1); i <= 13; i++ {
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: i}, {Key: "f", Value: pad}, {Key: "i", Value: i}}); err != nil {
					return err
				}
			}
			return nil
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "m", Value: bson.D{{Key: "$mergeObjects", Value: "$$ROOT"}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$strLenBytes", Value: "$m.f"}}}}}})
		},
	})
}

// mongod sizes kept values as its engine stores them: SBE's $group charges 9
// bytes for each element of an array of numbers, the classic engine 16, so a
// group that passes in one engine can fail in the other.
func TestAccumulatorMemoryPerEngine(t *testing.T) {
	ranged := bson.D{{Key: "$range", Value: bson.A{"$z", int32(1000000)}}}
	group := func(acc bson.D) bson.D {
		return bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "f", Value: acc}}}}
	}
	classic := bson.D{{Key: "$set", Value: bson.D{{Key: "q", Value: int32(1)}}}}
	cases := []struct {
		name   string
		docs   int32
		stages []bson.D
	}{
		{"SBEPush11", 11, []bson.D{group(bson.D{{Key: "$push", Value: ranged}})}},
		{"SBEPush12", 12, []bson.D{group(bson.D{{Key: "$push", Value: ranged}})}},
		{"SBEAddToSet11", 11, []bson.D{group(bson.D{{Key: "$addToSet", Value: bson.D{{Key: "$range", Value: bson.A{"$_id", int32(1000000)}}}}})}},
		{"SBEFirstN11", 11, []bson.D{group(bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "input", Value: ranged}}}})}},
		{"ClassicPush6", 6, []bson.D{classic, group(bson.D{{Key: "$push", Value: ranged}})}},
		{"ClassicPush7", 7, []bson.D{classic, group(bson.D{{Key: "$push", Value: ranged}})}},
	}
	for _, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    "AccumulatorMemoryPerEngine_" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				var docs []interface{}
				for i := int32(0); i < c.docs; i++ {
					docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "z", Value: int32(0)}})
				}
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, append(c.stages, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}})...)
			},
		})
	}
}

// SBE charges a value read from a stored document at its BSON size and an
// array it builds at 9 bytes a slot, with stored documents in it as views of
// their BSON. The classic engine's $bucketAuto charges each value $push keeps
// twice; its $group charges once.
func TestAccumulatorMemoryStoredValues(t *testing.T) {
	empties := make(bson.A, 1000000)
	for i := range empties {
		empties[i] = bson.D{}
	}
	pad := strings.Repeat("x", 8<<20)
	seedArrays := func(n int32) func(ctx context.Context, col *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			for i := int32(0); i < n; i++ {
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: i}, {Key: "a", Value: empties}}); err != nil {
					return err
				}
			}
			return nil
		}
	}
	seedStrings := func(n int32) func(ctx context.Context, col *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			for i := int32(0); i < n; i++ {
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: i}, {Key: "s", Value: pad}}); err != nil {
					return err
				}
			}
			return nil
		}
	}
	group := func(acc bson.D) bson.D {
		return bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "f", Value: acc}}}}
	}
	bucketAuto := func(acc bson.D) bson.D {
		return bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$_id"}, {Key: "buckets", Value: int32(1)},
			{Key: "output", Value: bson.D{{Key: "f", Value: acc}}}}}}
	}
	cases := []struct {
		name  string
		setup func(ctx context.Context, col *mongo.Collection) error
		stage bson.D
	}{
		{"SBEPushStored4", seedArrays(4), group(bson.D{{Key: "$push", Value: "$a"}})},
		{"SBEFirstNStored4", seedArrays(4), group(bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(10)}, {Key: "input", Value: "$a"}}}})},
		{"SBEPushBuilt4", seedArrays(4), group(bson.D{{Key: "$push", Value: bson.D{{Key: "$concatArrays", Value: bson.A{"$a"}}}}})},
		{"BucketAutoPush6", seedStrings(6), bucketAuto(bson.D{{Key: "$push", Value: "$s"}})},
		{"BucketAutoPush7", seedStrings(7), bucketAuto(bson.D{{Key: "$push", Value: "$s"}})},
		{"BucketAutoFirstN12", seedStrings(12), bucketAuto(bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(20)}, {Key: "input", Value: "$s"}}}})},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "AccumulatorMemoryStoredValues_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   c.setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, c.stage, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}})
			},
		})
	}
}
