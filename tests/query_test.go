package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
)

// queryDocs is a rich dataset for exercising query operators.
var queryDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "q1"}, {Key: "name", Value: "Alice"},
		{Key: "age", Value: int32(25)}, {Key: "score", Value: 8.5},
		{Key: "active", Value: true}, {Key: "tags", Value: bson.A{"go", "db", "nosql"}},
	},
	bson.D{
		{Key: "_id", Value: "q2"}, {Key: "name", Value: "Bob"},
		{Key: "age", Value: int32(30)}, {Key: "score", Value: 6.0},
		{Key: "active", Value: false}, {Key: "tags", Value: bson.A{"python", "db"}},
	},
	bson.D{
		{Key: "_id", Value: "q3"}, {Key: "name", Value: "Carol"},
		{Key: "age", Value: int32(22)}, {Key: "score", Value: 9.5},
		{Key: "active", Value: true}, {Key: "tags", Value: bson.A{"go", "nosql"}},
	},
	bson.D{
		{Key: "_id", Value: "q4"}, {Key: "name", Value: "Dave"},
		{Key: "age", Value: int32(35)}, {Key: "score", Value: 4.0},
		{Key: "active", Value: false}, {Key: "tags", Value: bson.A{"db"}},
	},
	bson.D{
		{Key: "_id", Value: "q5"}, {Key: "name", Value: "Eve"},
		{Key: "age", Value: int32(28)}, {Key: "score", Value: 7.5},
		{Key: "active", Value: true},
	},
	// q6 has no 'score' field — used for $exists tests.
	bson.D{
		{Key: "_id", Value: "q6"}, {Key: "name", Value: "Frank"},
		{Key: "age", Value: int32(40)}, {Key: "active", Value: true},
	},
}

func insertQueryDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, queryDocs)
	return err
}

// findIDs runs a Find sorted by _id and returns just the _id fields.
func findIDs(ctx context.Context, col *mongo.Collection, filter interface{}) ([]interface{}, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetProjection(bson.D{{Key: "_id", Value: 1}})
	cursor, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]interface{}, len(docs))
	for i, d := range docs {
		ids[i] = d
	}
	return ids, nil
}

// --- Comparison operators ---

func TestQuery_eq(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_eq",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "age", Value: bson.D{{Key: "$eq", Value: int32(30)}}}})
		},
	})
}

func TestQuery_ne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_ne",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "active", Value: bson.D{{Key: "$ne", Value: true}}}})
		},
	})
}

func TestQuery_gt_gte(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_gt_gte",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(30)}}}})
		},
	})
}

func TestQuery_lt_lte(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_lt_lte",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "age", Value: bson.D{{Key: "$lte", Value: int32(25)}}}})
		},
	})
}

func TestQuery_in(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_in",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{"q1", "q3", "q5"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_nin(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_nin",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$nin", Value: bson.A{"q1", "q2", "q3"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_range_compound(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_range_compound",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "age", Value: bson.D{
				{Key: "$gt", Value: int32(22)},
				{Key: "$lt", Value: int32(35)},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// --- Logical operators ---

func TestQuery_and(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_and",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "$and", Value: bson.A{
				bson.D{{Key: "active", Value: true}},
				bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(25)}}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_or(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_or",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "age", Value: bson.D{{Key: "$lt", Value: int32(23)}}}},
				bson.D{{Key: "age", Value: bson.D{{Key: "$gt", Value: int32(38)}}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_not(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_not",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "active", Value: bson.D{
				{Key: "$not", Value: bson.D{{Key: "$eq", Value: true}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_nor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_nor",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "$nor", Value: bson.A{
				bson.D{{Key: "active", Value: true}},
				bson.D{{Key: "age", Value: bson.D{{Key: "$lt", Value: int32(30)}}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// --- Element operators ---

func TestQuery_exists_true(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_exists_true",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: true}}}})
		},
	})
}

func TestQuery_exists_false(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_exists_false",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: false}}}})
		},
	})
}

func TestQuery_type_string(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_string",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "name", Value: bson.D{{Key: "$type", Value: "string"}}}})
		},
	})
}

func TestQuery_type_bool(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_bool",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "active", Value: bson.D{{Key: "$type", Value: "bool"}}}})
		},
	})
}

func TestQuery_type_number(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_number",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "number" matches int32, int64, double, decimal128.
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$type", Value: "number"}}}})
		},
	})
}

// --- Array operators ---

func TestQuery_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_all",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$all", Value: bson.A{"go", "db"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_elemMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_elemMatch",
		Support: harness.DongoFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "em1"}, {Key: "scores", Value: bson.A{int32(80), int32(90), int32(70)}}},
				bson.D{{Key: "_id", Value: "em2"}, {Key: "scores", Value: bson.A{int32(50), int32(60), int32(40)}}},
				bson.D{{Key: "_id", Value: "em3"}, {Key: "scores", Value: bson.A{int32(90), int32(95), int32(88)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Docs where at least one score is in [85, 95].
			filter := bson.D{{Key: "scores", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "$gte", Value: int32(85)},
				{Key: "$lte", Value: int32(95)},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_size(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_size",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Docs where the tags array has exactly 2 elements.
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$size", Value: int32(2)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// --- MONGO_ONLY: deprecated / unsupported operators ---

// TestQuery_where uses $where (JavaScript evaluation), deprecated since MongoDB 4.4
// and unsupported in Dongo.
func TestQuery_where(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_where",
		Support: harness.DongoMongoOnly,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "$where", Value: "this.age > 30"}})
		},
	})
}

// TestQuery_text exercises $text search (requires a text index).
// Dongo does not support text indexes or $text queries.
func TestQuery_text(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_text",
		Support: harness.DongoMongoOnly,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			idxModel := mongo.IndexModel{
				Keys: bson.D{{Key: "name", Value: "text"}},
			}
			if _, err := col.Indexes().CreateOne(ctx, idxModel); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "t1"}, {Key: "name", Value: "Alice Smith"}},
				bson.D{{Key: "_id", Value: "t2"}, {Key: "name", Value: "Bob Jones"}},
				bson.D{{Key: "_id", Value: "t3"}, {Key: "name", Value: "Alice Johnson"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "Alice"}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// TestQuery_regex_advanced exercises $regex with the 'x' (extended) flag.
// Dongo supports basic $regex but not all PCRE option flags.
func TestQuery_regex_advanced(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_regex_advanced",
		Support: harness.DongoMongoOnly,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 'x' flag: extended mode — whitespace in pattern is ignored.
			filter := bson.D{{Key: "name", Value: bson.D{
				{Key: "$regex", Value: "ali  ce"},
				{Key: "$options", Value: "ix"},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// TestQuery_regex_basic uses $regex with only case-insensitive flag — expected to work in Dongo.
func TestQuery_regex_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_regex_basic",
		Support: harness.DongoFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^[AB]"}}}}
			return findIDs(ctx, col, filter)
		},
	})
}
