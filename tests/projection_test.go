package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// projDocs are the shared seed docs for projection and sort tests.
// They include nested fields, arrays, nulls, and missing fields for edge-case coverage.
var projDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "p1"},
		{Key: "name", Value: "Alice"},
		{Key: "rank", Value: int32(3)},
		{Key: "active", Value: true},
		{Key: "dept", Value: "eng"},
		{Key: "addr", Value: bson.D{
			{Key: "city", Value: "Seattle"},
			{Key: "zip", Value: "98101"},
			{Key: "state", Value: "WA"},
		}},
		{Key: "scores", Value: bson.A{int32(85), int32(92), int32(78)}},
		{Key: "tags", Value: bson.A{"go", "db", "cloud"}},
	},
	bson.D{
		{Key: "_id", Value: "p2"},
		{Key: "name", Value: "Bob"},
		{Key: "rank", Value: int32(1)},
		{Key: "active", Value: false},
		{Key: "dept", Value: "ops"},
		{Key: "addr", Value: bson.D{
			{Key: "city", Value: "Portland"},
			{Key: "zip", Value: "97201"},
			{Key: "state", Value: "OR"},
		}},
		{Key: "scores", Value: bson.A{int32(60), int32(70), int32(80)}},
		{Key: "tags", Value: bson.A{"ops", "linux"}},
	},
	bson.D{
		{Key: "_id", Value: "p3"},
		{Key: "name", Value: "Carol"},
		{Key: "rank", Value: int32(2)},
		{Key: "active", Value: true},
		{Key: "dept", Value: nil},
		{Key: "addr", Value: bson.D{
			{Key: "city", Value: "Austin"},
			{Key: "zip", Value: "78701"},
			{Key: "state", Value: "TX"},
		}},
		{Key: "scores", Value: bson.A{int32(95), int32(88), int32(91)}},
		{Key: "tags", Value: bson.A{"db", "sql", "go"}},
	},
	bson.D{
		{Key: "_id", Value: "p4"},
		{Key: "name", Value: "Dave"},
		{Key: "rank", Value: int32(4)},
		{Key: "active", Value: true},
		// no "dept" field — missing entirely
		{Key: "addr", Value: bson.D{
			{Key: "city", Value: "Denver"},
			{Key: "zip", Value: "80201"},
			{Key: "state", Value: "CO"},
		}},
		{Key: "scores", Value: bson.A{int32(72), int32(68), int32(75)}},
		{Key: "tags", Value: bson.A{"cloud", "infra"}},
	},
	bson.D{
		{Key: "_id", Value: "p5"},
		{Key: "name", Value: "Eve"},
		{Key: "rank", Value: int32(5)},
		{Key: "active", Value: false},
		{Key: "dept", Value: "eng"},
		{Key: "addr", Value: bson.D{
			{Key: "city", Value: "Miami"},
			{Key: "zip", Value: "33101"},
			{Key: "state", Value: "FL"},
		}},
		{Key: "scores", Value: bson.A{int32(100), int32(99), int32(98)}},
		{Key: "tags", Value: bson.A{"go", "cloud", "ml"}},
	},
}

func insertProjDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, projDocs)
	return err
}

// runFindAll runs Find with the given filter and options and returns all docs sorted-stable.
func runFindAll(ctx context.Context, col *mongo.Collection, filter interface{}, opts *options.FindOptions) ([]interface{}, error) {
	cursor, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	result := make([]interface{}, len(docs))
	for i, d := range docs {
		result[i] = d
	}
	return result, nil
}

// ============================================================
// Projection — inclusion / exclusion basics
// ============================================================

func TestProjection_IncludeFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_IncludeFields",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ExcludeID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ExcludeID",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ExcludeField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ExcludeField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "scores", Value: 0}, {Key: "tags", Value: 0}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ExcludeIDAndAddr(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ExcludeIDAndAddr",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "addr", Value: 0}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_OnlyID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_OnlyID",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_EmptyProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_EmptyProjection",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_NonexistentField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_NonexistentField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "ghost", Value: 1}, {Key: "_id", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ArrayField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ArrayField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "tags", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_MultipleInclusions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_MultipleInclusions",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "rank", Value: 1},
					{Key: "active", Value: 1},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Projection — nested field dot-notation
// ============================================================

func TestProjection_NestedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_NestedField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "addr.city", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_NestedFieldExclude(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_NestedFieldExclude",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "addr.zip", Value: 0}, {Key: "addr.state", Value: 0}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_MultipleNestedFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_MultipleNestedFields",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 0},
					{Key: "addr.city", Value: 1},
					{Key: "addr.state", Value: 1},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_NestedAndTopLevel(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_NestedAndTopLevel",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 0},
					{Key: "name", Value: 1},
					{Key: "addr.city", Value: 1},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Projection — FindOne with projection
// ============================================================

func TestProjection_FindOne_IncludeFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_FindOne_IncludeFields",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}})
			var doc bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "p1"}}, opts).Decode(&doc)
			return doc, err
		},
	})
}

func TestProjection_FindOne_ExcludeField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_FindOne_ExcludeField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetProjection(bson.D{{Key: "scores", Value: 0}, {Key: "tags", Value: 0}})
			var doc bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "p2"}}, opts).Decode(&doc)
			return doc, err
		},
	})
}

func TestProjection_FindOne_NestedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_FindOne_NestedField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "addr.city", Value: 1}})
			var doc bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "p3"}}, opts).Decode(&doc)
			return doc, err
		},
	})
}

// ============================================================
// Projection — $slice operator
// ============================================================

func TestProjection_Slice_FirstN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Slice_FirstN",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "scores", Value: bson.D{{Key: "$slice", Value: int32(2)}}}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_Slice_LastN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Slice_LastN",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "scores", Value: bson.D{{Key: "$slice", Value: int32(-1)}}}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_Slice_SkipLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Slice_SkipLimit",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "scores", Value: bson.D{{Key: "$slice", Value: bson.A{int32(1), int32(2)}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_Slice_AllElements(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Slice_AllElements",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "tags", Value: bson.D{{Key: "$slice", Value: int32(10)}}}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_Slice_Tags_FirstOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Slice_Tags_FirstOne",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "tags", Value: bson.D{{Key: "$slice", Value: int32(1)}}}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_Slice_NegativeSkip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Slice_NegativeSkip",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "scores", Value: bson.D{{Key: "$slice", Value: bson.A{int32(-2), int32(1)}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Projection — $elemMatch in projection (DumboDBXFail)
// ============================================================

func TestProjection_ElemMatch_ScoresGte80(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ElemMatch_ScoresGte80",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "scores", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$gte", Value: int32(80)}}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ElemMatch_ScoresLt70(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ElemMatch_ScoresLt70",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "scores", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$lt", Value: int32(70)}}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ElemMatch_TagsValue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ElemMatch_TagsValue",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "tags", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: "go"}}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_ElemMatch_NoMatchInDoc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_ElemMatch_NoMatchInDoc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					// no score in these docs is 0; elemMatch returns nothing for non-matching docs
					{Key: "scores", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: int32(0)}}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Projection — positional $ operator (DumboDBXFail)
// ============================================================

func TestProjection_Positional_ScoresGte80(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Positional_ScoresGte80",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "scores.$", Value: 1}})
			return runFindAll(ctx, col, bson.D{{Key: "scores", Value: bson.D{{Key: "$gte", Value: int32(80)}}}}, opts)
		},
	})
}

func TestProjection_Positional_TagsGo(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_Positional_TagsGo",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "tags.$", Value: 1}})
			return runFindAll(ctx, col, bson.D{{Key: "tags", Value: "go"}}, opts)
		},
	})
}

// ============================================================
// Sort — basic ascending/descending
// ============================================================

func TestSort_Ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_Ascending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_Descending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_Descending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_StringAscending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_StringAscending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "name", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_StringDescending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_StringDescending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "name", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — nested fields
// ============================================================

func TestSort_NestedField_Asc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_NestedField_Asc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "addr.city", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "addr.city", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_NestedField_Desc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_NestedField_Desc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "addr.zip", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "addr.zip", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — null / missing values
// ============================================================

func TestSort_NullValues_Ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_NullValues_Ascending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// p3 has dept=null, p4 has no dept field — both sort before non-null values asc
			opts := options.Find().
				SetSort(bson.D{{Key: "dept", Value: 1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "dept", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_NullValues_Descending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_NullValues_Descending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "dept", Value: -1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "dept", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_MissingField_Ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_MissingField_Ascending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "nonexistent", Value: 1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — multi-field (3+ fields)
// ============================================================

func TestSort_TwoFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_TwoFields",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "active", Value: 1}, {Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "active", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_ThreeFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_ThreeFields",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{
					{Key: "active", Value: 1},
					{Key: "dept", Value: 1},
					{Key: "rank", Value: 1},
				}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "active", Value: 1}, {Key: "dept", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_FourFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_FourFields",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{
					{Key: "active", Value: -1},
					{Key: "dept", Value: 1},
					{Key: "rank", Value: 1},
					{Key: "_id", Value: 1},
				}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "active", Value: 1}, {Key: "dept", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — array field (uses minimum element; DumboDBXFail)
// ============================================================

func TestSort_ArrayField_Ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_ArrayField_Ascending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "scores", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "scores", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_ArrayField_Descending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_ArrayField_Descending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "scores", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "scores", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — $natural
// ============================================================

func TestSort_Natural_Ascending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_Natural_Ascending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "$natural", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_Natural_Descending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_Natural_Descending",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "$natural", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — $meta textScore (DumboDBXFail — requires text index)
// ============================================================

func TestSort_MetaTextScore(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_MetaTextScore",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// Create a text index required for $meta textScore sort.
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "name", Value: "text"}, {Key: "dept", Value: "text"}},
			})
			if err != nil {
				return err
			}
			return insertProjDocs(ctx, col)
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "$text", Value: bson.D{{Key: "$meta", Value: "textScore"}}}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "score", Value: bson.D{{Key: "$meta", Value: "textScore"}}},
				})
			return runFindAll(ctx, col, bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "eng"}}}}, opts)
		},
	})
}

// ============================================================
// Combined — sort + projection + limit + skip
// ============================================================

func TestCombined_SortProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SortProjection",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_SortProjectionLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SortProjectionLimit",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}}).
				SetLimit(3)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_SortProjectionSkip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SortProjectionSkip",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}}).
				SetSkip(2)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_SortProjectionLimitSkip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SortProjectionLimitSkip",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}}).
				SetSkip(1).
				SetLimit(2)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_FilterSortProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_FilterSortProjection",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{{Key: "active", Value: true}}, opts)
		},
	})
}

func TestCombined_FilterSortLimitSkip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_FilterSortLimitSkip",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "name", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}}).
				SetSkip(1).
				SetLimit(2)
			return runFindAll(ctx, col, bson.D{{Key: "active", Value: true}}, opts)
		},
	})
}

func TestCombined_SkipPastEnd(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SkipPastEnd",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(100)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_LimitZero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_LimitZero",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// limit=0 means no limit in MongoDB
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}}).
				SetLimit(0)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_FindOne_SortDesc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_FindOne_SortDesc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetSort(bson.D{{Key: "rank", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}})
			var doc bson.D
			err := col.FindOne(ctx, bson.D{}, opts).Decode(&doc)
			return doc, err
		},
	})
}

func TestCombined_FindOne_SortAsc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_FindOne_SortAsc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}})
			var doc bson.D
			err := col.FindOne(ctx, bson.D{}, opts).Decode(&doc)
			return doc, err
		},
	})
}

func TestCombined_FindOne_FilterSortProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_FindOne_FilterSortProjection",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "dept", Value: 1}})
			var doc bson.D
			err := col.FindOne(ctx, bson.D{{Key: "dept", Value: "eng"}}, opts).Decode(&doc)
			return doc, err
		},
	})
}

func TestCombined_SortProjectionNestedFieldLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SortProjectionNestedFieldLimit",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "addr.city", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "addr.city", Value: 1}}).
				SetLimit(3)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_Sort_Slice_Projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_Sort_Slice_Projection",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "name", Value: 1},
					{Key: "scores", Value: bson.D{{Key: "$slice", Value: int32(1)}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_Sort_ElemMatch_Projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_Sort_ElemMatch_Projection",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "tags", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: "go"}}}}},
				})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// FindOne with sort and $skip via FindOneOptions
// ============================================================

func TestCombined_FindOne_SkipViaFind(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_FindOne_SkipViaFind",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Use Find+Limit(1)+Skip to simulate FindOne-with-skip
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "rank", Value: 1}}).
				SetSkip(2).
				SetLimit(1)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Projection — NullField included in projection result
// ============================================================

func TestProjection_NullFieldIncluded(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_NullFieldIncluded",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "dept", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestProjection_NullFieldExcluded(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Projection_NullFieldExcluded",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "dept", Value: 0}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Sort — boolean field
// ============================================================

func TestSort_BooleanField_Asc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_BooleanField_Asc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "active", Value: 1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "active", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestSort_BooleanField_Desc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Sort_BooleanField_Desc",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "active", Value: -1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "active", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Combined — large limit (returns all)
// ============================================================

func TestCombined_LargeLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_LargeLimit",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}}).
				SetLimit(1000)
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

// ============================================================
// Combined — Projection + Sort on same field
// ============================================================

func TestCombined_ProjectAndSortSameField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_ProjectAndSortSameField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "rank", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}

func TestCombined_SortOnExcludedField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Combined_SortOnExcludedField",
		Support: harness.DumboDBFull,
		Setup:   insertProjDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sort on "rank" but exclude "rank" from projection
			opts := options.Find().
				SetSort(bson.D{{Key: "rank", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}})
			return runFindAll(ctx, col, bson.D{}, opts)
		},
	})
}
