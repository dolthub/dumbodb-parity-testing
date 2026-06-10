package tests

// Red-bar parity families for secondary-index maintenance gaps.
// See dumbodb docs/design/secondary-index-structural-sharing.md:
//
//   Index_UpdateReindex_*  -- behavior W2 (updates do not re-index)
//   Index_DeleteUnindex_*  -- behavior W3 (deletes leave stale entries)
//   Index_MixedNumeric_*   -- behavior T2 (non-integer float mis-bucketing)
//   Index_LossyTypes_*     -- behavior T3 (lossy encodings corrupt counts)
//
// Tests whose behavior is currently broken are DumboDBXFail; they flip
// to DumboDBFull as the fixing phase lands.

import (
	"context"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// idxmFindIDs returns the sorted _id values matching filter, via an
// _id-sorted, _id-projected find. Deterministic across both servers.
func idxmFindIDs(ctx context.Context, col *mongo.Collection, filter interface{}) (bson.A, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: int32(1)}}).
		SetProjection(bson.D{{Key: "_id", Value: int32(1)}})
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := bson.A{}
	for _, d := range docs {
		for _, e := range d {
			if e.Key == "_id" {
				ids = append(ids, e.Value)
			}
		}
	}
	return ids, nil
}

// idxmCount runs the count command (not CountDocuments, which uses an
// aggregate) so the backend's indexed-count fast path is exercised.
func idxmCount(ctx context.Context, col *mongo.Collection, filter interface{}) (int32, error) {
	var res bson.M
	err := col.Database().RunCommand(ctx, bson.D{
		{Key: "count", Value: col.Name()},
		{Key: "query", Value: filter},
	}).Decode(&res)
	if err != nil {
		return 0, err
	}
	switch n := res["n"].(type) {
	case int32:
		return n, nil
	case int64:
		return int32(n), nil
	case float64:
		return int32(n), nil
	}
	return 0, nil
}

// idxmProbe packages the find IDs and count for one filter.
func idxmProbe(ctx context.Context, col *mongo.Collection, label string, filter interface{}) (bson.D, error) {
	ids, err := idxmFindIDs(ctx, col, filter)
	if err != nil {
		return nil, err
	}
	n, err := idxmCount(ctx, col, filter)
	if err != nil {
		return nil, err
	}
	return bson.D{
		{Key: label + "_ids", Value: ids},
		{Key: label + "_count", Value: n},
	}, nil
}

func idxmSetupNamed(values bson.D, indexField string) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, len(values))
		for _, e := range values {
			docs = append(docs, bson.D{{Key: "_id", Value: e.Key}, {Key: indexField, Value: e.Value}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: indexField, Value: int32(1)}},
		})
		return err
	}
}

// ---------------------------------------------------------------------------
// W2: Index_UpdateReindex_*
// ---------------------------------------------------------------------------

func TestIndex_UpdateReindex_Set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UpdateReindex_Set",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "u1", Value: "alpha"}, {Key: "u2", Value: "bravo"}, {Key: "u3", Value: "charlie"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "field", Value: "zulu"}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"new", bson.D{{Key: "field", Value: "zulu"}}},
				{"old", bson.D{{Key: "field", Value: "alpha"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_UpdateReindex_Unset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UpdateReindex_Unset",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "u1", Value: "alpha"}, {Key: "u2", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "field", Value: ""}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"old", bson.D{{Key: "field", Value: "alpha"}}},
				{"null", bson.D{{Key: "field", Value: nil}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_UpdateReindex_Inc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UpdateReindex_Inc",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "u1", Value: int32(10)}, {Key: "u2", Value: int32(20)}, {Key: "u3", Value: int32(30)},
		}, "n"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(5)}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"new", bson.D{{Key: "n", Value: int32(15)}}},
				{"old", bson.D{{Key: "n", Value: int32(10)}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_UpdateReindex_Replace(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UpdateReindex_Replace",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "u1", Value: "alpha"}, {Key: "u2", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "_id", Value: "u1"}, {Key: "field", Value: "yankee"}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"new", bson.D{{Key: "field", Value: "yankee"}}},
				{"old", bson.D{{Key: "field", Value: "alpha"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_UpdateReindex_UpdateMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UpdateReindex_UpdateMany",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "u1", Value: "alpha"}, {Key: "u2", Value: "alpha"}, {Key: "u3", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateMany(ctx,
				bson.D{{Key: "field", Value: "alpha"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "field", Value: "mike"}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"new", bson.D{{Key: "field", Value: "mike"}}},
				{"old", bson.D{{Key: "field", Value: "alpha"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_UpdateReindex_Upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UpdateReindex_Upsert",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "u1", Value: "alpha"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Upsert that matches an existing doc behaves as an update.
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "field", Value: "victor"}}}},
				options.Update().SetUpsert(true)); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"new", bson.D{{Key: "field", Value: "victor"}}},
				{"old", bson.D{{Key: "field", Value: "alpha"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

// ---------------------------------------------------------------------------
// W3: Index_DeleteUnindex_*
// ---------------------------------------------------------------------------

func TestIndex_DeleteUnindex_DeleteOneByID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DeleteUnindex_DeleteOneByID",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "d1", Value: "alpha"}, {Key: "d2", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: "d1"}}); err != nil {
				return nil, err
			}
			return idxmProbe(ctx, col, "deleted", bson.D{{Key: "field", Value: "alpha"}})
		},
	})
}

func TestIndex_DeleteUnindex_DeleteOneByFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DeleteUnindex_DeleteOneByFilter",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "d1", Value: "alpha"}, {Key: "d2", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteOne(ctx, bson.D{{Key: "field", Value: "alpha"}}); err != nil {
				return nil, err
			}
			return idxmProbe(ctx, col, "deleted", bson.D{{Key: "field", Value: "alpha"}})
		},
	})
}

func TestIndex_DeleteUnindex_DeleteMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DeleteUnindex_DeleteMany",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "d1", Value: "alpha"}, {Key: "d2", Value: "alpha"}, {Key: "d3", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteMany(ctx, bson.D{{Key: "field", Value: "alpha"}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"deleted", bson.D{{Key: "field", Value: "alpha"}}},
				{"kept", bson.D{{Key: "field", Value: "bravo"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_DeleteUnindex_FindOneAndDelete(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DeleteUnindex_FindOneAndDelete",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "d1", Value: "alpha"}, {Key: "d2", Value: "bravo"},
		}, "field"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var removed bson.D
			err := col.FindOneAndDelete(ctx, bson.D{{Key: "field", Value: "alpha"}}).Decode(&removed)
			if err != nil {
				return nil, err
			}
			return idxmProbe(ctx, col, "deleted", bson.D{{Key: "field", Value: "alpha"}})
		},
	})
}

// ---------------------------------------------------------------------------
// T2: Index_MixedNumeric_*
//
// Docs span int32 and non-integer float64 values over one indexed field.
// MongoDB compares numerics by value across representations.
// ---------------------------------------------------------------------------

func idxmMixedNumericSetup(ctx context.Context, col *mongo.Collection) error {
	docs := []interface{}{
		bson.D{{Key: "_id", Value: "i1"}, {Key: "n", Value: int32(1)}},
		bson.D{{Key: "_id", Value: "i2"}, {Key: "n", Value: int32(2)}},
		bson.D{{Key: "_id", Value: "i3"}, {Key: "n", Value: int32(3)}},
		bson.D{{Key: "_id", Value: "f05"}, {Key: "n", Value: float64(0.5)}},
		bson.D{{Key: "_id", Value: "f25"}, {Key: "n", Value: float64(2.5)}},
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "n", Value: int32(1)}},
	})
	return err
}

func TestIndex_MixedNumeric_EqualityUnified(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedNumeric_EqualityUnified",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "i2"}, {Key: "n", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "f2"}, {Key: "n", Value: float64(2.0)}},
				bson.D{{Key: "_id", Value: "i9"}, {Key: "n", Value: int32(9)}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "n", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"int_eq", bson.D{{Key: "n", Value: int32(2)}}},
				{"float_eq", bson.D{{Key: "n", Value: float64(2.0)}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_MixedNumeric_GtCrossesBoundary(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedNumeric_GtCrossesBoundary",
		Support: harness.DumboDBFull,
		Setup:   idxmMixedNumericSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "gt25",
				bson.D{{Key: "n", Value: bson.D{{Key: "$gt", Value: float64(2.5)}}}})
		},
	})
}

func TestIndex_MixedNumeric_GteCrossesBoundary(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedNumeric_GteCrossesBoundary",
		Support: harness.DumboDBFull,
		Setup:   idxmMixedNumericSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "gte25",
				bson.D{{Key: "n", Value: bson.D{{Key: "$gte", Value: float64(2.5)}}}})
		},
	})
}

func TestIndex_MixedNumeric_LtCrossesBoundary(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedNumeric_LtCrossesBoundary",
		Support: harness.DumboDBFull,
		Setup:   idxmMixedNumericSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"lt25", bson.D{{Key: "n", Value: bson.D{{Key: "$lt", Value: float64(2.5)}}}}},
				{"lte25", bson.D{{Key: "n", Value: bson.D{{Key: "$lte", Value: float64(2.5)}}}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}

func TestIndex_MixedNumeric_SortAscending(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_MixedNumeric_SortAscending",
		Support: harness.DumboDBFull,
		Setup:   idxmMixedNumericSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetSort(bson.D{{Key: "n", Value: int32(1)}}).
					SetProjection(bson.D{{Key: "_id", Value: int32(1)}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			order := bson.A{}
			for _, d := range docs {
				for _, e := range d {
					if e.Key == "_id" {
						order = append(order, e.Value)
					}
				}
			}
			return bson.D{{Key: "order", Value: order}}, nil
		},
	})
}

// ---------------------------------------------------------------------------
// T3: Index_LossyTypes_*
//
// Values whose KeyString encoding is lossy (Decimal128, Timestamp, NaN,
// embedded documents) must still produce MongoDB-identical find and
// count results.
// ---------------------------------------------------------------------------

func TestIndex_LossyTypes_Decimal128(t *testing.T) {
	d15, _ := primitive.ParseDecimal128("1.5")
	d25, _ := primitive.ParseDecimal128("2.5")
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_LossyTypes_Decimal128",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "dec15", Value: d15},
			{Key: "dec25", Value: d25},
			{Key: "nul", Value: nil},
			{Key: "str", Value: "x"},
		}, "f"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "dec15", bson.D{{Key: "f", Value: d15}})
		},
	})
}

func TestIndex_LossyTypes_Timestamp(t *testing.T) {
	ts1 := primitive.Timestamp{T: 100, I: 1}
	ts2 := primitive.Timestamp{T: 200, I: 2}
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_LossyTypes_Timestamp",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "ts1", Value: ts1},
			{Key: "ts2", Value: ts2},
			{Key: "nul", Value: nil},
		}, "f"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "ts1", bson.D{{Key: "f", Value: ts1}})
		},
	})
}

func TestIndex_LossyTypes_NaN(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_LossyTypes_NaN",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "nan", Value: math.NaN()},
			{Key: "nul", Value: nil},
			{Key: "one", Value: float64(1.0)},
		}, "f"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "nan", bson.D{{Key: "f", Value: math.NaN()}})
		},
	})
}

func TestIndex_LossyTypes_ObjectValues(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_LossyTypes_ObjectValues",
		Support: harness.DumboDBFull,
		Setup: idxmSetupNamed(bson.D{
			{Key: "o1", Value: bson.D{{Key: "a", Value: int32(1)}}},
			{Key: "o2", Value: bson.D{{Key: "a", Value: int32(2)}}},
			{Key: "str", Value: "x"},
		}, "f"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return idxmProbe(ctx, col, "obj",
				bson.D{{Key: "f", Value: bson.D{{Key: "a", Value: int32(1)}}}})
		},
	})
}

func TestIndex_LossyTypes_NullCountExcludesLossy(t *testing.T) {
	d15, _ := primitive.ParseDecimal128("1.5")
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_LossyTypes_NullCountExcludesLossy",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "nul"}, {Key: "f", Value: nil}},
				bson.D{{Key: "_id", Value: "missing"}},
				bson.D{{Key: "_id", Value: "dec"}, {Key: "f", Value: d15}},
				bson.D{{Key: "_id", Value: "ts"}, {Key: "f", Value: primitive.Timestamp{T: 1, I: 1}}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "f", Value: int32(1)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// MongoDB: {f: null} matches the explicit null and the
			// missing-field doc only -- never Decimal128 or Timestamp.
			return idxmProbe(ctx, col, "null", bson.D{{Key: "f", Value: nil}})
		},
	})
}
