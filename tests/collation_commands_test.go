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

var caseInsensitive = &options.Collation{Locale: "en", Strength: 2}

// TestCollationArrayEquality: equality, $in, $ne, $nin and $all compare the
// elements of an array field under the collation.
func TestCollationArrayEquality(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: bson.A{"A", "b"}}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: "A"}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: bson.A{"x"}}},
		})
		return err
	}
	filters := map[string]bson.D{
		"eq":  {{Key: "t", Value: "a"}},
		"in":  {{Key: "t", Value: bson.D{{Key: "$in", Value: bson.A{"a"}}}}},
		"ne":  {{Key: "t", Value: bson.D{{Key: "$ne", Value: "a"}}}},
		"nin": {{Key: "t", Value: bson.D{{Key: "$nin", Value: bson.A{"a"}}}}},
		"all": {{Key: "t", Value: bson.D{{Key: "$all", Value: bson.A{"a", "B"}}}}},
	}
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "CollationArrayEquality_find_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, filter, options.Find().SetCollation(caseInsensitive).SetSort(bson.D{{Key: "_id", Value: 1}}))
				return drainCursor(ctx, cursor, err)
			},
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "CollationArrayEquality_count_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return col.CountDocuments(ctx, filter, options.Count().SetCollation(caseInsensitive))
			},
		})
	}
}

// TestCollationCommandExpressions: $expr in find, count, distinct and delete
// filters, find projection expressions, and update and findAndModify
// pipelines compare strings under the command's or the collection's
// collation.
func TestCollationCommandExpressions(t *testing.T) {
	seed := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "A"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "b"}},
	}
	expr := bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$s", "a"}}}}}
	projection := bson.D{{Key: "e", Value: bson.D{{Key: "$eq", Value: bson.A{"$s", "a"}}}}}
	pipeline := bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "e", Value: bson.D{{Key: "$eq", Value: bson.A{"$s", "a"}}}}}}}}
	all := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		return drainCursor(ctx, cursor, err)
	}
	type run func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error)
	runs := map[string]run{
		"find": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			cursor, err := col.Find(ctx, expr, options.Find().SetCollation(c))
			return drainCursor(ctx, cursor, err)
		},
		"count": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			return col.CountDocuments(ctx, expr, options.Count().SetCollation(c))
		},
		"distinct": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			return col.Distinct(ctx, "s", expr, options.Distinct().SetCollation(c))
		},
		"projection": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().SetCollation(c).SetProjection(projection).SetSort(bson.D{{Key: "_id", Value: 1}}))
			return drainCursor(ctx, cursor, err)
		},
		"delete": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			if _, err := col.DeleteMany(ctx, expr, options.Delete().SetCollation(c)); err != nil {
				return errOutcome(err), nil
			}
			return all(ctx, col)
		},
		"update_pipeline": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			if _, err := col.UpdateMany(ctx, bson.D{}, pipeline, options.Update().SetCollation(c)); err != nil {
				return errOutcome(err), nil
			}
			return all(ctx, col)
		},
		"find_and_modify_pipeline": func(ctx context.Context, col *mongo.Collection, c *options.Collation) (interface{}, error) {
			res := col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: int32(1)}}, pipeline,
				options.FindOneAndUpdate().SetCollation(c).SetReturnDocument(options.After))
			var doc bson.M
			if err := res.Decode(&doc); err != nil {
				return errOutcome(err), nil
			}
			return doc, nil
		},
	}
	for name, r := range runs {
		for _, source := range []string{"command", "collection"} {
			collated := source == "collection"
			harness.PairTest(t, harness.TestCase{
				Name:    "CollationCommandExpressions_" + name + "_" + source,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if err := col.Drop(ctx); err != nil {
						return err
					}
					opts := options.CreateCollection()
					if collated {
						opts.SetCollation(caseInsensitive)
					}
					if err := col.Database().CreateCollection(ctx, col.Name(), opts); err != nil {
						return err
					}
					_, err := col.InsertMany(ctx, seed)
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var c *options.Collation
					if !collated {
						c = caseInsensitive
					}
					return r(ctx, col, c)
				},
			})
		}
	}
}

// TestCollationForeignView: $lookup, $graphLookup and $unionWith may read a
// view only under the view's own collation (167).
func TestCollationForeignView(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		db := col.Database()
		if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "a"}}); err != nil {
			return err
		}
		other := db.Collection(col.Name() + "_o")
		if err := other.Drop(ctx); err != nil {
			return err
		}
		if _, err := other.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: "A"}}); err != nil {
			return err
		}
		for _, v := range []string{"_vci", "_vsimple"} {
			_ = db.Collection(col.Name() + v).Drop(ctx)
		}
		if err := db.CreateView(ctx, col.Name()+"_vci", other.Name(), bson.A{}, options.CreateView().SetCollation(caseInsensitive)); err != nil {
			return err
		}
		return db.CreateView(ctx, col.Name()+"_vsimple", other.Name(), bson.A{})
	}
	cases := map[string]struct {
		view      string
		collation *options.Collation
		stage     func(view string) bson.D
	}{
		"lookup_simple_over_ci": {"_vci", nil, lookupStage},
		"lookup_ci_over_ci":     {"_vci", caseInsensitive, lookupStage},
		"lookup_ci_over_simple": {"_vsimple", caseInsensitive, lookupStage},
		"lookup_pipeline_over_ci": {"_vci", nil, func(v string) bson.D {
			return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: v}, {Key: "pipeline", Value: bson.A{}}, {Key: "as", Value: "j"}}}}
		}},
		"graph_lookup_over_ci": {"_vci", nil, func(v string) bson.D {
			return bson.D{{Key: "$graphLookup", Value: bson.D{{Key: "from", Value: v}, {Key: "startWith", Value: "$s"},
				{Key: "connectFromField", Value: "k"}, {Key: "connectToField", Value: "k"}, {Key: "as", Value: "g"}}}}
		}},
		"union_with_over_ci":       {"_vci", nil, func(v string) bson.D { return bson.D{{Key: "$unionWith", Value: v}} }},
		"nested_lookup_over_ci":    {"_vci", nil, nestedLookupStage},
		"nested_lookup_ci_over_ci": {"_vci", caseInsensitive, nestedLookupStage},
		"nested_union_with_over_ci": {"_vci", nil, func(v string) bson.D {
			return bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: v}, {Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$unionWith", Value: v}},
			}}}}}
		}},
	}
	for name, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "CollationForeignView_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				opts := options.Aggregate()
				if tc.collation != nil {
					opts.SetCollation(tc.collation)
				}
				cursor, err := col.Aggregate(ctx, bson.A{tc.stage(col.Name() + tc.view)}, opts)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}

// nestedLookupStage joins each document to the view's documents, each of
// which joins the view again.
func nestedLookupStage(view string) bson.D {
	return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: view}, {Key: "pipeline", Value: bson.A{lookupStage(view)}}, {Key: "as", Value: "j"}}}}
}

func lookupStage(view string) bson.D {
	return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: view}, {Key: "localField", Value: "s"}, {Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"}}}}
}
