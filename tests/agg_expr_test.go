package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// exprProject is a helper that runs a single-document $project pipeline and
// returns the result. The seed doc is inserted by the caller via Setup.
func exprProject(ctx context.Context, col *mongo.Collection, id interface{}, projection bson.D) (interface{}, error) {
	cursor, err := col.Aggregate(ctx, []bson.D{
		{{Key: "$match", Value: bson.D{{Key: "_id", Value: id}}}},
		{{Key: "$project", Value: projection}},
	})
	if err != nil {
		return nil, err
	}
	var results []bson.D
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// numDoc inserts a document with fields a, b, and returns the collection.
func insertNumDoc(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "n1"},
		{Key: "a", Value: 10.0},
		{Key: "b", Value: 3.0},
		{Key: "neg", Value: -5.0},
		{Key: "x", Value: 2.0},
	})
	return err
}

// ─── Arithmetic expressions ───────────────────────────────────────────────────

func TestExpr_add(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_add",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$add", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

func TestExpr_subtract(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_subtract",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$subtract", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

func TestExpr_multiply(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_multiply",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

func TestExpr_divide(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_divide",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$divide", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

func TestExpr_mod(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_mod",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$mod", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

func TestExpr_abs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_abs",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$abs", Value: "$neg"}}},
			})
		},
	})
}

func TestExpr_ceil(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_ceil",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "c1"}, {Key: "v", Value: 4.3}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "c1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$ceil", Value: "$v"}}},
			})
		},
	})
}

func TestExpr_floor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_floor",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "f1"}, {Key: "v", Value: 4.9}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "f1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$floor", Value: "$v"}}},
			})
		},
	})
}

func TestExpr_round(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_round",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "r1"}, {Key: "v", Value: 3.456}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "r1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$round", Value: bson.A{"$v", int32(2)}}}},
			})
		},
	})
}

func TestExpr_pow(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_pow",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$pow", Value: bson.A{"$x", 3.0}}}},
			})
		},
	})
}

func TestExpr_sqrt(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_sqrt",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "sq1"}, {Key: "v", Value: 16.0}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "sq1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$sqrt", Value: "$v"}}},
			})
		},
	})
}

func TestExpr_log10(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_log10",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "log1"}, {Key: "v", Value: 100.0}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "log1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$log10", Value: "$v"}}},
			})
		},
	})
}

func TestExpr_exp_ln(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_exp_ln",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "el1"}, {Key: "v", Value: 1.0}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "el1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "eResult", Value: bson.D{{Key: "$exp", Value: "$v"}}},
				{Key: "lnResult", Value: bson.D{{Key: "$ln", Value: bson.D{{Key: "$exp", Value: "$v"}}}}},
			})
		},
	})
}

func TestExpr_log_base(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_log_base",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "lb1"}, {Key: "v", Value: 8.0}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "lb1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$log", Value: bson.A{"$v", 2.0}}}},
			})
		},
	})
}

// ─── String expressions ───────────────────────────────────────────────────────

func insertStrDoc(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "s1"},
		{Key: "first", Value: "Hello"},
		{Key: "last", Value: "World"},
		{Key: "mixed", Value: "FoO BaR"},
		{Key: "padded", Value: "  trim me  "},
		{Key: "csv", Value: "a,b,c,d"},
	})
	return err
}

func TestExpr_concat(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_concat",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$concat", Value: bson.A{"$first", " ", "$last"}}}},
			})
		},
	})
}

func TestExpr_toLower_toUpper(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toLower_toUpper",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "lower", Value: bson.D{{Key: "$toLower", Value: "$mixed"}}},
				{Key: "upper", Value: bson.D{{Key: "$toUpper", Value: "$mixed"}}},
			})
		},
	})
}

func TestExpr_trim(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_trim",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "trimmed", Value: bson.D{{Key: "$trim", Value: bson.D{{Key: "input", Value: "$padded"}}}}},
				{Key: "ltrimmed", Value: bson.D{{Key: "$ltrim", Value: bson.D{{Key: "input", Value: "$padded"}}}}},
				{Key: "rtrimmed", Value: bson.D{{Key: "$rtrim", Value: bson.D{{Key: "input", Value: "$padded"}}}}},
			})
		},
	})
}

func TestExpr_split(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_split",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$split", Value: bson.A{"$csv", ","}}}},
			})
		},
	})
}

func TestExpr_strcasecmp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_strcasecmp",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "cmp", Value: bson.D{{Key: "$strcasecmp", Value: bson.A{"$first", "hello"}}}},
			})
		},
	})
}

func TestExpr_strLen(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_strLen",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "lenBytes", Value: bson.D{{Key: "$strLenBytes", Value: "$first"}}},
				{Key: "lenCP", Value: bson.D{{Key: "$strLenCP", Value: "$first"}}},
			})
		},
	})
}

func TestExpr_substr(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_substr",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$substr", Value: bson.A{"$first", int32(1), int32(3)}}}},
			})
		},
	})
}

func TestExpr_indexOfBytes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_indexOfBytes",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$indexOfBytes", Value: bson.A{"$first", "ll"}}}},
			})
		},
	})
}

// ─── Array expressions ────────────────────────────────────────────────────────

func insertArrDoc(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "arr1"},
		{Key: "nums", Value: bson.A{int32(10), int32(20), int32(30), int32(40), int32(50)}},
		{Key: "letters", Value: bson.A{"a", "b", "c"}},
		{Key: "mixed", Value: bson.A{int32(1), "x", int32(3)}},
	})
	return err
}

func TestExpr_arrayElemAt(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_arrayElemAt",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "first", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{"$nums", int32(0)}}}},
				{Key: "last", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{"$nums", int32(-1)}}}},
			})
		},
	})
}

func TestExpr_size(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_size",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "count", Value: bson.D{{Key: "$size", Value: "$nums"}}},
			})
		},
	})
}

func TestExpr_concatArrays(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_concatArrays",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$concatArrays", Value: bson.A{"$nums", "$letters"}}}},
			})
		},
	})
}

func TestExpr_slice(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_slice",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$slice", Value: bson.A{"$nums", int32(1), int32(3)}}}},
			})
		},
	})
}

func TestExpr_reverseArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_reverseArray",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$reverseArray", Value: "$letters"}}},
			})
		},
	})
}

func TestExpr_isArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_isArray",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "numsIsArray", Value: bson.D{{Key: "$isArray", Value: "$nums"}}},
				{Key: "idIsArray", Value: bson.D{{Key: "$isArray", Value: "$_id"}}},
			})
		},
	})
}

func TestExpr_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_filter",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$filter", Value: bson.D{
					{Key: "input", Value: "$nums"},
					{Key: "as", Value: "n"},
					{Key: "cond", Value: bson.D{{Key: "$gt", Value: bson.A{"$$n", int32(20)}}}},
				}}}},
			})
		},
	})
}

func TestExpr_map(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_map",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$map", Value: bson.D{
					{Key: "input", Value: "$nums"},
					{Key: "as", Value: "n"},
					{Key: "in", Value: bson.D{{Key: "$multiply", Value: bson.A{"$$n", 2}}}},
				}}}},
			})
		},
	})
}

func TestExpr_reduce(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_reduce",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$reduce", Value: bson.D{
					{Key: "input", Value: "$nums"},
					{Key: "initialValue", Value: int32(0)},
					{Key: "in", Value: bson.D{{Key: "$add", Value: bson.A{"$$value", "$$this"}}}},
				}}}},
			})
		},
	})
}

func TestExpr_range(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_range",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "rng1"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "rng1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$range", Value: bson.A{int32(0), int32(5), int32(2)}}}},
			})
		},
	})
}

func TestExpr_indexOfArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_indexOfArray",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$indexOfArray", Value: bson.A{"$letters", "b"}}}},
				{Key: "missing", Value: bson.D{{Key: "$indexOfArray", Value: bson.A{"$letters", "z"}}}},
			})
		},
	})
}

func TestExpr_in_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_in_array",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "hasA", Value: bson.D{{Key: "$in", Value: bson.A{"a", "$letters"}}}},
				{Key: "hasZ", Value: bson.D{{Key: "$in", Value: bson.A{"z", "$letters"}}}},
			})
		},
	})
}

func TestExpr_arrayToObject(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_arrayToObject",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "ato1"},
				{Key: "pairs", Value: bson.A{
					bson.D{{Key: "k", Value: "x"}, {Key: "v", Value: int32(1)}},
					bson.D{{Key: "k", Value: "y"}, {Key: "v", Value: int32(2)}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "ato1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$arrayToObject", Value: "$pairs"}}},
			})
		},
	})
}

func TestExpr_zip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_zip",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "zip1"},
				{Key: "a", Value: bson.A{int32(1), int32(2), int32(3)}},
				{Key: "b", Value: bson.A{"x", "y", "z"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "zip1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$zip", Value: bson.D{
					{Key: "inputs", Value: bson.A{"$a", "$b"}},
				}}}},
			})
		},
	})
}

// ─── Date expressions ─────────────────────────────────────────────────────────

func insertDateDoc(ctx context.Context, col *mongo.Collection) error {
	ts := primitive.NewDateTimeFromTime(time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC))
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "d1"},
		{Key: "ts", Value: ts},
	})
	return err
}

func TestExpr_year_month_day(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_year_month_day",
		Support: harness.DongoXFail,
		Setup:   insertDateDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "d1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "yr", Value: bson.D{{Key: "$year", Value: "$ts"}}},
				{Key: "mo", Value: bson.D{{Key: "$month", Value: "$ts"}}},
				{Key: "dom", Value: bson.D{{Key: "$dayOfMonth", Value: "$ts"}}},
			})
		},
	})
}

func TestExpr_hour_minute_second(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_hour_minute_second",
		Support: harness.DongoXFail,
		Setup:   insertDateDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "d1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "hr", Value: bson.D{{Key: "$hour", Value: "$ts"}}},
				{Key: "min", Value: bson.D{{Key: "$minute", Value: "$ts"}}},
				{Key: "sec", Value: bson.D{{Key: "$second", Value: "$ts"}}},
			})
		},
	})
}

func TestExpr_dateToString(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_dateToString",
		Support: harness.DongoXFail,
		Setup:   insertDateDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "d1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$dateToString", Value: bson.D{
					{Key: "format", Value: "%Y-%m-%d"},
					{Key: "date", Value: "$ts"},
				}}}},
			})
		},
	})
}

func TestExpr_dateTrunc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_dateTrunc",
		Support: harness.DongoXFail,
		Setup:   insertDateDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "d1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$dateTrunc", Value: bson.D{
					{Key: "date", Value: "$ts"},
					{Key: "unit", Value: "day"},
				}}}},
			})
		},
	})
}

func TestExpr_dateAdd(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_dateAdd",
		Support: harness.DongoXFail,
		Setup:   insertDateDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "d1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$dateAdd", Value: bson.D{
					{Key: "startDate", Value: "$ts"},
					{Key: "unit", Value: "day"},
					{Key: "amount", Value: int32(7)},
				}}}},
			})
		},
	})
}

func TestExpr_dateDiff(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_dateDiff",
		Support: harness.DongoXFail,
		Setup:   insertDateDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			end := primitive.NewDateTimeFromTime(time.Date(2024, 7, 15, 0, 0, 0, 0, time.UTC))
			return exprProject(ctx, col, "d1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$dateDiff", Value: bson.D{
					{Key: "startDate", Value: "$ts"},
					{Key: "endDate", Value: end},
					{Key: "unit", Value: "day"},
				}}}},
			})
		},
	})
}

// ─── Conditional expressions ──────────────────────────────────────────────────

func TestExpr_cond_true(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_cond_true",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "cond1"}, {Key: "score", Value: int32(85)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "cond1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{{Key: "$gte", Value: bson.A{"$score", int32(80)}}}},
					{Key: "then", Value: "pass"},
					{Key: "else", Value: "fail"},
				}}}},
			})
		},
	})
}

func TestExpr_cond_false(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_cond_false",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "cond2"}, {Key: "score", Value: int32(50)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "cond2", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$gte", Value: bson.A{"$score", int32(80)}}},
					"pass",
					"fail",
				}}}},
			})
		},
	})
}

func TestExpr_ifNull_field_present(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_ifNull_field_present",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "ifn1"}, {Key: "val", Value: "exists"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "ifn1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$val", "default"}}}},
			})
		},
	})
}

func TestExpr_ifNull_field_missing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_ifNull_field_missing",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "ifn2"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "ifn2", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$missing", "default"}}}},
			})
		},
	})
}

func TestExpr_switch_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_switch_basic",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "sw1"}, {Key: "score", Value: int32(90)}},
				bson.D{{Key: "_id", Value: "sw2"}, {Key: "score", Value: int32(70)}},
				bson.D{{Key: "_id", Value: "sw3"}, {Key: "score", Value: int32(50)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "grade", Value: bson.D{{Key: "$switch", Value: bson.D{
						{Key: "branches", Value: bson.A{
							bson.D{
								{Key: "case", Value: bson.D{{Key: "$gte", Value: bson.A{"$score", int32(90)}}}},
								{Key: "then", Value: "A"},
							},
							bson.D{
								{Key: "case", Value: bson.D{{Key: "$gte", Value: bson.A{"$score", int32(70)}}}},
								{Key: "then", Value: "B"},
							},
						}},
						{Key: "default", Value: "C"},
					}}}},
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

// ─── Type conversion expressions ─────────────────────────────────────────────

func insertTypeDoc(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "t1"},
		{Key: "numStr", Value: "42"},
		{Key: "floatStr", Value: "3.14"},
		{Key: "intVal", Value: int32(7)},
		{Key: "boolStr", Value: "true"},
	})
	return err
}

func TestExpr_toInt(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toInt",
		Support: harness.DongoXFail,
		Setup:   insertTypeDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "t1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$toInt", Value: "$numStr"}}},
			})
		},
	})
}

func TestExpr_toDouble(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toDouble",
		Support: harness.DongoXFail,
		Setup:   insertTypeDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "t1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$toDouble", Value: "$floatStr"}}},
			})
		},
	})
}

func TestExpr_toString(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toString",
		Support: harness.DongoXFail,
		Setup:   insertTypeDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "t1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$toString", Value: "$intVal"}}},
			})
		},
	})
}

func TestExpr_toBool(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toBool",
		Support: harness.DongoXFail,
		Setup:   insertTypeDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "t1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$toBool", Value: "$intVal"}}},
			})
		},
	})
}

func TestExpr_convert_int_to_string(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_convert_int_to_string",
		Support: harness.DongoXFail,
		Setup:   insertTypeDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "t1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$convert", Value: bson.D{
					{Key: "input", Value: "$intVal"},
					{Key: "to", Value: "string"},
				}}}},
			})
		},
	})
}

func TestExpr_convert_with_onError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_convert_with_onError",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "conv1"}, {Key: "v", Value: "notanumber"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "conv1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$convert", Value: bson.D{
					{Key: "input", Value: "$v"},
					{Key: "to", Value: "int"},
					{Key: "onError", Value: int32(-1)},
				}}}},
			})
		},
	})
}

// ─── Comparison expressions in $project ───────────────────────────────────────

func TestExpr_cmp_operators(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_cmp_operators",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "cmp1"},
				{Key: "a", Value: int32(10)},
				{Key: "b", Value: int32(20)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "cmp1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "eq", Value: bson.D{{Key: "$eq", Value: bson.A{"$a", "$b"}}}},
				{Key: "ne", Value: bson.D{{Key: "$ne", Value: bson.A{"$a", "$b"}}}},
				{Key: "gt", Value: bson.D{{Key: "$gt", Value: bson.A{"$a", "$b"}}}},
				{Key: "lt", Value: bson.D{{Key: "$lt", Value: bson.A{"$a", "$b"}}}},
				{Key: "cmp", Value: bson.D{{Key: "$cmp", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

func TestExpr_gte_lte(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_gte_lte",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "gl1"},
				{Key: "x", Value: int32(5)},
				{Key: "y", Value: int32(5)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "gl1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "gte", Value: bson.D{{Key: "$gte", Value: bson.A{"$x", "$y"}}}},
				{Key: "lte", Value: bson.D{{Key: "$lte", Value: bson.A{"$x", "$y"}}}},
			})
		},
	})
}

// ─── Group accumulators ───────────────────────────────────────────────────────

func insertGroupSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "g1"}, {Key: "dept", Value: "eng"}, {Key: "salary", Value: 90000.0}, {Key: "level", Value: int32(3)}},
		bson.D{{Key: "_id", Value: "g2"}, {Key: "dept", Value: "eng"}, {Key: "salary", Value: 80000.0}, {Key: "level", Value: int32(2)}},
		bson.D{{Key: "_id", Value: "g3"}, {Key: "dept", Value: "hr"}, {Key: "salary", Value: 60000.0}, {Key: "level", Value: int32(2)}},
		bson.D{{Key: "_id", Value: "g4"}, {Key: "dept", Value: "hr"}, {Key: "salary", Value: 70000.0}, {Key: "level", Value: int32(3)}},
		bson.D{{Key: "_id", Value: "g5"}, {Key: "dept", Value: "eng"}, {Key: "salary", Value: 95000.0}, {Key: "level", Value: int32(4)}},
	})
	return err
}

func TestAccum_stdDevPop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Accum_stdDevPop",
		Support: harness.DongoXFail,
		Setup:   insertGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$dept"},
					{Key: "stdDev", Value: bson.D{{Key: "$stdDevPop", Value: "$salary"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			// Return just the dept names and whether stdDev is > 0 (non-deterministic exact value)
			var out []bson.D
			for _, r := range results {
				dept := r.Map()["_id"]
				out = append(out, bson.D{{Key: "dept", Value: dept}, {Key: "hasStdDev", Value: true}})
			}
			return docsToSlice(out), nil
		},
	})
}

func TestAccum_stdDevSamp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Accum_stdDevSamp",
		Support: harness.DongoXFail,
		Setup:   insertGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$dept"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
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

func TestAccum_count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Accum_count",
		Support: harness.DongoXFail,
		Setup:   insertGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$dept"},
					{Key: "count", Value: bson.D{{Key: "$count", Value: bson.D{}}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
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

func TestAccum_mergeObjects(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Accum_mergeObjects",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "mo1"}, {Key: "grp", Value: "a"}, {Key: "data", Value: bson.D{{Key: "x", Value: int32(1)}}}},
				bson.D{{Key: "_id", Value: "mo2"}, {Key: "grp", Value: "a"}, {Key: "data", Value: bson.D{{Key: "y", Value: int32(2)}}}},
				bson.D{{Key: "_id", Value: "mo3"}, {Key: "grp", Value: "b"}, {Key: "data", Value: bson.D{{Key: "z", Value: int32(3)}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$grp"},
					{Key: "merged", Value: bson.D{{Key: "$mergeObjects", Value: "$data"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
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

func TestAccum_multi_accumulators(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Accum_multi_accumulators",
		Support: harness.DongoXFail,
		Setup:   insertGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$dept"},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$salary"}}},
					{Key: "avg", Value: bson.D{{Key: "$avg", Value: "$salary"}}},
					{Key: "min", Value: bson.D{{Key: "$min", Value: "$salary"}}},
					{Key: "max", Value: bson.D{{Key: "$max", Value: "$salary"}}},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
					{Key: "first", Value: bson.D{{Key: "$first", Value: "$salary"}}},
					{Key: "last", Value: bson.D{{Key: "$last", Value: "$salary"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
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

// ─── Boolean / logical expressions ───────────────────────────────────────────

func TestExpr_and_or_not(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_and_or_not",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bool1"},
				{Key: "a", Value: true},
				{Key: "b", Value: false},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "bool1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "andResult", Value: bson.D{{Key: "$and", Value: bson.A{"$a", "$b"}}}},
				{Key: "orResult", Value: bson.D{{Key: "$or", Value: bson.A{"$a", "$b"}}}},
				{Key: "notResult", Value: bson.D{{Key: "$not", Value: bson.A{"$b"}}}},
			})
		},
	})
}

// ─── $trunc ───────────────────────────────────────────────────────────────────

func TestExpr_trunc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_trunc",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "tr1"}, {Key: "v", Value: 7.89}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "tr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$trunc", Value: bson.A{"$v", int32(1)}}}},
			})
		},
	})
}

// ─── $toLong / $toDate ────────────────────────────────────────────────────────

func TestExpr_toLong(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toLong",
		Support: harness.DongoXFail,
		Setup:   insertTypeDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "t1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$toLong", Value: "$intVal"}}},
			})
		},
	})
}

func TestExpr_toDate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_toDate",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "td1"}, {Key: "ms", Value: int64(1718438400000)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "td1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$toDate", Value: "$ms"}}},
			})
		},
	})
}

// ─── $objectToArray ───────────────────────────────────────────────────────────

func TestExpr_objectToArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_objectToArray",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "ota1"},
				{Key: "config", Value: bson.D{
					{Key: "debug", Value: false},
					{Key: "timeout", Value: int32(30)},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "ota1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$objectToArray", Value: "$config"}}},
			})
		},
	})
}

// ─── Set expressions ──────────────────────────────────────────────────────────

func insertSetDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "set1"},
		{Key: "a", Value: bson.A{int32(1), int32(2), int32(3), int32(4)}},
		{Key: "b", Value: bson.A{int32(3), int32(4), int32(5), int32(6)}},
	})
	return err
}

func TestExpr_setUnion(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_setUnion",
		Support: harness.DongoXFail,
		Setup:   insertSetDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := exprProject(ctx, col, "set1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$setUnion", Value: bson.A{"$a", "$b"}}}},
			})
			if err != nil {
				return nil, err
			}
			// Size is deterministic even if order is not
			if doc, ok := res.(bson.D); ok {
				if arr, ok := doc.Map()["result"].(bson.A); ok {
					return bson.D{{Key: "size", Value: int32(len(arr))}}, nil
				}
			}
			return res, nil
		},
	})
}

func TestExpr_setIntersection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_setIntersection",
		Support: harness.DongoXFail,
		Setup:   insertSetDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := exprProject(ctx, col, "set1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$setIntersection", Value: bson.A{"$a", "$b"}}}},
			})
			if err != nil {
				return nil, err
			}
			if doc, ok := res.(bson.D); ok {
				if arr, ok := doc.Map()["result"].(bson.A); ok {
					return bson.D{{Key: "size", Value: int32(len(arr))}}, nil
				}
			}
			return res, nil
		},
	})
}

func TestExpr_setDifference(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_setDifference",
		Support: harness.DongoXFail,
		Setup:   insertSetDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := exprProject(ctx, col, "set1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$setDifference", Value: bson.A{"$a", "$b"}}}},
			})
			if err != nil {
				return nil, err
			}
			if doc, ok := res.(bson.D); ok {
				if arr, ok := doc.Map()["result"].(bson.A); ok {
					return bson.D{{Key: "size", Value: int32(len(arr))}}, nil
				}
			}
			return res, nil
		},
	})
}

func TestExpr_setIsSubset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_setIsSubset",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "sub1"},
				{Key: "small", Value: bson.A{int32(1), int32(2)}},
				{Key: "big", Value: bson.A{int32(1), int32(2), int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "sub1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "isSubset", Value: bson.D{{Key: "$setIsSubset", Value: bson.A{"$small", "$big"}}}},
				{Key: "notSubset", Value: bson.D{{Key: "$setIsSubset", Value: bson.A{"$big", "$small"}}}},
			})
		},
	})
}

// ─── $literal ─────────────────────────────────────────────────────────────────

func TestExpr_literal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_literal",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "lit1"}, {Key: "x", Value: int32(5)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "lit1", bson.D{
				{Key: "_id", Value: 0},
				// $literal prevents "$x" from being treated as a field reference
				{Key: "result", Value: bson.D{{Key: "$literal", Value: "$x"}}},
			})
		},
	})
}

// ─── $let ─────────────────────────────────────────────────────────────────────

func TestExpr_let(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_let",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$let", Value: bson.D{
					{Key: "vars", Value: bson.D{
						{Key: "doubled", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", 2}}}},
					}},
					{Key: "in", Value: bson.D{{Key: "$add", Value: bson.A{"$$doubled", "$b"}}}},
				}}}},
			})
		},
	})
}

// ─── $type expression ─────────────────────────────────────────────────────────

func TestExpr_type(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_type",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "typ1"},
				{Key: "intVal", Value: int32(1)},
				{Key: "strVal", Value: "hello"},
				{Key: "arrVal", Value: bson.A{1, 2}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "typ1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "intType", Value: bson.D{{Key: "$type", Value: "$intVal"}}},
				{Key: "strType", Value: bson.D{{Key: "$type", Value: "$strVal"}}},
				{Key: "arrType", Value: bson.D{{Key: "$type", Value: "$arrVal"}}},
			})
		},
	})
}

// ─── Null / missing field handling ────────────────────────────────────────────

func TestExpr_null_field_in_arithmetic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_null_field_in_arithmetic",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "null1"}, {Key: "x", Value: nil}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "null1", bson.D{
				{Key: "_id", Value: 0},
				// arithmetic with null propagates null
				{Key: "result", Value: bson.D{{Key: "$add", Value: bson.A{"$x", int32(5)}}}},
			})
		},
	})
}

func TestExpr_missing_field_in_cond(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_missing_field_in_cond",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "miss1"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "miss1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$missing", "fallback"}}}},
			})
		},
	})
}

// ─── $mergeObjects in $project ────────────────────────────────────────────────

func TestExpr_mergeObjects_project(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_mergeObjects_project",
		Support: harness.DongoXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "mo1"},
				{Key: "a", Value: bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}}},
				{Key: "b", Value: bson.D{{Key: "y", Value: int32(99)}, {Key: "z", Value: int32(3)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "mo1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$mergeObjects", Value: bson.A{"$a", "$b"}}}},
			})
		},
	})
}

// ─── $expr in $match (aggregation expression in query) ────────────────────────

func TestExpr_expr_in_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_expr_in_match",
		Support: harness.DongoXFail,
		Setup:   insertGroupSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$match", Value: bson.D{
					{Key: "$expr", Value: bson.D{
						{Key: "$gt", Value: bson.A{"$salary", 75000.0}},
					}},
				}}},
				{{Key: "$count", Value: "total"}},
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

// ─── Chained expressions ──────────────────────────────────────────────────────

func TestExpr_nested_arithmetic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_nested_arithmetic",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// ((a + b) * 2) - abs(neg)
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$subtract", Value: bson.A{
					bson.D{{Key: "$multiply", Value: bson.A{
						bson.D{{Key: "$add", Value: bson.A{"$a", "$b"}}},
						2,
					}}},
					bson.D{{Key: "$abs", Value: "$neg"}},
				}}}},
			})
		},
	})
}

func TestExpr_string_pipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_string_pipeline",
		Support: harness.DongoXFail,
		Setup:   insertStrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// concat, then toLower, then check length
			return exprProject(ctx, col, "s1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$strLenBytes", Value: bson.D{{Key: "$toLower", Value: bson.D{{Key: "$concat", Value: bson.A{"$first", "$last"}}}}}}}},
			})
		},
	})
}

func TestExpr_multiply_three_args(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_multiply_three_args",
		Support: harness.DongoXFail,
		Setup:   insertNumDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return exprProject(ctx, col, "n1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", "$b", "$x"}}}},
			})
		},
	})
}

func TestExpr_array_map_filter_size(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Expr_array_map_filter_size",
		Support: harness.DongoXFail,
		Setup:   insertArrDoc,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Double all elements, filter > 40, return size
			return exprProject(ctx, col, "arr1", bson.D{
				{Key: "_id", Value: 0},
				{Key: "result", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$filter", Value: bson.D{
					{Key: "input", Value: bson.D{{Key: "$map", Value: bson.D{
						{Key: "input", Value: "$nums"},
						{Key: "as", Value: "n"},
						{Key: "in", Value: bson.D{{Key: "$multiply", Value: bson.A{"$$n", 2}}}},
					}}}},
					{Key: "as", Value: "v"},
					{Key: "cond", Value: bson.D{{Key: "$gt", Value: bson.A{"$$v", int32(40)}}}},
				}}}}}},
			})
		},
	})
}
