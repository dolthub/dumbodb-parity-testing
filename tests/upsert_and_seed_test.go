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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// An upsert seeds the new document from the equalities of its query,
// including those inside $and and a one-clause $or, and refuses a path that
// two equalities set.

func TestUpsertAndSeeding(t *testing.T) {
	re := primitive.Regex{Pattern: "x"}
	filters := map[string]bson.D{
		"AndWithRegex":     {{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: re}}, bson.D{{Key: "b", Value: int32(1)}}}}},
		"NestedAnd":        {{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: int32(2)}}, bson.D{{Key: "b", Value: bson.D{{Key: "$eq", Value: int32(3)}}}}, bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "d", Value: int32(4)}}}}}}}},
		"AndBesideField":   {{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: re}}}}, {Key: "e", Value: int32(5)}},
		"OneClauseOr":      {{Key: "$or", Value: bson.A{bson.D{{Key: "g", Value: int32(7)}}}}},
		"TwoClauseOr":      {{Key: "$or", Value: bson.A{bson.D{{Key: "g", Value: int32(7)}}, bson.D{{Key: "h", Value: int32(8)}}}}},
		"Nor":              {{Key: "$nor", Value: bson.A{bson.D{{Key: "g", Value: int32(7)}}}}},
		"PathTwice":        {{Key: "$and", Value: bson.A{bson.D{{Key: "i", Value: int32(1)}}, bson.D{{Key: "i", Value: int32(2)}}}}},
		"PathAndSubpath":   {{Key: "$and", Value: bson.A{bson.D{{Key: "i", Value: int32(1)}}, bson.D{{Key: "i.j", Value: int32(2)}}}}},
		"FieldAndClause":   {{Key: "i", Value: int32(1)}, {Key: "$and", Value: bson.A{bson.D{{Key: "i", Value: int32(1)}}}}},
		"RangeAndEquality": {{Key: "i", Value: bson.D{{Key: "$gt", Value: int32(1)}}}, {Key: "$and", Value: bson.A{bson.D{{Key: "i", Value: int32(3)}}}}},
	}
	for name, filter := range filters {
		filter := filter
		harness.PairTest(t, harness.TestCase{
			Name:    "UpsertAndSeeding_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: int32(1)}}}},
					options.Update().SetUpsert(true)); err != nil {
					return nil, err
				}
				cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}}))
				if err != nil {
					return nil, err
				}
				var out []bson.D
				err = cursor.All(ctx, &out)
				return out, err
			},
		})
	}
}

// A replacement upsert takes only _id from its query, and refuses a sub-path
// of _id. A pipeline upsert seeds as a modifier upsert does.
func TestUpsertSeedingByUpdateKind(t *testing.T) {
	filters := map[string]bson.D{
		"PathTwice":      {{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: int32(1)}}, bson.D{{Key: "a", Value: int32(2)}}}}},
		"PathThenSub":    {{Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}}}, {Key: "a.b", Value: int32(1)}},
		"SubThenPath":    {{Key: "a.b", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}}}},
		"IDTwice":        {{Key: "$and", Value: bson.A{bson.D{{Key: "_id", Value: int32(7)}}, bson.D{{Key: "_id", Value: int32(8)}}}}},
		"IDThenIDSub":    {{Key: "_id", Value: bson.D{{Key: "x", Value: int32(1)}}}, {Key: "_id.x", Value: int32(1)}},
		"IDSubs":         {{Key: "_id.x", Value: int32(1)}, {Key: "_id.y", Value: int32(2)}},
		"IDAndOther":     {{Key: "_id", Value: int32(5)}, {Key: "c", Value: int32(1)}},
		"NestedAndPaths": {{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: int32(2)}}, bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "d.e", Value: int32(4)}}}}}}}},
	}
	updates := map[string]interface{}{
		"Replacement":      bson.D{{Key: "r", Value: int32(1)}},
		"EmptyReplacement": bson.D{},
		"Pipeline":         mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "p", Value: int32(1)}}}}},
	}
	for fname, filter := range filters {
		for uname, update := range updates {
			filter, update := filter, update
			harness.PairTest(t, harness.TestCase{
				Name:    "UpsertSeedingByUpdateKind_" + uname + "_" + fname,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var err error
					if d, ok := update.(bson.D); ok {
						_, err = col.ReplaceOne(ctx, filter, d, options.Replace().SetUpsert(true))
					} else {
						_, err = col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
					}
					if err != nil {
						return nil, err
					}
					cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}, {Key: "a", Value: 1}, {Key: "c", Value: 1}, {Key: "d", Value: 1}, {Key: "r", Value: 1}, {Key: "p", Value: 1}}))
					if err != nil {
						return nil, err
					}
					var out []bson.D
					err = cursor.All(ctx, &out)
					return out, err
				},
			})
		}
	}
}
