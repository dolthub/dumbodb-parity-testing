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

// An upsert seeds the inserted document only from the filter's equalities:
// field values, $eq, and a one-element $in or $all, found at the top level, in
// $and and in an $or of one clause. A path bound twice fails with
// NotSingleValueField, $expr fails with QueryFeatureNotAllowed before any
// document is read, and a failing statement of an update batch fails alone.
// bulkWrite reports an upsert as {_id} and does not count it as matched
// (dolthub/dumbodb#262).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// docsWithoutID returns the collection's documents without _id, sorted by the
// given key, for upserts whose generated _ids differ between servers.
func docsWithoutID(ctx context.Context, col *mongo.Collection, sort bson.D) ([]bson.D, error) {
	cur, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}}).SetSort(sort))
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	err = cur.All(ctx, &docs)
	return docs, err
}

func TestUpsertSeeding(t *testing.T) {
	op := func(name string, v interface{}) bson.D {
		return bson.D{{Key: "a", Value: bson.D{{Key: name, Value: v}}}}
	}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"range", bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 1}, {Key: "$lte", Value: 1}}}}},
		{"in_one", op("$in", bson.A{5})},
		{"in_two", op("$in", bson.A{5, 6})},
		{"in_one_document", op("$in", bson.A{bson.D{{Key: "b", Value: 1}}})},
		{"in_one_array", op("$in", bson.A{bson.A{1, 2}})},
		{"in_one_null", op("$in", bson.A{nil})},
		{"in_one_regex", op("$in", bson.A{primitive.Regex{Pattern: "x"}})},
		{"all_one", op("$all", bson.A{1})},
		{"eq", op("$eq", 5)},
		{"eq_and_gt", bson.D{{Key: "a", Value: bson.D{{Key: "$eq", Value: 5}, {Key: "$gt", Value: 1}}}}},
		{"gt_and_eq", bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$eq", Value: 5}}}}},
		{"regex", bson.D{{Key: "a", Value: primitive.Regex{Pattern: "x"}}}},
		{"literal_document", bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: 1}, {Key: "c", Value: 2}}}}},
		{"and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "b", Value: bson.D{{Key: "$eq", Value: 2}}}}}}}},
		{"or_one", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: 1}}}}}},
		{"or_two", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "b", Value: 2}}}}}},
		{"nor", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "a", Value: 1}}}}}},
		{"dotted_siblings", bson.D{{Key: "a.b", Value: 1}, {Key: "a.c", Value: 2}}},
		{"matched_twice_and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "a", Value: 1}}}}}},
		{"matched_twice_ops", bson.D{{Key: "a", Value: bson.D{{Key: "$eq", Value: 1}, {Key: "$in", Value: bson.A{1}}}}}},
		{"prefix_matched", bson.D{{Key: "a", Value: 1}, {Key: "a.b", Value: 2}}},
		{"prefix_matched_reversed", bson.D{{Key: "a.b", Value: 2}, {Key: "a", Value: 1}}},
		{"expr", bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$a", 1}}}}}},
		{"expr_in_and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "$expr", Value: true}}}}}},
		{"expr_in_nor", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "$expr", Value: false}}}}}},
	}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}}
	pipeline := bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}}}
	writers := []struct {
		name  string
		write func(ctx context.Context, col *mongo.Collection, filter bson.D) error
	}{
		{"updateOne", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
			_, err := col.UpdateOne(ctx, filter, set, options.Update().SetUpsert(true))
			return err
		}},
		{"pipeline", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
			_, err := col.UpdateOne(ctx, filter, pipeline, options.Update().SetUpsert(true))
			return err
		}},
		{"findAndModify", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
			return col.FindOneAndUpdate(ctx, filter, set, options.FindOneAndUpdate().SetUpsert(true)).Err()
		}},
		{"bulkWrite", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(set).SetUpsert(true)})
			return err
		}},
		{"replaceOne", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
			_, err := col.ReplaceOne(ctx, filter, bson.D{{Key: "z", Value: 1}}, options.Replace().SetUpsert(true))
			return err
		}},
	}
	for _, f := range filters {
		for _, w := range writers {
			t.Run(f.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "UpsertSeeding_" + f.name + "_" + w.name,
					Support: harness.DumboDBFull,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						code := outcomeCode(w.write(ctx, col, f.filter))
						docs, err := docsWithoutID(ctx, col, bson.D{{Key: "_id", Value: 1}})
						return bson.D{{Key: "code", Value: code}, {Key: "docs", Value: docs}}, err
					},
				})
			})
		}
	}
}

// An upsert's filter errors only when no document matches, except $expr,
// which fails whenever upsert is set.
func TestUpsertSeedingWithMatch(t *testing.T) {
	for _, f := range []struct {
		name   string
		filter bson.D
	}{
		{"matched_twice", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "a", Value: bson.D{{Key: "$eq", Value: 1}}}}}}}},
		{"expr", bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$a", 1}}}}}},
	} {
		t.Run(f.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "UpsertSeedingWithMatch_" + f.name,
				Support: harness.DumboDBFull,
				Setup:   insertDocs(bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}}),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.UpdateOne(ctx, f.filter, bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 2}}}}, options.Update().SetUpsert(true))
						return err
					})
				},
			})
		})
	}
}

// updateBatchSummary runs an update command and returns its counts, its
// write errors' indexes and codes, the indexes of its upserts, and the
// documents left without their generated _ids.
func updateBatchSummary(ctx context.Context, col *mongo.Collection, cmd bson.D) (interface{}, error) {
	var res bson.M
	if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
		return nil, err
	}
	writeErrors := bson.A{}
	if we, ok := res["writeErrors"].(bson.A); ok {
		for _, e := range we {
			m := e.(bson.M)
			writeErrors = append(writeErrors, bson.A{m["index"], m["code"]})
		}
	}
	upserted := bson.A{}
	if up, ok := res["upserted"].(bson.A); ok {
		for _, e := range up {
			upserted = append(upserted, e.(bson.M)["index"])
		}
	}
	docs, err := docsWithoutID(ctx, col, bson.D{{Key: "y", Value: -1}, {Key: "z", Value: 1}})
	if err != nil {
		return nil, err
	}
	return bson.D{
		{Key: "n", Value: res["n"]},
		{Key: "nModified", Value: res["nModified"]},
		{Key: "writeErrors", Value: writeErrors},
		{Key: "upserted", Value: upserted},
		{Key: "docs", Value: docs},
	}, nil
}

func TestUpsertBatchStatementErrors(t *testing.T) {
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}
	stmt := func(q bson.D) bson.D {
		return bson.D{{Key: "q", Value: q}, {Key: "u", Value: set}, {Key: "upsert", Value: true}}
	}
	for _, bad := range []struct {
		name string
		q    bson.D
	}{
		{"matched_twice", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "a", Value: 2}}}}}},
		{"expr", bson.D{{Key: "$expr", Value: true}}},
		{"prefix_matched", bson.D{{Key: "a", Value: 1}, {Key: "a.b", Value: 2}}},
	} {
		for _, ordered := range []bool{true, false} {
			name := bad.name + "_ordered"
			if !ordered {
				name = bad.name + "_unordered"
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "UpsertBatchStatementErrors_" + name,
					Support: harness.DumboDBFull,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return updateBatchSummary(ctx, col, bson.D{
							{Key: "update", Value: col.Name()},
							{Key: "ordered", Value: ordered},
							{Key: "updates", Value: bson.A{
								stmt(bson.D{{Key: "y", Value: 1}}),
								stmt(bad.q),
								stmt(bson.D{{Key: "z", Value: 1}}),
							}},
						})
					},
				})
			})
		}
	}
}

// bulkWriteCursorEntries runs the bulkWrite command and returns its counts
// and per-operation entries.
func bulkWriteCursorEntries(ctx context.Context, col *mongo.Collection, ops bson.A) (interface{}, error) {
	ns := col.Database().Name() + "." + col.Name()
	var res bson.M
	err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
		{Key: "bulkWrite", Value: 1},
		{Key: "ops", Value: ops},
		{Key: "nsInfo", Value: bson.A{bson.D{{Key: "ns", Value: ns}}}},
	}).Decode(&res)
	if err != nil {
		return nil, err
	}
	cursor := res["cursor"].(bson.M)
	return bson.D{
		{Key: "firstBatch", Value: cursor["firstBatch"]},
		{Key: "nMatched", Value: res["nMatched"]},
		{Key: "nModified", Value: res["nModified"]},
		{Key: "nUpserted", Value: res["nUpserted"]},
		{Key: "nErrors", Value: res["nErrors"]},
	}, nil
}

func TestBulkWriteUpsertedEntry(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteUpsertedEntry",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return bulkWriteCursorEntries(ctx, col, bson.A{
				bson.D{{Key: "update", Value: 0}, {Key: "filter", Value: bson.D{{Key: "_id", Value: 77}}},
					{Key: "updateMods", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}}, {Key: "upsert", Value: true}},
				bson.D{{Key: "update", Value: 0}, {Key: "filter", Value: bson.D{{Key: "_id", Value: 77}}},
					{Key: "updateMods", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 2}}}}}, {Key: "upsert", Value: true}},
				bson.D{{Key: "update", Value: 0}, {Key: "filter", Value: bson.D{{Key: "_id", Value: 78}}},
					{Key: "updateMods", Value: bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}}}, {Key: "upsert", Value: true}},
			})
		},
	})
}
