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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// dupKeyCode extracts the duplicate-key error code from the shapes the driver
// returns across insert / bulk write / command paths. Returns 0 when err is
// nil and -1 for an unrecognized error shape, so a parity test can assert the
// exact code (11000) rather than only that "an error happened".
func dupKeyCode(err error) int {
	if err == nil {
		return 0
	}
	var we mongo.WriteException
	if errors.As(err, &we) && len(we.WriteErrors) > 0 {
		return we.WriteErrors[0].Code
	}
	var bwe mongo.BulkWriteException
	if errors.As(err, &bwe) && len(bwe.WriteErrors) > 0 {
		return bwe.WriteErrors[0].Code
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		return int(ce.Code)
	}
	return -1
}

// uniqIDs returns the collection's _id values (as int32) sorted ascending.
func uniqIDs(ctx context.Context, col *mongo.Collection) (bson.A, error) {
	cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(docs))
	for _, d := range docs {
		if v, ok := d["_id"].(int32); ok {
			ids = append(ids, int(v))
		}
	}
	sort.Ints(ids)
	out := bson.A{}
	for _, v := range ids {
		out = append(out, int32(v))
	}
	return out, nil
}

func uniqIndexSetup(field string, docs ...interface{}) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if len(docs) > 0 {
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: field, Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		return err
	}
}

// TestIndex_Unique_BuildOverExistingDuplicates: creating a unique index over a
// collection that already contains duplicate values must fail (E11000).
// XFail: dumbodb does not validate uniqueness against existing data at build
// time (workspace-k34); MongoDB rejects.
func TestIndex_Unique_BuildOverExistingDuplicates(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_BuildOverExistingDuplicates",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: "dup"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: "dup"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "f", Value: 1}},
				Options: options.Index().SetUnique(true),
			})
			return bson.D{{Key: "created", Value: err == nil}, {Key: "code", Value: dupKeyCode(err)}}, nil
		},
	})
}

// TestIndex_Unique_BulkWrite_Ordered: an ordered bulk write stops at the first
// duplicate; only inserts before it survive.
func TestIndex_Unique_BulkWrite_Ordered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_BulkWrite_Ordered",
		Support: harness.DumboDBFull,
		Setup:   uniqIndexSetup("f", bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: "a"}}),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: "b"}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: int32(3)}, {Key: "f", Value: "a"}}), // dup
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: int32(4)}, {Key: "f", Value: "c"}}),
			}, options.BulkWrite().SetOrdered(true))
			ids, idErr := uniqIDs(ctx, col)
			if idErr != nil {
				return nil, idErr
			}
			return bson.D{{Key: "code", Value: dupKeyCode(err)}, {Key: "present", Value: ids}}, nil
		},
	})
}

// TestIndex_Unique_BulkWrite_Unordered: an unordered bulk write applies every
// non-colliding op despite the duplicate.
func TestIndex_Unique_BulkWrite_Unordered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_BulkWrite_Unordered",
		Support: harness.DumboDBFull,
		Setup:   uniqIndexSetup("f", bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: "a"}}),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: "b"}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: int32(3)}, {Key: "f", Value: "a"}}), // dup
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: int32(4)}, {Key: "f", Value: "c"}}),
			}, options.BulkWrite().SetOrdered(false))
			ids, idErr := uniqIDs(ctx, col)
			if idErr != nil {
				return nil, idErr
			}
			return bson.D{{Key: "code", Value: dupKeyCode(err)}, {Key: "present", Value: ids}}, nil
		},
	})
}

// TestIndex_Unique_UpsertCollision: an upsert whose inserted document collides
// on the unique field is rejected.
func TestIndex_Unique_UpsertCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_UpsertCollision",
		Support: harness.DumboDBFull,
		Setup:   uniqIndexSetup("f", bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: "a"}}),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: int32(99)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "a"}}}},
				options.Update().SetUpsert(true))
			ids, idErr := uniqIDs(ctx, col)
			if idErr != nil {
				return nil, idErr
			}
			return bson.D{{Key: "code", Value: dupKeyCode(err)}, {Key: "present", Value: ids}}, nil
		},
	})
}

// TestIndex_Unique_Multikey: under a unique index on an array field, two docs
// sharing an element collide, while a single doc with a repeated element is
// allowed (the element set is deduplicated within a document).
func TestIndex_Unique_Multikey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_Multikey",
		Support: harness.DumboDBFull,
		Setup:   uniqIndexSetup("tags", bson.D{{Key: "_id", Value: int32(1)}, {Key: "tags", Value: bson.A{"a", "b"}}}),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Shares element "b" with doc 1 -> collision.
			_, crossErr := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(2)}, {Key: "tags", Value: bson.A{"b", "c"}}})
			// Repeated element within one doc -> allowed.
			_, withinErr := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(3)}, {Key: "tags", Value: bson.A{"x", "x"}}})
			ids, idErr := uniqIDs(ctx, col)
			if idErr != nil {
				return nil, idErr
			}
			return bson.D{
				{Key: "crossCode", Value: dupKeyCode(crossErr)},
				{Key: "withinErrored", Value: withinErr != nil},
				{Key: "present", Value: ids},
			}, nil
		},
	})
}

// TestIndex_Unique_FindAndModifyCollision: a findAndModify that sets the unique
// field to an existing value is rejected.
func TestIndex_Unique_FindAndModifyCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_FindAndModifyCollision",
		Support: harness.DumboDBFull,
		Setup: uniqIndexSetup("f",
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: "a"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: "b"}}),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: int32(1)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "b"}}}}).Err()
			return bson.D{{Key: "code", Value: dupKeyCode(err)}}, nil
		},
	})
}

// TestIndex_Unique_IncCollision: a numeric $inc update that drives the unique
// field onto another document's value is rejected.
func TestIndex_Unique_IncCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Unique_IncCollision",
		Support: harness.DumboDBFull,
		Setup: uniqIndexSetup("n",
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "n", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "n", Value: int32(2)}}),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: int32(1)}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}})
			return bson.D{{Key: "code", Value: dupKeyCode(err)}}, nil
		},
	})
}
