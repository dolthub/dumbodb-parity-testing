package tests

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Explain helpers
//
// runExplain wraps a sub-command in {explain: ..., verbosity: ...} and runs it.
// extractPlan walks a winningPlan/executionStages tree and returns an ordered
// summary of (stage, indexName) pairs along the inputStage chain.
// explainCritical pulls just the fields we care about for cross-impl parity:
// the stage chain, the optional executionStats counters, and ok.

// runExplain runs an explain command for the given inner command at the given verbosity.
func explainRunExplain(ctx context.Context, col *mongo.Collection, inner bson.D, verbosity string) (bson.D, error) {
	cmd := bson.D{
		{Key: "explain", Value: inner},
		{Key: "verbosity", Value: verbosity},
	}
	result := col.Database().RunCommand(ctx, cmd)
	var doc bson.D
	if err := result.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// lookup returns the value associated with key in a bson.D, preserving order semantics.
func lookup(d bson.D, key string) (interface{}, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// asDoc coerces a bson value to bson.D when possible.
func asDoc(v interface{}) (bson.D, bool) {
	switch t := v.(type) {
	case bson.D:
		return t, true
	case bson.M:
		out := make(bson.D, 0, len(t))
		for k, vv := range t {
			out = append(out, bson.E{Key: k, Value: vv})
		}
		return out, true
	}
	return nil, false
}

// extractPlan walks a plan tree (winningPlan or executionStages) and returns
// a flat slice describing each stage and the index it touches, if any.
// The chain follows inputStage / inputStages — for OR/AND nodes, every branch
// is recorded in stable order.
func explainExtractPlan(plan interface{}) []bson.D {
	out := []bson.D{}
	doc, ok := asDoc(plan)
	if !ok {
		return out
	}

	entry := bson.D{}
	if v, ok := lookup(doc, "stage"); ok {
		entry = append(entry, bson.E{Key: "stage", Value: v})
	}
	if v, ok := lookup(doc, "indexName"); ok {
		entry = append(entry, bson.E{Key: "indexName", Value: v})
	}
	if v, ok := lookup(doc, "keyPattern"); ok {
		entry = append(entry, bson.E{Key: "keyPattern", Value: v})
	}
	if len(entry) > 0 {
		out = append(out, entry)
	}

	if v, ok := lookup(doc, "inputStage"); ok {
		out = append(out, explainExtractPlan(v)...)
	}
	if v, ok := lookup(doc, "inputStages"); ok {
		if arr, ok := v.(bson.A); ok {
			for _, child := range arr {
				out = append(out, explainExtractPlan(child)...)
			}
		}
	}
	return out
}

// explainCritical reduces a full explain document to the comparable subset:
// the winningPlan stage chain plus, when present, executionStats counters.
func explainExtractCritical(doc bson.D) bson.D {
	out := bson.D{}

	if qp, ok := lookup(doc, "queryPlanner"); ok {
		if qpDoc, ok := asDoc(qp); ok {
			if wp, ok := lookup(qpDoc, "winningPlan"); ok {
				out = append(out, bson.E{Key: "winningPlan", Value: explainExtractPlan(wp)})
			}
		}
	}

	if es, ok := lookup(doc, "executionStats"); ok {
		if esDoc, ok := asDoc(es); ok {
			stats := bson.D{}
			for _, k := range []string{"nReturned", "totalKeysExamined", "totalDocsExamined"} {
				if v, ok := lookup(esDoc, k); ok {
					stats = append(stats, bson.E{Key: k, Value: v})
				}
			}
			out = append(out, bson.E{Key: "executionStats", Value: stats})
			if stages, ok := lookup(esDoc, "executionStages"); ok {
				out = append(out, bson.E{Key: "executionStages", Value: explainExtractPlan(stages)})
			}
		}
	}

	if v, ok := lookup(doc, "ok"); ok {
		out = append(out, bson.E{Key: "ok", Value: v})
	}
	return out
}

// explainTestDocs is the standard dataset used across explain tests.
// 20 documents with a mix of fields used to demonstrate each plan stage.
func explainTestDocs() []interface{} {
	docs := make([]interface{}, 0, 20)
	cities := []string{"NYC", "LA", "Chicago", "Houston"}
	for i := 1; i <= 20; i++ {
		docs = append(docs, bson.D{
			{Key: "_id", Value: fmt.Sprintf("ex%02d", i)},
			{Key: "n", Value: int32(i)},
			{Key: "name", Value: fmt.Sprintf("name%02d", i)},
			{Key: "city", Value: cities[i%len(cities)]},
			{Key: "score", Value: int32((i * 7) % 100)},
		})
	}
	return docs
}

func insertExplainDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, explainTestDocs())
	return err
}

// createIndex is a tiny helper for tests that need to install an index before explain.
func createIndex(ctx context.Context, col *mongo.Collection, keys bson.D) error {
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys})
	return err
}

func TestExplain_COLLSCAN_NoIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_COLLSCAN_NoIndex",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: int32(5)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_IXSCAN_Equality(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_IXSCAN_Equality",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: int32(7)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_IXSCAN_Range(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_IXSCAN_Range",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$gt", Value: int32(10)}}}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_FETCH_After_IXSCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_FETCH_After_IXSCAN",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Filter on indexed field, project a non-indexed field → must FETCH.
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: int32(3)}}},
				{Key: "projection", Value: bson.D{{Key: "name", Value: int32(1)}, {Key: "_id", Value: int32(0)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_CoveredQuery(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_CoveredQuery",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Project only the indexed field (and exclude _id) → covered, no FETCH.
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$gte", Value: int32(5)}}}}},
				{Key: "projection", Value: bson.D{{Key: "n", Value: int32(1)}, {Key: "_id", Value: int32(0)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_SORT_InMemory(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_SORT_InMemory",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "sort", Value: bson.D{{Key: "score", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_SORT_ViaIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_SORT_ViaIndex",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "score", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "sort", Value: bson.D{{Key: "score", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_SORT_FETCH_IXSCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_SORT_FETCH_IXSCAN",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Filter on indexed n, sort on non-indexed score → SORT → FETCH → IXSCAN.
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$gt", Value: int32(5)}}}}},
				{Key: "sort", Value: bson.D{{Key: "score", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_LIMIT(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_LIMIT",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "limit", Value: int64(3)},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_SKIP(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_SKIP",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "skip", Value: int64(5)},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_SKIP_LIMIT(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_SKIP_LIMIT",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "skip", Value: int64(2)},
				{Key: "limit", Value: int64(4)},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_PROJECTION_SIMPLE(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_PROJECTION_SIMPLE",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
				{Key: "projection", Value: bson.D{{Key: "name", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_PROJECTION_COVERED(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_PROJECTION_COVERED",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "score", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}},
				{Key: "projection", Value: bson.D{{Key: "score", Value: int32(1)}, {Key: "_id", Value: int32(0)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_COUNT_SCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_COUNT_SCAN",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "count", Value: col.Name()},
				{Key: "query", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$lte", Value: int32(10)}}}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_COUNT_COLLSCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_COUNT_COLLSCAN",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "count", Value: col.Name()},
				{Key: "query", Value: bson.D{{Key: "city", Value: "NYC"}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_DISTINCT_SCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_DISTINCT_SCAN",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "city", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "distinct", Value: col.Name()},
				{Key: "key", Value: "city"},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_OR_MultiIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_OR_MultiIndex",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			if err := createIndex(ctx, col, bson.D{{Key: "n", Value: 1}}); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "city", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "$or", Value: bson.A{
					bson.D{{Key: "n", Value: int32(3)}},
					bson.D{{Key: "city", Value: "NYC"}},
				}}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_AND_SORTED(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_AND_SORTED",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			if err := createIndex(ctx, col, bson.D{{Key: "n", Value: 1}}); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "score", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{
					{Key: "n", Value: bson.D{{Key: "$gt", Value: int32(2)}}},
					{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(10)}}},
				}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_CompoundIndex_PrefixMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_CompoundIndex_PrefixMatch",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{
				{Key: "city", Value: 1},
				{Key: "score", Value: 1},
			})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "city", Value: "NYC"}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_CompoundIndex_FullMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_CompoundIndex_FullMatch",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{
				{Key: "city", Value: 1},
				{Key: "score", Value: 1},
			})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{
					{Key: "city", Value: "NYC"},
					{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(0)}}},
				}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_CompoundIndex_SortCovered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_CompoundIndex_SortCovered",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{
				{Key: "city", Value: 1},
				{Key: "score", Value: 1},
			})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Filter on prefix, sort on suffix → index covers both, no SORT stage.
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "city", Value: "NYC"}}},
				{Key: "sort", Value: bson.D{{Key: "score", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_Aggregate_MatchIXSCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Aggregate_MatchIXSCAN",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "aggregate", Value: col.Name()},
				{Key: "pipeline", Value: bson.A{
					bson.D{{Key: "$match", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$gt", Value: int32(10)}}}}}},
				}},
				{Key: "cursor", Value: bson.D{}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_Aggregate_COLLSCAN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Aggregate_COLLSCAN",
		Support: harness.DumboDBFull,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "aggregate", Value: col.Name()},
				{Key: "pipeline", Value: bson.A{
					bson.D{{Key: "$match", Value: bson.D{{Key: "city", Value: "LA"}}}},
				}},
				{Key: "cursor", Value: bson.D{}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_ExecutionStats_Indexed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_ExecutionStats_Indexed",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: int32(7)}}},
			}, "executionStats")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_ExecutionStats_FullScan(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_ExecutionStats_FullScan",
		Support: harness.DumboDBXFail,
		Setup:   insertExplainDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{}},
			}, "executionStats")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_Hint_ForcesIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Hint_ForcesIndex",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			if err := createIndex(ctx, col, bson.D{{Key: "n", Value: 1}}); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "score", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Equality on n would normally pick the n index; force the score index instead.
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: int32(7)}}},
				{Key: "hint", Value: bson.D{{Key: "score", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

func TestExplain_Hint_Natural(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_Hint_Natural",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertExplainDocs(ctx, col); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "n", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $natural hint forces COLLSCAN even though n is indexed.
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "n", Value: int32(7)}}},
				{Key: "hint", Value: bson.D{{Key: "$natural", Value: int32(1)}}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}
