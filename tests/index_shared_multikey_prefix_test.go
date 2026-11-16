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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A compound index whose fields reach arrays through the same leading field
// holds keys from one array element at a time, so a document can match the
// two conditions through different elements; mongod still finds it through
// the index, as through a collection scan.
func TestIndexSharedMultikeyPrefix(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	for _, c := range []struct {
		name   string
		key    bson.D
		doc    bson.D
		filter bson.D
	}{
		{"nested_field_and_parent", d("a.b.b", 1, "a", 1), d("a", bson.A{d("b", d("b", 1)), 2}), d("a.b.b", 1, "a", 2)},
		{"sibling_fields", d("a.x", 1, "a.y", 1), d("a", bson.A{d("x", 1, "y", 3), d("x", 2, "y", 4)}), d("a.x", 1, "a.y", 4)},
		{"sibling_fields_reversed", d("a.c", 1, "a.b", 1), d("a", bson.A{d("b", 1, "c", 2), d("b", 3, "c", 4)}), d("a.c", 4, "a.b", 1)},
		{"sibling_fields_same_element", d("a.x", 1, "a.y", 1), d("a", bson.A{d("x", 1, "y", 3), d("x", 2, "y", 4)}), d("a.x", 1, "a.y", 3)},
		{"deeper_and_sibling", d("a.c", 1, "a.b.b", 1), d("a", bson.A{d("b", d("b", 1)), d("c", 1)}), d("a.c", 1, "a.b.b", 1)},
	} {
		for _, w := range []struct {
			name string
			run  func(ctx context.Context, col *mongo.Collection, hint interface{}) (interface{}, error)
		}{
			{"find", func(ctx context.Context, col *mongo.Collection, hint interface{}) (interface{}, error) {
				cur, err := col.Find(ctx, c.filter, options.Find().SetHint(hint).SetProjection(d("_id", 1)))
				if err != nil {
					return nil, err
				}
				var docs []bson.D
				err = cur.All(ctx, &docs)
				return docs, err
			}},
			{"count", func(ctx context.Context, col *mongo.Collection, hint interface{}) (interface{}, error) {
				return col.CountDocuments(ctx, c.filter, options.Count().SetHint(hint))
			}},
			{"deleteMany", func(ctx context.Context, col *mongo.Collection, hint interface{}) (interface{}, error) {
				return writeOutcome(ctx, col, func() error {
					_, err := col.DeleteMany(ctx, c.filter, options.Delete().SetHint(hint))
					return err
				})
			}},
		} {
			for _, h := range []struct {
				name string
				hint func() interface{}
			}{{"hinted", func() interface{} { return c.key }}, {"planned", func() interface{} { return nil }}} {
				name := c.name + "_" + w.name + "_" + h.name
				t.Run(name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    "IndexSharedMultikeyPrefix_" + name,
						Support: supportFor(t, indexSharedMultikeyPrefixXFail),
						Setup: func(ctx context.Context, col *mongo.Collection) error {
							if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: c.key}); err != nil {
								return err
							}
							_, err := col.InsertOne(ctx, append(d("_id", 1), c.doc...))
							return err
						},
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							hint := h.hint()
							if hint == nil {
								return w.run(ctx, col, nil)
							}
							return w.run(ctx, col, hint)
						},
					})
				})
			}
		}
	}
}

// indexSharedMultikeyPrefixXFail lists the cases DumboDB fails until its index
// plans stop bounding fields that share a multikey prefix.
var indexSharedMultikeyPrefixXFail = map[string]bool{
	"TestIndexSharedMultikeyPrefix/deeper_and_sibling_count_hinted":                true,
	"TestIndexSharedMultikeyPrefix/deeper_and_sibling_count_planned":               true,
	"TestIndexSharedMultikeyPrefix/deeper_and_sibling_deleteMany_hinted":           true,
	"TestIndexSharedMultikeyPrefix/deeper_and_sibling_deleteMany_planned":          true,
	"TestIndexSharedMultikeyPrefix/deeper_and_sibling_find_hinted":                 true,
	"TestIndexSharedMultikeyPrefix/deeper_and_sibling_find_planned":                true,
	"TestIndexSharedMultikeyPrefix/nested_field_and_parent_count_hinted":           true,
	"TestIndexSharedMultikeyPrefix/nested_field_and_parent_count_planned":          true,
	"TestIndexSharedMultikeyPrefix/nested_field_and_parent_deleteMany_hinted":      true,
	"TestIndexSharedMultikeyPrefix/nested_field_and_parent_deleteMany_planned":     true,
	"TestIndexSharedMultikeyPrefix/nested_field_and_parent_find_hinted":            true,
	"TestIndexSharedMultikeyPrefix/nested_field_and_parent_find_planned":           true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_count_hinted":                    true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_count_planned":                   true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_deleteMany_hinted":               true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_deleteMany_planned":              true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_find_hinted":                     true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_find_planned":                    true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_reversed_count_hinted":           true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_reversed_count_planned":          true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_reversed_deleteMany_hinted":      true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_reversed_deleteMany_planned":     true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_reversed_find_hinted":            true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_reversed_find_planned":           true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_same_element_count_hinted":       true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_same_element_count_planned":      true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_same_element_deleteMany_hinted":  true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_same_element_deleteMany_planned": true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_same_element_find_hinted":        true,
	"TestIndexSharedMultikeyPrefix/sibling_fields_same_element_find_planned":       true,
}
