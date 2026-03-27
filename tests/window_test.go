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
