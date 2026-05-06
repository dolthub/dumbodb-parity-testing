package tests

import (
	"context"
	"math"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// ─── ObjectID ─────────────────────────────────────────────────────────────────

func TestBSON_objectid_auto_generated(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_auto_generated",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			if err != nil {
				return nil, err
			}
			// ObjectID must be non-zero
			oid, ok := res.InsertedID.(primitive.ObjectID)
			if !ok || oid.IsZero() {
				return bson.D{{Key: "valid", Value: false}}, nil
			}
			return bson.D{{Key: "valid", Value: true}}, nil
		},
	})
}

func TestBSON_objectid_explicit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_explicit",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "val", Value: "explicit"}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_objectid_query_by_oid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_query_by_oid",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "v", Value: int32(42)}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			// Verify _id round-trips as ObjectID
			gotOID, ok := result.Map()["_id"].(primitive.ObjectID)
			return bson.D{{Key: "match", Value: ok && gotOID == oid}}, nil
		},
	})
}

func TestBSON_objectid_hex_string_not_equal_oid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_hex_string_not_equal_oid",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: oid}})
			if err != nil {
				return nil, err
			}
			// Querying by hex string should NOT match (different BSON types)
			count, err := col.CountDocuments(ctx, bson.D{{Key: "_id", Value: oid.Hex()}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_objectid_from_hex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_from_hex",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "v", Value: "hex-test"}})
			if err != nil {
				return nil, err
			}
			// Parse hex back to ObjectID and query
			parsed, err := primitive.ObjectIDFromHex(oid.Hex())
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: parsed}}).Decode(&result)
			return result, err
		},
	})
}

// ─── Int32 vs Int64 ───────────────────────────────────────────────────────────

func TestBSON_int32_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int32_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "i32"}, {Key: "v", Value: int32(2147483647)}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "i32"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_int64_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int64_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "i64"}, {Key: "v", Value: int64(9223372036854775807)}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "i64"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_int32_vs_int64_equality(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int32_vs_int64_equality",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i32eq"}, {Key: "v", Value: int32(100)}},
				bson.D{{Key: "_id", Value: "i64eq"}, {Key: "v", Value: int64(100)}},
			})
			if err != nil {
				return nil, err
			}
			// MongoDB treats int32(100) == int64(100) in queries
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: int32(100)}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_int32_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int32_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "t32"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "t64"}, {Key: "v", Value: int64(1)}},
				bson.D{{Key: "_id", Value: "tdbl"}, {Key: "v", Value: 1.0}},
			})
			if err != nil {
				return nil, err
			}
			// $type: "int" matches only int32
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "int"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_int64_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int64_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "t32"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "t64"}, {Key: "v", Value: int64(1)}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "long"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Double / Float64 ─────────────────────────────────────────────────────────

func TestBSON_double_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_double_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "dbl"}, {Key: "v", Value: 3.141592653589793}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "dbl"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_double_nan(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_double_nan",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "nan"}, {Key: "v", Value: math.NaN()}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "nan"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			v, ok := result.Map()["v"].(float64)
			return bson.D{{Key: "isNaN", Value: ok && math.IsNaN(v)}}, nil
		},
	})
}

func TestBSON_double_infinity(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_double_infinity",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "inf+"}, {Key: "v", Value: math.Inf(1)}},
				bson.D{{Key: "_id", Value: "inf-"}, {Key: "v", Value: math.Inf(-1)}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "double"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_double_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_double_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "v", Value: 1.5}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "v", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "double"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Decimal128 ───────────────────────────────────────────────────────────────

func TestBSON_decimal128_roundtrip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_decimal128_roundtrip",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			d, err := primitive.ParseDecimal128("9999999999999999999.99")
			if err != nil {
				return nil, err
			}
			_, err = col.InsertOne(ctx, bson.D{{Key: "_id", Value: "dec"}, {Key: "v", Value: d}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "dec"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_decimal128_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_decimal128_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			d, err := primitive.ParseDecimal128("1.23")
			if err != nil {
				return nil, err
			}
			_, err = col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "dec1"}, {Key: "v", Value: d}},
				bson.D{{Key: "_id", Value: "dbl1"}, {Key: "v", Value: 1.23}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "decimal"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_decimal128_high_precision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_decimal128_high_precision",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Decimal128 preserves precision beyond float64
			d, err := primitive.ParseDecimal128("0.1")
			if err != nil {
				return nil, err
			}
			_, err = col.InsertOne(ctx, bson.D{{Key: "_id", Value: "hp"}, {Key: "v", Value: d}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "hp"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			got, ok := result.Map()["v"].(primitive.Decimal128)
			if !ok {
				return bson.D{{Key: "ok", Value: false}}, nil
			}
			return bson.D{{Key: "ok", Value: got.String() == "0.1"}}, nil
		},
	})
}

// ─── Boolean ──────────────────────────────────────────────────────────────────

func TestBSON_bool_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_bool_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bt"}, {Key: "active", Value: true}},
				bson.D{{Key: "_id", Value: "bf"}, {Key: "active", Value: false}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "active", Value: 1}, {Key: "_id", Value: 0}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

func TestBSON_bool_query_true(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_bool_query_true",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bt"}, {Key: "active", Value: true}},
				bson.D{{Key: "_id", Value: "bf"}, {Key: "active", Value: false}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "active", Value: true}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_bool_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_bool_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bt"}, {Key: "v", Value: true}},
				bson.D{{Key: "_id", Value: "bi"}, {Key: "v", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "bool"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Date / DateTime ──────────────────────────────────────────────────────────

func TestBSON_date_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_date_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			ts := primitive.NewDateTimeFromTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "dt1"}, {Key: "ts", Value: ts}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "dt1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_date_range_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_date_range_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			t1 := primitive.NewDateTimeFromTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			t2 := primitive.NewDateTimeFromTime(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
			t3 := primitive.NewDateTimeFromTime(time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC))
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "ts", Value: t1}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "ts", Value: t2}},
				bson.D{{Key: "_id", Value: "d3"}, {Key: "ts", Value: t3}},
			})
			if err != nil {
				return nil, err
			}
			mid := primitive.NewDateTimeFromTime(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
			end := primitive.NewDateTimeFromTime(time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC))
			count, err := col.CountDocuments(ctx, bson.D{{Key: "ts", Value: bson.D{
				{Key: "$gte", Value: mid},
				{Key: "$lte", Value: end},
			}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_date_comparison(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_date_comparison",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			earlier := primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
			later := primitive.NewDateTimeFromTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "early"}, {Key: "ts", Value: earlier}},
				bson.D{{Key: "_id", Value: "late"}, {Key: "ts", Value: later}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.Find().SetSort(bson.D{{Key: "ts", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

func TestBSON_date_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_date_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			ts := primitive.NewDateTimeFromTime(time.Now().UTC())
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "dts"}, {Key: "v", Value: ts}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: "2024-01-01"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "date"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Null ─────────────────────────────────────────────────────────────────────

func TestBSON_null_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_null_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "null1"}, {Key: "v", Value: nil}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "null1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_null_vs_missing_exists(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_null_vs_missing_exists",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "has-null"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "missing"}},
			})
			if err != nil {
				return nil, err
			}
			// $exists: false matches missing fields only
			countMissing, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$exists", Value: false}}}})
			if err != nil {
				return nil, err
			}
			// $eq: null matches both null AND missing
			countNull, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: nil}})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "missingCount", Value: countMissing},
				{Key: "nullMatchCount", Value: countNull},
			}, nil
		},
	})
}

func TestBSON_null_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_null_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "null1"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "str1"}, {Key: "v", Value: "hello"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "null"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_null_in_comparison(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_null_in_comparison",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "n1"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "n2"}, {Key: "v", Value: int32(0)}},
				bson.D{{Key: "_id", Value: "n3"}, {Key: "v", Value: ""}},
			})
			if err != nil {
				return nil, err
			}
			// Only null == null
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$eq", Value: nil}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Array ────────────────────────────────────────────────────────────────────

func TestBSON_array_empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_empty",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "ea"}, {Key: "arr", Value: bson.A{}}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "ea"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_array_nested(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_nested",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "na"}, {Key: "arr", Value: bson.A{
				bson.A{int32(1), int32(2)},
				bson.A{int32(3), int32(4)},
			}}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "na"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_array_mixed_types(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_mixed_types",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "mixed"}, {Key: "arr", Value: bson.A{
				int32(1), "two", 3.0, true, nil,
			}}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "mixed"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_array_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "arr"}, {Key: "v", Value: bson.A{1, 2}}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: "hello"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "array"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_array_sort_mixed_types(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_sort_mixed_types",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "v", Value: "string"}},
				bson.D{{Key: "_id", Value: "s2"}, {Key: "v", Value: int32(42)}},
				bson.D{{Key: "_id", Value: "s3"}, {Key: "v", Value: nil}},
				bson.D{{Key: "_id", Value: "s4"}, {Key: "v", Value: true}},
			})
			if err != nil {
				return nil, err
			}
			// MongoDB sorts by BSON type order: MinKey < Null < Numbers < Symbol < String < Object < Array < BinData < OID < Bool < Date < Timestamp < Regex < MaxKey
			opts := options.Find().SetSort(bson.D{{Key: "v", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

// ─── Embedded document ────────────────────────────────────────────────────────

func TestBSON_embedded_doc_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_embedded_doc_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "ed1"},
				{Key: "address", Value: bson.D{
					{Key: "street", Value: "123 Main St"},
					{Key: "city", Value: "Seattle"},
					{Key: "zip", Value: "98101"},
				}},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "ed1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_embedded_doc_dot_notation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_embedded_doc_dot_notation",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "e1"}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}}}},
				bson.D{{Key: "_id", Value: "e2"}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(2)}}}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "a.b", Value: int32(1)}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_embedded_doc_deep_nesting(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_embedded_doc_deep_nesting",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "deep"},
				{Key: "l1", Value: bson.D{
					{Key: "l2", Value: bson.D{
						{Key: "l3", Value: bson.D{
							{Key: "l4", Value: bson.D{
								{Key: "l5", Value: "deepest"},
							}},
						}},
					}},
				}},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "l1.l2.l3.l4.l5", Value: "deepest"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_embedded_doc_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_embedded_doc_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "obj"}, {Key: "v", Value: bson.D{{Key: "x", Value: 1}}}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: "hello"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "object"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Binary (BinData) ─────────────────────────────────────────────────────────

func TestBSON_binary_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_binary_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			bin := primitive.Binary{Subtype: 0x00, Data: []byte{0x01, 0x02, 0x03, 0xAB, 0xCD}}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "bin1"}, {Key: "data", Value: bin}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "bin1"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			got, ok := result.Map()["data"].(primitive.Binary)
			return bson.D{
				{Key: "subtype", Value: got.Subtype},
				{Key: "len", Value: int32(len(got.Data))},
				{Key: "ok", Value: ok},
			}, nil
		},
	})
}

func TestBSON_binary_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_binary_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			bin := primitive.Binary{Subtype: 0x00, Data: []byte{0xFF}}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bin"}, {Key: "v", Value: bin}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: "hello"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "binData"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── UUID as Binary subtype 4 ─────────────────────────────────────────────────

func TestBSON_uuid_binary_subtype4(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_uuid_binary_subtype4",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// UUID stored as Binary subtype 4 (16 bytes)
			uuidBytes := []byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4,
				0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}
			uuid := primitive.Binary{Subtype: 0x04, Data: uuidBytes}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "uuid1"}, {Key: "uid", Value: uuid}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "uuid1"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			got, ok := result.Map()["uid"].(primitive.Binary)
			return bson.D{
				{Key: "subtype", Value: got.Subtype},
				{Key: "len", Value: int32(len(got.Data))},
				{Key: "ok", Value: ok},
			}, nil
		},
	})
}

// ─── Regex ────────────────────────────────────────────────────────────────────

func TestBSON_regex_insert_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_regex_insert_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			re := primitive.Regex{Pattern: "^hello", Options: "i"}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "re1"}, {Key: "pattern", Value: re}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "re1"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			got, ok := result.Map()["pattern"].(primitive.Regex)
			return bson.D{
				{Key: "pattern", Value: got.Pattern},
				{Key: "options", Value: got.Options},
				{Key: "ok", Value: ok},
			}, nil
		},
	})
}

func TestBSON_regex_query_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_regex_query_match",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "r1"}, {Key: "name", Value: "hello world"}},
				bson.D{{Key: "_id", Value: "r2"}, {Key: "name", Value: "goodbye"}},
				bson.D{{Key: "_id", Value: "r3"}, {Key: "name", Value: "Hello MongoDB"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "name", Value: primitive.Regex{Pattern: "^hello", Options: "i"}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_regex_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_regex_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			re := primitive.Regex{Pattern: "foo", Options: ""}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "re"}, {Key: "v", Value: re}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: "foo"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "regex"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Timestamp (internal BSON type) ──────────────────────────────────────────

func TestBSON_timestamp_insert_roundtrip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_timestamp_insert_roundtrip",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			ts := primitive.Timestamp{T: 1700000000, I: 1}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "ts1"}, {Key: "ts", Value: ts}})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "ts1"}}).Decode(&result)
			if err != nil {
				return nil, err
			}
			got, ok := result.Map()["ts"].(primitive.Timestamp)
			return bson.D{
				{Key: "T", Value: got.T},
				{Key: "I", Value: got.I},
				{Key: "ok", Value: ok},
			}, nil
		},
	})
}

func TestBSON_timestamp_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_timestamp_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			ts := primitive.Timestamp{T: 1700000000, I: 1}
			dt := primitive.NewDateTimeFromTime(time.Now())
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bts"}, {Key: "v", Value: ts}},
				bson.D{{Key: "_id", Value: "bdt"}, {Key: "v", Value: dt}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "timestamp"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── MinKey / MaxKey ──────────────────────────────────────────────────────────

func TestBSON_minkey_maxkey_insert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_minkey_maxkey_insert",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "mk-min"}, {Key: "v", Value: primitive.MinKey{}}},
				bson.D{{Key: "_id", Value: "mk-max"}, {Key: "v", Value: primitive.MaxKey{}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: true}}, nil
		},
	})
}

func TestBSON_minkey_sort_order(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_minkey_sort_order",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "mk-min"}, {Key: "v", Value: primitive.MinKey{}}},
				bson.D{{Key: "_id", Value: "mk-max"}, {Key: "v", Value: primitive.MaxKey{}}},
				bson.D{{Key: "_id", Value: "mk-mid"}, {Key: "v", Value: int32(42)}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.Find().SetSort(bson.D{{Key: "v", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

func TestBSON_minkey_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_minkey_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "min"}, {Key: "v", Value: primitive.MinKey{}}},
				bson.D{{Key: "_id", Value: "max"}, {Key: "v", Value: primitive.MaxKey{}}},
				bson.D{{Key: "_id", Value: "int"}, {Key: "v", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			minCount, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "minKey"}}}})
			if err != nil {
				return nil, err
			}
			maxCount, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "maxKey"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "minCount", Value: minCount}, {Key: "maxCount", Value: maxCount}}, nil
		},
	})
}

// ─── String type ─────────────────────────────────────────────────────────────

func TestBSON_string_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_string_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "v", Value: "hello"}},
				bson.D{{Key: "_id", Value: "i1"}, {Key: "v", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "string"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── $type with numeric codes ─────────────────────────────────────────────────

func TestBSON_type_numeric_code_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_numeric_code_double",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d"}, {Key: "v", Value: 1.5}},
				bson.D{{Key: "_id", Value: "i"}, {Key: "v", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 1 = double
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 1}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_type_numeric_code_string(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_numeric_code_string",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s"}, {Key: "v", Value: "hello"}},
				bson.D{{Key: "_id", Value: "i"}, {Key: "v", Value: int32(42)}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 2 = string
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 2}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── $type with "number" alias ────────────────────────────────────────────────

func TestBSON_type_number_alias(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_number_alias",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			d128, _ := primitive.ParseDecimal128("1.5")
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "dbl"}, {Key: "v", Value: 1.5}},
				bson.D{{Key: "_id", Value: "i32"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "i64"}, {Key: "v", Value: int64(1)}},
				bson.D{{Key: "_id", Value: "dec"}, {Key: "v", Value: d128}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: "1"}},
			})
			if err != nil {
				return nil, err
			}
			// "number" alias matches double, int32, int64, and decimal128
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "number"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── ObjectID type filter ─────────────────────────────────────────────────────

func TestBSON_objectid_type_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_type_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "oid"}, {Key: "v", Value: oid}},
				bson.D{{Key: "_id", Value: "str"}, {Key: "v", Value: oid.Hex()}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "objectId"}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Type coercion edge cases ─────────────────────────────────────────────────

func TestBSON_no_coercion_string_number(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_no_coercion_string_number",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s"}, {Key: "v", Value: "42"}},
				bson.D{{Key: "_id", Value: "i"}, {Key: "v", Value: int32(42)}},
			})
			if err != nil {
				return nil, err
			}
			// MongoDB does NOT coerce "42" to 42 for comparison
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: int32(42)}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_type_ordering_in_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_ordering_in_sort",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Numbers sort before strings in BSON type order
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s"}, {Key: "v", Value: "apple"}},
				bson.D{{Key: "_id", Value: "n"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: true}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.Find().SetSort(bson.D{{Key: "v", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

// ─── $type with multiple types array ─────────────────────────────────────────

func TestBSON_type_multi_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_multi_filter",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s"}, {Key: "v", Value: "hello"}},
				bson.D{{Key: "_id", Value: "i"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: true}},
				bson.D{{Key: "_id", Value: "n"}, {Key: "v", Value: nil}},
			})
			if err != nil {
				return nil, err
			}
			// $type accepts an array of types
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: bson.A{"string", "int"}}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── $exists edge cases ───────────────────────────────────────────────────────

func TestBSON_exists_true(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_exists_true",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "has"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "nohas"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$exists", Value: true}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_exists_false(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_exists_false",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "has"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "nohas"}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$exists", Value: false}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Array element queries ────────────────────────────────────────────────────

func TestBSON_array_elem_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_elem_match",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a1"}, {Key: "scores", Value: bson.A{int32(10), int32(80), int32(50)}}},
				bson.D{{Key: "_id", Value: "a2"}, {Key: "scores", Value: bson.A{int32(5), int32(15), int32(25)}}},
			})
			if err != nil {
				return nil, err
			}
			// $elemMatch: at least one element satisfies ALL conditions
			count, err := col.CountDocuments(ctx, bson.D{{Key: "scores", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "$gte", Value: int32(70)},
				{Key: "$lte", Value: int32(90)},
			}}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_array_size_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_size_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "s2"}, {Key: "arr", Value: bson.A{int32(1), int32(2)}}},
				bson.D{{Key: "_id", Value: "s3"}, {Key: "arr", Value: bson.A{int32(1), int32(2), int32(3)}}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "arr", Value: bson.D{{Key: "$size", Value: int32(2)}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_array_all_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_all_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "t1"}, {Key: "tags", Value: bson.A{"go", "db", "sql"}}},
				bson.D{{Key: "_id", Value: "t2"}, {Key: "tags", Value: bson.A{"go", "db"}}},
				bson.D{{Key: "_id", Value: "t3"}, {Key: "tags", Value: bson.A{"python"}}},
			})
			if err != nil {
				return nil, err
			}
			// $all: array contains all listed values
			count, err := col.CountDocuments(ctx, bson.D{{Key: "tags", Value: bson.D{{Key: "$all", Value: bson.A{"go", "db"}}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── String field operations ──────────────────────────────────────────────────

func TestBSON_string_empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_string_empty",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "empty-str"}, {Key: "v", Value: ""}})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: ""}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_string_unicode(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_string_unicode",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "unicode"},
				{Key: "v", Value: "héllo wörld 🌍"},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "unicode"}}).Decode(&result)
			return result, err
		},
	})
}

// ─── Integer overflow / boundary values ──────────────────────────────────────

func TestBSON_int32_max_min(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int32_max_min",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i32max"}, {Key: "v", Value: int32(2147483647)}},
				bson.D{{Key: "_id", Value: "i32min"}, {Key: "v", Value: int32(-2147483648)}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 0}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

func TestBSON_int64_max_min(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_int64_max_min",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i64max"}, {Key: "v", Value: int64(9223372036854775807)}},
				bson.D{{Key: "_id", Value: "i64min"}, {Key: "v", Value: int64(-9223372036854775808)}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 0}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
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

// ─── ObjectID timestamp extraction ───────────────────────────────────────────

func TestBSON_objectid_timestamp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_objectid_timestamp",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: oid}})
			if err != nil {
				return nil, err
			}
			// Use aggregation to extract the timestamp from ObjectID
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$project", Value: bson.D{
					{Key: "ts", Value: bson.D{{Key: "$toDate", Value: "$_id"}}},
					{Key: "_id", Value: 0},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "ok", Value: false}}, nil
			}
			_, hasTs := results[0].Map()["ts"]
			return bson.D{{Key: "ok", Value: hasTs}}, nil
		},
	})
}

// ─── Nested null in array ─────────────────────────────────────────────────────

func TestBSON_null_in_array(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_null_in_array",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "null-arr"},
				{Key: "arr", Value: bson.A{int32(1), nil, int32(3)}},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "null-arr"}}).Decode(&result)
			return result, err
		},
	})
}

// ─── Multi-key index on array (query behavior) ────────────────────────────────

func TestBSON_array_field_query_any_element(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_field_query_any_element",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "q1"}, {Key: "nums", Value: bson.A{int32(1), int32(2), int32(3)}}},
				bson.D{{Key: "_id", Value: "q2"}, {Key: "nums", Value: bson.A{int32(4), int32(5)}}},
			})
			if err != nil {
				return nil, err
			}
			// Querying an array field with a scalar matches any element
			count, err := col.CountDocuments(ctx, bson.D{{Key: "nums", Value: int32(2)}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── BSON document field order ────────────────────────────────────────────────

func TestBSON_doc_field_order_preserved(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_doc_field_order_preserved",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "order"},
				{Key: "z", Value: int32(1)},
				{Key: "a", Value: int32(2)},
				{Key: "m", Value: int32(3)},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "order"}}).Decode(&result)
			return result, err
		},
	})
}

// ─── Projection with array fields ─────────────────────────────────────────────

func TestBSON_array_projection_slice(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_projection_slice",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "ps1"},
				{Key: "items", Value: bson.A{"a", "b", "c", "d", "e"}},
			})
			if err != nil {
				return nil, err
			}
			opts := options.FindOne().SetProjection(bson.D{
				{Key: "items", Value: bson.D{{Key: "$slice", Value: int32(3)}}},
				{Key: "_id", Value: 0},
			})
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "ps1"}}, opts).Decode(&result)
			return result, err
		},
	})
}

// ─── $in / $nin with mixed BSON types ────────────────────────────────────────

func TestBSON_in_mixed_types(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_in_mixed_types",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "v", Value: "one"}},
				bson.D{{Key: "_id", Value: "m3"}, {Key: "v", Value: true}},
				bson.D{{Key: "_id", Value: "m4"}, {Key: "v", Value: nil}},
			})
			if err != nil {
				return nil, err
			}
			// $in matches by type+value — int32(1) does NOT match "1"
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), "one"}}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_nin_query(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_nin_query",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "n1"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "n2"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "n3"}, {Key: "v", Value: int32(3)}},
			})
			if err != nil {
				return nil, err
			}
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$nin", Value: bson.A{int32(1), int32(3)}}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// ─── Additional $type alias tests ─────────────────────────────────────────────

func TestBSON_type_oid_numeric_code(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_oid_numeric_code",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			oid := primitive.NewObjectID()
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "oid1"}, {Key: "v", Value: oid}},
				bson.D{{Key: "_id", Value: "str1"}, {Key: "v", Value: "hello"}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 7 = objectId
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 7}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_type_bool_numeric_code(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_bool_numeric_code",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bt"}, {Key: "v", Value: true}},
				bson.D{{Key: "_id", Value: "bf"}, {Key: "v", Value: false}},
				bson.D{{Key: "_id", Value: "bi"}, {Key: "v", Value: int32(1)}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 8 = bool
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 8}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_type_date_numeric_code(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_date_numeric_code",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			ts := primitive.NewDateTimeFromTime(time.Now().UTC())
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "dt"}, {Key: "v", Value: ts}},
				bson.D{{Key: "_id", Value: "st"}, {Key: "v", Value: "2024-01-01"}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 9 = date
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 9}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_type_int32_numeric_code(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_int32_numeric_code",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i32"}, {Key: "v", Value: int32(42)}},
				bson.D{{Key: "_id", Value: "i64"}, {Key: "v", Value: int64(42)}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 16 = int32
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 16}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_type_int64_numeric_code(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_type_int64_numeric_code",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "i32"}, {Key: "v", Value: int32(42)}},
				bson.D{{Key: "_id", Value: "i64"}, {Key: "v", Value: int64(42)}},
			})
			if err != nil {
				return nil, err
			}
			// BSON type code 18 = int64
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: 18}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_null_field_update_set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_null_field_update_set",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "nu1"}, {Key: "v", Value: nil}})
			if err != nil {
				return nil, err
			}
			_, err = col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "nu1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(42)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "nu1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestBSON_double_zero_negative_zero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_double_zero_negative_zero",
		Support: harness.DumboDBXFail, // BSON comparison does not treat +0.0 and -0.0 as equal
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "pos0"}, {Key: "v", Value: 0.0}},
				bson.D{{Key: "_id", Value: "neg0"}, {Key: "v", Value: math.Copysign(0, -1)}},
			})
			if err != nil {
				return nil, err
			}
			// In MongoDB, +0.0 == -0.0
			count, err := col.CountDocuments(ctx, bson.D{{Key: "v", Value: 0.0}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestBSON_array_index_dot_notation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_array_index_dot_notation",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "ai1"}, {Key: "arr", Value: bson.A{"x", "y", "z"}}},
				bson.D{{Key: "_id", Value: "ai2"}, {Key: "arr", Value: bson.A{"a", "b", "c"}}},
			})
			if err != nil {
				return nil, err
			}
			// Query by array index using dot notation
			count, err := col.CountDocuments(ctx, bson.D{{Key: "arr.0", Value: "x"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}
