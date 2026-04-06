package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/docudolt-parity-testing/harness"
)

// ─── Seed data ────────────────────────────────────────────────────────────────

// complexSeedDocs: orders with items, customer, and region fields.
var complexSeedDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "o1"},
		{Key: "customer", Value: "alice"},
		{Key: "region", Value: "west"},
		{Key: "status", Value: "shipped"},
		{Key: "total", Value: 120.0},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "sku", Value: "A"}, {Key: "qty", Value: int32(2)}, {Key: "price", Value: 30.0}},
			bson.D{{Key: "sku", Value: "B"}, {Key: "qty", Value: int32(2)}, {Key: "price", Value: 30.0}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "o2"},
		{Key: "customer", Value: "bob"},
		{Key: "region", Value: "east"},
		{Key: "status", Value: "pending"},
		{Key: "total", Value: 80.0},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "sku", Value: "A"}, {Key: "qty", Value: int32(4)}, {Key: "price", Value: 20.0}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "o3"},
		{Key: "customer", Value: "alice"},
		{Key: "region", Value: "west"},
		{Key: "status", Value: "shipped"},
		{Key: "total", Value: 200.0},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "sku", Value: "C"}, {Key: "qty", Value: int32(5)}, {Key: "price", Value: 40.0}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "o4"},
		{Key: "customer", Value: "carol"},
		{Key: "region", Value: "east"},
		{Key: "status", Value: "cancelled"},
		{Key: "total", Value: 50.0},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "sku", Value: "B"}, {Key: "qty", Value: int32(1)}, {Key: "price", Value: 50.0}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "o5"},
		{Key: "customer", Value: "bob"},
		{Key: "region", Value: "west"},
		{Key: "status", Value: "shipped"},
		{Key: "total", Value: 300.0},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "sku", Value: "C"}, {Key: "qty", Value: int32(3)}, {Key: "price", Value: 100.0}},
		}},
	},
}

func insertComplexSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, complexSeedDocs)
	return err
}

// graphNodes: tree with parent references for $graphLookup tests.
var graphNodes = []interface{}{
	bson.D{{Key: "_id", Value: "root"}, {Key: "name", Value: "root"}, {Key: "parent", Value: nil}},
	bson.D{{Key: "_id", Value: "a"}, {Key: "name", Value: "a"}, {Key: "parent", Value: "root"}},
	bson.D{{Key: "_id", Value: "b"}, {Key: "name", Value: "b"}, {Key: "parent", Value: "root"}},
	bson.D{{Key: "_id", Value: "a1"}, {Key: "name", Value: "a1"}, {Key: "parent", Value: "a"}},
	bson.D{{Key: "_id", Value: "a2"}, {Key: "name", Value: "a2"}, {Key: "parent", Value: "a"}},
	bson.D{{Key: "_id", Value: "b1"}, {Key: "name", Value: "b1"}, {Key: "parent", Value: "b"}},
}

func insertGraphNodes(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, graphNodes)
	return err
}

// redactDocs: documents with sensitivity field for $redact tests.
var redactDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "r1"},
		{Key: "level", Value: "public"},
		{Key: "data", Value: "public info"},
		{Key: "nested", Value: bson.D{
			{Key: "level", Value: "secret"},
			{Key: "data", Value: "secret info"},
		}},
	},
	bson.D{
		{Key: "_id", Value: "r2"},
		{Key: "level", Value: "public"},
		{Key: "data", Value: "another public"},
	},
}

func insertRedactDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, redactDocs)
	return err
}

// ─── Multi-stage: $match + $group + $sort + $project ─────────────────────────

func TestAggComplex_matchGroupSortProject(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchGroupSortProject",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "status", Value: "shipped"}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$customer"},
					{Key: "totalSpent", Value: bson.D{{Key: "$sum", Value: "$total"}}},
					{Key: "orderCount", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "customer", Value: "$_id"},
					{Key: "totalSpent", Value: 1},
					{Key: "orderCount", Value: 1},
					{Key: "_id", Value: 0},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_matchGroupSort_regionTotals(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchGroupSort_regionTotals",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "status", Value: bson.D{{Key: "$ne", Value: "cancelled"}}}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$region"},
					{Key: "totalRevenue", Value: bson.D{{Key: "$sum", Value: "$total"}}},
					{Key: "avgOrder", Value: bson.D{{Key: "$avg", Value: "$total"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "totalRevenue", Value: -1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_matchGroupProject_minMax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchGroupProject_minMax",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$customer"},
					{Key: "minOrder", Value: bson.D{{Key: "$min", Value: "$total"}}},
					{Key: "maxOrder", Value: bson.D{{Key: "$max", Value: "$total"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "customer", Value: "$_id"},
					{Key: "minOrder", Value: 1},
					{Key: "maxOrder", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_matchGroupProject_pushArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchGroupProject_pushArray",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "status", Value: "shipped"}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$region"},
					{Key: "customers", Value: bson.D{{Key: "$push", Value: "$customer"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_matchGroupProject_addToSet(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchGroupProject_addToSet",
		Support: harness.DocudoltXFail, // $addToSet element ordering is non-deterministic; docudolt ordering diverges from MongoDB
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$region"},
					{Key: "uniqueStatuses", Value: bson.D{{Key: "$addToSet", Value: "$status"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── Multi-stage: $unwind + $group ───────────────────────────────────────────

func TestAggComplex_unwindGroup_skuTotals(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_unwindGroup_skuTotals",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$items.sku"},
					{Key: "totalQty", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_unwindGroup_itemRevenue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_unwindGroup_itemRevenue",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$project", Value: bson.D{
					{Key: "sku", Value: "$items.sku"},
					{Key: "lineTotal", Value: bson.D{{Key: "$multiply", Value: bson.A{"$items.qty", "$items.price"}}}},
				}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$sku"},
					{Key: "revenue", Value: bson.D{{Key: "$sum", Value: "$lineTotal"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_unwindGroup_customerItems(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_unwindGroup_customerItems",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: bson.D{{Key: "customer", Value: "$customer"}, {Key: "sku", Value: "$items.sku"}}},
					{Key: "totalQty", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_unwindWithIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_unwindWithIndex",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "o1"}}}},
				{{Key: "$unwind", Value: bson.D{
					{Key: "path", Value: "$items"},
					{Key: "includeArrayIndex", Value: "itemIndex"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "sku", Value: "$items.sku"},
					{Key: "itemIndex", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── Multi-stage: $lookup + $unwind + $group ─────────────────────────────────

func TestAggComplex_lookupUnwindGroup(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_lookupUnwindGroup",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertComplexSeed(ctx, col); err != nil {
				return err
			}
			// Insert product catalog into a second collection (same DB).
			db := col.Database()
			products := db.Collection("products_lookup")
			_ = products.Drop(ctx)
			_, err := products.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "A"}, {Key: "name", Value: "Widget A"}, {Key: "category", Value: "widgets"}},
				bson.D{{Key: "_id", Value: "B"}, {Key: "name", Value: "Widget B"}, {Key: "category", Value: "widgets"}},
				bson.D{{Key: "_id", Value: "C"}, {Key: "name", Value: "Gadget C"}, {Key: "category", Value: "gadgets"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "products_lookup"},
					{Key: "localField", Value: "items.sku"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "product"},
				}}},
				{{Key: "$unwind", Value: "$product"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$product.category"},
					{Key: "totalQty", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_lookupNoMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_lookupNoMatch",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertComplexSeed(ctx, col); err != nil {
				return err
			}
			db := col.Database()
			other := db.Collection("empty_ref")
			_ = other.Drop(ctx)
			_, err := other.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "X"}, {Key: "val", Value: 99}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "o1"}}}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "empty_ref"},
					{Key: "localField", Value: "customer"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "refs"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "refsCount", Value: bson.D{{Key: "$size", Value: "$refs"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_lookupPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_lookupPipeline",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := insertComplexSeed(ctx, col); err != nil {
				return err
			}
			db := col.Database()
			inv := db.Collection("inventory_lkp")
			_ = inv.Drop(ctx)
			_, err := inv.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "A"}, {Key: "stock", Value: int32(100)}},
				bson.D{{Key: "_id", Value: "B"}, {Key: "stock", Value: int32(50)}},
				bson.D{{Key: "_id", Value: "C"}, {Key: "stock", Value: int32(200)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "o1"}}}},
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "inventory_lkp"},
					{Key: "let", Value: bson.D{{Key: "sku", Value: "$items.sku"}}},
					{Key: "pipeline", Value: bson.A{
						bson.D{{Key: "$match", Value: bson.D{
							{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$_id", "$$sku"}}}},
						}}},
						bson.D{{Key: "$project", Value: bson.D{{Key: "stock", Value: 1}, {Key: "_id", Value: 0}}}},
					}},
					{Key: "as", Value: "inventoryInfo"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "sku", Value: "$items.sku"},
					{Key: "inventoryInfo", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $facet ───────────────────────────────────────────────────────────────────

func TestAggComplex_facet_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_facet_basic",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$facet", Value: bson.D{
					{Key: "byStatus", Value: bson.A{
						bson.D{{Key: "$group", Value: bson.D{
							{Key: "_id", Value: "$status"},
							{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
						}}},
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					}},
					{Key: "byRegion", Value: bson.A{
						bson.D{{Key: "$group", Value: bson.D{
							{Key: "_id", Value: "$region"},
							{Key: "total", Value: bson.D{{Key: "$sum", Value: "$total"}}},
						}}},
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_facet_withBucket(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_facet_withBucket",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$facet", Value: bson.D{
					{Key: "priceBuckets", Value: bson.A{
						bson.D{{Key: "$bucket", Value: bson.D{
							{Key: "groupBy", Value: "$total"},
							{Key: "boundaries", Value: bson.A{0, 100, 200, 400}},
							{Key: "default", Value: "other"},
							{Key: "output", Value: bson.D{
								{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
								{Key: "totalRevenue", Value: bson.D{{Key: "$sum", Value: "$total"}}},
							}},
						}}},
					}},
					{Key: "topCustomers", Value: bson.A{
						bson.D{{Key: "$group", Value: bson.D{
							{Key: "_id", Value: "$customer"},
							{Key: "total", Value: bson.D{{Key: "$sum", Value: "$total"}}},
						}}},
						bson.D{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
						bson.D{{Key: "$limit", Value: 3}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_facet_withMatchAndCount(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_facet_withMatchAndCount",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$facet", Value: bson.D{
					{Key: "shippedCount", Value: bson.A{
						bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: "shipped"}}}},
						bson.D{{Key: "$count", Value: "n"}},
					}},
					{Key: "pendingCount", Value: bson.A{
						bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: "pending"}}}},
						bson.D{{Key: "$count", Value: "n"}},
					}},
					{Key: "cancelledCount", Value: bson.A{
						bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: "cancelled"}}}},
						bson.D{{Key: "$count", Value: "n"}},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $graphLookup ─────────────────────────────────────────────────────────────

func TestAggComplex_graphLookup_simpleTree(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_simpleTree",
		Support: harness.DocudoltFull,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find all descendants of "root".
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "root"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$_id"},
					{Key: "connectFromField", Value: "_id"},
					{Key: "connectToField", Value: "parent"},
					{Key: "as", Value: "descendants"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "descendantCount", Value: bson.D{{Key: "$size", Value: "$descendants"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_graphLookup_ancestors(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_ancestors",
		Support: harness.DocudoltFull,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Walk up from "a1" to find all ancestors.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$parent"},
					{Key: "connectFromField", Value: "parent"},
					{Key: "connectToField", Value: "_id"},
					{Key: "as", Value: "ancestors"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "ancestorCount", Value: bson.D{{Key: "$size", Value: "$ancestors"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_graphLookup_maxDepth(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_maxDepth",
		Support: harness.DocudoltFull,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "root"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$_id"},
					{Key: "connectFromField", Value: "_id"},
					{Key: "connectToField", Value: "parent"},
					{Key: "as", Value: "children"},
					{Key: "maxDepth", Value: 1},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "directChildCount", Value: bson.D{{Key: "$size", Value: "$children"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_graphLookup_depthField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_depthField",
		Support: harness.DocudoltFull,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "root"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$_id"},
					{Key: "connectFromField", Value: "_id"},
					{Key: "connectToField", Value: "parent"},
					{Key: "as", Value: "descendants"},
					{Key: "depthField", Value: "depth"},
				}}},
				{{Key: "$unwind", Value: "$descendants"}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: "$descendants.name"},
					{Key: "depth", Value: "$descendants.depth"},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_graphLookup_restrictSearch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_restrictSearch",
		Support: harness.DocudoltFull,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Only follow nodes whose name doesn't start with "b".
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "root"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$_id"},
					{Key: "connectFromField", Value: "_id"},
					{Key: "connectToField", Value: "parent"},
					{Key: "as", Value: "descendants"},
					{Key: "restrictSearchWithMatch", Value: bson.D{
						{Key: "name", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$regex", Value: "^b"}}}}},
					}},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "descendantCount", Value: bson.D{{Key: "$size", Value: "$descendants"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_graphLookup_withCycles(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_withCycles",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// Create a graph with a cycle: X -> Y -> Z -> X
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "X"}, {Key: "next", Value: "Y"}},
				bson.D{{Key: "_id", Value: "Y"}, {Key: "next", Value: "Z"}},
				bson.D{{Key: "_id", Value: "Z"}, {Key: "next", Value: "X"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $graphLookup handles cycles by not revisiting visited nodes.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "X"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$next"},
					{Key: "connectFromField", Value: "next"},
					{Key: "connectToField", Value: "_id"},
					{Key: "as", Value: "reachable"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "reachableCount", Value: bson.D{{Key: "$size", Value: "$reachable"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── $sample ──────────────────────────────────────────────────────────────────

func TestAggComplex_sample_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_sample_count",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sample", Value: bson.D{{Key: "size", Value: 3}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "sampledCount", Value: int32(len(results))}}, nil
		},
	})
}

func TestAggComplex_sample_one(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_sample_one",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sample", Value: bson.D{{Key: "size", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "sampledCount", Value: int32(len(results))}}, nil
		},
	})
}

func TestAggComplex_sample_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_sample_all",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sampling more than collection size returns all docs.
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sample", Value: bson.D{{Key: "size", Value: 100}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "sampledCount", Value: int32(len(results))}}, nil
		},
	})
}

// ─── $redact ──────────────────────────────────────────────────────────────────

func TestAggComplex_redact_prune(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_redact_prune",
		Support: harness.DocudoltFull,
		Setup:   insertRedactDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: bson.D{
					{Key: "$cond", Value: bson.D{
						{Key: "if", Value: bson.D{{Key: "$eq", Value: bson.A{"$level", "secret"}}}},
						{Key: "then", Value: "$$PRUNE"},
						{Key: "else", Value: "$$DESCEND"},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_redact_keep(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_redact_keep",
		Support: harness.DocudoltFull,
		Setup:   insertRedactDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: bson.D{
					{Key: "$cond", Value: bson.D{
						{Key: "if", Value: bson.D{{Key: "$eq", Value: bson.A{"$level", "public"}}}},
						{Key: "then", Value: "$$KEEP"},
						{Key: "else", Value: "$$PRUNE"},
					}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_redact_descend(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_redact_descend",
		Support: harness.DocudoltFull,
		Setup:   insertRedactDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Always DESCEND — returns all documents unchanged.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: "$$DESCEND"}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_redact_pruneAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_redact_pruneAll",
		Support: harness.DocudoltFull,
		Setup:   insertRedactDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Always PRUNE — no documents returned.
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$redact", Value: "$$PRUNE"}},
			})
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

func TestAggComplex_redact_nestedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_redact_nestedField",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "_id", Value: "x1"},
					{Key: "clearance", Value: "high"},
					{Key: "payload", Value: bson.D{
						{Key: "clearance", Value: "low"},
						{Key: "secret", Value: "hidden"},
					}},
					{Key: "public", Value: "visible"},
				},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Prune sub-documents where clearance == "low".
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$redact", Value: bson.D{
					{Key: "$cond", Value: bson.D{
						{Key: "if", Value: bson.D{{Key: "$eq", Value: bson.A{"$clearance", "low"}}}},
						{Key: "then", Value: "$$PRUNE"},
						{Key: "else", Value: "$$DESCEND"},
					}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── Complex combinations ─────────────────────────────────────────────────────

func TestAggComplex_matchUnwindGroupSort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchUnwindGroupSort",
		Support: harness.DocudoltXFail, // sort tie-breaking with equal totalQty diverges: docudolt orders ties ascending by _id, MongoDB descending
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "status", Value: "shipped"}}}},
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$items.sku"},
					{Key: "totalQty", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
					{Key: "orderCount", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "totalQty", Value: -1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_projectAddFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_projectAddFields",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "o1"}}}},
				{{Key: "$addFields", Value: bson.D{
					{Key: "itemCount", Value: bson.D{{Key: "$size", Value: "$items"}}},
					{Key: "discounted", Value: bson.D{{Key: "$multiply", Value: bson.A{"$total", 0.9}}}},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "itemCount", Value: 1},
					{Key: "discounted", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_replaceRoot(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_replaceRoot",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "o1"}}}},
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$items"}}}},
				{{Key: "$sort", Value: bson.D{{Key: "sku", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_count_stage(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_count_stage",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "status", Value: "shipped"}}}},
				{{Key: "$count", Value: "shippedOrders"}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_bucket_auto(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_bucket_auto",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$bucketAuto", Value: bson.D{
					{Key: "groupBy", Value: "$total"},
					{Key: "buckets", Value: 3},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_sortByCount(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_sortByCount",
		Support: harness.DocudoltXFail, // $sortByCount tiebreaking order diverges from MongoDB
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sortByCount", Value: "$status"}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_multiGroup_then_sort_limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_multiGroup_then_sort_limit",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$customer"},
					{Key: "spent", Value: bson.D{{Key: "$sum", Value: "$total"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "spent", Value: -1}}}},
				{{Key: "$limit", Value: 2}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_matchGroupHaving(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchGroupHaving",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Simulate SQL HAVING: group, then filter groups.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$customer"},
					{Key: "orderCount", Value: bson.D{{Key: "$sum", Value: 1}}},
					{Key: "totalSpent", Value: bson.D{{Key: "$sum", Value: "$total"}}},
				}}},
				{{Key: "$match", Value: bson.D{{Key: "orderCount", Value: bson.D{{Key: "$gt", Value: 1}}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_multiStage_skipLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_multiStage_skipLimit",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
				{{Key: "$skip", Value: 1}},
				{{Key: "$limit", Value: 2}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "total", Value: 1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_graphLookup_emptyStartWith(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_emptyStartWith",
		Support: harness.DocudoltFull,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Node with no children — result should be empty descendants array.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a1"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$_id"},
					{Key: "connectFromField", Value: "_id"},
					{Key: "connectToField", Value: "parent"},
					{Key: "as", Value: "children"},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "childCount", Value: bson.D{{Key: "$size", Value: "$children"}}},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

func TestAggComplex_setWindowFields_simple(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_setWindowFields_simple",
		Support: harness.DocudoltFull,
		Setup:   insertComplexSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$setWindowFields", Value: bson.D{
					{Key: "sortBy", Value: bson.D{{Key: "total", Value: 1}}},
					{Key: "output", Value: bson.D{
						{Key: "runningTotal", Value: bson.D{
							{Key: "$sum", Value: "$total"},
							{Key: "window", Value: bson.D{
								{Key: "documents", Value: bson.A{"unbounded", "current"}},
							}},
						}},
					}},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "total", Value: 1},
					{Key: "runningTotal", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}

// ─── targeted divergence tests ────────────────────────────────────────────────

// TestAggComplex_sortByCount_TieBreaking verifies $sortByCount with a three-way
// tie in counts. An explicit $sort on {count:-1, _id:1} after $sortByCount makes
// the tie-breaking deterministic across both MongoDB and Docudolt.
func TestAggComplex_sortByCount_TieBreaking(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_sortByCount_TieBreaking",
		Support: harness.DocudoltFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "cat", Value: "x"}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "cat", Value: "x"}},
				bson.D{{Key: "_id", Value: "d3"}, {Key: "cat", Value: "y"}},
				bson.D{{Key: "_id", Value: "d4"}, {Key: "cat", Value: "y"}},
				bson.D{{Key: "_id", Value: "d5"}, {Key: "cat", Value: "z"}},
				bson.D{{Key: "_id", Value: "d6"}, {Key: "cat", Value: "z"}},
				bson.D{{Key: "_id", Value: "d7"}, {Key: "cat", Value: "w"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// x=2, y=2, z=2, w=1 — tie at count=2 is resolved by ascending _id.
			// Per spec, $sortByCount is equivalent to $group+$sort{count:-1,_id:1};
			// the explicit $sort ensures a stable, comparable ordering.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sortByCount", Value: "$cat"}},
				{{Key: "$sort", Value: bson.D{{Key: "count", Value: int32(-1)}, {Key: "_id", Value: int32(1)}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// TestAggComplex_matchUnwindGroupSort_SameTotalQty captures the sort ordering
// divergence when grouped items have equal totalQty after unwind+group.
func TestAggComplex_matchUnwindGroupSort_SameTotalQty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_matchUnwindGroupSort_SameTotalQty",
		Support: harness.DocudoltXFail, // sort tie-breaking with equal totalQty diverges: docudolt orders ties ascending by _id, MongoDB has different implicit ordering
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{
					{Key: "_id", Value: "r1"},
					{Key: "status", Value: "active"},
					{Key: "items", Value: bson.A{
						bson.D{{Key: "sku", Value: "P"}, {Key: "qty", Value: int32(3)}},
						bson.D{{Key: "sku", Value: "Q"}, {Key: "qty", Value: int32(3)}},
					}},
				},
				bson.D{
					{Key: "_id", Value: "r2"},
					{Key: "status", Value: "active"},
					{Key: "items", Value: bson.A{
						bson.D{{Key: "sku", Value: "P"}, {Key: "qty", Value: int32(2)}},
						bson.D{{Key: "sku", Value: "R"}, {Key: "qty", Value: int32(5)}},
					}},
				},
				bson.D{
					{Key: "_id", Value: "r3"},
					{Key: "status", Value: "active"},
					{Key: "items", Value: bson.A{
						bson.D{{Key: "sku", Value: "Q"}, {Key: "qty", Value: int32(2)}},
						bson.D{{Key: "sku", Value: "R"}, {Key: "qty", Value: int32(0)}},
					}},
				},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// After grouping: P=5, Q=5, R=5 — three-way tie after summing qty.
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "status", Value: "active"}}}},
				{{Key: "$unwind", Value: "$items"}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$items.sku"},
					{Key: "totalQty", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "totalQty", Value: -1}}}},
			})
			return docsToSlice(results), err
		},
	})
}

// TestAggComplex_graphLookup_bfsOrder documents a docudolt divergence in
// $graphLookup result handling. Using $map to extract a field from the result
// array, MongoDB returns the name strings while docudolt returns nulls — it cannot
// resolve sub-field references ($$d.name) over the graphLookup output array.
// Related docudolt bug: do-0gb6 ($graphLookup result order / field access).
func TestAggComplex_graphLookup_bfsOrder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggComplex_graphLookup_bfsOrder",
		Support: harness.DocudoltXFail,
		Setup:   insertGraphNodes,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Collect all descendants of "root" and use $map to extract names.
			// MongoDB resolves $$d.name correctly from the graphLookup array.
			// Docudolt returns [null, null, ...] — sub-field access fails (do-0gb6).
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "_id", Value: "root"}}}},
				{{Key: "$graphLookup", Value: bson.D{
					{Key: "from", Value: col.Name()},
					{Key: "startWith", Value: "$_id"},
					{Key: "connectFromField", Value: "_id"},
					{Key: "connectToField", Value: "parent"},
					{Key: "as", Value: "descendants"},
				}}},
				{{Key: "$addFields", Value: bson.D{
					{Key: "descendantNames", Value: bson.D{
						{Key: "$map", Value: bson.D{
							{Key: "input", Value: "$descendants"},
							{Key: "as", Value: "d"},
							{Key: "in", Value: "$$d.name"},
						}},
					}},
				}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 1},
					{Key: "descendantNames", Value: 1},
				}}},
			})
			return docsToSlice(results), err
		},
	})
}
