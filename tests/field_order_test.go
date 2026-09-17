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

// How far DumboDB's lex-sorted field order actually reaches (workspace-fr9).
//
// internal/backends/dolt/bson_codec.go sorts document keys at every level on
// write, as the canonical form Dolt's diff and merge depend on. MongoDB
// preserves insertion order. The cosmetic consequence is known and the
// comparator normalizes it away. These cases measure the non-cosmetic
// consequence: MongoDB compares embedded documents field by field IN ORDER, so
// two documents differing only in field order are not equal to it and can sort
// differently.
//
// Every case returns order as a SLICE, never as document field order. The
// comparator normalizes documents to maps, so a case that returned a document
// would compare equal no matter what the servers did, and would be measuring
// nothing.

package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// orderedPair is {b, a}: written deliberately out of lexical order so a server
// that sorts keys produces a different byte sequence than one that does not.
func orderedPair() bson.D {
	return bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 2}}
}

// reversedPair is the same field values in lexical order.
func reversedPair() bson.D {
	return bson.D{{Key: "a", Value: 2}, {Key: "b", Value: 1}}
}

// storedFieldNames reports the field order a server actually returns for the
// embedded document, as a slice so the comparison is order sensitive.
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

// TestFieldOrder_StoredOrderIsObservable is the baseline the rest depend on.
// If this matches, DumboDB preserves order after all and the deviation does not
// exist; if it diverges, the remaining cases say how far the divergence reaches.
func TestFieldOrder_StoredOrderIsObservable(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_stored_order_is_observable",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return storedFieldNames(ctx, col, bson.D{{Key: "_id", Value: 1}})
		},
	})
}

// TestFieldOrder_WholeDocumentEqualityMatch covers find({addr: {...}}), which
// in MongoDB is an ordered exact match on the whole embedded document rather
// than a subset match. A server that sorts keys on write and sorts the query
// side identically stays self-consistent and matches; one that sorts only one
// side does not.
func TestFieldOrder_WholeDocumentEqualityMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_whole_document_equality_match",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: reversedPair()}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Query in the order the first document was written. MongoDB
			// matches only the document whose stored order is identical.
			cursor, err := col.Find(ctx, bson.D{{Key: "addr", Value: orderedPair()}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
	})
}

// TestFieldOrder_SortOnEmbeddedDocument covers sorting by a field whose value
// is a whole document. MongoDB orders those field by field in stored order, so
// the resulting sequence of _ids is the measurement.
//
// The two documents are chosen so the orderings MUST differ if the deviation
// reaches sorting, because most data hides it. MongoDB compares the first
// field of each: {b:1,...} against {a:9,...}, and "a" sorts before "b", so _id
// 2 comes first. DumboDB has sorted both to a-first, making them {a:9,b:1} and
// {a:9,b:2}, which tie on a and break on b, putting _id 1 first. An earlier
// version of this case used {b:1,a:9}, {a:2,b:8}, {a:1,b:7} and passed, but
// only because those three happen to sort identically under both rules. Do not
// weaken the data back.
func TestFieldOrder_SortOnEmbeddedDocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_sort_on_embedded_document",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 9}}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: bson.D{{Key: "a", Value: 9}, {Key: "b", Value: 2}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "addr", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
	})
}

// TestFieldOrder_IDsDifferingOnlyInOrderStayDistinct is the one place DumboDB
// is documented to agree with MongoDB: sortDocumentKeys exempts the top-level
// _id and hashID hashes the original bytes, so two _id values differing only in
// field order must remain two distinct documents on both servers.
//
// Worth proving rather than assuming, because if it ever regressed the second
// insert would silently collide with the first and a document would vanish.
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

// TestFieldOrder_GroupKeyIsEmbeddedDocument covers $group with a document
// valued _id. Two documents whose embedded values differ only in field order
// fall in the same group or in different ones depending on whether the server
// treats order as significant, so the group count is the measurement.
func TestFieldOrder_GroupKeyIsEmbeddedDocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_group_key_is_embedded_document",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: reversedPair()}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
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
	})
}

// TestFieldOrder_DistinctOverEmbeddedDocuments asks the same question through
// distinct, which dedupes by the same comparison rules.
func TestFieldOrder_DistinctOverEmbeddedDocuments(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_distinct_over_embedded_documents",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: orderedPair()}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: reversedPair()}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			values, err := col.Distinct(ctx, "addr", bson.D{})
			if err != nil {
				return nil, err
			}
			return []interface{}{len(values)}, nil
		},
	})
}

// TestFieldOrder_MinMaxOverEmbeddedDocuments covers $min and $max, which pick a
// winner using the same ordered comparison. The winner's _id is the
// measurement; returning the winning document would be normalized away.
func TestFieldOrder_MinMaxOverEmbeddedDocuments(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_min_max_over_embedded_documents",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "addr", Value: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 9}}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "addr", Value: bson.D{{Key: "a", Value: 2}, {Key: "b", Value: 8}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
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
			// Report the winners by their field order, as slices.
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
	})
}

// TestFieldOrder_IndexedSortOnEmbeddedDocument repeats the sort case with an
// index on the embedded field, since the index stores its own key encoding and
// can order differently from a collection scan. Same discriminating documents
// as the unindexed case, for the reason given there.
func TestFieldOrder_IndexedSortOnEmbeddedDocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_indexed_sort_on_embedded_document",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
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
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().
				SetSort(bson.D{{Key: "addr", Value: 1}}).
				SetHint(bson.D{{Key: "addr", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
	})
}

// TestFieldOrder_ArrayOfEmbeddedDocuments covers comparison of arrays whose
// elements are documents, where element order and field order interact.
func TestFieldOrder_ArrayOfEmbeddedDocuments(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "field_order_array_of_embedded_documents",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "items", Value: bson.A{orderedPair()}}},
				bson.D{{Key: "_id", Value: 2}, {Key: "items", Value: bson.A{reversedPair()}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "items", Value: orderedPair()}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		},
	})
}

// collectIDs returns the _id values in cursor order, as a slice, so the
// comparison stays sensitive to sequence.
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
