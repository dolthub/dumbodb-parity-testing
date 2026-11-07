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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Secondary index key order must be MongoDB's value order for every type, so
// index-served ranges and sorts return what a collection scan would
// (dolthub/dumbodb#206).

// iksInsertValues inserts {_id: i, v: value} for each value in a shuffled
// order and indexes v (and v, w when compound).
func iksInsertValues(values []interface{}, keys bson.D, unique bool) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		ids := make([]interface{}, len(values))
		for i := range ids {
			ids[i] = int32(i)
		}
		for _, id := range cluShuffled(ids) {
			i := id.(int32)
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: i}, {Key: "v", Value: values[i]}, {Key: "w", Value: int32(i % 3)}}); err != nil {
				return err
			}
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(unique)})
		return err
	}
}

var iksV = bson.D{{Key: "v", Value: 1}}

func iksCase(name string, support harness.DumboDBSupport, values []interface{}, keys bson.D, filter bson.D, opts *options.FindOptions) harness.TestCase {
	return harness.TestCase{
		Name:    name,
		Support: support,
		Setup:   iksInsertValues(values, keys, false),
		Run:     cluFindIDs(filter, opts),
	}
}

func TestIndexKeyString_SortHinted_Numeric(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_SortHinted_Numeric", harness.DumboDBFull, cluNumericIDs(), iksV, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_SortHinted_TinyNegatives(t *testing.T) {
	values := []interface{}{-1e-20, -3e-20, -2e-20, -4e-20}
	harness.PairTest(t, iksCase("IndexKeyString_SortHinted_TinyNegatives", harness.DumboDBFull, values, iksV, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_SortHinted_MixedTypes(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_SortHinted_MixedTypes", harness.DumboDBFull, cluMixedIDs(), iksV, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_SortHinted_Embedded(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_SortHinted_Embedded", harness.DumboDBFull, cluEmbeddedIDs(), iksV, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_SortHinted_BinData(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_SortHinted_BinData", harness.DumboDBFull, cluBinDataIDs(), iksV, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_RangeHinted_BinData(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_RangeHinted_BinData", harness.DumboDBFull, cluBinDataIDs(), iksV,
		bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: primitive.Binary{Subtype: 0x00, Data: []byte{0xff}}}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_RangeHinted_TinyNegatives(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_RangeHinted_TinyNegatives", harness.DumboDBFull, cluNumericIDs(), iksV,
		bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: -0.1}, {Key: "$lt", Value: 0}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint(iksV)))
}

func TestIndexKeyString_RangeHinted_Embedded(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_RangeHinted_Embedded", harness.DumboDBFull, cluEmbeddedIDs(), iksV,
		bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: bson.D{{Key: "a", Value: int32(1)}}}, {Key: "$lt", Value: bson.D{{Key: "a", Value: "x"}}}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint(iksV)))
}

var iksVW = bson.D{{Key: "v", Value: 1}, {Key: "w", Value: 1}}

func TestIndexKeyString_CompoundSort_BinData(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_CompoundSort_BinData", harness.DumboDBFull, cluBinDataIDs(), iksVW, bson.D{},
		options.Find().SetSort(iksVW).SetHint(iksVW)))
}

func TestIndexKeyString_CompoundRange_BinData(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_CompoundRange_BinData", harness.DumboDBFull, cluBinDataIDs(), iksVW,
		bson.D{{Key: "v", Value: primitive.Binary{Subtype: 0x00, Data: []byte{0x00, 0x00}}}, {Key: "w", Value: bson.D{{Key: "$gte", Value: int32(0)}}}},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint(iksVW)))
}

func TestIndexKeyString_CompoundSort_Numeric(t *testing.T) {
	harness.PairTest(t, iksCase("IndexKeyString_CompoundSort_Numeric", harness.DumboDBFull, cluNumericIDs(), iksVW, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}, {Key: "w", Value: 1}, {Key: "_id", Value: 1}}).SetHint(iksVW)))
}

func TestIndexKeyString_MultikeySort(t *testing.T) {
	values := []interface{}{
		bson.A{int32(5), -1e-20}, bson.A{-3e-20}, bson.A{primitive.Binary{Data: []byte{0xff}}, "a"},
		bson.A{primitive.Binary{Data: []byte{0, 0}}}, bson.A{bson.D{{Key: "a", Value: int32(1)}}}, bson.A{},
	}
	harness.PairTest(t, iksCase("IndexKeyString_MultikeySort", harness.DumboDBFull, values, iksV, bson.D{},
		options.Find().SetSort(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetHint(iksV)))
}

// Distinct values an inexact encoding would merge must not collide on a
// unique index (#206).
func TestIndexKeyString_Unique_TinyNegatives(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "IndexKeyString_Unique_TinyNegatives",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: iksV, Options: options.Index().SetUnique(true)})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var ok bson.A
			for i, v := range []interface{}{-0.1, -0.10000000000000002, -1e-20, -3e-20} {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: v}})
				ok = append(ok, err == nil)
			}
			return ok, nil
		},
	})
}

func TestIndexKeyString_Unique_Embedded(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "IndexKeyString_Unique_Embedded",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: iksV, Options: options.Index().SetUnique(true)})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var ok bson.A
			for i, v := range cluEmbeddedIDs() {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: v}})
				ok = append(ok, err == nil)
			}
			return ok, nil
		},
	})
}
