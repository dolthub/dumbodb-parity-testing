// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// DumboDB stores every collection clustered by _id (dolthub/dumbodb#207), so
// these cases compare against a clustered MongoDB collection: natural order is
// _id order, and _id equality, ranges and sorts are served by the collection
// itself.

// cluShuffled returns ids in a fixed pseudo-random order, so insertion order
// never matches _id order.
func cluShuffled(ids []interface{}) []interface{} {
	out := append([]interface{}(nil), ids...)
	r := rand.New(rand.NewPCG(207, 206))
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func cluIntIDs(n int) []interface{} {
	ids := make([]interface{}, n)
	for i := range ids {
		ids[i] = int32(i)
	}
	return ids
}

// cluInsertIDs inserts {_id: id, n: i} for each id in a shuffled order.
func cluInsertIDs(ids []interface{}) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		for i, id := range cluShuffled(ids) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "n", Value: int32(i)}}); err != nil {
				return err
			}
		}
		return nil
	}
}

// cluFindIDs returns the _ids find returns, in the order returned.
func cluFindIDs(filter bson.D, opts ...*options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cur, err := col.Find(ctx, filter, opts...)
		if err != nil {
			return nil, err
		}
		var docs []bson.D
		if err := cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		ids := bson.A{}
		for _, d := range docs {
			ids = append(ids, d[0].Value)
		}
		return ids, nil
	}
}

// cluExplain returns the comparable subset of an explain of cmd.
func cluExplain(cmd func(col *mongo.Collection) bson.D, verbosity string) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		doc, err := explainRunExplain(ctx, col, cmd(col), verbosity)
		if err != nil {
			return nil, err
		}
		return explainExtractCritical(doc), nil
	}
}

func cluFindCmd(filter bson.D, extra ...bson.E) func(col *mongo.Collection) bson.D {
	return func(col *mongo.Collection) bson.D {
		return append(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}}, extra...)
	}
}

func cluRange(lo, hi interface{}) bson.D {
	return bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: lo}, {Key: "$lt", Value: hi}}}}
}

func cluObjectIDs(n int) []interface{} {
	ids := make([]interface{}, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range ids {
		ids[i] = primitive.NewObjectIDFromTimestamp(base.Add(time.Duration(i) * time.Minute))
	}
	return ids
}

// cluMixedIDs is one _id of every type that may be an _id, in no order.
func cluMixedIDs() []interface{} {
	return []interface{}{
		primitive.MaxKey{},
		primitive.MinKey{},
		nil,
		int32(-3), int64(7), 2.5, math.Inf(1), math.Inf(-1), math.NaN(),
		primitive.NewDecimal128(0x3040000000000000, 11), // 11
		"", "a", "ab", "b", "\u00e9",
		bson.D{{Key: "a", Value: int32(1)}},
		bson.D{},
		primitive.Binary{Subtype: 0x00, Data: []byte{0xff}},
		primitive.Binary{Subtype: 0x80, Data: []byte{0x00, 0x00}},
		primitive.NewObjectIDFromTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		false, true,
		primitive.NewDateTimeFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		primitive.NewDateTimeFromTime(time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC)),
		primitive.Timestamp{T: 5, I: 1},
		primitive.Timestamp{T: 1, I: 9},
	}
}

// cluNumericIDs covers the numeric encodings: zero, small and large
// magnitudes, fractions on both sides of zero including (-0.5, 0) (#206),
// values past 2^53 and the int64 range, infinities and decimals.
func cluNumericIDs() []interface{} {
	return []interface{}{
		int32(0), int32(1), int32(-1), int32(255), int32(256), int32(-256), int32(math.MaxInt32), int32(math.MinInt32),
		int64(1) << 53, int64(1)<<53 + 1, int64(math.MaxInt64), int64(math.MinInt64), int64(-(1 << 40)),
		0.5, 0.25, 1e-300, 5e-324, 1.5, 2.000000000000001, 123456.789, 1e20, 1e300, math.MaxFloat64,
		-0.1, -0.10000000000000002, -1e-20, -3e-20, -2e-20, -0.49, -0.5, -0.75, -1.5, -1e20, -math.MaxFloat64,
		9.223372036854776e18, -9.3e18, 1.8446744073709552e19,
		math.Inf(1), math.Inf(-1),
		primitive.NewDecimal128(0x3040000000000000, 3),  // 3
		primitive.NewDecimal128(0x303e000000000000, 35), // 3.5
		primitive.NewDecimal128(0xb03e000000000000, 1),  // -0.1
		primitive.NewDecimal128(0x3000000000000000, 1),  // 1E-32
	}
}

func cluEmbeddedIDs() []interface{} {
	return []interface{}{
		bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}},
		bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(1)}},
		bson.D{{Key: "a", Value: int32(1)}},
		bson.D{{Key: "a", Value: int32(2)}},
		bson.D{{Key: "a", Value: "x"}},
		bson.D{{Key: "b", Value: int32(0)}},
		bson.D{{Key: "a", Value: bson.D{{Key: "c", Value: int32(1)}}}},
		bson.D{{Key: "a", Value: bson.A{int32(1), int32(2)}}},
		bson.D{{Key: "a", Value: bson.A{int32(1)}}},
		bson.D{{Key: "a", Value: bson.A{}}},
		bson.D{{Key: "a", Value: primitive.Regex{Pattern: "^x", Options: "i"}}},
		bson.D{{Key: "a", Value: nil}},
		bson.D{{Key: "", Value: int32(1)}},
		bson.D{},
	}
}

func cluBinDataIDs() []interface{} {
	return []interface{}{
		primitive.Binary{Subtype: 0x00, Data: []byte{}},
		primitive.Binary{Subtype: 0x00, Data: []byte{0xff}},
		primitive.Binary{Subtype: 0x00, Data: []byte{0x00, 0x00}},
		primitive.Binary{Subtype: 0x00, Data: []byte{0x00, 0x01}},
		primitive.Binary{Subtype: 0x04, Data: []byte{0x00}},
		primitive.Binary{Subtype: 0x80, Data: []byte{0x00}},
		primitive.Binary{Subtype: 0x00, Data: make([]byte, 300)},
	}
}

func cluStringIDs() []interface{} {
	return []interface{}{"", "a", "aa", "a\x00", "a\x00b", "ab", "b", "B", "\u00e9", "\U0001F600", "zz", "a b"}
}

func cluNatural(name string, support harness.DumboDBSupport, ids []interface{}, opts ...*options.FindOptions) harness.TestCase {
	return harness.TestCase{
		Name:      name,
		Support:   support,
		Clustered: true,
		Setup:     cluInsertIDs(ids),
		Run:       cluFindIDs(bson.D{}, opts...),
	}
}

func TestClustered_NaturalOrder_Int(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_Int", harness.DumboDBFull, cluIntIDs(40)))
}

func TestClustered_NaturalOrder_Limit(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_Limit", harness.DumboDBFull, cluIntIDs(200), options.Find().SetLimit(5)))
}

func TestClustered_NaturalOrder_Reverse(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_Reverse", harness.DumboDBFull, cluIntIDs(40),
		options.Find().SetSort(bson.D{{Key: "$natural", Value: -1}})))
}

func TestClustered_NaturalOrder_ObjectID(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_ObjectID", harness.DumboDBFull, cluObjectIDs(40)))
}

func TestClustered_NaturalOrder_String(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_String", harness.DumboDBFull, cluStringIDs()))
}

func TestClustered_NaturalOrder_MixedTypes(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_MixedTypes", harness.DumboDBFull, cluMixedIDs()))
}

func TestClustered_NaturalOrder_Numeric(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_Numeric", harness.DumboDBFull, cluNumericIDs()))
}

func TestClustered_NaturalOrder_Embedded(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_Embedded", harness.DumboDBFull, cluEmbeddedIDs()))
}

func TestClustered_NaturalOrder_BinData(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_NaturalOrder_BinData", harness.DumboDBFull, cluBinDataIDs()))
}

func TestClustered_SortID_Asc(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_SortID_Asc", harness.DumboDBFull, cluMixedIDs(),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_SortID_Desc(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_SortID_Desc", harness.DumboDBFull, cluNumericIDs(),
		options.Find().SetSort(bson.D{{Key: "_id", Value: -1}})))
}

func TestClustered_SortID_Embedded(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_SortID_Embedded", harness.DumboDBFull, cluEmbeddedIDs(),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_SortID_BinData(t *testing.T) {
	harness.PairTest(t, cluNatural("Clustered_SortID_BinData", harness.DumboDBFull, cluBinDataIDs(),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func cluFilter(name string, support harness.DumboDBSupport, ids []interface{}, filter bson.D, opts ...*options.FindOptions) harness.TestCase {
	return harness.TestCase{
		Name:      name,
		Support:   support,
		Clustered: true,
		Setup:     cluInsertIDs(ids),
		Run:       cluFindIDs(filter, opts...),
	}
}

func TestClustered_Range_Unsorted(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_Unsorted", harness.DumboDBFull, cluIntIDs(200), cluRange(int32(100), int32(110))))
}

func TestClustered_Range_Sorted(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_Sorted", harness.DumboDBFull, cluIntIDs(200), cluRange(int32(100), int32(110)),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_Range_Numeric(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_Numeric", harness.DumboDBFull, cluNumericIDs(), cluRange(-1.0, 1.0),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_Range_BinData(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_BinData", harness.DumboDBFull, cluBinDataIDs(),
		bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: primitive.Binary{Subtype: 0x00, Data: []byte{0xff}}}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_Range_Embedded(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_Embedded", harness.DumboDBFull, cluEmbeddedIDs(),
		bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: bson.D{{Key: "a", Value: int32(1)}}}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_Range_String(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_String", harness.DumboDBFull, cluStringIDs(), cluRange("a", "b"),
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_Range_Desc(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_Range_Desc", harness.DumboDBFull, cluIntIDs(200), cluRange(int32(10), int32(30)),
		options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(5)))
}

// Cursor pagination: each page is {_id: {$gt: last}} with a limit, natural order.
func TestClustered_Pagination_Natural(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Pagination_Natural",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(100)),
		Run:       cluPaginate(nil),
	})
}

func TestClustered_Pagination_Sorted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Pagination_Sorted",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(100)),
		Run:       cluPaginate(bson.D{{Key: "_id", Value: 1}}),
	})
}

func cluPaginate(sort bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		var pages bson.A
		var last interface{} = primitive.MinKey{}
		for range 12 {
			opts := options.Find().SetLimit(10)
			if sort != nil {
				opts.SetSort(sort)
			}
			res, err := cluFindIDs(bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: last}}}}, opts)(ctx, col)
			if err != nil {
				return nil, err
			}
			page := res.(bson.A)
			pages = append(pages, page)
			if len(page) == 0 {
				break
			}
			last = page[len(page)-1]
		}
		return pages, nil
	}
}

func TestClustered_MinMax(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_MinMax", harness.DumboDBFull, cluIntIDs(50), bson.D{},
		options.Find().SetMin(bson.D{{Key: "_id", Value: int32(10)}}).SetMax(bson.D{{Key: "_id", Value: int32(15)}}).SetHint(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_HintID_Sort(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_HintID_Sort", harness.DumboDBFull, cluIntIDs(50), bson.D{{Key: "n", Value: bson.D{{Key: "$gte", Value: int32(0)}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint(bson.D{{Key: "_id", Value: 1}}).SetLimit(7)))
}

func TestClustered_PointLookup_NumericEquivalence(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_PointLookup_NumericEquivalence", harness.DumboDBFull, cluNumericIDs(),
		bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{int64(1), 1.0, primitive.NewDecimal128(0x3040000000000000, 3), -0.1, -1e-20, int64(1) << 53}}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

func TestClustered_PointLookup_Embedded(t *testing.T) {
	harness.PairTest(t, cluFilter("Clustered_PointLookup_Embedded", harness.DumboDBFull, cluEmbeddedIDs(),
		bson.D{{Key: "_id", Value: bson.D{{Key: "a", Value: bson.A{int32(1), int32(2)}}}}}))
}

// Distinct numeric _ids that an inexact encoding would merge (#206) must
// both insert.
func TestClustered_DistinctIDs_TinyNegatives(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_DistinctIDs_TinyNegatives",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var errs bson.A
			for _, id := range []interface{}{-0.1, -0.10000000000000002, -1e-20, -3e-20} {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}})
				errs = append(errs, err == nil)
			}
			n, err := col.CountDocuments(ctx, bson.D{})
			return bson.A{errs, n}, err
		},
	})
}

// Equal _ids of different numeric types collide.
func TestClustered_DuplicateKey_NumericTypes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_DuplicateKey_NumericTypes",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var codes bson.A
			for _, id := range []interface{}{int32(1), int64(1), 1.0, primitive.NewDecimal128(0x3040000000000000, 1)} {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}})
				codes = append(codes, mongo.IsDuplicateKeyError(err))
			}
			return codes, nil
		},
	})
}

func TestClustered_CreateCollection_ClusteredOption(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Clustered_CreateCollection_ClusteredOption",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().CreateCollection(ctx, "clu", options.CreateCollection().SetClusteredIndex(harness.ClusteredIndexOption))
			if err != nil {
				return nil, err
			}
			specs, err := col.Database().ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: "clu"}})
			if err != nil || len(specs) != 1 {
				return nil, err
			}
			var opts bson.D
			if err := bson.Unmarshal(specs[0].Options, &opts); err != nil {
				return nil, err
			}
			return opts, nil
		},
	})
}

func TestClustered_Explain_PointLookup(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Explain_PointLookup",
		Support:   harness.DumboDBXFail,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(200)),
		Run:       cluExplain(cluFindCmd(bson.D{{Key: "_id", Value: int32(150)}}), "executionStats"),
	})
}

func TestClustered_Explain_Range(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Explain_Range",
		Support:   harness.DumboDBXFail,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(200)),
		Run:       cluExplain(cluFindCmd(cluRange(int32(100), int32(110))), "executionStats"),
	})
}

func TestClustered_Explain_SortID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Explain_SortID",
		Support:   harness.DumboDBXFail,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(200)),
		Run: cluExplain(cluFindCmd(bson.D{}, bson.E{Key: "sort", Value: bson.D{{Key: "_id", Value: 1}}}, bson.E{Key: "limit", Value: int32(5)}),
			"executionStats"),
	})
}

func TestClustered_Explain_CollScan(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Explain_CollScan",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(200)),
		Run:       cluExplain(cluFindCmd(bson.D{{Key: "n", Value: int32(3)}}), "executionStats"),
	})
}

// listIndexes on a clustered collection reports the clustered _id index.
func TestClustered_ListIndexes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_ListIndexes",
		Support:   harness.DumboDBXFail,
		Clustered: true,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var specs []bson.D
			err = cur.All(ctx, &specs)
			return specs, err
		},
	})
}
