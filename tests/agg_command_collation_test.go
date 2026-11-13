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

// TestAggCommandCollation: an aggregate's collation governs every string
// comparison in its pipeline: expressions, $sortByCount, $bucket,
// $bucketAuto, window functions, the accumulators that order or dedupe, both
// $lookup forms, $graphLookup, and the $facet, $lookup and $unionWith
// sub-pipelines, whether the collation comes from the command or from the
// collection's default.
func TestAggCommandCollation(t *testing.T) {
	ci := &options.Collation{Locale: "en", Strength: 2}
	docs := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "abc"}, {Key: "t", Value: bson.A{"a", "B"}}, {Key: "g", Value: "P"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "ABC"}, {Key: "t", Value: bson.A{"A", "b"}}, {Key: "g", Value: "p"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "s", Value: "x"}, {Key: "t", Value: bson.A{"x", "X"}}, {Key: "g", Value: "q"}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "s", Value: "Abd"}, {Key: "t", Value: bson.A{}}, {Key: "g", Value: "Q"}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "s", Value: "b"}, {Key: "t", Value: bson.A{"b"}}, {Key: "g", Value: "p"}},
	}
	setupColl := func(ctx context.Context, col *mongo.Collection, collated bool) error {
		db := col.Database()
		if err := col.Drop(ctx); err != nil {
			return err
		}
		opts := options.CreateCollection()
		if collated {
			opts.SetCollation(ci)
		}
		if err := db.CreateCollection(ctx, col.Name(), opts); err != nil {
			return err
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		other := db.Collection(col.Name() + "_o")
		if err := other.Drop(ctx); err != nil {
			return err
		}
		if _, err := other.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: "ABC"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "k", Value: "X"}},
		}); err != nil {
			return err
		}
		graph := db.Collection(col.Name() + "_g")
		if err := graph.Drop(ctx); err != nil {
			return err
		}
		_, err := graph.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "name", Value: "A"}, {Key: "parent", Value: "b"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "name", Value: "B"}, {Key: "parent", Value: "C"}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "name", Value: "c"}, {Key: "parent", Value: nil}},
		})
		return err
	}
	byID := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}
	pipelines := map[string]func(name string) bson.A{
		"expr_match": func(string) bson.A {
			return bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$s", "abc"}}}}}}}, byID}
		},
		"comparisons": func(string) bson.A {
			return bson.A{bson.D{{Key: "$project", Value: bson.D{
				{Key: "eq", Value: bson.D{{Key: "$eq", Value: bson.A{"$s", "abc"}}}},
				{Key: "ne", Value: bson.D{{Key: "$ne", Value: bson.A{"$s", "ABC"}}}},
				{Key: "lt", Value: bson.D{{Key: "$lt", Value: bson.A{"$s", "abd"}}}},
				{Key: "cmp", Value: bson.D{{Key: "$cmp", Value: bson.A{"$s", "abd"}}}},
				{Key: "in", Value: bson.D{{Key: "$in", Value: bson.A{"A", "$t"}}}},
				{Key: "io", Value: bson.D{{Key: "$indexOfArray", Value: bson.A{"$t", "b"}}}},
			}}}, byID}
		},
		"set_operators": func(string) bson.A {
			return bson.A{bson.D{{Key: "$project", Value: bson.D{
				{Key: "u", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$setUnion", Value: bson.A{"$t", bson.A{"b"}}}}}}},
				{Key: "i", Value: bson.D{{Key: "$setIntersection", Value: bson.A{"$t", bson.A{"A", "x"}}}}},
				{Key: "d", Value: bson.D{{Key: "$setDifference", Value: bson.A{"$t", bson.A{"A"}}}}},
				{Key: "e", Value: bson.D{{Key: "$setEquals", Value: bson.A{"$t", bson.A{"A", "b"}}}}},
				{Key: "ss", Value: bson.D{{Key: "$setIsSubset", Value: bson.A{"$t", bson.A{"a", "b", "x"}}}}},
			}}}, byID}
		},
		"ordering_expressions": func(string) bson.A {
			return bson.A{bson.D{{Key: "$project", Value: bson.D{
				{Key: "mx", Value: bson.D{{Key: "$max", Value: bson.A{"$s", "ABD"}}}},
				{Key: "mn", Value: bson.D{{Key: "$minN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: bson.A{"b", "A", "a", "B"}}}}}},
				{Key: "sa", Value: bson.D{{Key: "$sortArray", Value: bson.D{{Key: "input", Value: bson.A{"b", "A", "a", "B"}}, {Key: "sortBy", Value: int32(1)}}}}},
			}}}, byID}
		},
		"sort_by_count": func(string) bson.A {
			// mongod orders groups of equal count arbitrarily.
			return bson.A{byID, bson.D{{Key: "$sortByCount", Value: "$s"}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}}}
		},
		"bucket": func(string) bson.A {
			return bson.A{byID, bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$s"},
				{Key: "boundaries", Value: bson.A{"a", "abd", "z"}}, {Key: "default", Value: "other"},
				{Key: "output", Value: bson.D{{Key: "u", Value: bson.D{{Key: "$addToSet", Value: "$s"}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: 1}, {Key: "c", Value: bson.D{{Key: "$size", Value: "$u"}}}}}}}
		},
		"bucket_auto": func(string) bson.A {
			return bson.A{byID, bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$s"}, {Key: "buckets", Value: int32(2)}}}}}
		},
		"window": func(string) bson.A {
			return bson.A{bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "partitionBy", Value: "$g"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}},
				{Key: "output", Value: bson.D{
					{Key: "r", Value: bson.D{{Key: "$rank", Value: bson.D{}}}},
					{Key: "mn", Value: bson.D{{Key: "$min", Value: "$s"}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}}}},
					{Key: "u", Value: bson.D{{Key: "$addToSet", Value: "$s"}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}}}},
				}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}, {Key: "mn", Value: 1}, {Key: "c", Value: bson.D{{Key: "$size", Value: "$u"}}}}}}, byID}
		},
		"top_and_max_n": func(string) bson.A {
			return bson.A{byID, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
				{Key: "t", Value: bson.D{{Key: "$topN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, {Key: "output", Value: "$_id"}}}}},
				{Key: "mx", Value: bson.D{{Key: "$maxN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$s"}}}}},
			}}}}
		},
		"lookup_equality": func(name string) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: name + "_o"}, {Key: "localField", Value: "s"}, {Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "j", Value: "$j._id"}}}}, byID}
		},
		"lookup_pipeline": func(name string) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: name + "_o"}, {Key: "let", Value: bson.D{{Key: "s", Value: "$s"}}},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$k", "$$s"}}}}}}}}}, {Key: "as", Value: "j"}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "j", Value: "$j._id"}}}}, byID}
		},
		"graph_lookup": func(name string) bson.A {
			return bson.A{bson.D{{Key: "$graphLookup", Value: bson.D{{Key: "from", Value: name + "_g"}, {Key: "startWith", Value: "a"},
				{Key: "connectFromField", Value: "parent"}, {Key: "connectToField", Value: "name"}, {Key: "as", Value: "h"}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$h"}}}}}}, byID}
		},
		"facet": func(string) bson.A {
			return bson.A{bson.D{{Key: "$facet", Value: bson.D{
				{Key: "g", Value: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$s"}}}}, bson.D{{Key: "$count", Value: "n"}}}},
				{Key: "m", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "s", Value: "abc"}}}}, bson.D{{Key: "$count", Value: "n"}}}},
			}}}}
		},
		"union_with": func(name string) bson.A {
			return bson.A{bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: name + "_o"},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "k", Value: "abc"}}}}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}}, byID}
		},
	}
	for name, pipeline := range pipelines {
		for _, source := range []string{"command", "collection"} {
			collated := source == "collection"
			harness.PairTest(t, harness.TestCase{
				Name:    "AggCommandCollation_" + name + "_" + source,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					return setupColl(ctx, col, collated)
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Aggregate()
					if !collated {
						opts.SetCollation(ci)
					}
					cursor, err := col.Aggregate(ctx, pipeline(col.Name()), opts)
					return drainCursor(ctx, cursor, err)
				},
			})
		}
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "AggCommandCollation_documents",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Database().Aggregate(ctx, bson.A{
				bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "a", Value: "x"}}, bson.D{{Key: "a", Value: "X"}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "e", Value: bson.D{{Key: "$eq", Value: bson.A{"$a", "x"}}}}}}},
			}, options.Aggregate().SetCollation(ci))
			return drainCursor(ctx, cursor, err)
		},
	})
}
