package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// aggSeedDocs are shared seed documents for aggregation pipeline tests.
var aggSeedDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "a1"},
		{Key: "category", Value: "fruit"},
		{Key: "name", Value: "apple"},
		{Key: "price", Value: 1.5},
		{Key: "qty", Value: int32(10)},
		{Key: "tags", Value: bson.A{"red", "sweet"}},
	},
	bson.D{
		{Key: "_id", Value: "a2"},
		{Key: "category", Value: "fruit"},
		{Key: "name", Value: "banana"},
		{Key: "price", Value: 0.75},
		{Key: "qty", Value: int32(20)},
		{Key: "tags", Value: bson.A{"yellow"}},
	},
	bson.D{
		{Key: "_id", Value: "a3"},
		{Key: "category", Value: "veggie"},
		{Key: "name", Value: "carrot"},
		{Key: "price", Value: 0.5},
		{Key: "qty", Value: int32(30)},
		{Key: "tags", Value: bson.A{"orange", "crunchy"}},
	},
	bson.D{
		{Key: "_id", Value: "a4"},
		{Key: "category", Value: "veggie"},
		{Key: "name", Value: "daikon"},
		{Key: "price", Value: 1.0},
		{Key: "qty", Value: int32(5)},
		{Key: "tags", Value: bson.A{"white"}},
	},
	bson.D{
		{Key: "_id", Value: "a5"},
		{Key: "category", Value: "fruit"},
		{Key: "name", Value: "elderberry"},
		{Key: "price", Value: 3.0},
		{Key: "qty", Value: int32(8)},
		{Key: "tags", Value: bson.A{"purple", "sweet"}},
	},
}

func insertAggSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, aggSeedDocs)
	return err
}

func runPipeline(ctx context.Context, col *mongo.Collection, pipeline []bson.D) ([]bson.D, error) {
	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var results []bson.D
	return results, cursor.All(ctx, &results)
}

// ─── $match ───────────────────────────────────────────────────────────────────

func TestAgg_match_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "category", Value: "fruit"}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_match_comparison(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_comparison",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "price", Value: bson.D{{Key: "$gt", Value: 1.0}}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_match_no_results(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_no_results",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "category", Value: "meat"}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_match_empty_collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_empty_collection",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "x", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_match_and(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_and",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{
					{Key: "category", Value: "fruit"},
					{Key: "qty", Value: bson.D{{Key: "$gte", Value: int32(10)}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $project ─────────────────────────────────────────────────────────────────

func TestAgg_project_include(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_include",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "name", Value: 1},
					{Key: "price", Value: 1},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_project_exclude(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_exclude",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a2"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "tags", Value: 0},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_project_computed_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_computed_field",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "total", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_project_rename_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_rename_field",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a3"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "item", Value: "$name"},
					{Key: "cost", Value: "$price"},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $sort ────────────────────────────────────────────────────────────────────

func TestAgg_sort_ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sort_ascending",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "price", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_sort_descending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sort_descending",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "qty", Value: -1}}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_sort_multi_key(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sort_multi_key",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{
					{Key: "category", Value: 1},
					{Key: "price", Value: -1},
				}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $limit / $skip ───────────────────────────────────────────────────────────

func TestAgg_limit_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_limit_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$limit", Value: int32(2)}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_skip_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_skip_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$skip", Value: int32(3)}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_skip_and_limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_skip_and_limit",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$skip", Value: int32(1)}},
				{{Key: "$limit", Value: int32(2)}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_limit_exceeds_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_limit_exceeds_count",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$limit", Value: int32(100)}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $count ───────────────────────────────────────────────────────────────────

func TestAgg_count_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_count_all",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$count", Value: "total"}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_count_after_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_count_after_match",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "category", Value: "fruit"}}}},
				{{Key: "$count", Value: "fruitCount"}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_count_empty_collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_count_empty_collection",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $group ───────────────────────────────────────────────────────────────────

func TestAgg_group_sum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_sum",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "totalQty", Value: bson.D{{Key: "$sum", Value: "$qty"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_count",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_avg(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_avg",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "avgPrice", Value: bson.D{{Key: "$avg", Value: "$price"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_min_max(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_min_max",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "minPrice", Value: bson.D{{Key: "$min", Value: "$price"}}},
					{Key: "maxPrice", Value: bson.D{{Key: "$max", Value: "$price"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_first_last(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_first_last",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "firstName", Value: bson.D{{Key: "$first", Value: "$name"}}},
					{Key: "lastName", Value: bson.D{{Key: "$last", Value: "$name"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_push(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_push",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "names", Value: bson.D{{Key: "$push", Value: "$name"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_addToSet(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_addToSet",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "categories", Value: bson.D{{Key: "$addToSet", Value: "$category"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 1 {
				if cats, ok := results[0].Map()["categories"].(bson.A); ok {
					return bson.D{{Key: "uniqueCount", Value: int32(len(cats))}}, nil
				}
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_group_null_id(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_null_id",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$qty"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_group_empty_collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_empty_collection",
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$qty"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $unwind ──────────────────────────────────────────────────────────────────

func TestAgg_unwind_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unwind_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$unwind", Value: "$tags"}},
				{{Key: "$project", Value: bson.D{{Key: "tag", Value: "$tags"}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_unwind_preserveNullAndEmpty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unwind_preserveNullAndEmpty",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "uw1"}, {Key: "tags", Value: bson.A{"x"}}},
				bson.D{{Key: "_id", Value: "uw2"}, {Key: "tags", Value: bson.A{}}},
				bson.D{{Key: "_id", Value: "uw3"}},
				bson.D{{Key: "_id", Value: "uw4"}, {Key: "tags", Value: nil}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: bson.D{
					{Key: "path", Value: "$tags"},
					{Key: "preserveNullAndEmptyArrays", Value: true},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_unwind_includeArrayIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unwind_includeArrayIndex",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$unwind", Value: bson.D{
					{Key: "path", Value: "$tags"},
					{Key: "includeArrayIndex", Value: "tagIndex"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "tag", Value: "$tags"},
					{Key: "idx", Value: "$tagIndex"},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_unwind_empty_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unwind_empty_array",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "empty-arr"},
				{Key: "items", Value: bson.A{}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$items"}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $lookup ──────────────────────────────────────────────────────────────────

func TestAgg_lookup_equality(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_lookup_equality",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ord-1"}, {Key: "item", Value: "apple"}},
				bson.D{{Key: "_id", Value: "ord-2"}, {Key: "item", Value: "banana"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "inventory_" + col.Name()},
					{Key: "localField", Value: "item"},
					{Key: "foreignField", Value: "name"},
					{Key: "as", Value: "inventory"},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "item", Value: 1},
					{Key: "inventoryCount", Value: bson.D{{Key: "$size", Value: "$inventory"}}},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_lookup_pipeline_form(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_lookup_pipeline_form",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "lp-1"}, {Key: "dept", Value: "eng"}},
				bson.D{{Key: "_id", Value: "lp-2"}, {Key: "dept", Value: "hr"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "emp_" + col.Name()},
					{Key: "let", Value: bson.D{{Key: "deptName", Value: "$dept"}}},
					{Key: "pipeline", Value: []bson.D{
						{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{
							{Key: "$eq", Value: bson.A{"$department", "$$deptName"}},
						}}}}},
					}},
					{Key: "as", Value: "employees"},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "dept", Value: 1},
					{Key: "empCount", Value: bson.D{{Key: "$size", Value: "$employees"}}},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $addFields / $set / $unset ───────────────────────────────────────────────

func TestAgg_addFields_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_addFields_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$addFields", Value: bson.D{
					{Key: "total", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
					{Key: "inStock", Value: true},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "total", Value: 1},
					{Key: "inStock", Value: 1},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_addFields_multiple_docs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_addFields_multiple_docs",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "value", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "value", Value: -1}}}},
				{{Key: "$limit", Value: int32(3)}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "value", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_set_alias(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_set_alias",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a2"}}}},
				{{Key: "$set", Value: bson.D{{Key: "discounted", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", 0.9}}}}}}},
				{{Key: "$project", Value: bson.D{{Key: "discounted", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_unset_stage_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unset_stage_array",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$unset", Value: bson.A{"tags", "qty"}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_unset_stage_single(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unset_stage_single",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a3"}}}},
				{{Key: "$unset", Value: "tags"}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $replaceRoot / $replaceWith ──────────────────────────────────────────────

func TestAgg_replaceRoot_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_replaceRoot_basic",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "rr-1"}, {Key: "meta", Value: bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}}}},
				bson.D{{Key: "_id", Value: "rr-2"}, {Key: "meta", Value: bson.D{{Key: "x", Value: int32(3)}, {Key: "y", Value: int32(4)}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$meta"}}}},
				{{Key: "$sort", Value: bson.D{{Key: "x", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_replaceWith_alias(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_replaceWith_alias",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "rw-1"}, {Key: "inner", Value: bson.D{{Key: "val", Value: int32(10)}}}},
				bson.D{{Key: "_id", Value: "rw-2"}, {Key: "inner", Value: bson.D{{Key: "val", Value: int32(20)}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$replaceWith", Value: "$inner"}},
				{{Key: "$sort", Value: bson.D{{Key: "val", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_replaceRoot_mergeObjects(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_replaceRoot_mergeObjects",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$replaceRoot", Value: bson.D{
					{Key: "newRoot", Value: bson.D{{Key: "$mergeObjects", Value: bson.A{
						bson.D{{Key: "source", Value: "agg"}},
						"$$ROOT",
					}}}},
				}}},
				{{Key: "$project", Value: bson.D{{Key: "source", Value: 1}, {Key: "name", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $out ─────────────────────────────────────────────────────────────────────

func TestAgg_out_to_collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_out_to_collection",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			outColName := "out_" + col.Name()
			_, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "category", Value: "fruit"}}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}}}},
				{{Key: "$out", Value: outColName}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.Database().Collection(outColName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "outCount", Value: count}}, nil
		},
	})
}

// ─── $merge ───────────────────────────────────────────────────────────────────

func TestAgg_merge_insert_new(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_merge_insert_new",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			mergeColName := "merge_" + col.Name()
			_, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "category", Value: "veggie"}}}},
				{{Key: "$merge", Value: bson.D{
					{Key: "into", Value: mergeColName},
					{Key: "whenMatched", Value: "replace"},
					{Key: "whenNotMatched", Value: "insert"},
				}}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.Database().Collection(mergeColName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "mergedCount", Value: count}}, nil
		},
	})
}

func TestAgg_merge_whenMatched_merge(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_merge_whenMatched_merge",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "val", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "val", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			mergeColName := "merge2_" + col.Name()
			mergeCol := col.Database().Collection(mergeColName)
			_, _ = mergeCol.InsertOne(ctx, bson.D{{Key: "_id", Value: "m1"}, {Key: "existing", Value: true}})
			_, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$merge", Value: bson.D{
					{Key: "into", Value: mergeColName},
					{Key: "on", Value: "_id"},
					{Key: "whenMatched", Value: "merge"},
					{Key: "whenNotMatched", Value: "insert"},
				}}},
			})
			if err != nil {
				return nil, err
			}
			count, err := mergeCol.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── $facet ───────────────────────────────────────────────────────────────────

func TestAgg_facet_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_facet_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$facet", Value: bson.D{
					{Key: "byCategory", Value: []bson.D{
						{{Key: "$group", Value: bson.D{
							{Key: "_id", Value: "$category"},
							{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
						}}},
						{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					}},
					{Key: "priceStats", Value: []bson.D{
						{{Key: "$group", Value: bson.D{
							{Key: "_id", Value: nil},
							{Key: "minPrice", Value: bson.D{{Key: "$min", Value: "$price"}}},
							{Key: "maxPrice", Value: bson.D{{Key: "$max", Value: "$price"}}},
						}}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_facet_with_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_facet_with_count",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$facet", Value: bson.D{
					{Key: "total", Value: []bson.D{
						{{Key: "$count", Value: "n"}},
					}},
					{Key: "fruits", Value: []bson.D{
						{{Key: "$match", Value: bson.D{{Key: "category", Value: "fruit"}}}},
						{{Key: "$count", Value: "n"}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $bucket / $bucketAuto ────────────────────────────────────────────────────

func TestAgg_bucket_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_bucket_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$bucket", Value: bson.D{
					{Key: "groupBy", Value: "$price"},
					{Key: "boundaries", Value: bson.A{0.0, 1.0, 2.0, 5.0}},
					{Key: "default", Value: "other"},
					{Key: "output", Value: bson.D{
						{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
						{Key: "names", Value: bson.D{{Key: "$push", Value: "$name"}}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_bucket_default(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_bucket_default",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$bucket", Value: bson.D{
					{Key: "groupBy", Value: "$qty"},
					{Key: "boundaries", Value: bson.A{int32(0), int32(10)}},
					{Key: "default", Value: "highQty"},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_bucketAuto_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_bucketAuto_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$bucketAuto", Value: bson.D{
					{Key: "groupBy", Value: "$price"},
					{Key: "buckets", Value: int32(3)},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "bucketCount", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_bucketAuto_with_output(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_bucketAuto_with_output",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$bucketAuto", Value: bson.D{
					{Key: "groupBy", Value: "$qty"},
					{Key: "buckets", Value: int32(2)},
					{Key: "output", Value: bson.D{
						{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
						{Key: "avgPrice", Value: bson.D{{Key: "$avg", Value: "$price"}}},
					}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "bucketCount", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $sortByCount ─────────────────────────────────────────────────────────────

func TestAgg_sortByCount_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sortByCount_basic",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sortByCount", Value: "$category"}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_sortByCount_after_unwind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sortByCount_after_unwind",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$tags"}},
				{{Key: "$sortByCount", Value: "$tags"}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $graphLookup ─────────────────────────────────────────────────────────────

func TestAgg_graphLookup_hierarchy(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_graphLookup_hierarchy",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ceo"}, {Key: "name", Value: "Alice"}, {Key: "reportsTo", Value: nil}},
				bson.D{{Key: "_id", Value: "vp"}, {Key: "name", Value: "Bob"}, {Key: "reportsTo", Value: "ceo"}},
				bson.D{{Key: "_id", Value: "mgr"}, {Key: "name", Value: "Carol"}, {Key: "reportsTo", Value: "vp"}},
				bson.D{{Key: "_id", Value: "eng"}, {Key: "name", Value: "Dave"}, {Key: "reportsTo", Value: "mgr"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "eng"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$reportsTo"},
					{Key: "connectFromField", Value: "reportsTo"},
					{Key: "connectToField", Value: "_id"},
					{Key: "as", Value: "chain"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "name", Value: 1},
					{Key: "chainLength", Value: bson.D{{Key: "$size", Value: "$chain"}}},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_graphLookup_maxDepth(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_graphLookup_maxDepth",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "root"}, {Key: "parent", Value: nil}},
				bson.D{{Key: "_id", Value: "c1"}, {Key: "parent", Value: "root"}},
				bson.D{{Key: "_id", Value: "c2"}, {Key: "parent", Value: "c1"}},
				bson.D{{Key: "_id", Value: "c3"}, {Key: "parent", Value: "c2"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "c3"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$parent"},
					{Key: "connectFromField", Value: "parent"},
					{Key: "connectToField", Value: "_id"},
					{Key: "maxDepth", Value: int32(1)},
					{Key: "as", Value: "ancestors"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "ancestorCount", Value: bson.D{{Key: "$size", Value: "$ancestors"}}},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $sample ──────────────────────────────────────────────────────────────────

func TestAgg_sample_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sample_count",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sample", Value: bson.D{{Key: "size", Value: int32(3)}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_sample_exceeds_size(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sample_exceeds_size",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sample", Value: bson.D{{Key: "size", Value: int32(100)}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $redact ──────────────────────────────────────────────────────────────────

func TestAgg_redact_prune(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_redact_prune",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "_id", Value: "rd-1"},
					{Key: "level", Value: int32(1)},
					{Key: "data", Value: bson.D{
						{Key: "level", Value: int32(3)},
						{Key: "secret", Value: "hidden"},
					}},
				},
				bson.D{{Key: "_id", Value: "rd-2"}, {Key: "level", Value: int32(2)}, {Key: "info", Value: "public"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: bson.D{{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{{Key: "$gt", Value: bson.A{"$level", int32(2)}}}},
					{Key: "then", Value: "$$PRUNE"},
					{Key: "else", Value: "$$DESCEND"},
				}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_redact_keep(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_redact_keep",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: "$$KEEP"}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_redact_prune_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_redact_prune_all",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: "$$PRUNE"}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

// ─── Multi-stage pipelines ────────────────────────────────────────────────────

func TestAgg_pipeline_match_group_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_pipeline_match_group_sort",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "qty", Value: bson.D{{Key: "$gte", Value: int32(8)}}}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$qty"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_pipeline_project_sort_limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_pipeline_project_sort_limit",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$project", Value: bson.D{
					{Key: "name", Value: 1},
					{Key: "price", Value: 1},
					{Key: "_id", Value: 0},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "price", Value: 1}}}},
				{{Key: "$limit", Value: int32(3)}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_pipeline_unwind_group(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_pipeline_unwind_group",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$tags"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$tags"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAgg_pipeline_addFields_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_pipeline_addFields_match",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "value", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
				{{Key: "$match", Value: bson.D{{Key: "value", Value: bson.D{{Key: "$gt", Value: 10.0}}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}, {Key: "value", Value: 1}, {Key: "_id", Value: 0}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── Aggregate options ────────────────────────────────────────────────────────

func TestAgg_allowDiskUse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_allowDiskUse",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Aggregate().SetAllowDiskUse(true)
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "price", Value: 1}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
			}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_empty_pipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_empty_pipeline",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_group_multiply_price_qty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_multiply_price_qty",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: bson.D{
						{Key: "$multiply", Value: bson.A{"$price", "$qty"}},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "count", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_match_in_operator(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_in_operator",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "category", Value: bson.D{{Key: "$in", Value: bson.A{"electronics", "clothing"}}}}}}},
				{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_project_conditional_cond(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_conditional_cond",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "name", Value: "Widget A"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: int32(0)},
					{Key: "expensive", Value: bson.D{{Key: "$cond", Value: bson.D{
						{Key: "if", Value: bson.D{{Key: "$gt", Value: bson.A{"$price", 50}}}},
						{Key: "then", Value: "yes"},
						{Key: "else", Value: "no"},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "expensive", Value: "unknown"}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_sort_by_multiple_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_sort_by_multiple_fields",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{
					{Key: "category", Value: int32(1)},
					{Key: "price", Value: int32(-1)},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: int32(0)},
					{Key: "name", Value: int32(1)},
					{Key: "category", Value: int32(1)},
				}}},
				{{Key: "$limit", Value: int32(3)}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_group_by_category_min_max(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_by_category_min_max",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "minPrice", Value: bson.D{{Key: "$min", Value: "$price"}}},
					{Key: "maxPrice", Value: bson.D{{Key: "$max", Value: "$price"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_unwind_then_group_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_unwind_then_group_count",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$tags"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$tags"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "count", Value: int32(-1)}}}},
				{{Key: "$limit", Value: int32(3)}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_addFields_computed_total(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_addFields_computed_total",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "name", Value: "Widget A"}}}},
				{{Key: "$addFields", Value: bson.D{
					{Key: "total", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: int32(0)},
					{Key: "total", Value: int32(1)},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_match_exists(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_exists",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "tags", Value: bson.D{{Key: "$exists", Value: true}}}}}},
				{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_project_string_concat(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_string_concat",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "name", Value: "Widget A"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: int32(0)},
					{Key: "label", Value: bson.D{{Key: "$concat", Value: bson.A{"$name", " (", "$category", ")"}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "label", Value: ""}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_group_avg_price(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_group_avg_price",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "avgPrice", Value: bson.D{{Key: "$avg", Value: "$price"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "avgPrice", Value: float64(0)}}, nil
			}
			return bson.D{{Key: "hasResult", Value: true}}, nil
		},
	})
}

func TestAgg_skip_and_limit_page(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_skip_and_limit_page",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Page 2 with page size 2
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "name", Value: int32(1)}}}},
				{{Key: "$skip", Value: int64(2)}},
				{{Key: "$limit", Value: int32(2)}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestAgg_match_regex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_regex",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^Widget"}}}}}},
				{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_project_array_size(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_project_array_size",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "name", Value: "Widget A"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: int32(0)},
					{Key: "tagCount", Value: bson.D{{Key: "$size", Value: "$tags"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "tagCount", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestAgg_match_gt_lt_range(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Agg_match_gt_lt_range",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "price", Value: bson.D{
					{Key: "$gt", Value: float64(20)},
					{Key: "$lt", Value: float64(100)},
				}}}}},
				{{Key: "$count", Value: "total"}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

// ─── Additional $addFields / $project expression tests ────────────────────────

func TestProject_computed_add(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_computed_add",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "revenue", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_conditional_cond_array_form(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_conditional_cond_array_form",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "priceLabel", Value: bson.D{{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$gt", Value: bson.A{"$price", float64(1)}}},
						"expensive",
						"cheap",
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_ifNull_default(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_ifNull_default",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "discount", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$discount", float64(0)}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_string_toLower_name(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_string_toLower_name",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "nameLower", Value: bson.D{{Key: "$toLower", Value: "$name"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_string_concat_category_name(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_string_concat_category_name",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "label", Value: bson.D{{Key: "$concat", Value: bson.A{"$category", "/", "$name"}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_size_tags_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_size_tags_array",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "tagCount", Value: bson.D{{Key: "$size", Value: "$tags"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_arrayElemAt_first_tag(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_arrayElemAt_first_tag",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "firstTag", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{"$tags", int32(0)}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_switch_price_tier(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_switch_price_tier",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "tier", Value: bson.D{{Key: "$switch", Value: bson.D{
						{Key: "branches", Value: bson.A{
							bson.D{{Key: "case", Value: bson.D{{Key: "$gte", Value: bson.A{"$price", float64(2)}}}}, {Key: "then", Value: "premium"}},
							bson.D{{Key: "case", Value: bson.D{{Key: "$gte", Value: bson.A{"$price", float64(1)}}}}, {Key: "then", Value: "standard"}},
						}},
						{Key: "default", Value: "budget"},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_computed_revenue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_computed_revenue",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "revenue", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "revenue", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_category_upper(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_category_upper",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "catUpper", Value: bson.D{{Key: "$toUpper", Value: "$category"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "catUpper", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_is_expensive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_is_expensive",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "expensive", Value: bson.D{{Key: "$gt", Value: bson.A{"$price", float64(1)}}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "expensive", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_tag_count_and_first(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_tag_count_and_first",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "numTags", Value: bson.D{{Key: "$size", Value: "$tags"}}},
					{Key: "primaryTag", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{"$tags", int32(0)}}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "numTags", Value: 1}, {Key: "primaryTag", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_set_alias_expr(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_set_alias_expr",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$set", Value: bson.D{
					{Key: "fullLabel", Value: bson.D{{Key: "$concat", Value: bson.A{"$category", ": ", "$name"}}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "fullLabel", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_then_group(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_then_group",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "revenue", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$category"},
					{Key: "totalRevenue", Value: bson.D{{Key: "$sum", Value: "$revenue"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_multiple_computed_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_multiple_computed_fields",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$addFields", Value: bson.D{
					{Key: "revenue", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
					{Key: "discounted", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", 0.9}}}},
					{Key: "label", Value: bson.D{{Key: "$toUpper", Value: "$name"}}},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "revenue", Value: 1},
					{Key: "discounted", Value: 1},
					{Key: "label", Value: 1},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_exclude_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_exclude_field",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "tags", Value: 0},
					{Key: "qty", Value: 0},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_computed_boolean(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_computed_boolean",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "isFruit", Value: bson.D{{Key: "$eq", Value: bson.A{"$category", "fruit"}}}},
					{Key: "hasMultipleTags", Value: bson.D{{Key: "$gt", Value: bson.A{
						bson.D{{Key: "$size", Value: "$tags"}},
						int32(1),
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_toInt_price(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_toInt_price",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "priceInt", Value: bson.D{{Key: "$toInt", Value: "$price"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_toString_qty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_toString_qty",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "qtyStr", Value: bson.D{{Key: "$toString", Value: "$qty"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_in_array_check(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_in_array_check",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "isSweet", Value: bson.D{{Key: "$in", Value: bson.A{"sweet", "$tags"}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_mergeObjects_with_extra(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_mergeObjects_with_extra",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "info", Value: bson.D{{Key: "$mergeObjects", Value: bson.A{
						bson.D{{Key: "name", Value: "$name"}, {Key: "category", Value: "$category"}},
						bson.D{{Key: "source", Value: "catalog"}},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_let_vars_expr(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_let_vars_expr",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "adjusted", Value: bson.D{{Key: "$let", Value: bson.D{
						{Key: "vars", Value: bson.D{{Key: "markup", Value: 1.1}}},
						{Key: "in", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$$markup"}}}},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_filter_tags(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_filter_tags",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "longTags", Value: bson.D{{Key: "$filter", Value: bson.D{
						{Key: "input", Value: "$tags"},
						{Key: "as", Value: "t"},
						{Key: "cond", Value: bson.D{{Key: "$gte", Value: bson.A{
							bson.D{{Key: "$strLenCP", Value: "$$t"}},
							int32(5),
						}}}},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_map_tags_upper(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_map_tags_upper",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "upperTags", Value: bson.D{{Key: "$map", Value: bson.D{
						{Key: "input", Value: "$tags"},
						{Key: "as", Value: "t"},
						{Key: "in", Value: bson.D{{Key: "$toUpper", Value: "$$t"}}},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_literal_constant(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_literal_constant",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "source", Value: bson.D{{Key: "$literal", Value: "catalog"}}},
					{Key: "version", Value: bson.D{{Key: "$literal", Value: int32(1)}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}

func TestProject_and_or_in_project(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_and_or_in_project",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "affordable", Value: bson.D{{Key: "$and", Value: bson.A{
						bson.D{{Key: "$lt", Value: bson.A{"$price", float64(2)}}},
						bson.D{{Key: "$gt", Value: bson.A{"$qty", int32(5)}}},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestAddFields_then_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AddFields_then_match",
		Support: harness.DongoXFail,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$addFields", Value: bson.D{
					{Key: "revenue", Value: bson.D{{Key: "$multiply", Value: bson.A{"$price", "$qty"}}}},
				}}},
				{{Key: "$match", Value: bson.D{{Key: "revenue", Value: bson.D{{Key: "$gt", Value: float64(20)}}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "revenue", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestProject_slice_tags(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Project_slice_tags",
		Support: harness.DongoFull,
		Setup:   insertAggSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "oneTag", Value: bson.D{{Key: "$slice", Value: bson.A{"$tags", int32(1)}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, nil
			}
			return results[0], nil
		},
	})
}
