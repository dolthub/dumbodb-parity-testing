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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMergeOnPaths: $merge finds the target document for a dotted 'on' field
// as mongod's update query does, by value and through the target's arrays,
// and refuses an input whose 'on' value is missing, null, undefined, an
// array, or reached through an array; an 'on' without a matching unique
// index fails with 51183.
func TestMergeOnPaths(t *testing.T) {
	cases := map[string]struct {
		project bson.D
		on      interface{}
	}{
		"DottedMatch":         {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "m", Value: int32(1)}}}}}, {Key: "w", Value: "new"}}, "k.m"},
		"DottedNoMatch":       {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "m", Value: int32(5)}}}}}, {Key: "w", Value: "new"}}, "k.m"},
		"DottedNumericMatch":  {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "m", Value: 1.0}}}}}, {Key: "w", Value: "new"}}, "k.m"},
		"DottedIndexIntoArr":  {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "0", Value: bson.D{{Key: "m", Value: int32(2)}}}}}}}}, "k.0.m"},
		"DottedThroughArray":  {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.A{bson.D{{Key: "m", Value: int32(2)}}}}}}}, "k.0.m"},
		"DottedArrayValue":    {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "m", Value: bson.A{int32(1)}}}}}}}, "k.m"},
		"DottedNull":          {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "m", Value: nil}}}}}}, "k.m"},
		"DottedMissing":       {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$literal", Value: bson.D{{Key: "z", Value: int32(1)}}}}}}, "k.m"},
		"DottedParentScalar":  {bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: int32(5)}}, "k.m"},
		"IDNull":              {bson.D{{Key: "_id", Value: nil}}, "_id"},
		"IDNullArrayOn":       {bson.D{{Key: "_id", Value: nil}}, bson.A{"_id"}},
		"IDArray":             {bson.D{{Key: "_id", Value: bson.D{{Key: "$literal", Value: bson.A{int32(1)}}}}}, "_id"},
		"IDRemoved":           {bson.D{{Key: "_id", Value: "$$REMOVE"}, {Key: "w", Value: "new"}}, "_id"},
		"NonUniqueIndex":      {bson.D{{Key: "c", Value: int32(1)}}, "c"},
		"NoIndex":             {bson.D{{Key: "n", Value: int32(1)}}, "n"},
		"CompoundPrefix":      {bson.D{{Key: "a", Value: int32(1)}}, "a"},
		"CompoundAnyOrder":    {bson.D{{Key: "_id", Value: 0}, {Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}, bson.A{"b", "a"}},
		"CompoundExtraField":  {bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}, bson.A{"a", "b", "_id"}},
		"SparseUnique":        {bson.D{{Key: "_id", Value: 0}, {Key: "s", Value: int32(1)}}, "s"},
		"PartialUnique":       {bson.D{{Key: "_id", Value: 0}, {Key: "p", Value: int32(1)}}, "p"},
		"DescendingUnique":    {bson.D{{Key: "_id", Value: 0}, {Key: "d", Value: int32(1)}}, "d"},
		"CompoundArrayInput":  {bson.D{{Key: "_id", Value: 0}, {Key: "a", Value: bson.D{{Key: "$literal", Value: bson.A{int32(1)}}}}, {Key: "b", Value: int32(1)}}, bson.A{"a", "b"}},
		"CompoundMissingPart": {bson.D{{Key: "_id", Value: 0}, {Key: "a", Value: int32(1)}}, bson.A{"a", "b"}},
	}
	for name, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeOnPaths_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				tgt := col.Database().Collection(col.Name() + "_tgt")
				if err := tgt.Drop(ctx); err != nil {
					return nil, err
				}
				if _, err := tgt.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(100)}, {Key: "k", Value: bson.D{{Key: "m", Value: int32(1)}}}, {Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}},
					bson.D{{Key: "_id", Value: int32(101)}, {Key: "k", Value: bson.A{bson.D{{Key: "m", Value: int32(2)}}}}},
					bson.D{{Key: "_id", Value: int32(102)}, {Key: "k", Value: bson.D{{Key: "m", Value: int32(9)}}}},
				}); err != nil {
					return nil, err
				}
				if _, err := tgt.Indexes().CreateMany(ctx, []mongo.IndexModel{
					{Keys: bson.D{{Key: "k.m", Value: 1}}, Options: options.Index().SetUnique(true).SetName("km")},
					{Keys: bson.D{{Key: "k.0.m", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true).SetName("k0m")},
					{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true).SetName("ab")},
					{Keys: bson.D{{Key: "c", Value: 1}}},
					{Keys: bson.D{{Key: "s", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
					{Keys: bson.D{{Key: "p", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{{Key: "p", Value: bson.D{{Key: "$gt", Value: 0}}}})},
					{Keys: bson.D{{Key: "d", Value: -1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
				}); err != nil {
					return nil, err
				}
				outcome := bson.M{"ok": true}
				cursor, err := col.Aggregate(ctx, bson.A{
					bson.D{{Key: "$project", Value: tc.project}},
					bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: tgt.Name()}, {Key: "on", Value: tc.on}}}},
				})
				if err == nil {
					err = cursor.All(ctx, &[]bson.M{})
				}
				if err != nil {
					outcome = errOutcome(err)
				}
				cur, err := tgt.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}}).SetSort(bson.D{{Key: "w", Value: 1}, {Key: "k.m", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var docs []bson.M
				if err = cur.All(ctx, &docs); err != nil {
					return nil, err
				}
				return bson.M{"outcome": outcome, "target": docs}, nil
			},
		})
	}
}
