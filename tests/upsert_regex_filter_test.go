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

// A regex in an upsert filter is a condition, not a value: the inserted
// document does not take the field from it, so a regex on _id gets a
// generated ObjectId.

// upsertedDocsWithoutID returns the collection's documents sorted by "tag",
// each with _id replaced by its BSON type, since a generated id differs
// between servers.
func upsertedDocsWithoutID(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "tag", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]bson.D, 0, len(docs))
	for _, d := range docs {
		row := bson.D{}
		for _, e := range d {
			if e.Key == "_id" {
				_, isOID := e.Value.(primitive.ObjectID)
				row = append(row, bson.E{Key: "_idIsObjectID", Value: isOID})
				continue
			}
			row = append(row, e)
		}
		out = append(out, row)
	}
	return out, nil
}

func regexFilter(field, pattern string) bson.D {
	return bson.D{{Key: field, Value: primitive.Regex{Pattern: pattern}}}
}

func TestUpsertRegexFilter(t *testing.T) {
	cases := []struct {
		name   string
		filter bson.D
		update interface{}
	}{
		{"Upsert_RegexID_Operator", regexFilter("_id", "^zz"), bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "a"}}}}},
		{"Upsert_RegexID_Replacement", regexFilter("_id", "^zz"), bson.D{{Key: "tag", Value: "a"}}},
		{"Upsert_RegexID_Pipeline", regexFilter("_id", "^zz"), mongo.Pipeline{bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "a"}}}}}},
		{"Upsert_RegexField_WithEquality", append(regexFilter("name", "^n"), bson.E{Key: "b", Value: int32(2)}), bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "a"}}}}},
		{"Upsert_RegexDottedField", regexFilter("s.t", "^n"), bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "a"}}}}},
	}
	for _, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var res *mongo.UpdateResult
				var err error
				switch u := c.update.(type) {
				case bson.D:
					if len(u) > 0 && u[0].Key[0] == '$' {
						res, err = col.UpdateOne(ctx, c.filter, u, options.Update().SetUpsert(true))
					} else {
						res, err = col.ReplaceOne(ctx, c.filter, u, options.Replace().SetUpsert(true))
					}
				default:
					res, err = col.UpdateOne(ctx, c.filter, u, options.Update().SetUpsert(true))
				}
				if err != nil {
					return nil, err
				}
				docs, err := upsertedDocsWithoutID(ctx, col)
				return bson.D{{Key: "matched", Value: res.MatchedCount}, {Key: "upserted", Value: res.UpsertedID != nil}, {Key: "docs", Value: docs}}, err
			},
		})
	}
}

func TestUpsertRegexFilter_FindAndModify(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Upsert_RegexID_FindAndModify",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res := col.FindOneAndUpdate(ctx, regexFilter("_id", "^zz"),
				bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "a"}}}},
				options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After))
			var doc bson.D
			if err := res.Decode(&doc); err != nil {
				return nil, err
			}
			return upsertedDocsWithoutID(ctx, col)
		},
	})
}
