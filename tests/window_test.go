package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// windowSeedDocs: sales records for $setWindowFields tests.
var windowSeedDocs = []interface{}{
	bson.D{{Key: "_id", Value: "w1"}, {Key: "dept", Value: "eng"}, {Key: "emp", Value: "alice"}, {Key: "salary", Value: 90000.0}, {Key: "year", Value: int32(2023)}},
	bson.D{{Key: "_id", Value: "w2"}, {Key: "dept", Value: "eng"}, {Key: "emp", Value: "bob"}, {Key: "salary", Value: 75000.0}, {Key: "year", Value: int32(2023)}},
	bson.D{{Key: "_id", Value: "w3"}, {Key: "dept", Value: "eng"}, {Key: "emp", Value: "carol"}, {Key: "salary", Value: 85000.0}, {Key: "year", Value: int32(2023)}},
	bson.D{{Key: "_id", Value: "w4"}, {Key: "dept", Value: "sales"}, {Key: "emp", Value: "dave"}, {Key: "salary", Value: 60000.0}, {Key: "year", Value: int32(2023)}},
	bson.D{{Key: "_id", Value: "w5"}, {Key: "dept", Value: "sales"}, {Key: "emp", Value: "eve"}, {Key: "salary", Value: 70000.0}, {Key: "year", Value: int32(2023)}},
	bson.D{{Key: "_id", Value: "w6"}, {Key: "dept", Value: "sales"}, {Key: "emp", Value: "frank"}, {Key: "salary", Value: 65000.0}, {Key: "year", Value: int32(2023)}},
}

func insertWindowSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, windowSeedDocs)
	return err
}

// ─── $rank ────────────────────────────────────────────────────────────────────

func TestWindow_rank_overall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_rank_overall",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: -1}}},
					{Key: "output", Value: bson.D{
						{Key: "rank", Value: bson.D{{Key: "$rank", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "emp", Value: 1}, {Key: "salary", Value: 1}, {Key: "rank", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_rank_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_rank_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: -1}}},
					{Key: "output", Value: bson.D{
						{Key: "deptRank", Value: bson.D{{Key: "$rank", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: -1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "deptRank", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $denseRank ───────────────────────────────────────────────────────────────

func TestWindow_denseRank_overall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_denseRank_overall",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: -1}}},
					{Key: "output", Value: bson.D{
						{Key: "denseRank", Value: bson.D{{Key: "$denseRank", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "denseRank", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_denseRank_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_denseRank_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "denseRank", Value: bson.D{{Key: "$denseRank", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "denseRank", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $rowNumber ───────────────────────────────────────────────────────────────

func TestWindow_rowNumber_overall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_rowNumber_overall",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "rowNum", Value: bson.D{{Key: "$documentNumber", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "rowNum", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_rowNumber_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_rowNumber_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: -1}}},
					{Key: "output", Value: bson.D{
						{Key: "rowNum", Value: bson.D{{Key: "$documentNumber", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: -1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "rowNum", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $sum over window ─────────────────────────────────────────────────────────

func TestWindow_sum_unboundedCumulative(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_sum_unboundedCumulative",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "cumSalary", Value: bson.D{
							{Key: "$sum", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "salary", Value: 1}, {Key: "cumSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_sum_partitioned_cumulative(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_sum_partitioned_cumulative",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "deptCumSalary", Value: bson.D{
							{Key: "$sum", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "deptCumSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_sum_full_partition(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_sum_full_partition",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "output", Value: bson.D{
						{Key: "deptTotalSalary", Value: bson.D{
							{Key: "$sum", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "deptTotalSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $avg over window ─────────────────────────────────────────────────────────

func TestWindow_avg_overall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_avg_overall",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "output", Value: bson.D{
						{Key: "overallAvgSalary", Value: bson.D{
							{Key: "$avg", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "overallAvgSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_avg_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_avg_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "output", Value: bson.D{
						{Key: "deptAvgSalary", Value: bson.D{
							{Key: "$avg", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "deptAvgSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $first / $last over window ───────────────────────────────────────────────

func TestWindow_first_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_first_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "lowestInDept", Value: bson.D{
							{Key: "$first", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "lowestInDept", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_last_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_last_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "highestInDept", Value: bson.D{
							{Key: "$last", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "highestInDept", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_first_overall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_first_overall",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "globalFirst", Value: bson.D{
							{Key: "$first", Value: "$emp"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "globalFirst", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_last_overall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_last_overall",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "globalLast", Value: bson.D{
							{Key: "$last", Value: "$emp"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "globalLast", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── partitionBy + sortBy combinations ───────────────────────────────────────

func TestWindow_partitionSortMultiOutput(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_partitionSortMultiOutput",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "rank", Value: bson.D{{Key: "$rank", Value: bson.D{}}}},
						{Key: "cumSalary", Value: bson.D{
							{Key: "$sum", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "dept", Value: 1},
					{Key: "emp", Value: 1},
					{Key: "rank", Value: 1},
					{Key: "cumSalary", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_partitionOnly_noSort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_partitionOnly_noSort",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "output", Value: bson.D{
						{Key: "count", Value: bson.D{
							{Key: "$sum", Value: 1},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "count", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── Range-based windows ──────────────────────────────────────────────────────

func TestWindow_range_currentToCurrent(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_range_currentToCurrent",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "windowSum", Value: bson.D{
							{Key: "$sum", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "range", Value: bson.A{"current", "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "salary", Value: 1}, {Key: "windowSum", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_range_minusN_to_current(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_range_minusN_to_current",
		Support: harness.DongoXFail,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sum salary within 10000 below current salary to current.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "nearbySum", Value: bson.D{
							{Key: "$sum", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "range", Value: bson.A{-10000, "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "salary", Value: 1}, {Key: "nearbySum", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_documents_trailing3(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_documents_trailing3",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sliding window: previous 2 documents + current.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "movingAvg", Value: bson.D{
							{Key: "$avg", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{-2, "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "salary", Value: 1}, {Key: "movingAvg", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_documents_leadLag(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_documents_leadLag",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Window from -1 to +1 (current row with one neighbor each side).
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "salary", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "neighborhood", Value: bson.D{
							{Key: "$avg", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{-1, 1}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "salary", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "emp", Value: 1}, {Key: "salary", Value: 1}, {Key: "neighborhood", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $min / $max over window ─────────────────────────────────────────────────

func TestWindow_min_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_min_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "output", Value: bson.D{
						{Key: "minSalary", Value: bson.D{
							{Key: "$min", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "minSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_max_partitioned(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_max_partitioned",
		Support: harness.DongoFull,
		Setup:   insertWindowSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$dept"},
					{Key: "output", Value: bson.D{
						{Key: "maxSalary", Value: bson.D{
							{Key: "$max", Value: "$salary"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "unbounded"}},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "dept", Value: 1}, {Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "dept", Value: 1}, {Key: "emp", Value: 1}, {Key: "maxSalary", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── null handling ────────────────────────────────────────────────────────────

func TestWindow_sumNullHandling(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_sumNullHandling",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "c"}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "total", Value: bson.D{
							{Key: "$sum", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "total", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_avgNullHandling(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_avgNullHandling",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "avg", Value: bson.D{
							{Key: "$avg", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "avg", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_minNullHandling(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_minNullHandling",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "lo", Value: bson.D{
							{Key: "$min", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "lo", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_allNullSum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_allNullSum",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: nil}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "total", Value: bson.D{
							{Key: "$sum", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "total", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── offset windows ───────────────────────────────────────────────────────────

func TestWindow_numericOffsetWindow(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_numericOffsetWindow",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(4)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "prev2sum", Value: bson.D{
							{Key: "$sum", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{int32(-1), int32(0)}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "prev2sum", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_forwardOffsetWindow(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_forwardOffsetWindow",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(20)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(30)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "fwdSum", Value: bson.D{
							{Key: "$sum", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{int32(0), int32(1)}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "fwdSum", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── partition by multiple fields / edge cases ────────────────────────────────

func TestWindow_partitionByMultipleFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_partitionByMultipleFields",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "region", Value: "west"}, {Key: "score", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "region", Value: "west"}, {Key: "score", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "region", Value: "east"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "region", Value: "east"}, {Key: "score", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "region", Value: 1}, {Key: "score", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$region"},
					{Key: "sortBy", Value: bson.D{{Key: "score", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "rank", Value: bson.D{{Key: "$rank", Value: bson.D{}}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "region", Value: 1}, {Key: "rank", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_floatValues(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_floatValues",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: float64(1.5)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: float64(2.5)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: float64(3.0)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "total", Value: bson.D{
							{Key: "$sum", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
						{Key: "avg", Value: bson.D{
							{Key: "$avg", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "total", Value: 1}, {Key: "avg", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_singleDocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_singleDocument",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "only"}, {Key: "v", Value: int32(42)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "rank", Value: bson.D{{Key: "$rank", Value: bson.D{}}}},
						{Key: "denseRank", Value: bson.D{{Key: "$denseRank", Value: bson.D{}}}},
						{Key: "docNum", Value: bson.D{{Key: "$documentNumber", Value: bson.D{}}}},
						{Key: "total", Value: bson.D{
							{Key: "$sum", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $count window operator ───────────────────────────────────────────────────

func TestWindow_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_count",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "n", Value: bson.D{
							{Key: "$count", Value: bson.D{}},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "n", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $push and $addToSet window operators ─────────────────────────────────────

func TestWindow_push(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_push",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "all", Value: bson.D{
							{Key: "$push", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "all", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_addToSet(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_addToSet",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "unique", Value: bson.D{
							{Key: "$addToSet", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $shift window operator ───────────────────────────────────────────────────

func TestWindow_shift(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_shift",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(20)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(30)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "prevV", Value: bson.D{
							{Key: "$shift", Value: bson.D{
								{Key: "output", Value: "$v"},
								{Key: "by", Value: int32(-1)},
								{Key: "default", Value: nil},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: 1}, {Key: "prevV", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $stdDevPop and $stdDevSamp ───────────────────────────────────────────────

func TestWindow_stdDevPop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_stdDevPop",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(4)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(4)}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(4)}},
				bson.D{{Key: "_id", Value: "e"}, {Key: "v", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "f"}, {Key: "v", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "g"}, {Key: "v", Value: int32(7)}},
				bson.D{{Key: "_id", Value: "h"}, {Key: "v", Value: int32(9)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "stdPop", Value: bson.D{
							{Key: "$stdDevPop", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$limit", Value: int32(1)}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "stdPop", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_stdDevSamp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_stdDevSamp",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(4)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(4)}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "e"}, {Key: "v", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "f"}, {Key: "v", Value: int32(7)}},
				bson.D{{Key: "_id", Value: "g"}, {Key: "v", Value: int32(9)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "stdSamp", Value: bson.D{
							{Key: "$stdDevSamp", Value: "$v"},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$limit", Value: int32(1)}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "stdSamp", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $covariancePop ───────────────────────────────────────────────────────────

func TestWindow_covariancePop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_covariancePop",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "x", Value: int32(2)}, {Key: "y", Value: int32(4)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "x", Value: int32(3)}, {Key: "y", Value: int32(6)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "cov", Value: bson.D{
							{Key: "$covariancePop", Value: bson.A{"$x", "$y"}},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$limit", Value: int32(1)}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "cov", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $expMovingAvg ────────────────────────────────────────────────────────────

func TestWindow_expMovingAvg(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_expMovingAvg",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(3)}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(4)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "ema", Value: bson.D{
							{Key: "$expMovingAvg", Value: bson.D{
								{Key: "input", Value: "$v"},
								{Key: "N", Value: int32(2)},
							}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $derivative and $integral ────────────────────────────────────────────────

func TestWindow_derivative(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_derivative",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "pos", Value: int32(0)}, {Key: "t", Value: int32(0)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "pos", Value: int32(10)}, {Key: "t", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "pos", Value: int32(30)}, {Key: "t", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "t", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "vel", Value: bson.D{
							{Key: "$derivative", Value: bson.D{
								{Key: "input", Value: "$pos"},
								{Key: "unit", Value: "second"},
							}},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{int32(-1), int32(0)}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_integral(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_integral",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}, {Key: "t", Value: int32(0)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(3)}, {Key: "t", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(5)}, {Key: "t", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "t", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "area", Value: bson.D{
							{Key: "$integral", Value: bson.D{
								{Key: "input", Value: "$v"},
								{Key: "unit", Value: "second"},
							}},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "current"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $linearFill and $locf ────────────────────────────────────────────────────

func TestWindow_linearFill(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_linearFill",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: int32(30)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "filled", Value: bson.D{{Key: "$linearFill", Value: "$v"}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "filled", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_locf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_locf",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "e"}, {Key: "v", Value: nil}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "filled", Value: bson.D{{Key: "$locf", Value: "$v"}}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "filled", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $top and $bottom ─────────────────────────────────────────────────────────

func TestWindow_top(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_top",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "score", Value: int32(10)}, {Key: "name", Value: "alice"}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "score", Value: int32(30)}, {Key: "name", Value: "bob"}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "score", Value: int32(20)}, {Key: "name", Value: "charlie"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "score", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "topName", Value: bson.D{
							{Key: "$top", Value: bson.D{
								{Key: "output", Value: "$name"},
								{Key: "sortBy", Value: bson.D{{Key: "score", Value: -1}}},
							}},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "topName", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestWindow_bottom(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Window_bottom",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "score", Value: int32(10)}, {Key: "name", Value: "alice"}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "score", Value: int32(30)}, {Key: "name", Value: "bob"}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "score", Value: int32(20)}, {Key: "name", Value: "charlie"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "score", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "bottomName", Value: bson.D{
							{Key: "$bottom", Value: bson.D{
								{Key: "output", Value: "$name"},
								{Key: "sortBy", Value: bson.D{{Key: "score", Value: 1}}},
							}},
							{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "unbounded"}}}},
						}},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "bottomName", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}
