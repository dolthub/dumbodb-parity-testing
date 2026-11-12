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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// When the filter binds a compound index's leading fields by equality, the
// scan already yields the next field in order: MongoDB sorts on it without a
// SORT stage and a limit stops the scan early.

const compoundSortDocs = 1000

func compoundSortSetup(keys bson.D) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, compoundSortDocs)
		for i := 0; i < compoundSortDocs; i++ {
			docs = append(docs, bson.D{
				{Key: "_id", Value: int32(i)},
				{Key: "grp", Value: int32(i % 4)},
				{Key: "k", Value: int32((i * 7) % compoundSortDocs)},
			})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, keys)
	}
}

func TestIndex_CompoundEqualityPrefixSort(t *testing.T) {
	ascIdx := bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}}
	descIdx := bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: -1}}
	eq := bson.D{{Key: "grp", Value: int32(2)}}
	asc := bson.D{{Key: "k", Value: 1}}
	desc := bson.D{{Key: "k", Value: -1}}
	for _, tc := range []struct {
		name    string
		keys    bson.D
		filter  bson.D
		sort    bson.D
		limit   int64
		support harness.DumboDBSupport
	}{
		{"AscLimit", ascIdx, eq, asc, 5, harness.DumboDBFull},
		{"DescLimit", ascIdx, eq, desc, 5, harness.DumboDBFull},
		{"DescIndexDescLimit", descIdx, eq, desc, 5, harness.DumboDBFull},
		{"DescIndexAscLimit", descIdx, eq, asc, 5, harness.DumboDBFull},
		{"AscNoLimit", ascIdx, eq, asc, 0, harness.DumboDBFull},
		{"SortFieldRange", bson.D{{Key: "k", Value: 1}}, bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(100)}, {Key: "$lt", Value: int32(400)}}}}, desc, 5, harness.DumboDBFull},
		{"SortFieldRangeAsc", bson.D{{Key: "k", Value: 1}}, bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(100)}, {Key: "$lt", Value: int32(400)}}}}, asc, 5, harness.DumboDBFull},
		{"SortFieldGteAsc", bson.D{{Key: "k", Value: 1}}, bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(990)}}}}, asc, 5, harness.DumboDBFull},
		{"SortFieldGteDesc", bson.D{{Key: "k", Value: 1}}, bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(10)}}}}, desc, 5, harness.DumboDBFull},
		{"SortFieldLtDesc", bson.D{{Key: "k", Value: 1}}, bson.D{{Key: "k", Value: bson.D{{Key: "$lt", Value: int32(7)}}}}, desc, 10, harness.DumboDBFull},
		{"SortFieldIn", bson.D{{Key: "k", Value: 1}}, bson.D{{Key: "k", Value: bson.D{{Key: "$in", Value: bson.A{int32(500), int32(3), int32(70), int32(9)}}}}}, asc, 2, harness.DumboDBFull},
		// The index keys are sorted before fetching.
		{"RangePrefixSorts", ascIdx, bson.D{{Key: "grp", Value: bson.D{{Key: "$gte", Value: int32(2)}}}}, asc, 5, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_CompoundEqualityPrefixSort_" + tc.name,
				Support: tc.support,
				Setup:   compoundSortSetup(tc.keys),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Find().SetSort(tc.sort).SetProjection(bson.D{{Key: "_id", Value: 1}})
					cmd := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: tc.filter}, {Key: "sort", Value: tc.sort}}
					if tc.limit > 0 {
						opts.SetLimit(tc.limit)
						cmd = append(cmd, bson.E{Key: "limit", Value: int32(tc.limit)})
					}
					cur, err := col.Find(ctx, tc.filter, opts)
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					if err := cur.All(ctx, &docs); err != nil {
						return nil, err
					}
					explain, err := explainRunExplain(ctx, col, cmd, "executionStats")
					if err != nil {
						return nil, err
					}
					critical := explainExtractCritical(explain)
					return bson.D{
						{Key: "n", Value: len(docs)},
						{Key: "first", Value: firstIDs(docs, 5)},
						{Key: "winningPlan", Value: critical.Map()["winningPlan"]},
						{Key: "nReturned", Value: lookupStat(critical, "nReturned")},
						{Key: "docsExamined", Value: lookupStat(critical, "totalDocsExamined")},
					}, nil
				},
			})
		})
	}
}

func TestIndex_CompoundEqualityPrefixSortMixed(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		values := []interface{}{int32(5), "b", bson.D{{Key: "x", Value: int32(2)}}, bson.D{{Key: "x", Value: int32(1)}}, nil, 2.5, "a", int64(9), true}
		docs := make([]interface{}, 0, 200)
		for i := 0; i < 200; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "grp", Value: int32(i % 2)}, {Key: "odd", Value: i%3 == 0}, {Key: "k", Value: values[i%len(values)]}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}})
	}
	for _, tc := range []struct {
		name   string
		filter bson.D
		sort   bson.D
		limit  int64
	}{
		{"AscLimit", bson.D{{Key: "grp", Value: int32(1)}}, bson.D{{Key: "k", Value: 1}}, 30},
		{"DescLimit", bson.D{{Key: "grp", Value: int32(0)}}, bson.D{{Key: "k", Value: -1}}, 60},
		{"AscLimitSmall", bson.D{{Key: "grp", Value: int32(0)}}, bson.D{{Key: "k", Value: 1}}, 25},
		{"ExtraPredicateDesc", bson.D{{Key: "grp", Value: int32(1)}, {Key: "odd", Value: true}}, bson.D{{Key: "k", Value: -1}}, 15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_CompoundEqualityPrefixSortMixed_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Find().SetSort(tc.sort).SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: 1}})
					if tc.limit > 0 {
						opts.SetLimit(tc.limit)
					}
					cur, err := col.Find(ctx, tc.filter, opts)
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					return docs, cur.All(ctx, &docs)
				},
			})
		})
	}
}

// TestIndex_KeyFieldLimit: an index key pattern holds at most 32 fields.
func TestIndex_KeyFieldLimit(t *testing.T) {
	for _, n := range []int{32, 33} {
		t.Run(fmt.Sprintf("fields_%d", n), func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("Index_KeyFieldLimit_%d", n),
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					key := bson.D{}
					for i := range n {
						key = append(key, bson.E{Key: fmt.Sprintf("f%d", i), Value: 1})
					}
					err := col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: col.Name()},
						{Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: key}, {Key: "name", Value: "wide"}}}}}).Err()
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: ce.Message}}, nil
					}
					return bson.D{{Key: "ok", Value: err == nil}}, nil
				},
			})
		})
	}
}

// TestIndex_CreateConflicts: an index request beside an existing index on the
// same key is identical, a new index, or refused as mongod refuses it: a name
// in use with other options, or an equivalent index under another name.
// Indexes on one key differ by unique, sparse, partialFilterExpression and
// collation; hidden alone does not make another index.
func TestIndex_CreateConflicts(t *testing.T) {
	spec := func(name string, opts ...bson.E) bson.D {
		return append(bson.D{{Key: "key", Value: bson.D{{Key: "b", Value: 1}}}, {Key: "name", Value: name}}, opts...)
	}
	sparse := bson.E{Key: "sparse", Value: true}
	cases := []struct {
		name string
		spec bson.D
	}{
		{"same_name_sparse", spec("b1", sparse)},
		{"same_name_unique", spec("b1", bson.E{Key: "unique", Value: true})},
		{"same_name_partial", spec("b1", bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "b", Value: 1}}})},
		{"same_name_hidden", spec("b1", bson.E{Key: "hidden", Value: true})},
		{"same_name_other_key", bson.D{{Key: "key", Value: bson.D{{Key: "c", Value: 1}}}, {Key: "name", Value: "b1"}}},
		{"other_name", spec("b2")},
		{"other_name_sparse", spec("b2", sparse)},
		{"other_name_unique", spec("b2", bson.E{Key: "unique", Value: true})},
		{"other_name_hidden", spec("b2", bson.E{Key: "hidden", Value: true})},
		{"identical", spec("b1")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_CreateConflicts_" + tc.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}}); err != nil {
						return err
					}
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "b", Value: 1}}, Options: options.Index().SetName("b1")})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.M
					err := col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: col.Name()}, {Key: "indexes", Value: bson.A{tc.spec}}}).Decode(&res)
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: ce.Message}}, nil
					}
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "note", Value: res["note"]}, {Key: "after", Value: res["numIndexesAfter"]}}, nil
				},
			})
		})
	}
	t.Run("wide_named_id", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Index_CreateConflicts_wide_named_id",
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				key := bson.D{}
				for i := range 33 {
					key = append(key, bson.E{Key: fmt.Sprintf("f%d", i), Value: 1})
				}
				err := col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: col.Name()}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: key}, {Key: "name", Value: "_id_"}}}}}).Err()
				var ce mongo.CommandError
				if errors.As(err, &ce) {
					return bson.D{{Key: "code", Value: ce.Code}}, nil
				}
				return bson.D{{Key: "ok", Value: err == nil}}, nil
			},
		})
	})
	t.Run("wide_without_name", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Index_CreateConflicts_wide_without_name",
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				key := bson.D{}
				for i := range 33 {
					key = append(key, bson.E{Key: fmt.Sprintf("f%d", i), Value: 1})
				}
				err := col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: col.Name()}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: key}}}}}).Err()
				var ce mongo.CommandError
				if errors.As(err, &ce) {
					return bson.D{{Key: "code", Value: ce.Code}}, nil
				}
				return bson.D{{Key: "ok", Value: err == nil}}, nil
			},
		})
	})
}

// TestIndex_SixtyFourIndexLimit: a collection holds at most 64 indexes, its
// _id index included; re-requesting existing indexes stays within it.
func TestIndex_SixtyFourIndexLimit(t *testing.T) {
	specs := func(from, to int) bson.A {
		out := bson.A{}
		for i := from; i < to; i++ {
			out = append(out, bson.D{{Key: "key", Value: bson.D{{Key: fmt.Sprintf("f%d", i), Value: 1}}}, {Key: "name", Value: fmt.Sprintf("i%d", i)}})
		}
		return out
	}
	cases := []struct {
		name          string
		first, second bson.A
	}{
		{"one_past", specs(0, 63), specs(63, 64)},
		{"existing_plus_one", specs(0, 63), specs(0, 64)},
		{"existing_again", specs(0, 63), specs(0, 63)},
		{"many_at_once", specs(0, 10), specs(0, 65)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_SixtyFourIndexLimit_" + tc.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}}); err != nil {
						return err
					}
					return col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: col.Name()}, {Key: "indexes", Value: tc.first}}).Err()
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.M
					err := col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: col.Name()}, {Key: "indexes", Value: tc.second}}).Decode(&res)
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						return bson.D{{Key: "code", Value: ce.Code}}, nil
					}
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "after", Value: res["numIndexesAfter"]}}, nil
				},
			})
		})
	}
}
