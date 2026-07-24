package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// queryDocs is a rich dataset for exercising query operators.
var queryDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "q1"}, {Key: "name", Value: "Alice"},
		{Key: "age", Value: int32(25)}, {Key: "score", Value: 8.5},
		{Key: "active", Value: true}, {Key: "tags", Value: bson.A{"go", "cloud", "nosql"}},
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
		{Key: "active", Value: false}, {Key: "tags", Value: bson.A{"api"}},
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

func TestQuery_eq(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_eq",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "age", Value: bson.D{{Key: "$eq", Value: int32(30)}}}})
		},
	})
}

func TestQuery_ne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_ne",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "active", Value: bson.D{{Key: "$ne", Value: true}}}})
		},
	})
}

func TestQuery_gt_gte(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_gt_gte",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(30)}}}})
		},
	})
}

func TestQuery_lt_lte(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_lt_lte",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "age", Value: bson.D{{Key: "$lte", Value: int32(25)}}}})
		},
	})
}

func TestQuery_in(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_in",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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

func TestQuery_and(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_and",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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

func TestQuery_exists_true(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_exists_true",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: true}}}})
		},
	})
}

func TestQuery_exists_false(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_exists_false",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: false}}}})
		},
	})
}

func TestQuery_type_string(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_string",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "name", Value: bson.D{{Key: "$type", Value: "string"}}}})
		},
	})
}

func TestQuery_type_bool(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_bool",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "active", Value: bson.D{{Key: "$type", Value: "bool"}}}})
		},
	})
}

func TestQuery_type_number(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_number",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "number" matches int32, int64, double, decimal128.
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$type", Value: "number"}}}})
		},
	})
}

func TestQuery_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_all",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Docs where the tags array has exactly 2 elements.
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$size", Value: int32(2)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// TestQuery_where uses $where (JavaScript evaluation), deprecated since MongoDB 4.4
// and unsupported in DumboDB.
func TestQuery_where(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_where",
		Support: harness.DumboDBMongoOnly,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findIDs(ctx, col, bson.D{{Key: "$where", Value: "this.age > 30"}})
		},
	})
}

// TestQuery_text exercises $text search (requires a text index).
// DumboDB does not support text indexes or $text queries.
func TestQuery_text(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_text",
		Support: harness.DumboDBMongoOnly,
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
// DumboDB supports basic $regex but not all PCRE option flags.
func TestQuery_regex_advanced(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_regex_advanced",
		Support: harness.DumboDBMongoOnly,
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

// TestQuery_regex_basic uses $regex with only case-insensitive flag — expected to work in DumboDB.
func TestQuery_regex_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_regex_basic",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^[AB]"}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

// bitwiseDocs contains documents with integer flags for bitwise operator tests.
var bitwiseDocs = []interface{}{
	bson.D{{Key: "_id", Value: "b1"}, {Key: "flags", Value: int32(13)}},  // 0b00001101 — bits 0,2,3 set
	bson.D{{Key: "_id", Value: "b2"}, {Key: "flags", Value: int32(6)}},   // 0b00000110 — bits 1,2 set
	bson.D{{Key: "_id", Value: "b3"}, {Key: "flags", Value: int32(240)}}, // 0b11110000 — bits 4-7 set
	bson.D{{Key: "_id", Value: "b4"}, {Key: "flags", Value: int32(0)}},   // no bits set
	bson.D{{Key: "_id", Value: "b5"}, {Key: "flags", Value: int32(255)}}, // all bits 0-7 set
}

func insertBitwiseDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, bitwiseDocs)
	return err
}

// geoDocs contains documents with GeoJSON Point coordinates for geospatial tests.
var geoDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "g1"}, {Key: "name", Value: "New York"},
		{Key: "loc", Value: bson.D{
			{Key: "type", Value: "Point"},
			{Key: "coordinates", Value: bson.A{-74.006, 40.7128}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "g2"}, {Key: "name", Value: "Los Angeles"},
		{Key: "loc", Value: bson.D{
			{Key: "type", Value: "Point"},
			{Key: "coordinates", Value: bson.A{-118.2437, 34.0522}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "g3"}, {Key: "name", Value: "Chicago"},
		{Key: "loc", Value: bson.D{
			{Key: "type", Value: "Point"},
			{Key: "coordinates", Value: bson.A{-87.6298, 41.8781}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "g4"}, {Key: "name", Value: "London"},
		{Key: "loc", Value: bson.D{
			{Key: "type", Value: "Point"},
			{Key: "coordinates", Value: bson.A{-0.1276, 51.5074}},
		}},
	},
}

func insertGeoDocsWithIndex(ctx context.Context, col *mongo.Collection) error {
	idxModel := mongo.IndexModel{
		Keys: bson.D{{Key: "loc", Value: "2dsphere"}},
	}
	if _, err := col.Indexes().CreateOne(ctx, idxModel); err != nil {
		return err
	}
	_, err := col.InsertMany(ctx, geoDocs)
	return err
}

// nestedDocs contains documents with nested structures and embedded arrays.
var nestedDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "n1"}, {Key: "name", Value: "Alice"},
		{Key: "address", Value: bson.D{
			{Key: "city", Value: "New York"}, {Key: "zip", Value: "10001"},
		}},
		{Key: "grades", Value: bson.A{
			bson.D{{Key: "subject", Value: "math"}, {Key: "score", Value: int32(90)}},
			bson.D{{Key: "subject", Value: "english"}, {Key: "score", Value: int32(85)}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "n2"}, {Key: "name", Value: "Bob"},
		{Key: "address", Value: bson.D{
			{Key: "city", Value: "Los Angeles"}, {Key: "zip", Value: "90001"},
		}},
		{Key: "grades", Value: bson.A{
			bson.D{{Key: "subject", Value: "math"}, {Key: "score", Value: int32(75)}},
			bson.D{{Key: "subject", Value: "english"}, {Key: "score", Value: int32(92)}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "n3"}, {Key: "name", Value: "Carol"},
		{Key: "address", Value: bson.D{
			{Key: "city", Value: "Chicago"}, {Key: "zip", Value: "60601"},
		}},
		{Key: "grades", Value: bson.A{
			bson.D{{Key: "subject", Value: "math"}, {Key: "score", Value: int32(95)}},
			bson.D{{Key: "subject", Value: "english"}, {Key: "score", Value: int32(88)}},
		}},
	},
	bson.D{
		{Key: "_id", Value: "n4"}, {Key: "name", Value: "Dave"},
		{Key: "address", Value: bson.D{
			{Key: "city", Value: "New York"}, {Key: "zip", Value: "10002"},
		}},
		{Key: "grades", Value: bson.A{
			bson.D{{Key: "subject", Value: "math"}, {Key: "score", Value: int32(60)}},
			bson.D{{Key: "subject", Value: "science"}, {Key: "score", Value: int32(70)}},
		}},
	},
}

func insertNestedDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, nestedDocs)
	return err
}

// typedDocs is initialized in init() to allow use of primitive constructors.
var (
	typedSampleOID     primitive.ObjectID
	typedSampleDate    primitive.DateTime
	typedSampleDecimal primitive.Decimal128
	typedSampleBinary  primitive.Binary
	typedDocs          []interface{}
)

func init() {
	typedSampleOID = primitive.NewObjectIDFromTimestamp(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	typedSampleDate = primitive.NewDateTimeFromTime(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	var err error
	typedSampleDecimal, err = primitive.ParseDecimal128("3.14159")
	if err != nil {
		panic(err)
	}
	typedSampleBinary = primitive.Binary{Subtype: 0x00, Data: []byte{0xDE, 0xAD, 0xBE, 0xEF}}
	typedDocs = []interface{}{
		bson.D{
			{Key: "_id", Value: "ty1"},
			{Key: "int32Field", Value: int32(42)},
			{Key: "stringField", Value: "hello"},
		},
		bson.D{
			{Key: "_id", Value: "ty2"},
			{Key: "int64Field", Value: int64(9999999999)},
			{Key: "doubleField", Value: float64(3.14)},
		},
		bson.D{
			{Key: "_id", Value: "ty3"},
			{Key: "dateField", Value: typedSampleDate},
			{Key: "nullField", Value: nil},
		},
		bson.D{
			{Key: "_id", Value: "ty4"},
			{Key: "objectIdField", Value: typedSampleOID},
			{Key: "binaryField", Value: typedSampleBinary},
		},
		bson.D{
			{Key: "_id", Value: "ty5"},
			{Key: "decimalField", Value: typedSampleDecimal},
			{Key: "arrayField", Value: bson.A{int32(1), int32(2), int32(3)}},
		},
		bson.D{
			{Key: "_id", Value: "ty6"},
			{Key: "objectField", Value: bson.D{{Key: "nested", Value: "value"}}},
			{Key: "boolField", Value: true},
		},
	}
}

func insertTypedDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, typedDocs)
	return err
}

// findWithProjection runs a Find sorted by _id and returns documents with the given projection.
func findWithProjection(ctx context.Context, col *mongo.Collection, filter, projection interface{}) (interface{}, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetProjection(projection)
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

// findSortedIDs runs a Find with the given sort spec and returns just _id fields in sort order.
func findSortedIDs(ctx context.Context, col *mongo.Collection, filter, sort interface{}) (interface{}, error) {
	opts := options.Find().
		SetSort(sort).
		SetProjection(bson.D{{Key: "_id", Value: 1}})
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

func TestQuery_expr_field_eq(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_field_eq",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "x", Value: int32(5)}, {Key: "y", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "x", Value: int32(3)}, {Key: "y", Value: int32(7)}},
				bson.D{{Key: "_id", Value: "e3"}, {Key: "x", Value: int32(8)}, {Key: "y", Value: int32(8)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where x == y (field-to-field equality).
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$x", "$y"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_expr_field_gt(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_field_gt",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "x", Value: int32(5)}, {Key: "y", Value: int32(3)}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "x", Value: int32(2)}, {Key: "y", Value: int32(7)}},
				bson.D{{Key: "_id", Value: "e3"}, {Key: "x", Value: int32(4)}, {Key: "y", Value: int32(4)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where x > y.
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{"$x", "$y"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_expr_field_lt(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_field_lt",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "a", Value: int32(10)}, {Key: "b", Value: int32(20)}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "a", Value: int32(30)}, {Key: "b", Value: int32(15)}},
				bson.D{{Key: "_id", Value: "e3"}, {Key: "a", Value: int32(5)}, {Key: "b", Value: int32(5)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where a < b.
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{"$a", "$b"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_expr_add(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_add",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "price", Value: int32(80)}, {Key: "tax", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "price", Value: int32(40)}, {Key: "tax", Value: int32(5)}},
				bson.D{{Key: "_id", Value: "e3"}, {Key: "price", Value: int32(60)}, {Key: "tax", Value: int32(15)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where price + tax > 80.
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
				bson.D{{Key: "$add", Value: bson.A{"$price", "$tax"}}},
				int32(80),
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_expr_subtract(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_subtract",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "revenue", Value: int32(100)}, {Key: "cost", Value: int32(40)}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "revenue", Value: int32(100)}, {Key: "cost", Value: int32(90)}},
				bson.D{{Key: "_id", Value: "e3"}, {Key: "revenue", Value: int32(100)}, {Key: "cost", Value: int32(60)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where revenue - cost > 50.
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
				bson.D{{Key: "$subtract", Value: bson.A{"$revenue", "$cost"}}},
				int32(50),
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_expr_multiply(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_multiply",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "qty", Value: int32(3)}, {Key: "price", Value: int32(50)}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "qty", Value: int32(1)}, {Key: "price", Value: int32(200)}},
				bson.D{{Key: "_id", Value: "e3"}, {Key: "qty", Value: int32(5)}, {Key: "price", Value: int32(10)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where qty * price > 100.
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
				bson.D{{Key: "$multiply", Value: bson.A{"$qty", "$price"}}},
				int32(100),
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_expr_literal_compare(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_expr_literal_compare",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $expr comparing field against literal: find docs where score > 7.0.
			filter := bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{"$score", 7.0}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_mod_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_mod_basic",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where age % 5 == 0 (ages 25, 30, 35, 40).
			filter := bson.D{{Key: "age", Value: bson.D{{Key: "$mod", Value: bson.A{int32(5), int32(0)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_mod_nonzero_remainder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_mod_nonzero_remainder",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs where age % 3 == 1.
			filter := bson.D{{Key: "age", Value: bson.D{{Key: "$mod", Value: bson.A{int32(3), int32(1)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_mod_even(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_mod_even",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "val", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "val", Value: int32(11)}},
				bson.D{{Key: "_id", Value: "m3"}, {Key: "val", Value: int32(12)}},
				bson.D{{Key: "_id", Value: "m4"}, {Key: "val", Value: int32(13)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find even values: val % 2 == 0.
			filter := bson.D{{Key: "val", Value: bson.D{{Key: "$mod", Value: bson.A{int32(2), int32(0)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_jsonSchema_required(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_jsonSchema_required",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Match docs that have both 'name' and 'score' fields.
			filter := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: bson.A{"name", "score"}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_jsonSchema_bsonType(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_jsonSchema_bsonType",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Match docs where 'age' is an integer.
			filter := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "properties", Value: bson.D{
					{Key: "age", Value: bson.D{{Key: "bsonType", Value: "int"}}},
				}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_jsonSchema_properties_minimum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_jsonSchema_properties_minimum",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Match docs where 'score' exists and is at least 7.0.
			filter := bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: bson.A{"score"}},
				{Key: "properties", Value: bson.D{
					{Key: "score", Value: bson.D{
						{Key: "bsonType", Value: "double"},
						{Key: "minimum", Value: 7.0},
					}},
				}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_regex_multiline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_regex_multiline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "r1"}, {Key: "text", Value: "line one\nline two"}},
				bson.D{{Key: "_id", Value: "r2"}, {Key: "text", Value: "first line\nsecond line"}},
				bson.D{{Key: "_id", Value: "r3"}, {Key: "text", Value: "single line only"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 'm' flag: ^ matches start of each line. Matches r1 (has a line starting with "line").
			filter := bson.D{{Key: "text", Value: bson.D{
				{Key: "$regex", Value: "^line"},
				{Key: "$options", Value: "m"},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_regex_dotall(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_regex_dotall",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "r1"}, {Key: "text", Value: "start\nmiddle\nend"}},
				bson.D{{Key: "_id", Value: "r2"}, {Key: "text", Value: "all on one line"}},
				bson.D{{Key: "_id", Value: "r3"}, {Key: "text", Value: "no match here"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 's' flag: dot matches newlines. Without 's', start.+end would not cross lines.
			filter := bson.D{{Key: "text", Value: bson.D{
				{Key: "$regex", Value: "start.+end"},
				{Key: "$options", Value: "s"},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAllClear_bitmask(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAllClear_bitmask",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// bitmask 2 (bit 1): find docs where bit 1 is clear.
			// b1=13 (bit1=0 ✓), b3=240 (bit1=0 ✓), b4=0 (bit1=0 ✓).
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAllClear", Value: int32(2)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAllClear_positions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAllClear_positions",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Positions [1, 4]: find docs where bits 1 and 4 are both clear.
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAllClear", Value: bson.A{int32(1), int32(4)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAllSet_bitmask(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAllSet_bitmask",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// bitmask 5 (0b101): find docs where bits 0 and 2 are both set.
			// b1=13 (0b1101 ✓), b5=255 (all ✓).
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAllSet", Value: int32(5)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAllSet_positions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAllSet_positions",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Positions [0, 3]: find docs where bits 0 and 3 are both set.
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAllSet", Value: bson.A{int32(0), int32(3)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAnyClear_bitmask(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAnyClear_bitmask",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// bitmask 255: find docs where any of bits 0-7 is clear.
			// Only b5=255 (all bits set) does NOT match.
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAnyClear", Value: int32(255)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAnyClear_positions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAnyClear_positions",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Positions [0,1,2,3]: find docs where any of bits 0-3 is clear.
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAnyClear", Value: bson.A{int32(0), int32(1), int32(2), int32(3)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAnySet_bitmask(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAnySet_bitmask",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// bitmask 48 (0b110000): find docs where bit 4 or bit 5 is set.
			// b3=240 (0b11110000 ✓), b5=255 (all ✓).
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAnySet", Value: int32(48)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_bitsAnySet_positions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_bitsAnySet_positions",
		Support: harness.DumboDBFull,
		Setup:   insertBitwiseDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Positions [4,5,6,7]: find docs where any high bit (4-7) is set.
			filter := bson.D{{Key: "flags", Value: bson.D{{Key: "$bitsAnySet", Value: bson.A{int32(4), int32(5), int32(6), int32(7)}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_within_box(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_within_box",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Box covering eastern US: lon [-80,-65], lat [35,45]. Matches New York.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$box", Value: bson.A{
					bson.A{-80.0, 35.0},
					bson.A{-65.0, 45.0},
				}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_within_polygon(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_within_polygon",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Rough bounding box around UK. Matches London.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$polygon", Value: bson.A{
					bson.A{-5.0, 50.0},
					bson.A{2.0, 50.0},
					bson.A{2.0, 55.0},
					bson.A{-5.0, 55.0},
				}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_within_centerSphere(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_within_centerSphere",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// 1000km radius around Chicago. Radius in radians = 1000/6371.
			radiusRad := 1000.0 / 6371.0
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoWithin", Value: bson.D{
				{Key: "$centerSphere", Value: bson.A{bson.A{-87.6298, 41.8781}, radiusRad}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_near(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_near",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $near from New York, max 500km (500000m). Matches New York itself.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{-74.006, 40.7128}},
				}},
				{Key: "$maxDistance", Value: int32(500000)},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_nearSphere(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_nearSphere",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $nearSphere from Chicago, max 800km.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{-87.6298, 41.8781}},
				}},
				{Key: "$maxDistance", Value: int32(800000)},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_intersects_point(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_intersects_point",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $geoIntersects with a Point: finds documents whose coordinates match exactly.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Point"},
					{Key: "coordinates", Value: bson.A{-74.006, 40.7128}},
				}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_geo_intersects_polygon(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_geo_intersects_polygon",
		Support: harness.DumboDBFull,
		Setup:   insertGeoDocsWithIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Polygon covering western US. Should intersect Los Angeles.
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$geoIntersects", Value: bson.D{
				{Key: "$geometry", Value: bson.D{
					{Key: "type", Value: "Polygon"},
					{Key: "coordinates", Value: bson.A{bson.A{
						bson.A{-125.0, 30.0},
						bson.A{-115.0, 30.0},
						bson.A{-115.0, 42.0},
						bson.A{-125.0, 42.0},
						bson.A{-125.0, 30.0}, // close ring
					}}},
				}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_elemMatch_embedded_docs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_elemMatch_embedded_docs",
		Support: harness.DumboDBFull,
		Setup:   insertNestedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs with a grade where subject="math" AND score >= 90.
			filter := bson.D{{Key: "grades", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "subject", Value: "math"},
				{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(90)}}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_elemMatch_embedded_multi_cond(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_elemMatch_embedded_multi_cond",
		Support: harness.DumboDBFull,
		Setup:   insertNestedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Find docs with any grade scoring between 85 and 92 inclusive.
			filter := bson.D{{Key: "grades", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "score", Value: bson.D{
					{Key: "$gte", Value: int32(85)},
					{Key: "$lte", Value: int32(92)},
				}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_array_dot_notation_index(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_array_dot_notation_index",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a1"}, {Key: "vals", Value: bson.A{int32(10), int32(20), int32(30)}}},
				bson.D{{Key: "_id", Value: "a2"}, {Key: "vals", Value: bson.A{int32(5), int32(15), int32(25)}}},
				bson.D{{Key: "_id", Value: "a3"}, {Key: "vals", Value: bson.A{int32(10), int32(10), int32(10)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Query first element by position: vals.0 == 10.
			filter := bson.D{{Key: "vals.0", Value: int32(10)}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_array_dot_notation_subfield(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_array_dot_notation_subfield",
		Support: harness.DumboDBFull,
		Setup:   insertNestedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Dot-notation query on embedded document field.
			filter := bson.D{{Key: "address.city", Value: "New York"}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_all_embedded_docs(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_all_embedded_docs",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ae1"}, {Key: "items", Value: bson.A{
					bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}},
					bson.D{{Key: "x", Value: int32(3)}, {Key: "y", Value: int32(4)}},
				}}},
				bson.D{{Key: "_id", Value: "ae2"}, {Key: "items", Value: bson.A{
					bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}},
				}}},
				bson.D{{Key: "_id", Value: "ae3"}, {Key: "items", Value: bson.A{
					bson.D{{Key: "x", Value: int32(5)}, {Key: "y", Value: int32(6)}},
				}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $all matching exact embedded documents.
			filter := bson.D{{Key: "items", Value: bson.D{{Key: "$all", Value: bson.A{
				bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}},
			}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_all_single_item(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_all_single_item",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $all with single item — semantically equivalent to value-in-array match.
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$all", Value: bson.A{"nosql"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_array_exact_equality(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_array_exact_equality",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Exact array equality match (order and length must match).
			filter := bson.D{{Key: "tags", Value: bson.A{"db"}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_size_zero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_size_zero",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "items", Value: bson.A{}}},
				bson.D{{Key: "_id", Value: "s2"}, {Key: "items", Value: bson.A{int32(1)}}},
				bson.D{{Key: "_id", Value: "s3"}, {Key: "items", Value: bson.A{int32(1), int32(2)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "items", Value: bson.D{{Key: "$size", Value: int32(0)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_nested_document_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_nested_document_query",
		Support: harness.DumboDBFull,
		Setup:   insertNestedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "address.city", Value: bson.D{{Key: "$in", Value: bson.A{"Chicago", "Los Angeles"}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_deep_nested_dot_notation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_deep_nested_dot_notation",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "a", Value: bson.D{
					{Key: "b", Value: bson.D{{Key: "c", Value: int32(10)}}},
				}}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "a", Value: bson.D{
					{Key: "b", Value: bson.D{{Key: "c", Value: int32(20)}}},
				}}},
				bson.D{{Key: "_id", Value: "d3"}, {Key: "a", Value: bson.D{
					{Key: "b", Value: bson.D{{Key: "c", Value: int32(30)}}},
				}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Three-level deep dot notation.
			filter := bson.D{{Key: "a.b.c", Value: bson.D{{Key: "$gt", Value: int32(15)}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_proj_include_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_include_fields",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			proj := bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}, {Key: "age", Value: 1}}
			return findWithProjection(ctx, col, bson.D{}, proj)
		},
	})
}

func TestQuery_proj_exclude_fields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_exclude_fields",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			proj := bson.D{{Key: "tags", Value: 0}, {Key: "active", Value: 0}}
			return findWithProjection(ctx, col, bson.D{}, proj)
		},
	})
}

func TestQuery_proj_exclude_id(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_exclude_id",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			proj := bson.D{{Key: "_id", Value: 0}, {Key: "name", Value: 1}}
			return findWithProjection(ctx, col, bson.D{}, proj)
		},
	})
}

func TestQuery_proj_slice_first_n(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_slice_first_n",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $slice: return only the first 2 tags.
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$exists", Value: true}}}}
			proj := bson.D{{Key: "_id", Value: 1}, {Key: "tags", Value: bson.D{{Key: "$slice", Value: int32(2)}}}}
			return findWithProjection(ctx, col, filter, proj)
		},
	})
}

func TestQuery_proj_slice_last_n(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_slice_last_n",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $slice: negative value returns last N elements.
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$exists", Value: true}}}}
			proj := bson.D{{Key: "_id", Value: 1}, {Key: "tags", Value: bson.D{{Key: "$slice", Value: int32(-1)}}}}
			return findWithProjection(ctx, col, filter, proj)
		},
	})
}

func TestQuery_proj_slice_skip_limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_slice_skip_limit",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "sl1"}, {Key: "nums", Value: bson.A{int32(1), int32(2), int32(3), int32(4), int32(5)}}},
				bson.D{{Key: "_id", Value: "sl2"}, {Key: "nums", Value: bson.A{int32(10), int32(20), int32(30), int32(40)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $slice [skip, limit]: skip 1, take 3 — elements at index 1,2,3.
			proj := bson.D{{Key: "_id", Value: 1}, {Key: "nums", Value: bson.D{{Key: "$slice", Value: bson.A{int32(1), int32(3)}}}}}
			return findWithProjection(ctx, col, bson.D{}, proj)
		},
	})
}

func TestQuery_proj_positional(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_positional",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p1"}, {Key: "scores", Value: bson.A{int32(70), int32(85), int32(90)}}},
				bson.D{{Key: "_id", Value: "p2"}, {Key: "scores", Value: bson.A{int32(60), int32(95), int32(80)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Positional $ projection: returns the first element matching the query condition.
			filter := bson.D{{Key: "scores", Value: bson.D{{Key: "$gte", Value: int32(85)}}}}
			proj := bson.D{{Key: "_id", Value: 1}, {Key: "scores.$", Value: 1}}
			return findWithProjection(ctx, col, filter, proj)
		},
	})
}

func TestQuery_proj_elemMatch_projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_proj_elemMatch_projection",
		Support: harness.DumboDBFull,
		Setup:   insertNestedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $elemMatch in projection: return only the first matching grade per document.
			proj := bson.D{
				{Key: "_id", Value: 1},
				{Key: "grades", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
					{Key: "subject", Value: "math"},
					{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(90)}}},
				}}}},
			}
			return findWithProjection(ctx, col, bson.D{}, proj)
		},
	})
}

func TestQuery_sort_multi_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_multi_field",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sort by active DESC, then name ASC.
			return findSortedIDs(ctx, col, bson.D{},
				bson.D{{Key: "active", Value: -1}, {Key: "name", Value: 1}})
		},
	})
}

func TestQuery_sort_desc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_desc",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findSortedIDs(ctx, col, bson.D{}, bson.D{{Key: "age", Value: -1}})
		},
	})
}

func TestQuery_sort_natural_asc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_natural_asc",
		// MongoOnly: $natural (physical/insertion order) is not a supported
		// concept in DumboDB. Documents are keyed by hash(_id) in a versioned
		// prolly tree, which has no insertion/disk order to expose. Support
		// is not planned, so this runs on MongoDB only.
		Support: harness.DumboDBMongoOnly,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findSortedIDs(ctx, col, bson.D{}, bson.D{{Key: "$natural", Value: 1}})
		},
	})
}

func TestQuery_sort_natural_desc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_natural_desc",
		// MongoOnly: $natural (physical/insertion order) is not a supported
		// concept in DumboDB. Documents are keyed by hash(_id) in a versioned
		// prolly tree, which has no insertion/disk order to expose. Support
		// is not planned, so this runs on MongoDB only.
		Support: harness.DumboDBMongoOnly,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findSortedIDs(ctx, col, bson.D{}, bson.D{{Key: "$natural", Value: -1}})
		},
	})
}

func TestQuery_sort_with_limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_with_limit",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Top 3 by score descending (only docs with score field).
			opts := options.Find().
				SetSort(bson.D{{Key: "score", Value: -1}}).
				SetLimit(3).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: true}}}}, opts)
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
		},
	})
}

func TestQuery_sort_on_array_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_on_array_field",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sort on array field: MongoDB sorts by the minimum element of the array.
			filter := bson.D{{Key: "tags", Value: bson.D{{Key: "$exists", Value: true}}}}
			return findSortedIDs(ctx, col, filter, bson.D{{Key: "tags", Value: 1}})
		},
	})
}

func TestQuery_sort_missing_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_missing_field",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Sort on 'score' which is absent from q6. MongoDB places null/missing first on ASC.
			return findSortedIDs(ctx, col, bson.D{}, bson.D{{Key: "score", Value: 1}})
		},
	})
}

func TestQuery_sort_nested_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_sort_nested_field",
		Support: harness.DumboDBFull,
		Setup:   insertNestedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findSortedIDs(ctx, col, bson.D{}, bson.D{{Key: "address.city", Value: 1}})
		},
	})
}

func TestQuery_type_int(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_int",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "int" alias matches BSON int32.
			return findIDs(ctx, col, bson.D{{Key: "int32Field", Value: bson.D{{Key: "$type", Value: "int"}}}})
		},
	})
}

func TestQuery_type_long(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_long",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "long" alias matches BSON int64.
			return findIDs(ctx, col, bson.D{{Key: "int64Field", Value: bson.D{{Key: "$type", Value: "long"}}}})
		},
	})
}

func TestQuery_type_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_double",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "double" alias matches BSON float64.
			return findIDs(ctx, col, bson.D{{Key: "doubleField", Value: bson.D{{Key: "$type", Value: "double"}}}})
		},
	})
}

func TestQuery_type_decimal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_decimal",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "decimal" alias matches BSON Decimal128.
			return findIDs(ctx, col, bson.D{{Key: "decimalField", Value: bson.D{{Key: "$type", Value: "decimal"}}}})
		},
	})
}

func TestQuery_type_objectid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_objectid",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "objectId" alias matches BSON ObjectID.
			return findIDs(ctx, col, bson.D{{Key: "objectIdField", Value: bson.D{{Key: "$type", Value: "objectId"}}}})
		},
	})
}

func TestQuery_type_date(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_date",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "date" alias matches BSON DateTime.
			return findIDs(ctx, col, bson.D{{Key: "dateField", Value: bson.D{{Key: "$type", Value: "date"}}}})
		},
	})
}

func TestQuery_type_bindata(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_bindata",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "binData" alias matches BSON Binary.
			return findIDs(ctx, col, bson.D{{Key: "binaryField", Value: bson.D{{Key: "$type", Value: "binData"}}}})
		},
	})
}

func TestQuery_type_null(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_null",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "null" alias matches BSON null values.
			return findIDs(ctx, col, bson.D{{Key: "nullField", Value: bson.D{{Key: "$type", Value: "null"}}}})
		},
	})
}

func TestQuery_type_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_array",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "array" alias matches BSON array fields.
			return findIDs(ctx, col, bson.D{{Key: "arrayField", Value: bson.D{{Key: "$type", Value: "array"}}}})
		},
	})
}

func TestQuery_type_object(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_object",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "object" alias matches BSON embedded documents.
			return findIDs(ctx, col, bson.D{{Key: "objectField", Value: bson.D{{Key: "$type", Value: "object"}}}})
		},
	})
}

func TestQuery_type_multiple(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_multiple",
		Support: harness.DumboDBFull,
		Setup:   insertTypedDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $type accepts an array of type aliases: match int32 OR int64.
			return findIDs(ctx, col, bson.D{{Key: "int32Field", Value: bson.D{{Key: "$type", Value: bson.A{"int", "long"}}}}})
		},
	})
}

func TestQuery_type_numeric_code(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_type_numeric_code",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Numeric type code: 2 = string.
			return findIDs(ctx, col, bson.D{{Key: "name", Value: bson.D{{Key: "$type", Value: int32(2)}}}})
		},
	})
}

func TestQuery_eq_null(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_eq_null",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $eq null matches both explicit null values and missing fields.
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$eq", Value: nil}}}})
		},
	})
}

func TestQuery_ne_null(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_ne_null",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $ne null matches only docs where the field exists and is not null.
			return findIDs(ctx, col, bson.D{{Key: "score", Value: bson.D{{Key: "$ne", Value: nil}}}})
		},
	})
}

func TestQuery_in_empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_in_empty",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $in with empty array: matches no documents.
			return findIDs(ctx, col, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{}}}}})
		},
	})
}

func TestQuery_nin_empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_nin_empty",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $nin with empty array: matches all documents.
			return findIDs(ctx, col, bson.D{{Key: "_id", Value: bson.D{{Key: "$nin", Value: bson.A{}}}}})
		},
	})
}

func TestQuery_implicit_and_multi_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_implicit_and_multi_field",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Implicit AND: multiple top-level keys must all match.
			filter := bson.D{
				{Key: "active", Value: true},
				{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(25)}}},
			}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_and_same_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_and_same_field",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Explicit $and needed when the same field appears in multiple conditions.
			filter := bson.D{{Key: "$and", Value: bson.A{
				bson.D{{Key: "age", Value: bson.D{{Key: "$gt", Value: int32(24)}}}},
				bson.D{{Key: "age", Value: bson.D{{Key: "$lt", Value: int32(31)}}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_or_overlapping(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_or_overlapping",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $or with overlapping conditions: docs satisfying both should appear exactly once.
			filter := bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(28)}}}},
				bson.D{{Key: "active", Value: true}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_not_regex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_not_regex",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $not with $regex: names NOT starting with A, B, or C.
			filter := bson.D{{Key: "name", Value: bson.D{
				{Key: "$not", Value: bson.D{{Key: "$regex", Value: "^[ABC]"}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_nested_and_or(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_nested_and_or",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Nested $and inside $or.
			filter := bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "$and", Value: bson.A{
					bson.D{{Key: "active", Value: true}},
					bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: 8.0}}}},
				}}},
				bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(38)}}}},
			}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_gt_string(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_gt_string",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// String comparison: names lexicographically > "C".
			return findIDs(ctx, col, bson.D{{Key: "name", Value: bson.D{{Key: "$gt", Value: "C"}}}})
		},
	})
}

func TestQuery_ne_on_array_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_ne_on_array_field",
		Support: harness.DumboDBFull,
		Setup:   insertQueryDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $ne on an array field: docs where the tags array does NOT contain "go".
			return findIDs(ctx, col, bson.D{{Key: "tags", Value: bson.D{{Key: "$ne", Value: "go"}}}})
		},
	})
}

func TestQuery_in_mixed_types(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_in_mixed_types",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "mx1"}, {Key: "val", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "mx2"}, {Key: "val", Value: "one"}},
				bson.D{{Key: "_id", Value: "mx3"}, {Key: "val", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "mx4"}, {Key: "val", Value: nil}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $in with mixed types: int, string, and nil.
			filter := bson.D{{Key: "val", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), "one", nil}}}}}
			return findIDs(ctx, col, filter)
		},
	})
}

func TestQuery_jsonSchema_required_invalid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Query_jsonSchema_required_invalid",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "name", Value: "alice"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "name", Value: "bob"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $jsonSchema with required as a string (not an array) must return an error.
			// Both MongoDB and DumboDB should reject this malformed schema.
			_, err := col.Find(ctx, bson.D{{Key: "$jsonSchema", Value: bson.D{
				{Key: "required", Value: "not-an-array"},
			}}})
			return nil, err
		},
	})
}
