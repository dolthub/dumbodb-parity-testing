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

func duplicateFilterKeysSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "y", Value: bson.A{1, 2}}, {Key: "a", Value: bson.A{bson.D{{Key: "b", Value: 1}}, bson.D{{Key: "b", Value: 2}}}}},
		bson.D{{Key: "_id", Value: 2}, {Key: "y", Value: 1}, {Key: "a", Value: bson.A{bson.D{{Key: "b", Value: bson.A{1, 2}}}}}},
		bson.D{{Key: "_id", Value: 3}, {Key: "y", Value: 2}, {Key: "z", Value: bson.D{{Key: "a", Value: 1}}}},
	})
	return err
}

// TestDuplicateFilterKeys: a filter repeating a key, at the top level, in a
// logical operator's clause, in an $elemMatch query or as a repeated
// operator, is an implicit $and in find, count, distinct, aggregate $match
// (also in $facet and $lookup), update, findAndModify and delete, whether the
// statements travel in the command body or in a document sequence.
func TestDuplicateFilterKeys(t *testing.T) {
	filters := map[string]bson.D{
		"equalities":  {{Key: "y", Value: 1}, {Key: "y", Value: 2}},
		"ranges":      {{Key: "y", Value: bson.D{{Key: "$gte", Value: 1}}}, {Key: "y", Value: bson.D{{Key: "$lte", Value: 1}}}},
		"id":          {{Key: "_id", Value: 1}, {Key: "_id", Value: 1}},
		"dotted":      {{Key: "z.a", Value: 1}, {Key: "z.a", Value: 1}},
		"and":         {{Key: "$and", Value: bson.A{bson.D{{Key: "y", Value: 1}}}}, {Key: "$and", Value: bson.A{bson.D{{Key: "y", Value: 2}}}}},
		"in_or":       {{Key: "$or", Value: bson.A{bson.D{{Key: "y", Value: 1}, {Key: "y", Value: 2}}, bson.D{{Key: "_id", Value: 3}}}}},
		"elem_match":  {{Key: "a", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "b", Value: 1}, {Key: "b", Value: 2}}}}}},
		"operators":   {{Key: "y", Value: bson.D{{Key: "$gt", Value: 0}, {Key: "$gt", Value: 1}}}},
		"eq_twice":    {{Key: "y", Value: bson.D{{Key: "$eq", Value: 1}, {Key: "$eq", Value: 2}}}},
		"mixed":       {{Key: "y", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: 1}}}}}, {Key: "y", Value: 2}},
		"expr":        {{Key: "$expr", Value: true}, {Key: "$expr", Value: false}},
		"json_schema": {{Key: "$jsonSchema", Value: bson.D{{Key: "required", Value: bson.A{"y"}}}}, {Key: "$jsonSchema", Value: bson.D{{Key: "required", Value: bson.A{"z"}}}}},
		"comment":     {{Key: "$comment", Value: "a"}, {Key: "$comment", Value: "b"}, {Key: "y", Value: 2}},
	}
	byID := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "DuplicateFilterKeys_" + name,
			Support: harness.DumboDBFull,
			Setup:   duplicateFilterKeysSetup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				out := bson.D{}
				add := func(key string, v interface{}, err error) {
					if err != nil {
						v = errOutcome(err)
					}
					out = append(out, bson.E{Key: key, Value: v})
				}
				ids, err := findIDs(ctx, col, filter)
				add("find", ids, err)
				n, err := col.CountDocuments(ctx, filter)
				add("count", n, err)
				distinct, err := col.Distinct(ctx, "_id", filter)
				add("distinct", distinct, err)
				cur, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$match", Value: filter}}, byID})
				match, err := drainCursor(ctx, cur, err)
				add("match", match, err)
				cur, err = col.Aggregate(ctx, bson.A{
					bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{bson.D{{Key: "$match", Value: filter}}, byID}}}}},
				})
				facet, err := drainCursor(ctx, cur, err)
				add("facet", facet, err)
				cur, err = col.Aggregate(ctx, bson.A{
					bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: 1}}}},
					bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "as", Value: "l"},
						{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: filter}}, byID, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}}}}}}},
					bson.D{{Key: "$project", Value: bson.D{{Key: "l", Value: 1}}}},
				})
				lookup, err := drainCursor(ctx, cur, err)
				add("lookup", lookup, err)
				res, err := runCommandDoc(ctx, col, bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{
					bson.D{{Key: "q", Value: filter}, {Key: "u", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "body", Value: 1}}}}}, {Key: "multi", Value: true}},
				}}})
				add("update_body", pickFields(asD(res), "n", "nModified", "writeErrors"), err)
				upd, err := col.UpdateMany(ctx, filter, bson.D{{Key: "$inc", Value: bson.D{{Key: "seq", Value: 1}}}})
				if err == nil {
					add("update_sequence", bson.A{upd.MatchedCount, upd.ModifiedCount}, nil)
				} else {
					add("update_sequence", nil, err)
				}
				var fam bson.M
				err = col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "fam", Value: 1}}}},
					options.FindOneAndUpdate().SetSort(bson.D{{Key: "_id", Value: 1}})).Decode(&fam)
				if err == mongo.ErrNoDocuments {
					err = nil
				}
				add("find_and_modify", fam["_id"], err)
				cur, err = col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
				after, err := drainCursor(ctx, cur, err)
				add("after_updates", after, err)
				res, err = runCommandDoc(ctx, col, bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{
					bson.D{{Key: "q", Value: filter}, {Key: "limit", Value: 1}},
				}}})
				add("delete_body", pickFields(asD(res), "n", "writeErrors"), err)
				del, err := col.DeleteMany(ctx, filter)
				if err == nil {
					add("delete_sequence", del.DeletedCount, nil)
				} else {
					add("delete_sequence", nil, err)
				}
				return out, nil
			},
		})
	}
}

// TestDuplicateFilterKeysUpsert: an upsert whose filter repeats an equality
// on a path fails with NotSingleValueField; a repeated key whose other
// condition is no equality seeds the equality alone.
func TestDuplicateFilterKeysUpsert(t *testing.T) {
	filters := map[string]bson.D{
		"two_equalities": {{Key: "w", Value: 1}, {Key: "w", Value: 2}},
		"same_equality":  {{Key: "w", Value: 1}, {Key: "w", Value: 1}},
		"with_range":     {{Key: "w", Value: 1}, {Key: "w", Value: bson.D{{Key: "$gt", Value: 0}}}},
		"two_eq_ops":     {{Key: "w", Value: bson.D{{Key: "$eq", Value: 5}, {Key: "$eq", Value: 6}}}},
	}
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "DuplicateFilterKeysUpsert_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				out := bson.D{}
				_, err := col.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: 1}}}}, options.Update().SetUpsert(true))
				out = append(out, bson.E{Key: "update", Value: errOutcome(err)})
				err = col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: 2}}}},
					options.FindOneAndUpdate().SetUpsert(true)).Err()
				if err == mongo.ErrNoDocuments {
					err = nil
				}
				out = append(out, bson.E{Key: "find_and_modify", Value: errOutcome(err)})
				cur, err := col.Find(ctx, bson.D{})
				if err != nil {
					return nil, err
				}
				var docs []bson.M
				if err := cur.All(ctx, &docs); err != nil {
					return nil, err
				}
				for _, d := range docs {
					if _, ok := d["_id"].(primitive.ObjectID); ok {
						d["_id"] = "generated"
					}
				}
				return append(out, bson.E{Key: "docs", Value: docs}), nil
			},
		})
	}
}

func asD(v interface{}) bson.D {
	d, _ := v.(bson.D)
	return d
}
