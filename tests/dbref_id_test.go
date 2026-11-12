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

// mongod accepts a DBRef _id with $-prefixed fields inside $id on insert, but
// a write that produces a whole new document (a replacement or the document an
// upsert inserts) rejects them with DollarPrefixedFieldName.

var dbrefIDShapes = map[string]bson.D{
	"operator_shaped": {{Key: "$ref", Value: "x"}, {Key: "$id", Value: bson.D{{Key: "$gt", Value: int32(1)}}}},
	"nested_dbref":    {{Key: "a", Value: bson.D{{Key: "$ref", Value: "x"}, {Key: "$id", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}},
	"array":           {{Key: "$ref", Value: "x"}, {Key: "$id", Value: bson.A{bson.D{{Key: "$x", Value: int32(1)}}}}},
	"plain":           {{Key: "$ref", Value: "x"}, {Key: "$id", Value: int32(1)}, {Key: "meta", Value: int32(1)}},
}

func dbrefCollectionContents(ctx context.Context, col *mongo.Collection) ([]bson.M, error) {
	cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "v", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	err = cur.All(ctx, &docs)
	return docs, err
}

type dbrefWrite func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error)

func runDBRefIDWrite(t *testing.T, name string, seed bool, write dbrefWrite) {
	for shape, id := range dbrefIDShapes {
		harness.PairTest(t, harness.TestCase{
			Name:    "DBRef_id_" + name + "_" + shape,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if seed {
					if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "v", Value: int32(1)}}); err != nil {
						return nil, err
					}
				}
				result, err := write(ctx, col, id)
				docs, findErr := dbrefCollectionContents(ctx, col)
				if findErr != nil {
					return nil, findErr
				}
				return bson.A{result, docs}, err
			},
		})
	}
}

func byID(id bson.D) bson.D {
	return bson.D{{Key: "_id", Value: bson.D{{Key: "$eq", Value: id}}}}
}

func TestDBRef_id_update_set_existing(t *testing.T) {
	runDBRefIDWrite(t, "update_set_existing", true, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		res, err := col.UpdateOne(ctx, byID(id), bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(2)}}}})
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.ModifiedCount}, nil
	})
}

func TestDBRef_id_replace_one(t *testing.T) {
	runDBRefIDWrite(t, "replace_one", true, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		res, err := col.ReplaceOne(ctx, byID(id), bson.D{{Key: "v", Value: int32(3)}})
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.ModifiedCount}, nil
	})
}

func TestDBRef_id_find_one_and_replace(t *testing.T) {
	runDBRefIDWrite(t, "find_one_and_replace", true, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		var doc bson.M
		err := col.FindOneAndReplace(ctx, byID(id), bson.D{{Key: "v", Value: int32(3)}}).Decode(&doc)
		return doc, err
	})
}

func TestDBRef_id_find_one_and_update_set(t *testing.T) {
	runDBRefIDWrite(t, "find_one_and_update_set", true, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		var doc bson.M
		err := col.FindOneAndUpdate(ctx, byID(id), bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(2)}}}}).Decode(&doc)
		return doc, err
	})
}

func TestDBRef_id_upsert_set(t *testing.T) {
	runDBRefIDWrite(t, "upsert_set", false, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		res, err := col.UpdateOne(ctx, byID(id), bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(1)}}}}, options.Update().SetUpsert(true))
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.UpsertedCount}, nil
	})
}

func TestDBRef_id_upsert_set_id(t *testing.T) {
	runDBRefIDWrite(t, "upsert_set_id", false, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		update := bson.D{{Key: "$set", Value: bson.D{{Key: "_id", Value: id}, {Key: "v", Value: int32(1)}}}}
		res, err := col.UpdateOne(ctx, bson.D{{Key: "k", Value: int32(1)}}, update, options.Update().SetUpsert(true))
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.UpsertedCount}, nil
	})
}

func TestDBRef_id_replace_one_upsert(t *testing.T) {
	runDBRefIDWrite(t, "replace_one_upsert", false, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		replacement := bson.D{{Key: "_id", Value: id}, {Key: "v", Value: int32(1)}}
		res, err := col.ReplaceOne(ctx, bson.D{{Key: "k", Value: int32(1)}}, replacement, options.Replace().SetUpsert(true))
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.UpsertedCount}, nil
	})
}

func TestDBRef_id_pipeline_update_existing(t *testing.T) {
	runDBRefIDWrite(t, "pipeline_update_existing", true, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		pipeline := mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(2)}}}}}
		res, err := col.UpdateOne(ctx, byID(id), pipeline)
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.ModifiedCount}, nil
	})
}

func TestDBRef_id_pipeline_upsert(t *testing.T) {
	runDBRefIDWrite(t, "pipeline_upsert", false, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		pipeline := mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(1)}}}}}
		res, err := col.UpdateOne(ctx, byID(id), pipeline, options.Update().SetUpsert(true))
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.UpsertedCount}, nil
	})
}

func TestDBRef_id_bulk_write_replace(t *testing.T) {
	runDBRefIDWrite(t, "bulk_write_replace", true, func(ctx context.Context, col *mongo.Collection, id bson.D) (interface{}, error) {
		model := mongo.NewReplaceOneModel().SetFilter(byID(id)).SetReplacement(bson.D{{Key: "v", Value: int32(3)}})
		res, err := col.BulkWrite(ctx, []mongo.WriteModel{model})
		if err != nil {
			return nil, err
		}
		return bson.A{res.MatchedCount, res.ModifiedCount}, nil
	})
}

// mongod compares a DBRef in a filter literally, never as an operator
// expression: as a bare equality value and as an $in or $nin element.

func runDBRefIDFilter(t *testing.T, name string, filter func(id bson.D) bson.D) {
	for shape, id := range dbrefIDShapes {
		harness.PairTest(t, harness.TestCase{
			Name:    "DBRef_id_filter_" + name + "_" + shape,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				docs := []interface{}{
					bson.D{{Key: "_id", Value: id}, {Key: "v", Value: int32(1)}},
					bson.D{{Key: "_id", Value: int32(5)}, {Key: "v", Value: int32(2)}},
				}
				if _, err := col.InsertMany(ctx, docs); err != nil {
					return nil, err
				}
				cur, err := col.Find(ctx, filter(id), options.Find().SetSort(bson.D{{Key: "v", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var found []bson.M
				err = cur.All(ctx, &found)
				return found, err
			},
		})
	}
}

func TestDBRef_id_filter_bare_equality(t *testing.T) {
	runDBRefIDFilter(t, "bare_equality", func(id bson.D) bson.D { return bson.D{{Key: "_id", Value: id}} })
}

func TestDBRef_id_filter_in(t *testing.T) {
	runDBRefIDFilter(t, "in", func(id bson.D) bson.D {
		return bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{id, int32(7)}}}}}
	})
}

func TestDBRef_id_filter_nin(t *testing.T) {
	runDBRefIDFilter(t, "nin", func(id bson.D) bson.D {
		return bson.D{{Key: "_id", Value: bson.D{{Key: "$nin", Value: bson.A{id}}}}}
	})
}

// The operand of $not is always parsed as operators, so a DBRef there fails
// with BadValue instead of matching every other document.
func TestDBRef_id_filter_not(t *testing.T) {
	runDBRefIDFilter(t, "not", func(id bson.D) bson.D {
		return bson.D{{Key: "_id", Value: bson.D{{Key: "$not", Value: id}}}}
	})
}

func runDBRefFieldFilter(t *testing.T, name string, indexed bool, filter func(ref bson.D) bson.D) {
	for shape, ref := range dbrefIDShapes {
		harness.PairTest(t, harness.TestCase{
			Name:    "DBRef_field_filter_" + name + "_" + shape,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if indexed {
					if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "ref", Value: 1}}}); err != nil {
						return nil, err
					}
				}
				docs := []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "ref", Value: ref}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "ref", Value: int32(5)}},
				}
				if _, err := col.InsertMany(ctx, docs); err != nil {
					return nil, err
				}
				cur, err := col.Find(ctx, filter(ref), options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var found []bson.M
				err = cur.All(ctx, &found)
				return found, err
			},
		})
	}
}

func TestDBRef_field_filter(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		suffix := "scan"
		if indexed {
			suffix = "indexed"
		}
		runDBRefFieldFilter(t, "bare_equality_"+suffix, indexed, func(ref bson.D) bson.D { return bson.D{{Key: "ref", Value: ref}} })
		runDBRefFieldFilter(t, "in_"+suffix, indexed, func(ref bson.D) bson.D {
			return bson.D{{Key: "ref", Value: bson.D{{Key: "$in", Value: bson.A{ref, int32(7)}}}}}
		})
		runDBRefFieldFilter(t, "not_"+suffix, indexed, func(ref bson.D) bson.D {
			return bson.D{{Key: "ref", Value: bson.D{{Key: "$not", Value: ref}}}}
		})
		runDBRefFieldFilter(t, "elem_match_not_"+suffix, indexed, func(ref bson.D) bson.D {
			return bson.D{{Key: "ref", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$not", Value: ref}}}}}}
		})
	}
}
