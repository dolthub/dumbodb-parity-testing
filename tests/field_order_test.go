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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func orderedPair() bson.D {
	return bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 2}}
}

func reversedPair() bson.D {
	return bson.D{{Key: "a", Value: 2}, {Key: "b", Value: 1}}
}

func storedFieldNames(ctx context.Context, col *mongo.Collection, filter interface{}) (interface{}, error) {
	var raw bson.Raw
	if err := col.FindOne(ctx, filter).Decode(&raw); err != nil {
		return nil, err
	}
	nested, err := raw.LookupErr("addr")
	if err != nil {
		return nil, err
	}
	doc, ok := nested.DocumentOK()
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	elements, err := doc.Elements()
	if err != nil {
		return nil, err
	}
	names := make([]interface{}, 0, len(elements))
	for _, element := range elements {
		names = append(names, element.Key())
	}
	return names, nil
}

func TestFieldOrder_StoredOrderIsObservable(t *testing.T) {
	fieldOrderDeviation(t, "field_order_stored_order_is_observable",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return storedFieldNames(ctx, col, bson.D{{Key: "_id", Value: 1}})
		},
		"[b a]", "[a b]")
}

func TestFieldOrder_WholeDocumentEqualityMatch(t *testing.T) {
	fieldOrderDeviation(t, "field_order_whole_document_equality_match",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: reversedPair()}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "addr", Value: orderedPair()}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
		"[1]", "[1 2]")
}

func TestFieldOrder_SortOnEmbeddedDocument(t *testing.T) {
	fieldOrderDeviation(t, "field_order_sort_on_embedded_document",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 9}}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: bson.D{{Key: "a", Value: 9}, {Key: "b", Value: 2}}}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "addr", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
		"[2 1]", "[1 2]")
}

func TestFieldOrder_IDsDifferingOnlyInOrderStayDistinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_ids_differing_only_in_order_stay_distinct",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: orderedPair()}, {Key: "tag", Value: "first"}}); err != nil {
				return nil, err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: reversedPair()}, {Key: "tag", Value: "second"}})
			duplicate := mongo.IsDuplicateKeyError(err)
			if err != nil && !duplicate {
				return nil, err
			}
			count, countErr := col.CountDocuments(ctx, bson.D{})
			if countErr != nil {
				return nil, countErr
			}
			return []interface{}{duplicate, count}, nil
		},
	})
}

func TestFieldOrder_GroupKeyIsEmbeddedDocument(t *testing.T) {
	fieldOrderDeviation(t, "field_order_group_key_is_embedded_document",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: reversedPair()}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$addr"},
					{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "groups", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.M
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return []interface{}{int32(0)}, nil
			}
			return []interface{}{results[0]["groups"]}, nil
		},
		"[2]", "[1]")
}

func TestFieldOrder_DistinctOverEmbeddedDocuments(t *testing.T) {
	fieldOrderDeviation(t, "field_order_distinct_over_embedded_documents",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: reversedPair()}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			values, err := col.Distinct(ctx, "addr", bson.D{})
			if err != nil {
				return nil, err
			}
			return []interface{}{len(values)}, nil
		},
		"[2]", "[1]")
}

func TestFieldOrder_MinMaxOverEmbeddedDocuments(t *testing.T) {
	fieldOrderDeviation(t, "field_order_min_max_over_embedded_documents",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 9}}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: bson.D{{Key: "a", Value: 2}, {Key: "b", Value: 8}}}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, mongo.Pipeline{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "low", Value: bson.D{{Key: "$min", Value: "$addr"}}},
					{Key: "high", Value: bson.D{{Key: "$max", Value: "$addr"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.Raw
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return nil, mongo.ErrNoDocuments
			}
			out := make([]interface{}, 0, 2)
			for _, field := range []string{"low", "high"} {
				value, err := results[0].LookupErr(field)
				if err != nil {
					return nil, err
				}
				doc, ok := value.DocumentOK()
				if !ok {
					return nil, mongo.ErrNoDocuments
				}
				elements, err := doc.Elements()
				if err != nil {
					return nil, err
				}
				names := make([]interface{}, 0, len(elements))
				for _, element := range elements {
					names = append(names, element.Key())
				}
				out = append(out, names)
			}
			return out, nil
		},
		"[[a b] [b a]]", "[[a b] [a b]]")
}

func TestFieldOrder_IndexedSortOnEmbeddedDocument(t *testing.T) {
	fieldOrderDeviation(t, "field_order_indexed_sort_on_embedded_document",
		func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "addr", Value: 1}},
			}); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 9}}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: bson.D{{Key: "a", Value: 9}, {Key: "b", Value: 2}}}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().
				SetSort(bson.D{{Key: "addr", Value: 1}}).
				SetHint(bson.D{{Key: "addr", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
		"[2 1]", "[1 2]")
}

func TestFieldOrder_ArrayOfEmbeddedDocuments(t *testing.T) {
	fieldOrderDeviation(t, "field_order_array_of_embedded_documents",
		func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "items", Value: bson.A{orderedPair()}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "items", Value: bson.A{reversedPair()}}},
			})
			return err
		},
		func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "items", Value: orderedPair()}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
		"[1]", "[1 2]")
}

// fieldOrderDeviation runs a case where DumboDB deliberately differs: it
// stores documents with their keys sorted, which the history independence of
// its prolly tree requires (workspace-7ob). Each server's result is checked
// against its own expectation.
func fieldOrderDeviation(t *testing.T, name string,
	setup func(context.Context, *mongo.Collection) error,
	run func(context.Context, *mongo.Collection) (interface{}, error),
	mongoWant, dumboWant interface{},
) {
	expect := func(server string, want interface{}) func(*testing.T, interface{}, error) {
		return func(t *testing.T, res interface{}, err error) {
			if err != nil || fmt.Sprint(res) != fmt.Sprint(want) {
				t.Errorf("%s: want %v, got %v (err %v)", server, want, res, err)
			}
		}
	}
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    name,
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := tgt.Admin.Database(tgt.NS)
			defer func() { _ = db.Drop(ctx) }()
			col := db.Collection("c")
			if setup != nil {
				tgt.Setup(setup(ctx, col))
			}
			return run(ctx, col)
		},
		MongoExpect: expect("mongo", mongoWant),
		DumboExpect: expect("dumbodb", dumboWant),
	})
}

func collectIDs(ctx context.Context, cursor *mongo.Cursor) (interface{}, error) {
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]interface{}, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc["_id"])
	}
	return ids, nil
}
