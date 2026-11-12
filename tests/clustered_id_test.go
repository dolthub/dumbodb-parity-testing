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
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"
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

// cluFindDocs returns the documents find returns, in the order returned.
func cluFindDocs(filter bson.D, opts ...*options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
	return cluFindDocsIn("", filter, opts...)
}

// cluFindDocsIn is cluFindDocs on the collection named col's name plus suffix.
func cluFindDocsIn(suffix string, filter bson.D, opts ...*options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cur, err := col.Database().Collection(col.Name()+suffix).Find(ctx, filter, opts...)
		if err != nil {
			return nil, err
		}
		var docs []bson.D
		err = cur.All(ctx, &docs)
		return docs, err
	}
}

// cluAggregateDocs returns the documents an aggregate of stages returns.
func cluAggregateDocs(stages ...bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return runPipeline(ctx, col, stages)
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
		Run:       cluPaginateNatural,
	})
}

// cluPaginateNatural pages through the collection in natural order, by skip
// and limit and then by one cursor with a small batch size. A clustered
// collection's natural order is _id order.
func cluPaginateNatural(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	var pages bson.A
	for skip := int64(0); skip <= 100; skip += 10 {
		page, err := cluFindIDs(bson.D{}, options.Find().SetSkip(skip).SetLimit(10))(ctx, col)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	batched, err := cluFindIDs(bson.D{}, options.Find().SetBatchSize(7))(ctx, col)
	if err != nil {
		return nil, err
	}
	return bson.A{pages, batched}, nil
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

// cluPaginate pages through _id ranges, optionally sorted.
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
		Support: harness.DumboDBFull,
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
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(200)),
		Run:       cluExplain(cluFindCmd(bson.D{{Key: "_id", Value: int32(150)}}), "executionStats"),
	})
}

func TestClustered_Explain_Range(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Explain_Range",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     cluInsertIDs(cluIntIDs(200)),
		Run:       cluExplain(cluFindCmd(cluRange(int32(100), int32(110))), "executionStats"),
	})
}

func TestClustered_Explain_SortID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_Explain_SortID",
		Support:   harness.DumboDBFull,
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
		Support:   harness.DumboDBFull,
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

// TestClustered_ExplainShapes covers how a clustered collection plans each
// shape of _id predicate and sort.
func TestClustered_ExplainShapes(t *testing.T) {
	sortID := func(dir int32) bson.E { return bson.E{Key: "sort", Value: bson.D{{Key: "_id", Value: dir}}} }
	limit := func(n int32) bson.E { return bson.E{Key: "limit", Value: n} }
	gte := func(v interface{}) bson.D { return bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: v}}}} }
	lt := func(v interface{}) bson.D { return bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: v}}}} }
	withN := func(f bson.D) bson.D {
		return append(f, bson.E{Key: "n", Value: bson.D{{Key: "$gte", Value: int32(0)}}})
	}
	cases := []struct {
		name   string
		filter bson.D
		extra  []bson.E
	}{
		{"point_and_field", withN(bson.D{{Key: "_id", Value: int32(150)}}), nil},
		{"point_missing", bson.D{{Key: "_id", Value: int32(5000)}}, nil},
		{"range_and_field", withN(cluRange(int32(100), int32(110))), nil},
		{"range_lower_only", gte(int32(190)), nil},
		{"range_upper_only", lt(int32(5)), nil},
		{"range_past_end", cluRange(int32(195), int32(1000)), nil},
		{"range_other_type", gte("a"), nil},
		{"in_ids", bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{int32(3), int32(7), int32(9)}}}}}, nil},
		{"sort_id", bson.D{}, []bson.E{sortID(1)}},
		{"sort_id_desc_limit", bson.D{}, []bson.E{sortID(-1), limit(5)}},
		{"sort_id_filter_limit", bson.D{{Key: "n", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}, []bson.E{sortID(1), limit(5)}},
		{"range_sort_id", cluRange(int32(100), int32(110)), []bson.E{sortID(1)}},
		{"range_sort_id_desc_limit", cluRange(int32(100), int32(110)), []bson.E{sortID(-1), limit(3)}},
		{"range_limit", cluRange(int32(100), int32(110)), []bson.E{limit(3)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_ExplainShapes_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     cluInsertIDs(cluIntIDs(200)),
				Run:       cluExplain(cluFindCmd(tc.filter, tc.extra...), "executionStats"),
			})
		})
	}
}

// TestClustered_ExplainBrackets covers where a clustered scan of an _id range
// starts and stops among _ids of other types, and how it competes with a
// secondary index.
func TestClustered_ExplainBrackets(t *testing.T) {
	ids := cluIntIDs(10)
	ids = append(ids, "a", "b", "c", "d", "e", nil, primitive.MinKey{}, primitive.MaxKey{})
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if err := cluInsertIDs(ids)(ctx, col); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "n", Value: 1}}})
		return err
	}
	op := func(o string, v interface{}) bson.D { return bson.D{{Key: "_id", Value: bson.D{{Key: o, Value: v}}}} }
	cases := []struct {
		name   string
		filter bson.D
		extra  []bson.E
	}{
		{"gte_number", op("$gte", int32(5)), nil},
		{"gt_number_desc", op("$gt", int32(5)), []bson.E{{Key: "sort", Value: bson.D{{Key: "_id", Value: -1}}}}},
		{"lt_number", op("$lt", int32(3)), nil},
		{"lt_string", op("$lt", "c"), nil},
		{"gte_string", op("$gte", "c"), nil},
		{"eq_null", op("$eq", nil), nil},
		{"range_and_indexed_field", append(cluRange(int32(2), int32(8)), bson.E{Key: "n", Value: int32(4)}), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_ExplainBrackets_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       cluExplain(cluFindCmd(tc.filter, tc.extra...), "executionStats"),
			})
		})
	}
}

// TestClustered_ExplainWrites covers the _id plans of writes and counts on a
// clustered collection.
func TestClustered_ExplainWrites(t *testing.T) {
	point := bson.D{{Key: "_id", Value: int32(150)}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}}}
	cases := []struct {
		name string
		cmd  func(col *mongo.Collection) bson.D
	}{
		{"update_point", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: point}, {Key: "u", Value: set}}}}}
		}},
		{"update_range", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: cluRange(int32(100), int32(110))}, {Key: "u", Value: set}, {Key: "multi", Value: true}}}}}
		}},
		{"delete_point", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: point}, {Key: "limit", Value: int32(1)}}}}}
		}},
		{"delete_range", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: cluRange(int32(100), int32(110))}, {Key: "limit", Value: int32(0)}}}}}
		}},
		{"find_and_modify_point", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: point}, {Key: "update", Value: set}}
		}},
		{"count_range", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: cluRange(int32(100), int32(110))}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_ExplainWrites_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     cluInsertIDs(cluIntIDs(200)),
				Run:       cluExplain(tc.cmd, "executionStats"),
			})
		})
	}
}

// TestClustered_ListCollections covers how listCollections reports a
// clustered collection.
func TestClustered_ListCollections(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_ListCollections",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			specs, err := col.Database().ListCollections(ctx, bson.D{{Key: "name", Value: col.Name()}})
			if err != nil {
				return nil, err
			}
			var out []bson.D
			for specs.Next(ctx) {
				var spec bson.D
				if err := specs.Decode(&spec); err != nil {
					return nil, err
				}
				kept := bson.D{}
				for _, e := range spec {
					if e.Key != "info" {
						kept = append(kept, e)
					}
				}
				out = append(out, kept)
			}
			return out, specs.Err()
		},
	})
}

// errorMessage returns run's error text as its result, so the comparison
// checks the message and not only the code. A TypeMismatch message's list of
// expected types is sorted: mongod's order is fixed per build but differs
// between builds.
func errorMessage(run func(context.Context, *mongo.Collection) (interface{}, error)) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		res, err := run(ctx, col)
		if err != nil {
			return sortExpectedTypes(err.Error()), nil
		}
		return res, nil
	}
}

var expectedTypesList = regexp.MustCompile(`expected types '\[([^\]']*)'?\]'?`)

// sortExpectedTypes sorts the type names in each "expected types '[...]'" of msg.
func sortExpectedTypes(msg string) string {
	return expectedTypesList.ReplaceAllStringFunc(msg, func(m string) string {
		inner := expectedTypesList.FindStringSubmatch(m)[1]
		names := strings.Split(inner, ", ")
		sort.Strings(names)
		return "expected types [" + strings.Join(names, ", ") + "]"
	})
}

// TestClustered_CreateOptions covers the clusteredIndex option's validation.
func TestClustered_CreateOptions(t *testing.T) {
	create := func(opts ...bson.E) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.D
			cmd := append(bson.D{{Key: "create", Value: "clu"}}, opts...)
			err := col.Database().RunCommand(ctx, cmd).Decode(&res)
			return res, err
		}
	}
	idKey := bson.E{Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}
	unique := bson.E{Key: "unique", Value: true}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"other_key", create(bson.E{Key: "clusteredIndex", Value: bson.D{{Key: "key", Value: bson.D{{Key: "a", Value: int32(1)}}}, unique}})},
		{"descending", create(bson.E{Key: "clusteredIndex", Value: bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: int32(-1)}}}, unique}})},
		{"not_unique", create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, {Key: "unique", Value: false}}})},
		{"missing_unique", create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey}})},
		{"named", create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "byID"}}})},
		{"version_1", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: int32(1)}, {Key: "name", Value: "nm"}}}))},
		{"version_3", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: int32(3)}}}))},
		{"version_null", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: nil}}}))},
		{"version_decimal_2", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("2")}}}))},
		{"version_decimal_2_5", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("2.5")}}}))},
		{"version_string", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: "x"}}}))},
		{"version_bool", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: true}}}))},
		{"version_1_5", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: 1.5}}}))},
		{"version_2_5", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: 2.5}}}))},
		{"version_negative", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: int32(-1)}}}))},
		{"version_long_3", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: int64(3)}}}))},
		{"version_decimal_1_5", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("1.5")}}}))},
		{"version_decimal_2_6", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("2.6")}}}))},
		{"version_decimal_3_9", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("3.9")}}}))},
		{"version_decimal_minus_1_5", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("-1.5")}}}))},
		{"version_decimal_1_4", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("1.4")}}}))},
		{"version_decimal_nan", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("NaN")}}}))},
		{"version_decimal_inf", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("Infinity")}}}))},
		{"version_decimal_minus_inf", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("-Infinity")}}}))},
		{"version_decimal_1e300", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: mustDecimal("1E300")}}}))},
		{"version_double_nan", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: math.NaN()}}}))},
		{"version_double_inf", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: math.Inf(1)}}}))},
		{"version_double_1e300", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: 1e300}}}))},
		{"version_double_2_31", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: 2147483648.0}}}))},
		{"version_long_3e9", errorMessage(create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: int64(3000000000)}}}))},
		{"bool_true", create(bson.E{Key: "clusteredIndex", Value: true})},
		{"bool_false", create(bson.E{Key: "clusteredIndex", Value: false})},
		{"capped", create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique}}, bson.E{Key: "capped", Value: true}, bson.E{Key: "size", Value: int32(4096)})},
		{"twice", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique}})(ctx, col); err != nil {
				return nil, err
			}
			return create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique}})(ctx, col)
		}},
		{"named_on_existing", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique}})(ctx, col); err != nil {
				return nil, err
			}
			return create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "zz"}}})(ctx, col)
		}},
		{"default_name_on_existing", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique}})(ctx, col); err != nil {
				return nil, err
			}
			return create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "_id_"}, {Key: "v", Value: int32(2)}}})(ctx, col)
		}},
		{"named_again", recreateNamed(create, idKey, unique, bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "byID"}}})},
		{"named_then_plain", recreateNamed(create, idKey, unique)},
		{"named_then_default", recreateNamed(create, idKey, unique, bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique}})},
		{"named_then_other", recreateNamed(create, idKey, unique, bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "other"}}})},
		{"plain_then_clustered", twoCreates(create, nil, []bson.E{{Key: "clusteredIndex", Value: bson.D{idKey, unique}}})},
		{"clustered_then_plain", twoCreates(create, []bson.E{{Key: "clusteredIndex", Value: bson.D{idKey, unique}}}, nil)},
		{"existing_bad_version", twoCreates(create, nil, []bson.E{{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "v", Value: int32(3)}}}})},
		{"existing_not_unique", twoCreates(create, nil, []bson.E{{Key: "clusteredIndex", Value: bson.D{idKey, {Key: "unique", Value: false}}}})},
		{"existing_key_not_id", twoCreates(create, nil, []bson.E{{Key: "clusteredIndex", Value: bson.D{{Key: "key", Value: bson.D{{Key: "a", Value: int32(1)}}}, unique}}})},
		{"existing_legacy_bool", twoCreates(create, nil, []bson.E{{Key: "clusteredIndex", Value: true}})},
		{"validator_differs", twoCreates(create, []bson.E{{Key: "validator", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}}, []bson.E{{Key: "validator", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 2}}}}}})},
		{"validator_same", twoCreates(create, []bson.E{{Key: "validator", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}}, []bson.E{{Key: "validator", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}})},
		{"plain_over_validator", twoCreates(create, []bson.E{{Key: "validator", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}}, nil)},
		{"validator_over_plain", twoCreates(create, nil, []bson.E{{Key: "validator", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}})},
		{"existing_clustered_validator", twoCreates(create, []bson.E{{Key: "clusteredIndex", Value: bson.D{idKey, unique}}},
			[]bson.E{{Key: "clusteredIndex", Value: bson.D{idKey, unique}}, {Key: "validator", Value: bson.D{{Key: "a", Value: 1}}}})},
		{"named_then_index_same_name", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "a_1"}}})(ctx, col); err != nil {
				return nil, err
			}
			err := col.Database().RunCommand(ctx, bson.D{{Key: "createIndexes", Value: "clu"}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "name", Value: "a_1"}}}}}).Err()
			var ce mongo.CommandError
			if errors.As(err, &ce) {
				return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: ce.Message}}, nil
			}
			return bson.D{{Key: "ok", Value: err == nil}}, nil
		}},
		{"named_options", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "byID"}}})(ctx, col); err != nil {
				return nil, err
			}
			specs, err := col.Database().ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: "clu"}})
			if err != nil || len(specs) != 1 {
				return nil, err
			}
			var options bson.D
			err = bson.Unmarshal(specs[0].Options, &options)
			return options, err
		}},
		{"named_listed", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "byID"}}})(ctx, col); err != nil {
				return nil, err
			}
			cur, err := col.Database().Collection("clu").Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var specs []bson.D
			err = cur.All(ctx, &specs)
			return specs, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Clustered_CreateOptions_" + tc.name,
				Support: harness.DumboDBFull,
				Run:     tc.run,
			})
		})
	}
}

// recreateNamed creates the collection with a clusteredIndex named byID, then
// runs create again with opts.
func recreateNamed(create func(...bson.E) func(context.Context, *mongo.Collection) (interface{}, error), idKey, unique bson.E, opts ...bson.E) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		if _, err := create(bson.E{Key: "clusteredIndex", Value: bson.D{idKey, unique, {Key: "name", Value: "byID"}}})(ctx, col); err != nil {
			return nil, err
		}
		return create(opts...)(ctx, col)
	}
}

// An _id document with an empty field name is a value inside $in, not an
// operator.
func TestClustered_InEmptyFieldName(t *testing.T) {
	id := bson.D{{Key: "", Value: int32(1)}}
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_InEmptyFieldName",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     cluInsertIDs([]interface{}{id, int32(2)}),
		Run:       cluFindIDs(bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{id}}}}}),
	})
}

// TestClustered_IDRegex: an _id regex matches string _ids by pattern in every
// read and write command, alone, beside another field, and inside $in.
func TestClustered_IDRegex(t *testing.T) {
	re := primitive.Regex{Pattern: "^a"}
	setup := cluInsertIDs([]interface{}{"abc", "abd", "b", int32(1)})
	count := func(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, filter)
		}
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"find", cluFindIDs(bson.D{{Key: "_id", Value: re}})},
		{"find_beside_field", cluFindIDs(bson.D{{Key: "_id", Value: re}, {Key: "q", Value: bson.D{{Key: "$exists", Value: false}}}})},
		{"find_in", cluFindIDs(bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{re, "b"}}}}})},
		{"count", count(bson.D{{Key: "_id", Value: re}})},
		{"aggregate", cluAggregateDocs(bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: re}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})},
		{"distinct", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.Distinct(ctx, "_id", bson.D{{Key: "_id", Value: re}})
		}},
		{"update_many", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx, bson.D{{Key: "_id", Value: re}}, bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: 1}}}})
			if err != nil {
				return nil, err
			}
			ids, err := cluFindIDs(bson.D{{Key: "t", Value: 1}})(ctx, col)
			return bson.A{res.MatchedCount, res.ModifiedCount, ids}, err
		}},
		{"delete_many", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteMany(ctx, bson.D{{Key: "_id", Value: re}})
			if err != nil {
				return nil, err
			}
			ids, err := cluFindIDs(bson.D{})(ctx, col)
			return bson.A{res.DeletedCount, ids}, err
		}},
		{"find_and_modify", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.D
			err := col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: re}}, bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: 1}}}},
				options.FindOneAndUpdate().SetSort(bson.D{{Key: "_id", Value: 1}})).Decode(&doc)
			return doc, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_IDRegex_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_DecimalMatchesNumber: a number in a filter matches an equal
// stored Decimal128, on _id beside another field and on any other field.
func TestClustered_DecimalMatchesNumber(t *testing.T) {
	dec := mustDecimal
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: dec("7.000")}, {Key: "w", Value: 1}, {Key: "x", Value: dec("7.000")}},
			bson.D{{Key: "_id", Value: dec("1.50")}, {Key: "w", Value: 1}, {Key: "x", Value: dec("1.50")}},
			bson.D{{Key: "_id", Value: 3}, {Key: "w", Value: 1}, {Key: "x", Value: 3}},
		})
		return err
	}
	cases := []struct {
		name   string
		filter bson.D
	}{
		{"id_int_beside_field", bson.D{{Key: "_id", Value: 7}, {Key: "w", Value: 1}}},
		{"id_double_beside_field", bson.D{{Key: "_id", Value: 1.5}, {Key: "w", Value: 1}}},
		{"field_int", bson.D{{Key: "x", Value: 7}}},
		{"field_long_beside_field", bson.D{{Key: "x", Value: int64(7)}, {Key: "w", Value: 1}}},
		{"field_double", bson.D{{Key: "x", Value: 1.5}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_DecimalMatchesNumber_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					found, err := cluFindIDs(tc.filter)(ctx, col)
					if err != nil {
						return nil, err
					}
					n, err := col.CountDocuments(ctx, tc.filter)
					if err != nil {
						return nil, err
					}
					upd, err := col.UpdateMany(ctx, tc.filter, bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: 1}}}})
					if err != nil {
						return nil, err
					}
					del, err := col.DeleteMany(ctx, tc.filter)
					if err != nil {
						return nil, err
					}
					return bson.A{found, n, upd.ModifiedCount, del.DeletedCount}, nil
				},
			})
		})
	}
}

// TestClustered_NaNAndLargeLongEquality (dolthub/dumbodb#210): NaN equals
// NaN, and a long beyond the doubles' exact range matches itself, on _id and
// on another field, alone and beside another predicate.
func TestClustered_NaNAndLargeLongEquality(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: math.NaN()}, {Key: "a", Value: math.NaN()}, {Key: "n", Value: 1}},
			bson.D{{Key: "_id", Value: int64(math.MaxInt64)}, {Key: "a", Value: int64(math.MaxInt64)}, {Key: "n", Value: 1}},
			bson.D{{Key: "_id", Value: int64(1<<53 + 1)}, {Key: "a", Value: int64(1<<53 + 1)}, {Key: "n", Value: 1}},
		})
		return err
	}
	n := bson.E{Key: "n", Value: bson.D{{Key: "$gte", Value: 1}}}
	cases := []struct {
		name   string
		filter bson.D
	}{
		{"id_nan_beside_field", bson.D{{Key: "_id", Value: math.NaN()}, n}},
		{"id_max_long_beside_field", bson.D{{Key: "_id", Value: int64(math.MaxInt64)}, n}},
		{"field_nan", bson.D{{Key: "a", Value: math.NaN()}}},
		{"field_nan_beside_field", bson.D{{Key: "a", Value: math.NaN()}, n}},
		{"field_max_long", bson.D{{Key: "a", Value: int64(math.MaxInt64)}}},
		{"field_max_long_beside_field", bson.D{{Key: "a", Value: int64(math.MaxInt64)}, n}},
		{"field_long_above_2_53", bson.D{{Key: "a", Value: int64(1<<53 + 1)}, n}},
		{"field_double_2_53", bson.D{{Key: "a", Value: float64(1 << 53)}, n}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_NaNAndLargeLongEquality_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       cluFindIDs(tc.filter),
			})
		})
		// Writes find the same documents: an updateMany marks them, then a
		// deleteMany removes them.
		t.Run(tc.name+"/writes", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_NaNAndLargeLongEquality_writes_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					updated, err := col.UpdateMany(ctx, tc.filter, bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: true}}}})
					if err != nil {
						return nil, err
					}
					marked, err := col.CountDocuments(ctx, bson.D{{Key: "w", Value: true}})
					if err != nil {
						return nil, err
					}
					deleted, err := col.DeleteMany(ctx, tc.filter)
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "matched", Value: updated.MatchedCount}, {Key: "marked", Value: marked}, {Key: "deleted", Value: deleted.DeletedCount}}, nil
				},
			})
		})
	}
}

// TestClustered_TinyDecimalBesideDouble: a Decimal128 below 2^-255 that is not
// exactly a double is distinct from, and ordered against, the doubles beside
// it, as _id and as an indexed value.
func TestClustered_TinyDecimalBesideDouble(t *testing.T) {
	dec := mustDecimal
	ids := func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
		return cluFindIDs(filter)(ctx, col)
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"id_both_stored", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 2e-300}}); err != nil {
				return nil, err
			}
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: dec("2E-300")}}); err != nil {
				return nil, err
			}
			found, err := ids(ctx, col, bson.D{{Key: "_id", Value: dec("2E-300")}})
			if err != nil {
				return nil, err
			}
			all, err := ids(ctx, col, bson.D{})
			return bson.A{found, all}, err
		}},
		{"id_range_above_double", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1.000155232458693e-307}},
				bson.D{{Key: "_id", Value: dec("1.0001552324586932E-307")}},
			}); err != nil {
				return nil, err
			}
			return ids(ctx, col, bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: 1.000155232458693e-307}}}})
		}},
		{"unique_index", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}, Options: options.Index().SetUnique(true)}); err != nil {
				return nil, err
			}
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: dec("1E-78")}}); err != nil {
				return nil, err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 1e-78}})
			return nil, err
		}},
		{"count_and_distinct", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}}); err != nil {
				return nil, err
			}
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: dec("1E-78")}},
				bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 1e-78}},
				bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: dec("1E-78")}},
			}); err != nil {
				return nil, err
			}
			n, err := col.CountDocuments(ctx, bson.D{{Key: "a", Value: 1e-78}})
			if err != nil {
				return nil, err
			}
			values, err := col.Distinct(ctx, "a", bson.D{})
			return bson.A{n, values}, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_TinyDecimalBesideDouble_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Run:       tc.run,
			})
		})
	}
}

// TestRange_MinKeyMaxKeyBounds: a range bounded by MinKey or MaxKey spans
// every type, on _id and on another field, including null and missing.
func TestRange_MinKeyMaxKeyBounds(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: primitive.MinKey{}}, {Key: "a", Value: primitive.MinKey{}}},
			bson.D{{Key: "_id", Value: nil}, {Key: "a", Value: nil}},
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}},
			bson.D{{Key: "_id", Value: "s"}, {Key: "a", Value: "s"}},
			bson.D{{Key: "_id", Value: bson.D{{Key: "x", Value: 1}}}, {Key: "a", Value: bson.A{1, 2}}},
			bson.D{{Key: "_id", Value: true}},
			bson.D{{Key: "_id", Value: primitive.MaxKey{}}, {Key: "a", Value: primitive.MaxKey{}}},
		})
		return err
	}
	op := func(field, o string, v interface{}) bson.D {
		return bson.D{{Key: field, Value: bson.D{{Key: o, Value: v}}}}
	}
	cases := []struct {
		name   string
		filter bson.D
	}{
		{"id_gte_minkey", op("_id", "$gte", primitive.MinKey{})},
		{"id_gt_minkey", op("_id", "$gt", primitive.MinKey{})},
		{"id_lte_maxkey", op("_id", "$lte", primitive.MaxKey{})},
		{"id_lt_maxkey", op("_id", "$lt", primitive.MaxKey{})},
		{"field_gte_minkey", op("a", "$gte", primitive.MinKey{})},
		{"field_gt_minkey", op("a", "$gt", primitive.MinKey{})},
		{"field_lte_maxkey", op("a", "$lte", primitive.MaxKey{})},
		{"field_lt_maxkey", op("a", "$lt", primitive.MaxKey{})},
		{"field_between", bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: primitive.MinKey{}}, {Key: "$lt", Value: primitive.MaxKey{}}}}}},
	}
	for _, tc := range cases {
		for _, indexed := range []bool{false, true} {
			name := tc.name
			if indexed {
				name += "_indexed"
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "Range_MinKeyMaxKeyBounds_" + name,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if err := setup(ctx, col); err != nil {
							return err
						}
						if indexed {
							_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
							return err
						}
						return nil
					},
					Run: cluFindIDs(tc.filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})),
				})
			})
		}
	}
}

// TestClustered_HintNaturalDescending: {$natural: -1} as a hint reads the
// collection in reverse _id order, alone and with a filter or limit.
func TestClustered_HintNaturalDescending(t *testing.T) {
	cases := []struct {
		name string
		opts *options.FindOptions
		f    bson.D
	}{
		{"all", options.Find().SetHint(bson.D{{Key: "$natural", Value: -1}}), bson.D{}},
		{"filtered", options.Find().SetHint(bson.D{{Key: "$natural", Value: -1}}), bson.D{{Key: "n", Value: bson.D{{Key: "$gte", Value: 3}}}}},
		{"limit", options.Find().SetHint(bson.D{{Key: "$natural", Value: -1}}).SetLimit(3), bson.D{}},
		{"ascending", options.Find().SetHint(bson.D{{Key: "$natural", Value: 1}}), bson.D{}},
		{"double", options.Find().SetHint(bson.D{{Key: "$natural", Value: -1.0}}), bson.D{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_HintNaturalDescending_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     cluInsertIDs(cluIntIDs(12)),
				Run:       cluFindIDs(tc.f, tc.opts),
			})
		})
	}
}

// TestDistinct_EqualNumbers: distinct returns one of each value, numbers of
// different types that compare equal counting once, with or without a filter
// and an index.
func TestDistinct_EqualNumbers(t *testing.T) {
	setup := func(indexed bool) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}},
				bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: mustDecimal("1.00")}},
				bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 2.0}},
				bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: mustDecimal("2")}},
				bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: int64(3)}},
				bson.D{{Key: "_id", Value: 6}, {Key: "a", Value: 3.0}},
			}); err != nil {
				return err
			}
			if indexed {
				_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
				return err
			}
			return nil
		}
	}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"no_filter", bson.D{}},
		{"in", bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A{1, 2}}}}}},
		{"range", bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 2}}}}},
		{"other_field", bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: 2}}}}},
	}
	for _, f := range filters {
		for _, indexed := range []bool{false, true} {
			name := f.name
			if indexed {
				name += "_indexed"
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "Distinct_EqualNumbers_" + name,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup:     setup(indexed),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return col.Distinct(ctx, "a", f.filter)
					},
				})
			})
		}
	}
}

// TestCount_SparseHint: a count hinted to a sparse index counts only the
// documents the index holds, with and without a filter, for count, the
// countDocuments pipeline and find.
func TestCount_SparseHint(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 300)
		for i := 0; i < 300; i++ {
			d := bson.D{{Key: "_id", Value: int32(i)}, {Key: "g", Value: int32(i % 4)}}
			if i%4 == 0 {
				d = append(d, bson.E{Key: "s", Value: int32(i % 10)})
			}
			docs = append(docs, d)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "s", Value: 1}}, Options: options.Index().SetSparse(true)})
		return err
	}
	hint := bson.D{{Key: "s", Value: 1}}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"all", bson.D{}},
		{"other_field", bson.D{{Key: "g", Value: int32(0)}}},
		{"other_field_none_sparse", bson.D{{Key: "g", Value: int32(1)}}},
	}
	for _, f := range filters {
		t.Run(f.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Count_SparseHint_" + f.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var count bson.D
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: f.filter}, {Key: "hint", Value: hint}}).Decode(&count); err != nil {
						return nil, err
					}
					n, err := col.CountDocuments(ctx, f.filter, options.Count().SetHint(hint))
					if err != nil {
						return nil, err
					}
					found, err := cluFindIDs(f.filter, options.Find().SetHint(hint))(ctx, col)
					if err != nil {
						return nil, err
					}
					return bson.A{count, n, len(found.(bson.A))}, nil
				},
			})
		})
	}
}

// TestDistinctHintAndCoveredMissing: distinct accepts a hint, and a covered
// projection of a field a document lacks reports the null its index entry
// holds.
func TestDistinctHintAndCoveredMissing(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}, {Key: "b", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "b", Value: 2}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 2}, {Key: "b", Value: 3}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
		return err
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"distinct_hint_name", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.D
			err := col.Database().RunCommand(ctx, bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "b"}, {Key: "hint", Value: "a_1"}}).Decode(&res)
			return res, err
		}},
		{"distinct_hint_spec", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.D
			err := col.Database().RunCommand(ctx, bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "a"}, {Key: "hint", Value: bson.D{{Key: "a", Value: 1}}}}).Decode(&res)
			return res, err
		}},
		{"covered_missing_field", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{{Key: "a", Value: bson.D{{Key: "$lte", Value: 5}}}},
				options.Find().SetProjection(bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 0}}).SetHint("a_1"))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}},
		{"covered_full_index_scan", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetProjection(bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 0}}).SetHint("a_1"))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}},
		{"covered_null_scan", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{{Key: "a", Value: nil}},
				options.Find().SetProjection(bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 0}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "DistinctHintAndCoveredMissing_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_TimeSeriesListIndexes: a time-series collection lists no _id
// index.
func TestClustered_TimeSeriesListIndexes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Clustered_TimeSeriesListIndexes",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			_ = db.Collection("ts").Drop(ctx)
			if err := db.CreateCollection(ctx, "ts", options.CreateCollection().SetTimeSeriesOptions(options.TimeSeries().SetTimeField("t"))); err != nil {
				return nil, err
			}
			cur, err := db.Collection("ts").Indexes().List(ctx)
			if err != nil {
				return nil, err
			}
			var specs []bson.D
			err = cur.All(ctx, &specs)
			return specs, err
		},
	})
}

// TestClustered_DuplicateKey_SignedZero: -0 equals 0 and a Decimal128 -0
// equals both, so each second insert is a duplicate key. NaN is covered by
// TestClustered_DuplicateKey_DecodedRecordID.
func TestClustered_DuplicateKey_SignedZero(t *testing.T) {
	decNegZero := primitive.NewDecimal128(0xb040000000000000, 0)
	pairs := [][2]interface{}{
		{int32(0), math.Copysign(0, -1)},
		{math.Copysign(0, -1), decNegZero},
		{decNegZero, int64(0)},
	}
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_DuplicateKey_SignedZero",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var dups bson.A
			for _, pair := range pairs {
				if _, err := col.DeleteMany(ctx, bson.D{}); err != nil {
					return nil, err
				}
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: pair[0]}}); err != nil {
					return nil, err
				}
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: pair[1]}})
				dups = append(dups, mongo.IsDuplicateKeyError(err))
			}
			return dups, nil
		},
	})
}

// TestClustered_SortID_SignedZeroAndNaN orders -0, NaN and Decimal128 edge
// values among other numbers.
func TestClustered_SortID_SignedZeroAndNaN(t *testing.T) {
	ids := []interface{}{
		math.Copysign(0, -1), math.NaN(), int32(1), -1e-300, 1e-300, math.Inf(-1),
		primitive.NewDecimal128(0x0000000000000000, 1), // 1E-6176
		primitive.NewDecimal128(0x8000000000000000, 1), // -1E-6176
	}
	harness.PairTest(t, cluFilter("Clustered_SortID_SignedZeroAndNaN", harness.DumboDBFull, ids, bson.D{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})))
}

// TestClustered_RecordID: a clustered collection's record id is the _id's
// KeyString, returned as BinData by $meta and showRecordId.
func TestClustered_RecordID(t *testing.T) {
	oid, err := primitive.ObjectIDFromHex("65a0333600000000000000aa")
	if err != nil {
		t.Fatal(err)
	}
	ids := []interface{}{int32(1), "a", -0.5, oid, bson.D{{Key: "k", Value: int32(2)}}, primitive.NewDecimal128(0x303e000000000000, 35)}
	recordIDs := func(opts *options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{}, opts.SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}
	}
	cases := []struct {
		name string
		opts *options.FindOptions
	}{
		{"meta", options.Find().SetProjection(bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}})},
		{"show_record_id", options.Find().SetShowRecordID(true)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_RecordID_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     cluInsertIDs(ids),
				Run:       recordIDs(tc.opts),
			})
		})
	}
}

// dupIDValues are _id values of every type, each inserted twice by the
// duplicate-key cases.
func dupIDValues() []interface{} {
	oid, _ := primitive.ObjectIDFromHex("65a0333600000000000000aa")
	return []interface{}{
		int32(1), int64(5), int64(math.MaxInt64), 0.5, math.Copysign(0, -1), 1e20, 5e-324, math.NaN(), math.Inf(1), math.Inf(-1),
		primitive.NewDecimal128(0x303e000000000000, 1), // 0.1
		primitive.NewDecimal128(0x7c00000000000000, 0), // NaN
		primitive.NewDecimal128(0x2d20000000000000, 1), // 1E-400
		"a", oid, bson.D{{Key: "a", Value: int32(1)}}, bson.D{{Key: "a", Value: 1.5}}, bson.D{{Key: "a", Value: bson.A{int32(1)}}},
		primitive.DateTime(0), true, primitive.Binary{Data: []byte{1}}, primitive.MinKey{}, primitive.Timestamp{T: 1, I: 1},
	}
}

// duplicateIDErrors inserts each value twice and returns each second insert's
// error code and message.
func duplicateIDErrors(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	var out bson.A
	for _, id := range dupIDValues() {
		if _, err := col.DeleteMany(ctx, bson.D{}); err != nil {
			return nil, err
		}
		if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}}); err != nil {
			return nil, err
		}
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}})
		var we mongo.WriteException
		if errors.As(err, &we) && len(we.WriteErrors) > 0 {
			out = append(out, bson.A{int32(we.WriteErrors[0].Code), we.WriteErrors[0].Message})
		} else {
			out = append(out, fmt.Sprint(err))
		}
	}
	return out, nil
}

// TestDuplicateKey_IDTypes: a duplicate _id of each type fails with E11000 and
// MongoDB's message on a default collection.
func TestDuplicateKey_IDTypes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DuplicateKey_IDTypes",
		Support: harness.DumboDBFull,
		Run:     duplicateIDErrors,
	})
}

// TestClustered_DuplicateKey_DecodedRecordID records a clustered mongod quirk
// DumboDB does not copy. Both servers refuse every duplicate _id below and keep
// the first document unchanged; only the error differs. mongod builds a
// clustered collection's E11000 message by decoding the RecordId, which keeps
// no TypeBits, so every number decodes as an int32: NumberLong(MaxInt64) is
// reported as -1, and NaN, infinities, fractions and large doubles fail with a
// KeyString format error (50819, 50826, 50825, 50823) instead of E11000.
// DumboDB returns what a default MongoDB collection returns
// (TestDuplicateKey_IDTypes), so code that detects duplicates by 11000 works.
func TestClustered_DuplicateKey_DecodedRecordID(t *testing.T) {
	ids := []interface{}{int64(math.MaxInt64), math.NaN(), primitive.NewDecimal128(0x7c00000000000000, 0), math.Inf(1), 0.5, 1e20, 5e-324}
	// outcome is the second insert's error code and message, with the
	// namespace replaced by "NS", and the documents stored afterwards.
	type outcome struct {
		code    int32
		message string
		stored  []string
	}
	run := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		ns := col.Database().Name() + "." + col.Name()
		var out []outcome
		for _, id := range ids {
			if _, err := col.DeleteMany(ctx, bson.D{}); err != nil {
				return nil, err
			}
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "v", Value: "first"}}); err != nil {
				return nil, err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "v", Value: "second"}})
			var we mongo.WriteException
			if !errors.As(err, &we) || len(we.WriteErrors) == 0 {
				return nil, fmt.Errorf("second insert of %v: %v", id, err)
			}
			cur, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var docs []bson.M
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			var stored []string
			for _, d := range docs {
				stored = append(stored, fmt.Sprint(d["v"]))
			}
			out = append(out, outcome{
				code:    int32(we.WriteErrors[0].Code),
				message: strings.ReplaceAll(we.WriteErrors[0].Message, ns, "NS"),
				stored:  stored,
			})
		}
		return out, nil
	}
	dup := func(key string) string {
		return "E11000 duplicate key error collection: NS index: _id_ dup key: { _id: " + key + " }"
	}
	first := []string{"first"}
	keyStringError := func(code int32, message string) outcome {
		return outcome{code: code, message: "KeyString format error: " + message, stored: first}
	}
	harness.PairTest(t, harness.TestCase{
		Name:      "Clustered_DuplicateKey_DecodedRecordID",
		Support:   harness.DumboDBDeviates,
		Clustered: true,
		Run:       run,
		MongoExpect: expectResult("mongo", []outcome{
			{code: 11000, message: "E11000 duplicate key error collection: NS dup key: { _id: -1 } found value: {}", stored: first},
			keyStringError(50819, "Invalid type bits for numeric NaN"),
			keyStringError(50819, "Invalid type bits for numeric NaN"),
			keyStringError(50826, "Unexpected type of large number."),
			keyStringError(50825, "Unexpected type for small number, expected decimal."),
			keyStringError(50826, "Unexpected type of large number."),
			keyStringError(50823, "Expected decimal continuation."),
		}),
		DumboExpect: expectResult("dumbodb", []outcome{
			{code: 11000, message: dup("9223372036854775807"), stored: first},
			{code: 11000, message: dup("nan.0"), stored: first},
			{code: 11000, message: dup("NaN"), stored: first},
			{code: 11000, message: dup("inf.0"), stored: first},
			{code: 11000, message: dup("0.5"), stored: first},
			{code: 11000, message: dup("1e+20.0"), stored: first},
			{code: 11000, message: dup("4.940656458412465e-324"), stored: first},
		}),
	})
}

// TestClustered_RecordIDThroughViews: a view stage that reshapes documents
// leaves no RecordId, so find on the view cannot reveal the base _id through
// showRecordId or $meta "recordId".
func TestClustered_RecordIDThroughViews(t *testing.T) {
	views := []struct {
		name     string
		pipeline bson.A
	}{
		{"project", bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "a", Value: 1}}}}}},
		{"set", bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "_id", Value: "hidden"}}}}}},
		{"add_fields", bson.A{bson.D{{Key: "$addFields", Value: bson.D{{Key: "_id", Value: "hidden"}}}}}},
		{"replace_root", bson.A{bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: bson.D{{Key: "a", Value: "$a"}}}}}}}},
		{"group", bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$a"}}}}}},
		{"match", bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 1}}}}}}}},
	}
	reads := []struct {
		name string
		opts *options.FindOptions
	}{
		{"show_record_id", options.Find().SetShowRecordID(true)},
		{"meta", options.Find().SetProjection(bson.D{{Key: "a", Value: 1}, {Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}})},
		{"meta_only", options.Find().SetProjection(bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}})},
		{"meta_only_no_id", options.Find().SetProjection(bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}, {Key: "_id", Value: 0}})},
	}
	for _, v := range views {
		for _, r := range reads {
			t.Run(v.name+"/"+r.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "Clustered_RecordIDThroughViews_" + v.name + "_" + r.name,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "SSN-123-45-6789"}, {Key: "a", Value: int32(1)}}); err != nil {
							return err
						}
						return col.Database().RunCommand(ctx, bson.D{{Key: "create", Value: col.Name() + "_v"}, {Key: "viewOn", Value: col.Name()}, {Key: "pipeline", Value: v.pipeline}}).Err()
					},
					Run: cluFindDocsIn("_v", bson.D{}, r.opts),
				})
			})
		}
	}
}

// TestClustered_CoveredNullOrMissing: an index key of null stands for null or
// a missing field. A covered read may report it as null only when the filter
// cannot tell the two apart; otherwise it must fetch the document.
func TestClustered_CoveredNullOrMissing(t *testing.T) {
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"empty", bson.D{}},
		{"exists_true", bson.D{{Key: "a", Value: bson.D{{Key: "$exists", Value: true}}}}},
		{"exists_false", bson.D{{Key: "a", Value: bson.D{{Key: "$exists", Value: false}}}}},
		{"type_null", bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "null"}}}}},
		{"not_exists_false", bson.D{{Key: "a", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$exists", Value: false}}}}}}},
		{"not_ne_null", bson.D{{Key: "a", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$ne", Value: nil}}}}}}},
		{"all_null", bson.D{{Key: "a", Value: bson.D{{Key: "$all", Value: bson.A{nil}}}}}},
		{"eq_null", bson.D{{Key: "a", Value: nil}}},
		{"ne_null", bson.D{{Key: "a", Value: bson.D{{Key: "$ne", Value: nil}}}}},
		{"nin", bson.D{{Key: "a", Value: bson.D{{Key: "$nin", Value: bson.A{int32(1)}}}}}},
		{"nin_null", bson.D{{Key: "a", Value: bson.D{{Key: "$nin", Value: bson.A{nil}}}}}},
		{"ne_value", bson.D{{Key: "a", Value: bson.D{{Key: "$ne", Value: int32(1)}}}}},
		{"lte_maxkey", bson.D{{Key: "a", Value: bson.D{{Key: "$lte", Value: primitive.MaxKey{}}}}}},
		{"gt", bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}},
		{"gte_minkey", bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: primitive.MinKey{}}}}}},
		{"in_values", bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), "s"}}}}}},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "g", Value: int32(1)}, {Key: "a", Value: nil}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "g", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "g", Value: int32(1)}, {Key: "a", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(4)}, {Key: "g", Value: int32(1)}, {Key: "a", Value: "s"}},
		})
		if err != nil {
			return err
		}
		_, err = col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "a", Value: 1}}, Options: options.Index().SetName("a_1")},
			{Keys: bson.D{{Key: "g", Value: 1}, {Key: "a", Value: 1}}, Options: options.Index().SetName("g_a")},
		})
		return err
	}
	for _, f := range filters {
		for _, hint := range []string{"a_1", "g_a"} {
			t.Run(f.name+"/"+hint, func(t *testing.T) {
				filter := f.filter
				if hint == "g_a" {
					filter = append(bson.D{{Key: "g", Value: int32(1)}}, filter...)
				}
				harness.PairTest(t, harness.TestCase{
					Name:      "Clustered_CoveredNullOrMissing_" + f.name + "_" + hint,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup:     setup,
					Run:       cluFindDocs(filter, options.Find().SetProjection(bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 0}}).SetHint(hint)),
				})
			})
		}
	}
}

// TestRange_NestedIDMinKeyMaxKey: an _id inside an array element is an
// ordinary field, so MinKey and MaxKey bounds on it span every type, unlike a
// clustered collection's top-level _id.
func TestRange_NestedIDMinKeyMaxKey(t *testing.T) {
	bound := func(op string, v interface{}) bson.D { return bson.D{{Key: op, Value: v}} }
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"gte_minkey", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "_id", Value: bound("$gte", primitive.MinKey{})}}}}}}},
		{"gt_minkey", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "_id", Value: bound("$gt", primitive.MinKey{})}}}}}}},
		{"lte_maxkey", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "_id", Value: bound("$lte", primitive.MaxKey{})}}}}}}},
		{"lt_maxkey", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "_id", Value: bound("$lt", primitive.MaxKey{})}}}}}}},
		{"or_gte_minkey", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "_id", Value: bound("$gte", primitive.MinKey{})}}, bson.D{{Key: "k", Value: int32(9)}},
		}}}}}}}},
		{"dotted_gte_minkey", bson.D{{Key: "arr._id", Value: bound("$gte", primitive.MinKey{})}}},
		{"top_level_gte_minkey", bson.D{{Key: "_id", Value: bound("$gte", primitive.MinKey{})}}},
	}
	for _, f := range filters {
		t.Run(f.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Range_NestedIDMinKeyMaxKey_" + f.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: bson.A{bson.D{{Key: "_id", Value: int32(5)}}}}},
						bson.D{{Key: "_id", Value: int32(2)}, {Key: "arr", Value: bson.A{bson.D{{Key: "_id", Value: "x"}}}}},
						bson.D{{Key: "_id", Value: int32(3)}, {Key: "arr", Value: bson.A{bson.D{{Key: "k", Value: int32(1)}}}}},
						bson.D{{Key: "_id", Value: int32(4)}, {Key: "arr", Value: bson.A{bson.D{{Key: "_id", Value: primitive.MinKey{}}}}}},
					})
					return err
				},
				Run: cluFindIDs(f.filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})),
			})
		})
	}
}

// TestClustered_WriteNaturalHint: a single-document write picks the first
// match in the hinted order, so {$natural: -1} acts on the last match; a hint
// naming no index is refused.
func TestClustered_WriteNaturalHint(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 10)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 3)}}
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	reverse := bson.D{{Key: "$natural", Value: -1}}
	// touched returns the _ids whose "w" field the write set, or its error.
	touched := func(write func(context.Context, *mongo.Collection) error) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := write(ctx, col); err != nil {
				return fmt.Sprint(err), nil
			}
			return cluFindIDs(bson.D{{Key: "w", Value: true}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		}
	}
	remaining := func(write func(context.Context, *mongo.Collection) error) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := write(ctx, col); err != nil {
				return fmt.Sprint(err), nil
			}
			return cluFindIDs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		}
	}
	// refused returns whether write failed as a command or a write error, and
	// its code. mongod's message embeds its query tree, so it is not compared.
	refused := func(write func(context.Context, *mongo.Collection) error) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := write(ctx, col)
			var we mongo.WriteException
			var ce mongo.CommandError
			switch {
			case errors.As(err, &we) && len(we.WriteErrors) > 0:
				return bson.A{"write error", int32(we.WriteErrors[0].Code)}, nil
			case errors.As(err, &ce):
				return bson.A{"command error", ce.Code}, nil
			}
			return fmt.Sprint(err), nil
		}
	}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: true}}}}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"find_and_modify", touched(func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, bson.D{{Key: "a", Value: int32(1)}}, set, options.FindOneAndUpdate().SetHint(reverse)).Err()
		})},
		{"update_one", touched(func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateOne(ctx, bson.D{{Key: "a", Value: int32(0)}}, set, options.Update().SetHint(reverse))
			return err
		})},
		{"delete_one", remaining(func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteOne(ctx, bson.D{{Key: "a", Value: int32(2)}}, options.Delete().SetHint(reverse))
			return err
		})},
		{"find_and_modify_unknown_index", refused(func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, bson.D{{Key: "a", Value: int32(1)}}, set, options.FindOneAndUpdate().SetHint("nope")).Err()
		})},
		{"update_unknown_index", refused(func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateOne(ctx, bson.D{{Key: "a", Value: int32(0)}}, set, options.Update().SetHint("nope"))
			return err
		})},
		{"delete_unknown_index", refused(func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteOne(ctx, bson.D{{Key: "a", Value: int32(2)}}, options.Delete().SetHint("nope"))
			return err
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_WriteNaturalHint_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_RecordIDPaths: showRecordId returns the KeyString on sort-walk
// and $or reads, and a pipeline's $meta "recordId" is present after $match and
// missing after $sort, $limit and $set.
func TestClustered_RecordIDPaths(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 6)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 3)}, {Key: "b", Value: int32(5 - i)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{{Keys: bson.D{{Key: "a", Value: 1}}}, {Keys: bson.D{{Key: "b", Value: 1}}}})
		return err
	}
	find := func(filter bson.D, opts *options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
		return cluFindDocs(filter, opts.SetShowRecordID(true))
	}
	meta := bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}}}}
	pipeline := func(stages ...bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return cluAggregateDocs(append(stages, meta)...)
	}
	// joined runs sub as a $lookup sub-pipeline over the collection itself
	// and returns each document's joined array.
	joined := func(sub bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runPipeline(ctx, col, mongo.Pipeline{
				{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "as", Value: "j"}, {Key: "pipeline", Value: sub}}}},
				{{Key: "$project", Value: bson.D{{Key: "j", Value: 1}}}},
				{{Key: "$limit", Value: 1}},
			})
		}
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"sort_walk", find(bson.D{}, options.Find().SetSort(bson.D{{Key: "b", Value: -1}}).SetLimit(2))},
		{"or_union", find(bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: int32(2)}}, bson.D{{Key: "b", Value: int32(2)}}}}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))},
		{"meta_first", pipeline()},
		{"meta_after_match", pipeline(bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: int32(2)}}}})},
		{"meta_after_sort", pipeline(bson.D{{Key: "$sort", Value: bson.D{{Key: "b", Value: 1}}}})},
		{"meta_after_limit", pipeline(bson.D{{Key: "$limit", Value: int32(2)}})},
		{"meta_after_set", pipeline(bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: int32(1)}}}})},
		{"lookup_sub_pipeline", joined(bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: int32(2)}}}}, meta})},
		{"facet", cluAggregateDocs(bson.D{{Key: "$facet", Value: bson.D{{Key: "x", Value: bson.A{meta}}}}})},
		{"find_and_modify", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.D
			err := col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: int32(1)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}},
				options.FindOneAndUpdate().SetProjection(bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}})).Decode(&doc)
			return doc, err
		}},
		{"find_and_modify_with_field", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.D
			err := col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: int32(1)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}},
				options.FindOneAndUpdate().SetProjection(bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}, {Key: "a", Value: 1}}).
					SetReturnDocument(options.After)).Decode(&doc)
			return doc, err
		}},
		{"lookup_sub_pipeline_reshaped", joined(bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "q", Value: 1}}}}, meta})},
		{"set_meta", cluAggregateDocs(bson.D{{Key: "$set", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$meta", Value: "recordId"}}}}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_RecordIDPaths_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_HintDescendingIndex: an unsorted read hinted to an index with a
// descending key returns documents in index order.
func TestClustered_HintDescendingIndex(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(2)}, {Key: "b", Value: int32(3)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: int32(2)}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: int32(1)}, {Key: "b", Value: int32(5)}},
			bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: int32(2)}, {Key: "b", Value: nil}},
			bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: int32(1)}, {Key: "b", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: int32(2)}, {Key: "b", Value: int32(2)}},
		})
		if err != nil {
			return err
		}
		_, err = col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "b", Value: -1}}, Options: options.Index().SetName("bd")},
			{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}, Options: options.Index().SetName("a_bd")},
		})
		return err
	}
	read := cluFindDocs
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"whole_index", read(bson.D{}, options.Find().SetHint("bd"))},
		{"covered", read(bson.D{{Key: "a", Value: int32(2)}}, options.Find().SetHint(bson.D{{Key: "b", Value: -1}}).SetProjection(bson.D{{Key: "b", Value: 1}, {Key: "_id", Value: 0}}))},
		{"mixed_compound", read(bson.D{}, options.Find().SetHint("a_bd"))},
		{"mixed_compound_filtered", read(bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: int32(1)}}}}, options.Find().SetHint("a_bd").SetLimit(4))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_HintDescendingIndex_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_HintDescendingMultikey: a read or single-document write hinted
// to a descending multikey index takes documents in index order, which reaches
// a document first at the entry of an array element inside the filter's
// bounds, not at its largest element.
func TestClustered_HintDescendingMultikey(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10)
		for i := int32(1); i <= 10; i++ {
			var a, b interface{} = i % 3, i * 7 % 10
			switch i {
			case 7:
				a = bson.A{int32(1), int32(4)}
			case 8:
				b = bson.A{int32(0), int32(5)}
			}
			docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "a", Value: a}, {Key: "b", Value: b}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "b", Value: -1}}, Options: options.Index().SetName("bd")},
			{Keys: bson.D{{Key: "a", Value: -1}, {Key: "b", Value: 1}}, Options: options.Index().SetName("ad_b")},
		})
		return err
	}
	bLow := bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: int32(3)}}}}
	aIn := bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2)}}}}, {Key: "b", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: true}}}}
	touched := cluFindIDs(bson.D{{Key: "w", Value: true}})
	remaining := cluFindIDs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"find_bd", cluFindIDs(bLow, options.Find().SetHint("bd"))},
		{"find_ad_b", cluFindIDs(aIn, options.Find().SetHint("ad_b"))},
		{"find_and_modify", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := col.FindOneAndUpdate(ctx, bLow, set, options.FindOneAndUpdate().SetHint("bd")).Err(); err != nil {
				return nil, err
			}
			return touched(ctx, col)
		}},
		{"update_one", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx, aIn, set, options.Update().SetHint("ad_b")); err != nil {
				return nil, err
			}
			return touched(ctx, col)
		}},
		{"delete_one", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteOne(ctx, bLow, options.Delete().SetHint("bd")); err != nil {
				return nil, err
			}
			return remaining(ctx, col)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_HintDescendingMultikey_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_HintDescendingAggregate: an aggregate hinted to an index
// with a descending key and no $sort returns documents in index order.
func TestClustered_HintDescendingAggregate(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10)
		for i := int32(1); i <= 10; i++ {
			var b interface{} = i * 7 % 10
			if i == 8 {
				b = bson.A{int32(0), int32(5)}
			}
			docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "a", Value: i % 3}, {Key: "b", Value: b}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "b", Value: -1}}, Options: options.Index().SetName("bd")},
			{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}, Options: options.Index().SetName("a_bd")},
		})
		return err
	}
	ids := func(pipeline mongo.Pipeline, hint string) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			docs, err := runPipeline(ctx, col, pipeline, options.Aggregate().SetHint(hint))
			if err != nil {
				return nil, err
			}
			out := bson.A{}
			for _, d := range docs {
				out = append(out, d[0].Value)
			}
			return out, nil
		}
	}
	match := func(f bson.D) bson.D { return bson.D{{Key: "$match", Value: f}} }
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"whole_index", ids(mongo.Pipeline{}, "bd")},
		{"multikey_range", ids(mongo.Pipeline{match(bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 3}}}})}, "bd")},
		{"compound", ids(mongo.Pipeline{match(bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 1}}}})}, "a_bd")},
		{"then_project", ids(mongo.Pipeline{match(bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: 2}}}}), {{Key: "$project", Value: bson.D{{Key: "b", Value: 1}}}}}, "bd")},
		{"limit", ids(mongo.Pipeline{{{Key: "$limit", Value: 3}}}, "bd")},
		{"project_id_only", ids(mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}}}, "bd")},
		{"project_other_field", ids(mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}}, "bd")},
		{"group_push", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runPipeline(ctx, col, mongo.Pipeline{{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "x", Value: bson.D{{Key: "$push", Value: "$_id"}}}}}}}, options.Aggregate().SetHint("bd"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_HintDescendingAggregate_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestRange_MinMaxKeySparse: count and find on MinKey and MaxKey bounds, with
// and without a sparse index on the field. Which documents match depends on the
// plan: one that reads the sparse index leaves out the documents without the
// field, and one that scans the collection does not. mongod answers count and
// an unsorted find from the index, and an _id-sorted find from the collection.
func TestRange_MinMaxKeySparse(t *testing.T) {
	setup := func(sparse bool) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 0, 200)
			for i := 0; i < 200; i++ {
				d := bson.D{{Key: "_id", Value: int32(i)}}
				switch {
				case i%6 == 0:
				case i%6 == 1:
					d = append(d, bson.E{Key: "b", Value: nil})
				default:
					d = append(d, bson.E{Key: "b", Value: int32(i)})
				}
				docs = append(docs, d)
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			if !sparse {
				return nil
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "b", Value: 1}}, Options: options.Index().SetSparse(true)})
			return err
		}
	}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"lte_maxkey", bson.D{{Key: "b", Value: bson.D{{Key: "$lte", Value: primitive.MaxKey{}}}}}},
		{"lt_maxkey", bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: primitive.MaxKey{}}}}}},
		{"gte_minkey", bson.D{{Key: "b", Value: bson.D{{Key: "$gte", Value: primitive.MinKey{}}}}}},
		{"gt_minkey", bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: primitive.MinKey{}}}}}},
	}
	for _, f := range filters {
		for _, sparse := range []bool{false, true} {
			name := f.name
			if sparse {
				name += "_sparse"
			}
			t.Run(name, func(t *testing.T) {
				tc := harness.TestCase{
					Name:      "Range_MinMaxKeySparse_" + name,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup:     setup(sparse),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						n, err := col.CountDocuments(ctx, f.filter)
						if err != nil {
							return nil, err
						}
						var cmd bson.M
						if err := col.Database().RunCommand(ctx, bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: f.filter}}).Decode(&cmd); err != nil {
							return nil, err
						}
						unsorted, err := cluFindIDs(f.filter)(ctx, col)
						if err != nil {
							return nil, err
						}
						sorted, err := cluFindIDs(f.filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
						if err != nil {
							return nil, err
						}
						return bson.D{
							{Key: "countDocuments", Value: n},
							{Key: "count", Value: cmd["n"]},
							{Key: "find", Value: int32(len(unsorted.(bson.A)))},
							{Key: "findSortedByID", Value: int32(len(sorted.(bson.A)))},
						}, nil
					},
				}
				harness.PairTest(t, tc)
			})
		}
	}
}

// TestMultikey_FirstOfEqualElements: array elements with equal keys share one
// index entry, which keeps the first element's value and type.
func TestMultikey_FirstOfEqualElements(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "Multikey_FirstOfEqualElements",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(2), 2.0}}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{3.0, int32(3)}}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: bson.A{int64(4), mustDecimal("4.0"), 4.0}}},
			})
			if err != nil {
				return err
			}
			_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.D
			err := col.Database().RunCommand(ctx, bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "a"}}).Decode(&res)
			return res, err
		},
	})
}

// TestClustered_WriteDescendingHint: a single-document write hinted to an
// index with a descending key acts on the first match in that index's order.
func TestClustered_WriteDescendingHint(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 10)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 3)}, {Key: "b", Value: int32(i * 7 % 10)}, {Key: "arr", Value: bson.A{int32(1), int32(2)}}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "b", Value: -1}}, Options: options.Index().SetName("bd")},
			{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}, Options: options.Index().SetName("ab")},
		})
		return err
	}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: true}}}}
	touched := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return cluFindIDs(bson.D{{Key: "w", Value: true}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
	}
	remaining := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return cluFindIDs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
	}
	for _, hint := range []interface{}{"bd", bson.D{{Key: "b", Value: -1}}, "ab"} {
		hint := hint
		name := fmt.Sprint(hint)
		if _, ok := hint.(bson.D); ok {
			name = "b_desc_spec"
		}
		cases := []struct {
			name string
			run  func(context.Context, *mongo.Collection) (interface{}, error)
		}{
			{"find_and_modify", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if err := col.FindOneAndUpdate(ctx, bson.D{{Key: "a", Value: int32(1)}}, set, options.FindOneAndUpdate().SetHint(hint)).Err(); err != nil {
					return nil, err
				}
				return touched(ctx, col)
			}},
			{"update_one", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.UpdateOne(ctx, bson.D{{Key: "a", Value: int32(0)}}, set, options.Update().SetHint(hint)); err != nil {
					return nil, err
				}
				return touched(ctx, col)
			}},
			{"update_one_array_filters", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				update := bson.D{{Key: "$set", Value: bson.D{{Key: "arr.$[e]", Value: int32(9)}}}}
				opts := options.Update().SetHint(hint).SetArrayFilters(options.ArrayFilters{Filters: []interface{}{bson.D{{Key: "e", Value: int32(2)}}}})
				if _, err := col.UpdateOne(ctx, bson.D{{Key: "a", Value: int32(2)}}, update, opts); err != nil {
					return nil, err
				}
				return cluFindIDs(bson.D{{Key: "arr", Value: int32(9)}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
			}},
			{"delete_one", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.DeleteOne(ctx, bson.D{{Key: "a", Value: int32(2)}}, options.Delete().SetHint(hint)); err != nil {
					return nil, err
				}
				return remaining(ctx, col)
			}},
		}
		for _, tc := range cases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "Clustered_WriteDescendingHint_" + name + "_" + tc.name,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup:     setup,
					Run:       tc.run,
				})
			})
		}
	}
}

// TestClustered_RecordIDOutsideProjection: $meta "recordId" has a value only
// in a projection; in a filter or an accumulator it is missing.
func TestClustered_RecordIDOutsideProjection(t *testing.T) {
	meta := bson.D{{Key: "$meta", Value: "recordId"}}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"find_expr", cluFindDocs(bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{meta, nil}}}}})},
		{"match_expr", cluAggregateDocs(bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{meta, nil}}}}}}})},
		{"match_type", cluAggregateDocs(bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: meta}}, "binData"}}}}}}})},
		{"group_push", cluAggregateDocs(bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "ids", Value: bson.D{{Key: "$push", Value: meta}}}}}})},
		{"project", cluAggregateDocs(bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: meta}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_RecordIDOutsideProjection_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     cluInsertIDs([]interface{}{int32(1), int32(2), int32(3)}),
				Run:       tc.run,
			})
		})
	}
}

// TestHint_Invalid: hints of the wrong type, $natural values other than 1 and
// -1, empty or unknown names, a $natural sort against the hint, an unknown
// index hinted on an upsert into a missing collection, and a bad hint in a
// later or unordered update statement are refused as mongod refuses them.
func TestHint_Invalid(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}}, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 2}}}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
		return err
	}
	// outcome is the command error's code, or n and the write errors' codes.
	// mongod's message for an unknown index embeds its query tree, so only
	// the $natural messages are compared.
	outcome := func(res bson.M, err error) interface{} {
		var we mongo.WriteException
		if errors.As(err, &we) {
			var codes bson.A
			for _, e := range we.WriteErrors {
				codes = append(codes, int32(e.Index), int32(e.Code))
			}
			n, _ := we.Raw.Lookup("n").AsInt64OK()
			return bson.A{"n", n, codes}
		}
		var ce mongo.CommandError
		if errors.As(err, &ce) {
			if strings.Contains(ce.Message, "$natural") {
				return bson.A{"command", ce.Code, ce.Message}
			}
			return bson.A{"command", ce.Code}
		}
		if err != nil {
			return fmt.Sprint(err)
		}
		n, _ := res["n"].(int32)
		return bson.A{"n", int64(n), bson.A(nil)}
	}
	run := func(cmd func(col *mongo.Collection) bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.M
			err := col.Database().RunCommand(ctx, cmd(col)).Decode(&res)
			return outcome(res, err), nil
		}
	}
	hints := []struct {
		name string
		hint interface{}
	}{
		{"natural_2", bson.D{{Key: "$natural", Value: 2}}},
		{"natural_0", bson.D{{Key: "$natural", Value: 0}}},
		{"natural_string", bson.D{{Key: "$natural", Value: "x"}}},
		{"natural_decimal_1", bson.D{{Key: "$natural", Value: mustDecimal("1")}}},
		{"natural_decimal_minus_1", bson.D{{Key: "$natural", Value: mustDecimal("-1.0")}}},
		{"natural_decimal_2", bson.D{{Key: "$natural", Value: mustDecimal("2")}}},
		{"int", 5},
		{"array", bson.A{1}},
		{"empty_name", ""},
		{"empty_document", bson.D{}},
		{"bool", true},
		{"unknown_name", "zz"},
	}
	for _, h := range hints {
		cmds := []struct {
			name string
			cmd  func(col *mongo.Collection) bson.D
		}{
			{"update", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 1}}}}}, {Key: "hint", Value: h.hint}}}}}
			}},
			{"delete", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 2}}}, {Key: "limit", Value: 1}, {Key: "hint", Value: h.hint}}}}}
			}},
			{"find_and_modify", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 2}}}}}, {Key: "hint", Value: h.hint}}
			}},
			{"find", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "hint", Value: h.hint}}
			}},
			{"count", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "hint", Value: h.hint}}
			}},
			{"distinct", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "a"}, {Key: "hint", Value: h.hint}}
			}},
			{"aggregate", func(col *mongo.Collection) bson.D {
				return bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: bson.A{}}, {Key: "cursor", Value: bson.D{}}, {Key: "hint", Value: h.hint}}
			}},
		}
		for _, c := range cmds {
			t.Run(h.name+"/"+c.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Hint_Invalid_" + h.name + "_" + c.name,
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run:     run(c.cmd),
				})
			})
		}
	}
	extra := []struct {
		name string
		cmd  func(col *mongo.Collection) bson.D
	}{
		{"find_and_modify_upsert_missing_collection", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name() + "_missing"}, {Key: "query", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 2}}}}}, {Key: "upsert", Value: true}, {Key: "hint", Value: "a1"}}
		}},
		{"update_upsert_missing_collection", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name() + "_missing"}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 2}}}}}, {Key: "upsert", Value: true}, {Key: "hint", Value: "a1"}}}}}
		}},
		{"find_and_modify_natural_sort_against_hint", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 3}}}}}, {Key: "sort", Value: bson.D{{Key: "$natural", Value: 1}}}, {Key: "hint", Value: bson.D{{Key: "$natural", Value: -1}}}}
		}},
		{"find_natural_sort_against_hint", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{}}, {Key: "sort", Value: bson.D{{Key: "$natural", Value: 1}}}, {Key: "hint", Value: bson.D{{Key: "$natural", Value: -1}}}}
		}},
		{"update_unordered_bad_hint_in_first_statement", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name()}, {Key: "ordered", Value: false}, {Key: "updates", Value: bson.A{
				bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 5}}}}}, {Key: "hint", Value: "zz"}},
				bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 2}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 5}}}}}},
			}}}
		}},
		{"update_bad_hint_in_second_statement", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{
				bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 5}}}}}},
				bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: 2}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 5}}}}}, {Key: "hint", Value: "zz"}},
			}}}
		}},
	}
	for _, c := range extra {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Hint_Invalid_" + c.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     run(c.cmd),
			})
		})
	}
}

// TestClustered_CoveredLeadingKeyUnfiltered: a hinted find whose filter does
// not name the index's leading field fetches each document, so a document
// missing that field shows no null for it; with no filter the read is
// covered and does.
func TestClustered_CoveredLeadingKeyUnfiltered(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "c", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "b", Value: nil}, {Key: "c", Value: 2}},
			bson.D{{Key: "_id", Value: 3}, {Key: "b", Value: 5}, {Key: "c", Value: 3}},
		}); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "b", Value: 1}, {Key: "c", Value: 1}})
	}
	opts := func() *options.FindOptions {
		return options.Find().SetProjection(bson.D{{Key: "b", Value: 1}, {Key: "c", Value: 1}, {Key: "_id", Value: 0}}).
			SetHint(bson.D{{Key: "b", Value: 1}, {Key: "c", Value: 1}})
	}
	cases := []struct {
		name   string
		filter bson.D
	}{
		{"trailing_only", bson.D{{Key: "c", Value: bson.D{{Key: "$gte", Value: 0}}}}},
		{"leading_exists", bson.D{{Key: "b", Value: bson.D{{Key: "$exists", Value: true}}}, {Key: "c", Value: bson.D{{Key: "$gte", Value: 0}}}}},
		{"no_filter", bson.D{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_CoveredLeadingKeyUnfiltered_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       cluFindDocs(tc.filter, opts()),
			})
		})
	}
}

// TestClustered_HintDescendingBounds: a single-document write or read hinted
// to an index with a descending key takes the first document in index order,
// which follows the filter's bounds under $and and $or, ignores bounds below
// an unbounded leading field, and keys an empty array as undefined.
func TestClustered_HintDescendingBounds(t *testing.T) {
	cases := []struct {
		name   string
		docs   []interface{}
		index  bson.D
		filter bson.D
	}{
		{"and", []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "b", Value: 4}}, bson.D{{Key: "_id", Value: 2}, {Key: "b", Value: bson.A{1, 9}}}, bson.D{{Key: "_id", Value: 3}, {Key: "b", Value: 3}}},
			bson.D{{Key: "b", Value: -1}}, bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 5}}}}}}}},
		{"or", []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "b", Value: 4}}, bson.D{{Key: "_id", Value: 2}, {Key: "b", Value: bson.A{1, 9}}}, bson.D{{Key: "_id", Value: 3}, {Key: "b", Value: 3}}},
			bson.D{{Key: "b", Value: -1}}, bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 5}}}}, bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 2}}}}}}}},
		{"trailing_field", []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: -1}, {Key: "b", Value: 4}}, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: -1}, {Key: "b", Value: bson.A{1, 9}}}},
			bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}, bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 5}}}}},
		{"empty_array", []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{}}}, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: primitive.MinKey{}}}},
			bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}, bson.D{}},
	}
	for _, tc := range cases {
		setup := func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.InsertMany(ctx, tc.docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: tc.index, Options: options.Index().SetName("h")})
			return err
		}
		t.Run(tc.name+"/find_and_modify", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name: "Clustered_HintDescendingBounds_" + tc.name + "_find_and_modify", Support: harness.DumboDBFull, Clustered: true, Setup: setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var doc bson.D
					err := col.FindOneAndUpdate(ctx, tc.filter, bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 1}}}}, options.FindOneAndUpdate().SetHint("h")).Decode(&doc)
					return doc, err
				},
			})
		})
		t.Run(tc.name+"/find", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name: "Clustered_HintDescendingBounds_" + tc.name + "_find", Support: harness.DumboDBFull, Clustered: true, Setup: setup,
				Run: cluFindIDs(tc.filter, options.Find().SetHint("h")),
			})
		})
	}
}

// TestHint_EmptyNameMissingCollection: an empty hint on a missing collection
// reads nothing, as any hint there does, but an upsert refuses it.
func TestHint_EmptyNameMissingCollection(t *testing.T) {
	cmds := []struct {
		name string
		cmd  func(name string) bson.D
	}{
		{"find", func(name string) bson.D { return bson.D{{Key: "find", Value: name}, {Key: "hint", Value: ""}} }},
		{"find_and_modify", func(name string) bson.D {
			return bson.D{{Key: "findAndModify", Value: name}, {Key: "query", Value: bson.D{}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}}}, {Key: "hint", Value: ""}}
		}},
		{"find_and_modify_upsert", func(name string) bson.D {
			return bson.D{{Key: "findAndModify", Value: name}, {Key: "query", Value: bson.D{}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}}}, {Key: "upsert", Value: true}, {Key: "hint", Value: ""}}
		}},
	}
	for _, c := range cmds {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Hint_EmptyNameMissingCollection_" + c.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					err := col.Database().RunCommand(ctx, c.cmd(col.Name()+"_missing")).Err()
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						return bson.D{{Key: "code", Value: ce.Code}}, nil
					}
					return bson.D{{Key: "ok", Value: err == nil}}, nil
				},
			})
		})
	}
}

// twoCreates creates the collection with first, then runs create again with
// second, returning the second's error code and its message without the
// collection's UUID.
func twoCreates(create func(...bson.E) func(context.Context, *mongo.Collection) (interface{}, error), first, second []bson.E) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		if _, err := create(first...)(ctx, col); err != nil {
			return nil, err
		}
		_, err := create(second...)(ctx, col)
		var ce mongo.CommandError
		if errors.As(err, &ce) {
			msg := regexp.MustCompile(`UUID\("[^"]+"\)`).ReplaceAllString(ce.Message, "UUID(...)")
			msg = strings.ReplaceAll(msg, col.Database().Name()+".", "<db>.")
			return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: msg}}, nil
		}
		return bson.D{{Key: "ok", Value: err == nil}}, nil
	}
}

// TestClustered_HintTopLevelOperator: a read or single-document write hinted
// to an ascending index takes documents in index order when its filter's top
// level is $or, $and, $nor or $expr.
func TestClustered_HintTopLevelOperator(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 5}, {Key: "b", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 9}, {Key: "b", Value: 3}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 7}, {Key: "b", Value: 2}},
			bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: 0}, {Key: "b", Value: 2}},
			bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: 3}, {Key: "b", Value: 1}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}, Options: options.Index().SetName("a1")})
		return err
	}
	or := bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "b", Value: 1}}, bson.D{{Key: "b", Value: 2}}}}}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"or", or},
		{"and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "b", Value: bson.D{{Key: "$gte", Value: 1}}}}}}}},
		{"nor", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "b", Value: 3}}}}}},
		{"expr", bson.D{{Key: "$expr", Value: bson.D{{Key: "$gte", Value: bson.A{"$b", 2}}}}}},
	}
	type tcase struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}
	var cases []tcase
	for _, f := range filters {
		cases = append(cases, tcase{"find_" + f.name, cluFindIDs(f.filter, options.Find().SetHint("a1"))})
	}
	cases = append(cases,
		tcase{"find_and_modify_or", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.D
			err := col.FindOneAndUpdate(ctx, or, bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: 1}}}}, options.FindOneAndUpdate().SetHint("a1")).Decode(&doc)
			return doc, err
		}},
		tcase{"delete_one_or", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteOne(ctx, or, options.Delete().SetHint("a1")); err != nil {
				return nil, err
			}
			return cluFindIDs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		}},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Clustered_HintTopLevelOperator_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestClustered_NamedClusteredIndex: a collection whose clustered index is
// named "foo" takes "foo" as a hint in every command, no longer knows "_id_",
// and refuses to drop the clustered index by name or key.
func TestClustered_NamedClusteredIndex(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		db := col.Database()
		name := col.Name() + "_n"
		if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: name}, {Key: "clusteredIndex", Value: bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "unique", Value: true}, {Key: "name", Value: "foo"}}}}).Err(); err != nil {
			return err
		}
		_, err := db.Collection(name).InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 1}}, bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 2}}, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 1}}})
		return err
	}
	q := bson.D{{Key: "a", Value: 1}}
	cmds := []struct {
		name string
		cmd  func(n string) bson.D
	}{
		{"find", func(n string) bson.D {
			return bson.D{{Key: "find", Value: n}, {Key: "filter", Value: q}, {Key: "hint", Value: "foo"}}
		}},
		{"find_default_name", func(n string) bson.D {
			return bson.D{{Key: "find", Value: n}, {Key: "filter", Value: q}, {Key: "hint", Value: "_id_"}}
		}},
		{"count", func(n string) bson.D {
			return bson.D{{Key: "count", Value: n}, {Key: "query", Value: q}, {Key: "hint", Value: "foo"}}
		}},
		{"distinct", func(n string) bson.D {
			return bson.D{{Key: "distinct", Value: n}, {Key: "key", Value: "a"}, {Key: "hint", Value: "foo"}}
		}},
		{"aggregate", func(n string) bson.D {
			return bson.D{{Key: "aggregate", Value: n}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: q}}}}, {Key: "cursor", Value: bson.D{}}, {Key: "hint", Value: "foo"}}
		}},
		{"update", func(n string) bson.D {
			return bson.D{{Key: "update", Value: n}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: q}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}}}, {Key: "hint", Value: "foo"}}}}}
		}},
		{"find_and_modify", func(n string) bson.D {
			return bson.D{{Key: "findAndModify", Value: n}, {Key: "query", Value: q}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "y", Value: 1}}}}}, {Key: "hint", Value: "foo"}}
		}},
		{"drop_by_name", func(n string) bson.D { return bson.D{{Key: "dropIndexes", Value: n}, {Key: "index", Value: "foo"}} }},
		{"drop_by_key", func(n string) bson.D {
			return bson.D{{Key: "dropIndexes", Value: n}, {Key: "index", Value: bson.D{{Key: "_id", Value: 1}}}}
		}},
		{"drop_default_name", func(n string) bson.D { return bson.D{{Key: "dropIndexes", Value: n}, {Key: "index", Value: "_id_"}} }},
		{"create_name_clash_unique_sparse", createIndexSpec(bson.D{{Key: "key", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "name", Value: "foo"},
			{Key: "unique", Value: true}, {Key: "sparse", Value: true}})},
		{"create_name_clash_partial", createIndexSpec(bson.D{{Key: "key", Value: bson.D{{Key: "a", Value: 1}}}, {Key: "name", Value: "foo"},
			{Key: "partialFilterExpression", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}})},
		{"create_id_sparse", createIndexSpec(bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "name", Value: "foo"}, {Key: "sparse", Value: true}})},
		{"create_id_partial", createIndexSpec(bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "name", Value: "p"},
			{Key: "partialFilterExpression", Value: bson.D{{Key: "a", Value: 1}}}})},
	}
	for _, c := range cmds {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Clustered_NamedClusteredIndex_" + c.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.M
					err := col.Database().RunCommand(ctx, c.cmd(col.Name()+"_n")).Decode(&res)
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						// mongod's message for an unknown hint embeds its query tree.
						if strings.HasPrefix(ce.Message, "error processing query") || strings.Contains(ce.Message, "hint provided does not correspond") {
							return bson.D{{Key: "code", Value: ce.Code}}, nil
						}
						return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: ce.Message}}, nil
					}
					if err != nil {
						return nil, err
					}
					out := bson.D{}
					for _, k := range []string{"n", "nModified", "values"} {
						if v, ok := res[k]; ok {
							out = append(out, bson.E{Key: k, Value: v})
						}
					}
					if cur, ok := res["cursor"].(bson.M); ok {
						out = append(out, bson.E{Key: "firstBatch", Value: cur["firstBatch"]})
					}
					if v, ok := res["value"].(bson.M); ok {
						out = append(out, bson.E{Key: "value_id", Value: v["_id"]})
					}
					return out, nil
				},
			})
		})
	}
}

// createIndexSpec returns a createIndexes command of spec on the named
// collection.
func createIndexSpec(spec bson.D) func(n string) bson.D {
	return func(n string) bson.D {
		return bson.D{{Key: "createIndexes", Value: n}, {Key: "indexes", Value: bson.A{spec}}}
	}
}

// TestClustered_HintPartialIndex: a read or write hinted to a partial index
// sees only the documents the index holds, even ones matching the filter.
func TestClustered_HintPartialIndex(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 2}, {Key: "c", Value: 1}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 3}, {Key: "c", Value: 1}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}},
			Options: options.Index().SetName("a_part").SetPartialFilterExpression(bson.D{{Key: "c", Value: bson.D{{Key: "$exists", Value: true}}}})})
		return err
	}
	q := bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 1}}}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: 1}}}}
	all := cluFindIDs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	hit := cluFindIDs(bson.D{{Key: "hit", Value: 1}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"find", cluFindIDs(q, options.Find().SetHint("a_part"))},
		{"count", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, q, options.Count().SetHint("a_part"))
		}},
		{"distinct", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.M
			err := col.Database().RunCommand(ctx, bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "_id"}, {Key: "query", Value: q}, {Key: "hint", Value: "a_part"}}).Decode(&res)
			return res["values"], err
		}},
		{"aggregate", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runPipeline(ctx, col, mongo.Pipeline{{{Key: "$match", Value: q}}, {{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}}}, options.Aggregate().SetHint("a_part"))
		}},
		{"find_and_modify", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.D
			err := col.FindOneAndUpdate(ctx, q, set, options.FindOneAndUpdate().SetHint("a_part")).Decode(&doc)
			return doc, err
		}},
		{"update_one", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx, q, set, options.Update().SetHint("a_part")); err != nil {
				return nil, err
			}
			return hit(ctx, col)
		}},
		{"update_many", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateMany(ctx, q, set, options.Update().SetHint("a_part")); err != nil {
				return nil, err
			}
			return hit(ctx, col)
		}},
		{"delete_one", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteOne(ctx, q, options.Delete().SetHint("a_part")); err != nil {
				return nil, err
			}
			return all(ctx, col)
		}},
		{"delete_many", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.DeleteMany(ctx, q, options.Delete().SetHint("a_part")); err != nil {
				return nil, err
			}
			return all(ctx, col)
		}},
	}
	for _, tc := range cases {
		for _, clustered := range []bool{true, false} {
			name := tc.name
			if !clustered {
				name += "_unclustered"
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "Clustered_HintPartialIndex_" + name,
					Support:   harness.DumboDBFull,
					Clustered: clustered,
					Setup:     setup,
					Run:       tc.run,
				})
			})
		}
	}
}

// TestHint_TypeRegexFirstInBounds: a single write hinted to a multikey index
// under $type or $regex picks the document whose first key inside the bounds
// comes first, not the one with the least element.
func TestHint_TypeRegexFirstInBounds(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: ""}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: bson.A{2.5, ""}}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: "x"}},
			bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: bson.A{primitive.MinKey{}, "q"}}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
		return err
	}
	hint := bson.D{{Key: "a", Value: 1}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: 1}}}}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"type_string", bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "string"}}}}},
		{"regex_empty", bson.D{{Key: "a", Value: bson.D{{Key: "$regex", Value: "^$"}}}}},
		{"regex_literal", bson.D{{Key: "a", Value: primitive.Regex{Pattern: "^$"}}}},
		{"type_double", bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "double"}}}}},
	}
	ops := []struct {
		name string
		run  func(bson.D) func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"find_and_modify", func(f bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
			return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var doc bson.D
				err := col.FindOneAndUpdate(ctx, f, set, options.FindOneAndUpdate().SetHint(hint)).Decode(&doc)
				return doc, err
			}
		}},
		{"update_one", func(f bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
			return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.UpdateOne(ctx, f, set, options.Update().SetHint(hint)); err != nil {
					return nil, err
				}
				return cluFindIDs(bson.D{{Key: "hit", Value: 1}})(ctx, col)
			}
		}},
		{"delete_one", func(f bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
			return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.DeleteOne(ctx, f, options.Delete().SetHint(hint)); err != nil {
					return nil, err
				}
				return cluFindIDs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
			}
		}},
		{"find", func(f bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
			return cluFindIDs(f, options.Find().SetHint(hint))
		}},
	}
	for _, f := range filters {
		for _, op := range ops {
			t.Run(f.name+"_"+op.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Hint_TypeRegexFirstInBounds_" + f.name + "_" + op.name,
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run:     op.run(f.filter),
				})
			})
		}
	}
}

// TestHint_NaturalByCommand: an aggregate led by $match overlooks a bad
// $natural value, as distinct does on a missing collection; both refuse a
// compound $natural hint with their own message, as find does with another.
func TestHint_NaturalByCommand(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 2}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
		return err
	}
	hints := []struct {
		name string
		hint bson.D
	}{
		{"two", bson.D{{Key: "$natural", Value: 2}}},
		{"zero", bson.D{{Key: "$natural", Value: 0}}},
		{"string", bson.D{{Key: "$natural", Value: "1"}}},
		{"compound", bson.D{{Key: "$natural", Value: 1}, {Key: "a", Value: 1}}},
	}
	cmds := []struct {
		name string
		cmd  func(coll string, hint bson.D) bson.D
	}{
		{"aggregate_match", func(c string, h bson.D) bson.D {
			return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 1}}}}}}}},
				{Key: "hint", Value: h}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"aggregate_project", func(c string, h bson.D) bson.D {
			return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}}},
				{Key: "hint", Value: h}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"distinct", func(c string, h bson.D) bson.D {
			return bson.D{{Key: "distinct", Value: c}, {Key: "key", Value: "a"}, {Key: "hint", Value: h}}
		}},
		{"find", func(c string, h bson.D) bson.D { return bson.D{{Key: "find", Value: c}, {Key: "hint", Value: h}} }},
	}
	for _, h := range hints {
		for _, c := range cmds {
			for _, missing := range []bool{false, true} {
				name := fmt.Sprintf("%s_%s_missing_%t", c.name, h.name, missing)
				t.Run(name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    "Hint_NaturalByCommand_" + name,
						Support: harness.DumboDBFull,
						Setup:   setup,
						Run: errorMessage(func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							coll := col.Name()
							if missing {
								coll += "_missing"
							}
							var res bson.M
							if err := col.Database().RunCommand(ctx, c.cmd(coll, h.hint)).Decode(&res); err != nil {
								return nil, err
							}
							if cur, ok := res["cursor"].(bson.M); ok {
								return cur["firstBatch"], nil
							}
							return res["values"], nil
						}),
					})
				})
			}
		}
	}
}

// TestHint_DottedDescendingAggregate: an aggregate hinted to an index with a
// dotted descending key returns that index's order even when its stages do
// not use the dotted field.
func TestHint_DottedDescendingAggregate(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}, {Key: "c", Value: bson.D{{Key: "d", Value: 3}}}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 1}, {Key: "c", Value: bson.D{{Key: "d", Value: 1}}}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 2}, {Key: "c", Value: bson.D{{Key: "d", Value: 2}}}},
			bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: 2}, {Key: "c", Value: bson.D{{Key: "d", Value: 5}}}},
			bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: 3}},
		}); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "c.d", Value: -1}}},
			{Keys: bson.D{{Key: "a", Value: -1}, {Key: "c.d", Value: 1}}},
		})
		return err
	}
	hints := []struct {
		name string
		hint bson.D
	}{
		{"dotted_descending", bson.D{{Key: "c.d", Value: -1}}},
		{"compound_dotted", bson.D{{Key: "a", Value: -1}, {Key: "c.d", Value: 1}}},
	}
	pipelines := []struct {
		name     string
		pipeline mongo.Pipeline
	}{
		{"empty", mongo.Pipeline{}},
		{"project_other", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}}},
		{"project_dotted", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "c.d", Value: 1}}}}}},
	}
	for _, h := range hints {
		for _, p := range pipelines {
			name := h.name + "_" + p.name
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Hint_DottedDescendingAggregate_" + name,
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return runPipeline(ctx, col, p.pipeline, options.Aggregate().SetHint(h.hint))
					},
				})
			})
		}
	}
}

// TestHint_FirstInBoundsEdges: a hinted single write and find take documents
// in mongod's order when an element is a nested array, under $type "array"
// or a list of types, across $or branches mongod runs one at a time, and when
// a multikey field has two comparisons, of which mongod bounds the walk by
// the one holding fewer keys.
func TestHint_FirstInBoundsEdges(t *testing.T) {
	d := func(id int, v interface{}) bson.D { return bson.D{{Key: "_id", Value: id}, {Key: "a", Value: v}} }
	cases := []struct {
		name   string
		docs   []interface{}
		index  bson.D
		filter bson.D
	}{
		{"nested_type_desc", []interface{}{d(3, bson.A{"b", bson.A{"z"}}), d(4, "c")}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "string"}}}}},
		{"nested_regex_desc", []interface{}{d(3, bson.A{"b", bson.A{"z"}}), d(4, "c")}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$regex", Value: "^[a-z]"}}}}},
		{"nested_gt_desc", []interface{}{d(3, bson.A{"b", bson.A{"z"}}), d(4, "c")}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: "a"}}}}},
		{"or_two_predicate_branch", []interface{}{d(1, 0), d(2, 2)}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: 0}}, bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$lt", Value: 3}}}}}}}},
		{"or_lt_gt", []interface{}{d(1, 0), d(2, 9), d(3, 5)}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 7}}}}, bson.D{{Key: "a", Value: bson.D{{Key: "$lt", Value: 1}}}}}}}},
		{"type_array", []interface{}{d(2, bson.A{3, 1}), d(4, bson.A{0, 9}), d(5, bson.A{5, 2})}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "array"}}}}},
		{"type_array_nested", []interface{}{d(1, bson.A{7, bson.A{0}}), d(6, bson.A{bson.A{-1}})}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "array"}}}}},
		{"type_list", []interface{}{d(1, ""), d(2, bson.A{2.5, ""}), d(3, "x"), d(4, bson.A{primitive.MinKey{}, "q"}), d(5, bson.A{bson.A{1}, "z"})},
			bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: bson.A{"string", "double"}}}}}},
		{"type_list_with_array", []interface{}{d(1, bson.A{7, bson.A{0}}), d(6, bson.A{bson.A{-1}}), d(3, "s")}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: bson.A{"array", "string"}}}}}},
		{"multikey_gt_lt", []interface{}{d(1, bson.A{"ab", 0}), d(2, "ab"), d(3, bson.A{"", "b"})}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: "a"}, {Key: "$lt", Value: "b"}}}}},
		{"multikey_gt_lt_desc", []interface{}{d(1, bson.A{"ab", 0}), d(2, "ab"), d(3, bson.A{"", "b"})}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: "a"}, {Key: "$lt", Value: "b"}}}}},
		{"or_range_and_equality_merge", []interface{}{d(1, 0), d(2, 2)}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: bson.D{{Key: "$lt", Value: 1}}}}, bson.D{{Key: "a", Value: 2}}}}}},
		{"or_branch_on_other_field", []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 0}, {Key: "b", Value: 0}}, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 5}, {Key: "b", Value: 9}}},
			bson.D{{Key: "a", Value: 1}}, bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 1}}}}, bson.D{{Key: "a", Value: 5}}}}}},
		{"or_in_and_range", []interface{}{d(1, 0), d(2, 5), d(3, 3)}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A{5, 3}}}}}, bson.D{{Key: "a", Value: bson.D{{Key: "$lt", Value: 1}}}}}}}},
		{"nested_ne_desc", []interface{}{d(1, bson.A{"x", bson.A{1}}), d(2, "y")}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$ne", Value: "b"}}}}},
		{"nested_nin_desc", []interface{}{d(1, bson.A{"x", bson.A{1}}), d(2, "y")}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$nin", Value: bson.A{"b"}}}}}},
		{"nested_exists_desc", []interface{}{d(1, bson.A{"x", bson.A{1}}), d(2, "y")}, bson.D{{Key: "a", Value: -1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$exists", Value: true}}}}},
		{"range_beside_type_array", []interface{}{d(1, bson.A{2, 3}), d(2, bson.A{0, 4}), d(3, 3)}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$lt", Value: 5}, {Key: "$type", Value: "array"}}}}},
		{"range_beside_type_list", []interface{}{d(1, bson.A{2, 3}), d(2, bson.A{0, 4}), d(3, 3)}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$lt", Value: 5}, {Key: "$type", Value: bson.A{"array", "number"}}}}}},
		{"multikey_gte_lte", []interface{}{d(1, bson.A{1, 9}), d(2, 5), d(3, bson.A{0, 4})}, bson.D{{Key: "a", Value: 1}},
			bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 2}, {Key: "$lte", Value: 6}}}}},
	}
	for _, tc := range cases {
		setup := func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.InsertMany(ctx, tc.docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: tc.index})
			return err
		}
		for _, op := range []string{"find_and_modify", "find"} {
			t.Run(tc.name+"_"+op, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Hint_FirstInBoundsEdges_" + tc.name + "_" + op,
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						if op == "find" {
							return cluFindIDs(tc.filter, options.Find().SetHint(tc.index))(ctx, col)
						}
						var doc bson.D
						err := col.FindOneAndUpdate(ctx, tc.filter, bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: 1}}}},
							options.FindOneAndUpdate().SetHint(tc.index)).Decode(&doc)
						return doc, err
					},
				})
			})
		}
	}
}

// TestCreate_OverExistingComparesStoredOptions: a create over an existing
// namespace succeeds only when it gives the options the namespace was created
// with, as stored: validationLevel and validationAction count even at their
// defaults, validator fields count in order and numbers by value, a collation
// is compared resolved, and a view compares its source and pipeline.
func TestCreate_OverExistingComparesStoredOptions(t *testing.T) {
	validator := func(pairs ...bson.E) bson.E { return bson.E{Key: "validator", Value: bson.D(pairs)} }
	a1b2 := validator(bson.E{Key: "a", Value: 1}, bson.E{Key: "b", Value: 2})
	collation := func(locale string, strength int) bson.E {
		return bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: locale}, {Key: "strength", Value: strength}}}
	}
	cases := []struct {
		name          string
		first, second bson.D
	}{
		{"validator_same", bson.D{a1b2}, bson.D{a1b2}},
		{"validator_plus_moderate", bson.D{a1b2}, bson.D{a1b2, {Key: "validationLevel", Value: "moderate"}}},
		{"validator_plus_strict", bson.D{a1b2}, bson.D{a1b2, {Key: "validationLevel", Value: "strict"}}},
		{"validator_unsorted_same", bson.D{validator(bson.E{Key: "b", Value: 2}, bson.E{Key: "a", Value: 1})}, bson.D{validator(bson.E{Key: "b", Value: 2}, bson.E{Key: "a", Value: 1})}},
		{"validator_unsorted_then_sorted", bson.D{validator(bson.E{Key: "b", Value: 2}, bson.E{Key: "a", Value: 1})}, bson.D{a1b2}},
		{"validator_sorted_then_unsorted", bson.D{a1b2}, bson.D{validator(bson.E{Key: "b", Value: 2}, bson.E{Key: "a", Value: 1})}},
		{"validator_long_vs_int", bson.D{a1b2}, bson.D{validator(bson.E{Key: "a", Value: int64(1)}, bson.E{Key: "b", Value: 2})}},
		{"action_dropped", bson.D{validator(bson.E{Key: "a", Value: 1}), {Key: "validationAction", Value: "warn"}}, bson.D{validator(bson.E{Key: "a", Value: 1})}},
		{"action_same", bson.D{validator(bson.E{Key: "a", Value: 1}), {Key: "validationAction", Value: "warn"}},
			bson.D{validator(bson.E{Key: "a", Value: 1}), {Key: "validationAction", Value: "warn"}}},
		{"action_only", bson.D{validator(bson.E{Key: "a", Value: 1}), {Key: "validationAction", Value: "warn"}}, bson.D{{Key: "validationAction", Value: "warn"}}},
		{"collation_same", bson.D{collation("en", 2)}, bson.D{collation("en", 2)}},
		{"plain_over_collated", bson.D{collation("en", 2)}, bson.D{}},
		{"collation_differs", bson.D{collation("en", 2)}, bson.D{collation("en", 1)}},
		{"collated_over_plain", bson.D{}, bson.D{collation("en", 2)}},
	}
	clean := func(err error) interface{} {
		var ce mongo.CommandError
		if errors.As(err, &ce) {
			msg := regexp.MustCompile(`UUID\("[^"]+"\)`).ReplaceAllString(ce.Message, "UUID(...)")
			msg = regexp.MustCompile(`version: "[^"]+"`).ReplaceAllString(msg, "version")
			return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: msg}}
		}
		return bson.D{{Key: "ok", Value: err == nil}}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Create_OverExistingComparesStoredOptions_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					name := col.Name() + "_c"
					if err := col.Database().RunCommand(ctx, append(bson.D{{Key: "create", Value: name}}, tc.first...)).Err(); err != nil {
						return nil, err
					}
					err := col.Database().RunCommand(ctx, append(bson.D{{Key: "create", Value: name}}, tc.second...)).Err()
					return clean(err), nil
				},
			})
		})
	}
	views := []struct {
		name   string
		second bson.D
	}{
		{"view_same", bson.D{{Key: "viewOn", Value: "base"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: 1}}}}}}}},
		{"view_pipeline_differs", bson.D{{Key: "viewOn", Value: "base"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: 2}}}}}}}},
		{"view_source_differs", bson.D{{Key: "viewOn", Value: "other"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: 1}}}}}}}},
		{"plain_over_view", bson.D{}},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Create_OverExistingComparesStoredOptions_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					name := col.Name() + "_v"
					first := bson.D{{Key: "create", Value: name}, {Key: "viewOn", Value: "base"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: 1}}}}}}}
					if err := col.Database().RunCommand(ctx, first).Err(); err != nil {
						return nil, err
					}
					err := col.Database().RunCommand(ctx, append(bson.D{{Key: "create", Value: name}}, tc.second...)).Err()
					res := clean(err)
					if d, ok := res.(bson.D); ok && len(d) == 2 {
						d[1].Value = strings.ReplaceAll(d[1].Value.(string), col.Database().Name()+".", "<db>.")
						d[1].Value = strings.ReplaceAll(d[1].Value.(string), col.Database().Name(), "<db>")
					}
					return res, nil
				},
			})
		})
	}
}

// TestHint_OnView: a hint on a read of a view is checked against the view's
// base collection, with the command run as an aggregate there: a bad
// $natural value is overlooked only when the stages start with a $match, and
// never by find.
func TestHint_OnView(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}}); err != nil {
			return err
		}
		db := col.Database()
		if err := db.CreateView(ctx, col.Name()+"_v", col.Name(), mongo.Pipeline{}); err != nil {
			return err
		}
		return db.CreateView(ctx, col.Name()+"_vm", col.Name(), mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "a", Value: 1}}}}})
	}
	hints := []struct {
		name string
		hint interface{}
	}{
		{"natural_2", bson.D{{Key: "$natural", Value: 2}}},
		{"natural_string", bson.D{{Key: "$natural", Value: "1"}}},
		{"natural_compound", bson.D{{Key: "$natural", Value: 1}, {Key: "a", Value: 1}}},
		{"natural_1", bson.D{{Key: "$natural", Value: 1}}},
		{"unknown_name", "zz"},
	}
	match := bson.D{{Key: "a", Value: 1}}
	cmds := []struct {
		name string
		cmd  func(c string, h interface{}) bson.D
	}{
		{"find", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "find", Value: c + "_v"}, {Key: "hint", Value: h}}
		}},
		{"find_filter", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "find", Value: c + "_v"}, {Key: "filter", Value: match}, {Key: "hint", Value: h}}
		}},
		{"find_match_view", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "find", Value: c + "_vm"}, {Key: "hint", Value: h}}
		}},
		{"aggregate", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "aggregate", Value: c + "_v"}, {Key: "pipeline", Value: bson.A{}}, {Key: "hint", Value: h}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"aggregate_match", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "aggregate", Value: c + "_v"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: match}}}}, {Key: "hint", Value: h}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"distinct", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "distinct", Value: c + "_v"}, {Key: "key", Value: "a"}, {Key: "hint", Value: h}}
		}},
		{"count", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "count", Value: c + "_v"}, {Key: "hint", Value: h}}
		}},
		{"count_query", func(c string, h interface{}) bson.D {
			return bson.D{{Key: "count", Value: c + "_v"}, {Key: "query", Value: match}, {Key: "hint", Value: h}}
		}},
	}
	for _, h := range hints {
		for _, c := range cmds {
			t.Run(h.name+"_"+c.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Hint_OnView_" + h.name + "_" + c.name,
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						err := col.Database().RunCommand(ctx, c.cmd(col.Name(), h.hint)).Err()
						var ce mongo.CommandError
						if errors.As(err, &ce) {
							// mongod's message for an unknown index embeds its query tree.
							if strings.Contains(ce.Message, "$natural") {
								return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: ce.Message}}, nil
							}
							return bson.D{{Key: "code", Value: ce.Code}}, nil
						}
						return bson.D{{Key: "ok", Value: err == nil}}, nil
					},
				})
			})
		}
	}
}

// TestCollMod_ValidationOptionsAsStored: listCollections reports a validator
// in the order it was given, and collMod of any validation option stores the
// validation level and action, dropping an emptied validator.
func TestCollMod_ValidationOptionsAsStored(t *testing.T) {
	cases := []struct {
		name        string
		create, mod bson.D
	}{
		{"create_unsorted", bson.D{{Key: "validator", Value: bson.D{{Key: "b", Value: 2}, {Key: "a", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$exists", Value: true}}}}}}, nil},
		{"mod_validator", nil, bson.D{{Key: "validator", Value: bson.D{{Key: "z", Value: 1}, {Key: "y", Value: 2}}}}},
		{"mod_level", nil, bson.D{{Key: "validationLevel", Value: "moderate"}}},
		{"mod_action", nil, bson.D{{Key: "validationAction", Value: "warn"}}},
		{"mod_clear", bson.D{{Key: "validator", Value: bson.D{{Key: "b", Value: 1}}}}, bson.D{{Key: "validator", Value: bson.D{}}}},
		{"mod_level_keeps_validator", bson.D{{Key: "validator", Value: bson.D{{Key: "b", Value: 1}}}}, bson.D{{Key: "validationLevel", Value: "moderate"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "CollMod_ValidationOptionsAsStored_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					name := col.Name() + "_c"
					db := col.Database()
					if err := db.RunCommand(ctx, append(bson.D{{Key: "create", Value: name}}, tc.create...)).Err(); err != nil {
						return nil, err
					}
					if tc.mod != nil {
						if err := db.RunCommand(ctx, append(bson.D{{Key: "collMod", Value: name}}, tc.mod...)).Err(); err != nil {
							return nil, err
						}
					}
					specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: name}})
					if err != nil || len(specs) != 1 {
						return nil, err
					}
					var opts bson.D
					if err := bson.Unmarshal(specs[0].Options, &opts); err != nil {
						return nil, err
					}
					out := bson.D{}
					for _, e := range opts {
						switch e.Key {
						case "validator", "validationLevel", "validationAction":
							out = append(out, e)
						}
					}
					return out, nil
				},
			})
		})
	}
}

// TestCreate_OverExistingCollations: the simple collation counts as none
// when a create meets an existing collection, and a view re-create must give
// the view's collation, as mongod reports in its message.
func TestCreate_OverExistingCollations(t *testing.T) {
	simple := bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: "simple"}}}
	en := bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: "en"}}}
	fr := bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: "fr"}}}
	view := func(opts ...bson.E) bson.D {
		return append(bson.D{{Key: "viewOn", Value: "base"}, {Key: "pipeline", Value: bson.A{}}}, opts...)
	}
	cases := []struct {
		name          string
		first, second bson.D
	}{
		{"plain_over_simple", bson.D{simple}, bson.D{}},
		{"simple_over_plain", bson.D{}, bson.D{simple}},
		{"simple_over_simple", bson.D{simple}, bson.D{simple}},
		{"view_without_its_collation", view(en), view()},
		{"view_same_collation", view(en), view(en)},
		{"view_other_collation", view(en), view(fr)},
		{"view_adds_collation", view(), view(en)},
		{"view_simple_collation", view(), view(simple)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Create_OverExistingCollations_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					db := col.Database()
					name := col.Name() + "_c"
					if err := db.RunCommand(ctx, append(bson.D{{Key: "create", Value: name}}, tc.first...)).Err(); err != nil {
						return nil, err
					}
					err := db.RunCommand(ctx, append(bson.D{{Key: "create", Value: name}}, tc.second...)).Err()
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						msg := regexp.MustCompile(`UUID\("[^"]+"\)`).ReplaceAllString(ce.Message, "UUID(...)")
						msg = regexp.MustCompile(`version: "[^"]+"`).ReplaceAllString(msg, "version")
						msg = strings.ReplaceAll(msg, db.Name()+".", "<db>.")
						return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: msg}}, nil
					}
					return bson.D{{Key: "ok", Value: err == nil}}, nil
				},
			})
		})
	}
}
