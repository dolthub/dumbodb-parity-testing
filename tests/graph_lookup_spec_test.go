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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestGraphLookupSpec: $graphLookup reads 'from' first, then its other fields
// in order, checking each one's type and field path, refuses unknown fields,
// and only then requires the missing ones (40105). The same parse runs when a
// view holding the stage is created.
func TestGraphLookupSpec(t *testing.T) {
	base := func(extra ...bson.E) bson.D {
		d := bson.D{{Key: "from", Value: "c"}, {Key: "startWith", Value: "$x"},
			{Key: "connectFromField", Value: "a"}, {Key: "connectToField", Value: "b"}, {Key: "as", Value: "r"}}
		return append(d, extra...)
	}
	with := func(key string, value any) bson.D {
		d := base()
		for i := range d {
			if d[i].Key == key {
				d[i].Value = value
			}
		}
		return d
	}
	dec, _ := primitive.ParseDecimal128("2")
	decFrac, _ := primitive.ParseDecimal128("2.5")
	cases := map[string]any{
		"not_object":            int32(5),
		"missing_from":          bson.D{{Key: "as", Value: int32(5)}},
		"from_not_string":       bson.D{{Key: "bogus", Value: 1}, {Key: "from", Value: int32(5)}},
		"from_document":         with("from", bson.D{{Key: "db", Value: "x"}, {Key: "coll", Value: "c"}}),
		"only_from":             bson.D{{Key: "from", Value: "c"}},
		"unknown_before_bad_as": bson.D{{Key: "from", Value: "c"}, {Key: "bogus", Value: 1}, {Key: "as", Value: int32(5)}},
		"bad_as_before_unknown": bson.D{{Key: "from", Value: "c"}, {Key: "as", Value: int32(5)}, {Key: "bogus", Value: 1}},
		"unknown_field":         base(bson.E{Key: "bogus", Value: 1}),
		"max_depth_string":      base(bson.E{Key: "maxDepth", Value: "x"}),
		"max_depth_null":        base(bson.E{Key: "maxDepth", Value: nil}),
		"max_depth_negative":    base(bson.E{Key: "maxDepth", Value: int32(-1)}),
		"max_depth_fraction":    base(bson.E{Key: "maxDepth", Value: 1.5}),
		"max_depth_huge":        base(bson.E{Key: "maxDepth", Value: 1e30}),
		"max_depth_decimal":     base(bson.E{Key: "maxDepth", Value: dec}),
		"max_depth_decimal_fr":  base(bson.E{Key: "maxDepth", Value: decFrac}),
		"depth_field_int":       base(bson.E{Key: "depthField", Value: int32(5)}),
		"depth_field_empty":     base(bson.E{Key: "depthField", Value: ""}),
		"match_not_object":      base(bson.E{Key: "restrictSearchWithMatch", Value: int32(5)}),
		"match_bad_operator":    base(bson.E{Key: "restrictSearchWithMatch", Value: bson.D{{Key: "$bogus", Value: 1}}}),
		"match_text":            base(bson.E{Key: "restrictSearchWithMatch", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "x"}}}}}),
		"connect_from_dollar":   with("connectFromField", "$a"),
		"connect_to_empty_part": with("connectToField", "b..c"),
		"as_trailing_dot":       with("as", "r."),
		"as_nul":                with("as", "p\x00q"),
		"connect_from_nul":      with("connectFromField", "p\x00q"),
		"depth_field_nul":       base(bson.E{Key: "depthField", Value: "p\x00q"}),
	}
	for name, spec := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "GraphLookupSpec_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$graphLookup", Value: spec}}})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
	views := map[string]bson.D{
		"start_with_undefined": with("startWith", "$$nope"),
		"start_with_empty":     with("startWith", "$a..b"),
		"missing_fields":       bson.D{{Key: "from", Value: "c"}, {Key: "as", Value: "r"}},
		"valid":                base(),
	}
	for name, spec := range views {
		harness.PairTest(t, harness.TestCase{
			Name:    "GraphLookupSpec_view_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				view := col.Name() + "_v"
				_ = col.Database().Collection(view).Drop(ctx)
				return runCommandDoc(ctx, col, bson.D{{Key: "create", Value: view}, {Key: "viewOn", Value: col.Name()},
					{Key: "pipeline", Value: bson.A{bson.D{{Key: "$graphLookup", Value: spec}}}}})
			},
		})
	}
}
