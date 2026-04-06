// Parity tests migrated from dolthub/docudolt/tests/agg_stages_test.go.
// Covers: $match, $group, $sort, $limit, $skip, $project, $unwind, $addFields,
// $set, $unset, $count, $replaceRoot, $replaceWith, $sortByCount, $facet,
// $bucket, $bucketAuto, $lookup, $out, $merge, $graphLookup, multi-stage combos,
// unsupported stage errors, $collStats.
package tests

import (
	"context"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/docudolt-parity-testing/harness"
)

// sortBsonAByID sorts a bson.A of bson.D documents by their "_id" string field.
// Used to normalize unordered $graphLookup result arrays for deterministic comparison.
func sortBsonAByID(a bson.A) {
	sort.Slice(a, func(i, j int) bool {
		di, _ := a[i].(bson.D)
		dj, _ := a[j].(bson.D)
		var si, sj string
		for _, e := range di {
			if e.Key == "_id" {
				si, _ = e.Value.(string)
				break
			}
		}
		for _, e := range dj {
			if e.Key == "_id" {
				sj, _ = e.Value.(string)
				break
			}
		}
		return si < sj
	})
}

// ─── $match ───────────────────────────────────────────────────────────────────

func aggMatchSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "x", Value: int32(1)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "x", Value: int32(2)}},
		bson.D{{Key: "_id", Value: "c"}, {Key: "x", Value: int32(3)}},
		bson.D{{Key: "_id", Value: "d"}, {Key: "x", Value: int32(4)}},
	}
}

func insertAggMatchSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggMatchSeed())
	return err
}

func TestAggStage_match_EqualityFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_match_EqualityFilter",
		Support: harness.DocudoltFull,
		Setup:   insertAggMatchSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: int32(2)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_match_ComparisonOperator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_match_ComparisonOperator",
		Support: harness.DocudoltFull,
		Setup:   insertAggMatchSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_match_NoMatchReturnsEmpty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_match_NoMatchReturnsEmpty",
		Support: harness.DocudoltFull,
		Setup:   insertAggMatchSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: int32(99)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_match_MatchAllWithExists(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_match_MatchAllWithExists",
		Support: harness.DocudoltFull,
		Setup:   insertAggMatchSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$exists", Value: true}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_match_AndCondition(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_match_AndCondition",
		Support: harness.DocudoltFull,
		Setup:   insertAggMatchSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: bson.D{
					{Key: "$gte", Value: int32(2)},
					{Key: "$lte", Value: int32(3)},
				}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_match_InOperator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_match_InOperator",
		Support: harness.DocudoltFull,
		Setup:   insertAggMatchSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(3)}}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

// ─── $group ───────────────────────────────────────────────────────────────────

func aggGroupSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a1"}, {Key: "cat", Value: "A"}, {Key: "v", Value: int32(10)}},
		bson.D{{Key: "_id", Value: "a2"}, {Key: "cat", Value: "A"}, {Key: "v", Value: int32(20)}},
		bson.D{{Key: "_id", Value: "b1"}, {Key: "cat", Value: "B"}, {Key: "v", Value: int32(5)}},
		bson.D{{Key: "_id", Value: "b2"}, {Key: "cat", Value: "B"}, {Key: "v", Value: int32(15)}},
		bson.D{{Key: "_id", Value: "b3"}, {Key: "cat", Value: "B"}, {Key: "v", Value: int32(25)}},
	}
}

func insertAggGroupSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggGroupSeed())
	return err
}

func TestAggStage_group_CountAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_CountAll",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_GroupByField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_GroupByField",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_SumAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_SumAccumulator",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$v"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_AvgAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_AvgAccumulator",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "avg", Value: bson.D{{Key: "$avg", Value: "$v"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_MinAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_MinAccumulator",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "lo", Value: bson.D{{Key: "$min", Value: "$v"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_MaxAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_MaxAccumulator",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "hi", Value: bson.D{{Key: "$max", Value: "$v"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_MinMaxTogether(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_MinMaxTogether",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "lo", Value: bson.D{{Key: "$min", Value: "$v"}}},
					{Key: "hi", Value: bson.D{{Key: "$max", Value: "$v"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_PushAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_PushAccumulator",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "vals", Value: bson.D{{Key: "$push", Value: "$v"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_group_FirstAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_FirstAccumulator",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "x1"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "x2"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "x3"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$g"},
					{Key: "first", Value: bson.D{{Key: "$first", Value: "$v"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_LastAccumulator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_LastAccumulator",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "x1"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "x2"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "x3"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$g"},
					{Key: "last", Value: bson.D{{Key: "$last", Value: "$v"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_FirstLastTogether(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_FirstLastTogether",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "x1"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "x2"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "x3"}, {Key: "g", Value: "X"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$g"},
					{Key: "first", Value: bson.D{{Key: "$first", Value: "$v"}}},
					{Key: "last", Value: bson.D{{Key: "$last", Value: "$v"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_group_NullIdGroupsAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_group_NullIdGroupsAll",
		Support: harness.DocudoltFull,
		Setup:   insertAggGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_groupErrors_MissingIDField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_groupErrors_MissingIDField",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
			})
			return nil, err
		},
	})
}

func TestAggStage_groupErrors_NonDocumentSpec(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_groupErrors_NonDocumentSpec",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: "not-a-doc"}},
			})
			return nil, err
		},
	})
}

// ─── $sort ────────────────────────────────────────────────────────────────────

func aggSortSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
		bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
	}
}

func insertAggSortSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggSortSeed())
	return err
}

func TestAggStage_sort_Ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_sort_Ascending",
		Support: harness.DocudoltFull,
		Setup:   insertAggSortSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "v", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_sort_Descending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_sort_Descending",
		Support: harness.DocudoltFull,
		Setup:   insertAggSortSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "v", Value: int32(-1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_sort_ByID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_sort_ByID",
		Support: harness.DocudoltFull,
		Setup:   insertAggSortSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_sort_EmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_sort_EmptyCollection",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "v", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

// ─── $limit ───────────────────────────────────────────────────────────────────

func aggLimitSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}},
		bson.D{{Key: "_id", Value: "b"}},
		bson.D{{Key: "_id", Value: "c"}},
		bson.D{{Key: "_id", Value: "d"}},
		bson.D{{Key: "_id", Value: "e"}},
	}
}

func insertAggLimitSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggLimitSeed())
	return err
}

func TestAggStage_limit_LimitOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_limit_LimitOne",
		Support: harness.DocudoltFull,
		Setup:   insertAggLimitSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$limit", Value: int64(1)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_limit_LimitThree(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_limit_LimitThree",
		Support: harness.DocudoltFull,
		Setup:   insertAggLimitSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$limit", Value: int64(3)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_limit_LimitExceedsCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_limit_LimitExceedsCollection",
		Support: harness.DocudoltFull,
		Setup:   insertAggLimitSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$limit", Value: int64(100)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_limit_LimitZeroError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_limit_LimitZeroError",
		Support: harness.DocudoltXFail, // error code mismatch: mongo=15958, docudolt=5107201
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$limit", Value: int64(0)}},
			})
			return nil, err
		},
	})
}

func TestAggStage_limit_LimitNegativeError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_limit_LimitNegativeError",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$limit", Value: int64(-1)}},
			})
			return nil, err
		},
	})
}

// ─── $skip ────────────────────────────────────────────────────────────────────

func aggSkipSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}},
		bson.D{{Key: "_id", Value: "b"}},
		bson.D{{Key: "_id", Value: "c"}},
		bson.D{{Key: "_id", Value: "d"}},
	}
}

func insertAggSkipSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggSkipSeed())
	return err
}

func TestAggStage_skip_SkipOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_skip_SkipOne",
		Support: harness.DocudoltFull,
		Setup:   insertAggSkipSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$skip", Value: int64(1)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_skip_SkipAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_skip_SkipAll",
		Support: harness.DocudoltFull,
		Setup:   insertAggSkipSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$skip", Value: int64(100)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_skip_SkipZero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_skip_SkipZero",
		Support: harness.DocudoltFull,
		Setup:   insertAggSkipSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$skip", Value: int64(0)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_skip_SkipNegativeError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_skip_SkipNegativeError",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$skip", Value: int64(-1)}},
			})
			return nil, err
		},
	})
}

// ─── $project ─────────────────────────────────────────────────────────────────

func aggProjectSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}, {Key: "z", Value: int32(3)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "x", Value: int32(4)}, {Key: "y", Value: int32(5)}, {Key: "z", Value: int32(6)}},
	}
}

func insertAggProjectSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggProjectSeed())
	return err
}

func TestAggStage_project_IncludeFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_project_IncludeFields",
		Support: harness.DocudoltFull,
		Setup:   insertAggProjectSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_project_ExcludeFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_project_ExcludeFields",
		Support: harness.DocudoltFull,
		Setup:   insertAggProjectSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "z", Value: int32(0)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_project_ExcludeID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_project_ExcludeID",
		Support: harness.DocudoltFull,
		Setup:   insertAggProjectSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "x", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_project_ComputedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_project_ComputedField",
		Support: harness.DocudoltFull,
		Setup:   insertAggProjectSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: int32(0)},
					{Key: "sum", Value: bson.D{{Key: "$add", Value: bson.A{"$x", "$y"}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $unwind ──────────────────────────────────────────────────────────────────

func aggUnwindSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "tags", Value: bson.A{"x", "y", "z"}}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "tags", Value: bson.A{"p", "q"}}},
		bson.D{{Key: "_id", Value: "c"}, {Key: "tags", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: "d"}}, // missing field
	}
}

func insertAggUnwindSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggUnwindSeed())
	return err
}

func TestAggStage_unwind_BasicUnwind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unwind_BasicUnwind",
		Support: harness.DocudoltFull,
		Setup:   insertAggUnwindSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a"}}}},
				bson.D{{Key: "$unwind", Value: "$tags"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_unwind_EmptyArraySkipped(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unwind_EmptyArraySkipped",
		Support: harness.DocudoltFull,
		Setup:   insertAggUnwindSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "c"}}}},
				bson.D{{Key: "$unwind", Value: "$tags"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_unwind_MissingFieldSkipped(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unwind_MissingFieldSkipped",
		Support: harness.DocudoltFull,
		Setup:   insertAggUnwindSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "d"}}}},
				bson.D{{Key: "$unwind", Value: "$tags"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_unwind_PreserveNullAndEmptyArrays(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unwind_PreserveNullAndEmptyArrays",
		Support: harness.DocudoltFull,
		Setup:   insertAggUnwindSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$unwind", Value: bson.D{
					{Key: "path", Value: "$tags"},
					{Key: "preserveNullAndEmptyArrays", Value: true},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// a: 3, b: 2, c: 1 (empty preserved), d: 1 (missing preserved) = 7
			return len(results), nil
		},
	})
}

func TestAggStage_unwind_IncludeArrayIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unwind_IncludeArrayIndex",
		Support: harness.DocudoltFull,
		Setup:   insertAggUnwindSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a"}}}},
				bson.D{{Key: "$unwind", Value: bson.D{
					{Key: "path", Value: "$tags"},
					{Key: "includeArrayIndex", Value: "idx"},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "idx", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $addFields ───────────────────────────────────────────────────────────────

func aggAddFieldsSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "x", Value: int32(10)}, {Key: "y", Value: int32(5)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "x", Value: int32(20)}, {Key: "y", Value: int32(3)}},
	}
}

func insertAggAddFieldsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggAddFieldsSeed())
	return err
}

func TestAggStage_addFields_AddLiteralField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_addFields_AddLiteralField",
		Support: harness.DocudoltFull,
		Setup:   insertAggAddFieldsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "status", Value: "active"}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_addFields_AddComputedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_addFields_AddComputedField",
		Support: harness.DocudoltFull,
		Setup:   insertAggAddFieldsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "total", Value: bson.D{{Key: "$add", Value: bson.A{"$x", "$y"}}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_addFields_OriginalFieldsPreserved(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_addFields_OriginalFieldsPreserved",
		Support: harness.DocudoltFull,
		Setup:   insertAggAddFieldsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "extra", Value: int32(99)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $set ─────────────────────────────────────────────────────────────────────

func TestAggStage_set_SetNewField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_set_SetNewField",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$set", Value: bson.D{{Key: "doubled", Value: bson.D{{Key: "$multiply", Value: bson.A{"$v", int32(2)}}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_set_SetOverwritesField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_set_SetOverwritesField",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(42)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $unset ───────────────────────────────────────────────────────────────────

func TestAggStage_unset_UnsetSingleField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unset_UnsetSingleField",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}, {Key: "z", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$unset", Value: "z"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_unset_UnsetMultipleFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unset_UnsetMultipleFields",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}, {Key: "z", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$unset", Value: bson.A{"y", "z"}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $count ───────────────────────────────────────────────────────────────────

func aggCountSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "active", Value: true}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "active", Value: true}},
		bson.D{{Key: "_id", Value: "c"}, {Key: "active", Value: false}},
	}
}

func insertAggCountSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggCountSeed())
	return err
}

func TestAggStage_count_CountAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_count_CountAll",
		Support: harness.DocudoltFull,
		Setup:   insertAggCountSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_count_CountAfterMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_count_CountAfterMatch",
		Support: harness.DocudoltFull,
		Setup:   insertAggCountSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "active", Value: true}}}},
				bson.D{{Key: "$count", Value: "n"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_count_CountEmptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_count_CountEmptyCollection",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$count", Value: "n"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_countErrors_EmptyFieldName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_countErrors_EmptyFieldName",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$count", Value: ""}},
			})
			return nil, err
		},
	})
}

func TestAggStage_countErrors_FieldNameWithDot(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_countErrors_FieldNameWithDot",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$count", Value: "a.b"}},
			})
			return nil, err
		},
	})
}

// ─── $replaceRoot / $replaceWith ──────────────────────────────────────────────

func TestAggStage_replaceRoot_ReplaceWithNestedDoc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_replaceRoot_ReplaceWithNestedDoc",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "nested", Value: bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}}}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "nested", Value: bson.D{{Key: "x", Value: int32(3)}, {Key: "y", Value: int32(4)}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
				bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$nested"}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_replaceWith_ReplaceWithNestedDoc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_replaceWith_ReplaceWithNestedDoc",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "info", Value: bson.D{{Key: "v", Value: int32(42)}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$replaceWith", Value: "$info"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $sortByCount ─────────────────────────────────────────────────────────────

func TestAggStage_sortByCount_SortByCountDescending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_sortByCount_SortByCountDescending",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "1"}, {Key: "tag", Value: "go"}},
				bson.D{{Key: "_id", Value: "2"}, {Key: "tag", Value: "go"}},
				bson.D{{Key: "_id", Value: "3"}, {Key: "tag", Value: "go"}},
				bson.D{{Key: "_id", Value: "4"}, {Key: "tag", Value: "rust"}},
				bson.D{{Key: "_id", Value: "5"}, {Key: "tag", Value: "rust"}},
				bson.D{{Key: "_id", Value: "6"}, {Key: "tag", Value: "c"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sortByCount", Value: "$tag"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_sortByCount_TieBreakingOrder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_sortByCount_TieBreakingOrder",
		Support: harness.DocudoltXFail, // $sortByCount tiebreaking order diverges from MongoDB
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "score", Value: int32(3)}},
				bson.D{{Key: "_id", Value: 2}, {Key: "score", Value: int32(1)}},
				bson.D{{Key: "_id", Value: 3}, {Key: "score", Value: int32(2)}},
				bson.D{{Key: "_id", Value: 4}, {Key: "score", Value: int32(3)}},
				bson.D{{Key: "_id", Value: 5}, {Key: "score", Value: int32(1)}},
				bson.D{{Key: "_id", Value: 6}, {Key: "score", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// All three score values appear exactly twice (count=2). Tie-breaking
			// by ascending _id should yield [1, 2, 3]. Docudolt returns [3, 1, 2].
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sortByCount", Value: "$score"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $facet ───────────────────────────────────────────────────────────────────

func TestAggStage_facet_MultipleFacets(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_facet_MultipleFacets",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "price", Value: int32(10)}, {Key: "cat", Value: "food"}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "price", Value: int32(20)}, {Key: "cat", Value: "food"}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "price", Value: int32(30)}, {Key: "cat", Value: "gear"}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "price", Value: int32(40)}, {Key: "cat", Value: "gear"}},
				bson.D{{Key: "_id", Value: "e"}, {Key: "price", Value: int32(50)}, {Key: "cat", Value: "gear"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$facet", Value: bson.D{
					{Key: "byCategory", Value: bson.A{
						bson.D{{Key: "$sortByCount", Value: "$cat"}},
					}},
					{Key: "totalCount", Value: bson.A{
						bson.D{{Key: "$count", Value: "n"}},
					}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $bucket ──────────────────────────────────────────────────────────────────

func aggBucketSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "price", Value: int32(5)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "price", Value: int32(15)}},
		bson.D{{Key: "_id", Value: "c"}, {Key: "price", Value: int32(25)}},
		bson.D{{Key: "_id", Value: "d"}, {Key: "price", Value: int32(35)}},
		bson.D{{Key: "_id", Value: "e"}, {Key: "price", Value: int32(45)}},
	}
}

func insertAggBucketSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggBucketSeed())
	return err
}

func TestAggStage_bucket_BasicBuckets(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucket_BasicBuckets",
		Support: harness.DocudoltFull,
		Setup:   insertAggBucketSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucket", Value: bson.D{
					{Key: "groupBy", Value: "$price"},
					{Key: "boundaries", Value: bson.A{int32(0), int32(20), int32(40), int32(60)}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_bucket_WithDefault(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucket_WithDefault",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "x"}, {Key: "price", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "y"}, {Key: "price", Value: int32(999)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucket", Value: bson.D{
					{Key: "groupBy", Value: "$price"},
					{Key: "boundaries", Value: bson.A{int32(0), int32(100)}},
					{Key: "default", Value: "other"},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_bucket_MissingBoundariesError(t *testing.T) {
	// Diverge (do-slq1): $bucket without a boundaries field — mongo returns
	// error code 40198; docudolt returns code 9 (FailedToParse). Fix in docudolt's
	// $bucket validation to emit the correct Location40198 error.
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucket_MissingBoundariesError",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucket", Value: bson.D{
					{Key: "groupBy", Value: "$price"},
				}}},
			})
			return nil, err
		},
	})
}

// ─── $bucketAuto ──────────────────────────────────────────────────────────────

func aggBucketAutoSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(20)}},
		bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(30)}},
		bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(40)}},
		bson.D{{Key: "_id", Value: "e"}, {Key: "v", Value: int32(50)}},
		bson.D{{Key: "_id", Value: "f"}, {Key: "v", Value: int32(60)}},
	}
}

func insertAggBucketAutoSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggBucketAutoSeed())
	return err
}

func TestAggStage_bucketAuto_TwoBuckets(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucketAuto_TwoBuckets",
		Support: harness.DocudoltFull,
		Setup:   insertAggBucketAutoSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucketAuto", Value: bson.D{
					{Key: "groupBy", Value: "$v"},
					{Key: "buckets", Value: int32(2)},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_bucketAuto_ThreeBuckets(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucketAuto_ThreeBuckets",
		Support: harness.DocudoltFull,
		Setup:   insertAggBucketAutoSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucketAuto", Value: bson.D{
					{Key: "groupBy", Value: "$v"},
					{Key: "buckets", Value: int32(3)},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_bucketAuto_MissingBucketsError(t *testing.T) {
	// Diverge (do-slq1): $bucketAuto without a buckets field — mongo returns
	// error code 40246; docudolt returns code 9 (FailedToParse). Fix in docudolt's
	// $bucketAuto validation to emit the correct Location40246 error.
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucketAuto_MissingBucketsError",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucketAuto", Value: bson.D{
					{Key: "groupBy", Value: "$v"},
				}}},
			})
			return nil, err
		},
	})
}

// ─── $lookup ──────────────────────────────────────────────────────────────────

func TestAggStage_lookup_SimpleEqualityJoin(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_lookup_SimpleEqualityJoin",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// col = orders collection
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "o1"}, {Key: "item", Value: "widget"}, {Key: "qty", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "o2"}, {Key: "item", Value: "gadget"}, {Key: "qty", Value: int32(2)}},
			}); err != nil {
				return err
			}
			inv := col.Database().Collection("inventory")
			_, err := inv.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i1"}, {Key: "sku", Value: "widget"}, {Key: "instock", Value: int32(100)}},
				bson.D{{Key: "_id", Value: "i2"}, {Key: "sku", Value: "gadget"}, {Key: "instock", Value: int32(50)}},
				bson.D{{Key: "_id", Value: "i3"}, {Key: "sku", Value: "doohickey"}, {Key: "instock", Value: int32(0)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "inventory"},
					{Key: "localField", Value: "item"},
					{Key: "foreignField", Value: "sku"},
					{Key: "as", Value: "stockInfo"},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_lookup_NoMatchProducesEmptyArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_lookup_NoMatchProducesEmptyArray",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "item", Value: "unknown-item"}},
			}); err != nil {
				return err
			}
			inv := col.Database().Collection("inv_nomatch")
			_, err := inv.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i1"}, {Key: "sku", Value: "widget"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "inv_nomatch"},
					{Key: "localField", Value: "item"},
					{Key: "foreignField", Value: "sku"},
					{Key: "as", Value: "stockInfo"},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── $out ─────────────────────────────────────────────────────────────────────

func TestAggStage_out_OutToNewCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_out_OutToNewCollection",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: int32(2)}}}}}},
				bson.D{{Key: "$out", Value: "out_target"}},
			})
			if err != nil {
				return nil, err
			}
			// $out cursor returns no documents
			var cursorDocs []bson.D
			if err := cursor.All(ctx, &cursorDocs); err != nil {
				return nil, err
			}
			// Verify target collection has the written docs
			target := col.Database().Collection("out_target")
			count, err := target.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

// ─── $merge ───────────────────────────────────────────────────────────────────

func TestAggStage_merge_MergeIntoNewCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_merge_MergeIntoNewCollection",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$merge", Value: bson.D{
					{Key: "into", Value: "merge_target"},
					{Key: "whenMatched", Value: "replace"},
					{Key: "whenNotMatched", Value: "insert"},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var cursorDocs []bson.D
			if err := cursor.All(ctx, &cursorDocs); err != nil {
				return nil, err
			}
			target := col.Database().Collection("merge_target")
			count, err := target.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

func TestAggStage_merge_MergeStringForm(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_merge_MergeStringForm",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$merge", Value: "merge_str_target"}},
			})
			if err != nil {
				return nil, err
			}
			var cursorDocs []bson.D
			if err := cursor.All(ctx, &cursorDocs); err != nil {
				return nil, err
			}
			target := col.Database().Collection("merge_str_target")
			count, err := target.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return count, nil
		},
	})
}

// ─── $graphLookup ─────────────────────────────────────────────────────────────

func insertOrgHierarchy(ctx context.Context, col *mongo.Collection) error {
	// Build an org hierarchy: CEO → VP → Manager → Employee.
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "ceo"}, {Key: "reportsTo", Value: nil}},
		bson.D{{Key: "_id", Value: "vp"}, {Key: "reportsTo", Value: "ceo"}},
		bson.D{{Key: "_id", Value: "mgr"}, {Key: "reportsTo", Value: "vp"}},
		bson.D{{Key: "_id", Value: "emp"}, {Key: "reportsTo", Value: "mgr"}},
	})
	return err
}

func TestAggStage_graphLookup_TraverseHierarchyFromLeaf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_graphLookup_TraverseHierarchyFromLeaf",
		Support: harness.DocudoltXFail, // $graphLookup result array order non-deterministic
		Setup:   insertOrgHierarchy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "emp"}}}},
				bson.D{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$reportsTo"},
					{Key: "connectFromField", Value: "reportsTo"},
					{Key: "connectToField", Value: "_id"},
					{Key: "as", Value: "chain"},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggStage_graphLookup_MaxDepthLimitsTraversal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_graphLookup_MaxDepthLimitsTraversal",
		Support: harness.DocudoltFull,
		Setup:   insertOrgHierarchy,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "emp"}}}},
				bson.D{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$reportsTo"},
					{Key: "connectFromField", Value: "reportsTo"},
					{Key: "connectToField", Value: "_id"},
					{Key: "as", Value: "chain"},
					{Key: "maxDepth", Value: int64(1)},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// $graphLookup does not guarantee ordering of the result array.
			// Sort by _id for deterministic comparison.
			for _, doc := range results {
				for i, elem := range doc {
					if elem.Key == "chain" {
						if chain, ok := elem.Value.(bson.A); ok {
							sortBsonAByID(chain)
							doc[i].Value = chain
						}
					}
				}
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Multi-stage pipelines ────────────────────────────────────────────────────

func aggMultiStageSeed() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "a"}, {Key: "dept", Value: "eng"}, {Key: "salary", Value: int32(100)}},
		bson.D{{Key: "_id", Value: "b"}, {Key: "dept", Value: "eng"}, {Key: "salary", Value: int32(200)}},
		bson.D{{Key: "_id", Value: "c"}, {Key: "dept", Value: "mkt"}, {Key: "salary", Value: int32(150)}},
		bson.D{{Key: "_id", Value: "d"}, {Key: "dept", Value: "mkt"}, {Key: "salary", Value: int32(120)}},
		bson.D{{Key: "_id", Value: "e"}, {Key: "dept", Value: "eng"}, {Key: "salary", Value: int32(90)}},
	}
}

func insertAggMultiStageSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggMultiStageSeed())
	return err
}

func TestAggPipeline_multiStage_MatchGroupSort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_multiStage_MatchGroupSort",
		Support: harness.DocudoltFull,
		Setup:   insertAggMultiStageSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "dept", Value: "eng"}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$dept"},
					{Key: "avgSalary", Value: bson.D{{Key: "$avg", Value: "$salary"}}},
					{Key: "headcount", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggPipeline_multiStage_SortLimitSkip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_multiStage_SortLimitSkip",
		Support: harness.DocudoltFull,
		Setup:   insertAggMultiStageSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "salary", Value: int32(-1)}}}},
				bson.D{{Key: "$skip", Value: int64(1)}},
				bson.D{{Key: "$limit", Value: int64(2)}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggPipeline_multiStage_AddFieldsThenGroup(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_multiStage_AddFieldsThenGroup",
		Support: harness.DocudoltFull,
		Setup:   insertAggMultiStageSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "bonus", Value: bson.D{{Key: "$multiply", Value: bson.A{"$salary", 0.1}}}}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$dept"},
					{Key: "totalBonus", Value: bson.D{{Key: "$sum", Value: "$bonus"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggPipeline_multiStage_UnwindThenGroup(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_multiStage_UnwindThenGroup",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// Use distinct per-tag counts so $sortByCount output is deterministic.
			// go=3, db=2, api=1 — no ties means a stable comparison regardless of implementation.
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "tags", Value: bson.A{"go", "db", "api"}}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "_id", Value: "p3"}, {Key: "tags", Value: bson.A{"go"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$unwind", Value: "$tags"}},
				bson.D{{Key: "$sortByCount", Value: "$tags"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggPipeline_multiStage_UnwindThenGroup_tiebreakOrder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_multiStage_UnwindThenGroup_tiebreakOrder",
		Support: harness.DocudoltXFail, // $sortByCount tiebreaking order diverges from MongoDB
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "tags", Value: bson.A{"go", "api"}}},
				bson.D{{Key: "_id", Value: "p3"}, {Key: "tags", Value: bson.A{"db", "api"}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Unwind tags, then count occurrences. All three tags appear exactly
			// twice (count=2), so sort order is determined entirely by the
			// secondary _id tiebreaker. Docudolt ignores that tiebreaker.
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$unwind", Value: "$tags"}},
				bson.D{{Key: "$sortByCount", Value: "$tags"}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAggPipeline_multiStage_ProjectThenSort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_multiStage_ProjectThenSort",
		Support: harness.DocudoltFull,
		Setup:   insertAggMultiStageSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: bson.D{{Key: "dept", Value: int32(1)}, {Key: "salary", Value: int32(1)}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "salary", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

// ─── Unsupported stage errors ─────────────────────────────────────────────────

func TestAggStage_unsupportedErrors_changeStream(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unsupportedErrors_changeStream",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$changeStream", Value: bson.D{}}}})
			return nil, err
		},
	})
}

func TestAggStage_unsupportedErrors_densify(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unsupportedErrors_densify",
		Support: harness.DocudoltXFail, // error code diverges: mongo=IDLFailedToParse, docudolt=Location40414
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$densify", Value: bson.D{}}}})
			return nil, err
		},
	})
}

func TestAggStage_unsupportedErrors_fill(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unsupportedErrors_fill",
		Support: harness.DocudoltXFail, // error code diverges: mongo=IDLFailedToParse, docudolt=Location40414
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$fill", Value: bson.D{}}}})
			return nil, err
		},
	})
}

func TestAggStage_unsupportedErrors_indexStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unsupportedErrors_indexStats",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$indexStats", Value: bson.D{}}}})
			return nil, err
		},
	})
}

func TestAggStage_unsupportedErrors_search(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unsupportedErrors_search",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$search", Value: bson.D{}}}})
			return nil, err
		},
	})
}

// ─── Error cases ──────────────────────────────────────────────────────────────

func TestAggStage_unknownStageError(t *testing.T) {
	// Diverge (do-gx4x): unknown aggregation stage — mongo returns error code
	// 40324 (Location40324); docudolt returns 40234 (Location40234). A separate
	// documented variant (TestAggStage_unknown_stage_error) also captures the
	// quote-style difference in the message text.
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unknownStageError",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$definitelyNotAStage", Value: bson.D{}}},
			})
			return nil, err
		},
	})
}

func TestAggStage_emptyPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_emptyPipeline",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}},
				bson.D{{Key: "_id", Value: "b"}},
				bson.D{{Key: "_id", Value: "c"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_invalidPipelineSpec(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_invalidPipelineSpec",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Two fields in one stage document is invalid.
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{}}, {Key: "$sort", Value: bson.D{}}},
			})
			return nil, err
		},
	})
}

// ─── $collStats ───────────────────────────────────────────────────────────────

func TestAggStage_collStats_StorageStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_collStats_StorageStats",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}},
				bson.D{{Key: "_id", Value: "b"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$collStats", Value: bson.D{{Key: "storageStats", Value: bson.D{}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

func TestAggStage_collStats_Count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_collStats_Count",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}},
				bson.D{{Key: "_id", Value: "b"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$collStats", Value: bson.D{{Key: "count", Value: bson.D{}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return len(results), nil
		},
	})
}

// ─── Error-code divergence XFail tests ────────────────────────────────────────

func TestAggStage_bucket_OneBoundaryError(t *testing.T) {
	// Diverge (do-t63k): $bucket with only one boundary value — mongo returns
	// error code Location40192; docudolt returns BadValue with a different message.
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_bucket_OneBoundaryError",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$bucket", Value: bson.D{
					{Key: "groupBy", Value: "$price"},
					{Key: "boundaries", Value: bson.A{int32(0)}},
				}}},
			})
			return nil, err
		},
	})
}

func TestAggStage_unknown_stage_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStage_unknown_stage_error",
		Support: harness.DocudoltFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$unknownStage", Value: bson.D{}}},
			})
			return nil, err
		},
	})
}

// ─── Sort tie-breaking divergence XFail tests ─────────────────────────────────

func TestAggPipeline_sort_TieBreakingAfterGroup(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggPipeline_sort_TieBreakingAfterGroup",
		Support: harness.DocudoltXFail, // $sortByCount tiebreaking order diverges from MongoDB
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "1"}, {Key: "cat", Value: "C"}},
				bson.D{{Key: "_id", Value: "2"}, {Key: "cat", Value: "A"}},
				bson.D{{Key: "_id", Value: "3"}, {Key: "cat", Value: "B"}},
				bson.D{{Key: "_id", Value: "4"}, {Key: "cat", Value: "C"}},
				bson.D{{Key: "_id", Value: "5"}, {Key: "cat", Value: "A"}},
				bson.D{{Key: "_id", Value: "6"}, {Key: "cat", Value: "B"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Group by cat → each group has count=2 (all tied).
			// Sort by count asc — all values equal, so tie-breaking determines order.
			// MongoDB and docudolt produce different orderings of the tied groups.
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$cat"},
					{Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "n", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}
