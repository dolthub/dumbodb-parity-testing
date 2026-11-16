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
	"math"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod's message for a value a geo index refuses names null as null, says
// which level of coordinates is malformed, repeats the value in full, and
// cuts each string it repeats to 150 bytes. Every document here has its keys
// in sorted order, the order DumboDB stores and repeats them in. DumboDB never
// cuts a string inside a UTF-8 character, which mongod does when byte 150
// falls inside one.
func TestGeoErrorMessages(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	geo := func(typ string, coords interface{}) bson.D { return d("coordinates", coords, "type", typ) }
	circle := func(n int, closed bool) bson.A {
		var ring bson.A
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			ring = append(ring, bson.A{math.Round(math.Cos(a)*1e6) / 1e6, math.Round(math.Sin(a)*1e6) / 1e6})
		}
		if closed {
			ring = append(ring, ring[0])
		}
		return ring
	}
	for _, c := range []struct {
		name  string
		index string
		loc   interface{}
		pad   interface{}
	}{
		{"point_coordinates_null", "2dsphere", geo("Point", nil), ""},
		{"point_coordinates_missing", "2dsphere", d("type", "Point"), ""},
		{"geometries_null", "2dsphere", d("geometries", nil, "type", "GeometryCollection"), ""},
		{"geometries_element_null", "2dsphere", d("geometries", bson.A{nil}, "type", "GeometryCollection"), ""},
		{"linestring_coordinates_null", "2dsphere", geo("LineString", nil), ""},
		{"linestring_coordinates_string", "2dsphere", geo("LineString", "x"), ""},
		{"linestring_position_number", "2dsphere", geo("LineString", bson.A{bson.A{0, 0}, 5}), ""},
		{"linestring_position_null", "2dsphere", geo("LineString", bson.A{nil}), ""},
		{"multipoint_position_null", "2dsphere", geo("MultiPoint", bson.A{nil}), ""},
		{"polygon_coordinates_null", "2dsphere", geo("Polygon", nil), ""},
		{"polygon_ring_number", "2dsphere", geo("Polygon", bson.A{5}), ""},
		{"polygon_position_number", "2dsphere", geo("Polygon", bson.A{bson.A{bson.A{0, 0}, 5, bson.A{1, 1}, bson.A{0, 0}}}), ""},
		{"polygon_unclosed_short", "2dsphere", geo("Polygon", bson.A{bson.A{bson.A{0, 0}, bson.A{1, 1}, bson.A{2, 0}}}), ""},
		{"multilinestring_coordinates_null", "2dsphere", geo("MultiLineString", nil), ""},
		{"multilinestring_line_null", "2dsphere", geo("MultiLineString", bson.A{nil}), ""},
		{"multilinestring_position_number", "2dsphere", geo("MultiLineString", bson.A{bson.A{5}}), ""},
		{"multipolygon_coordinates_null", "2dsphere", geo("MultiPolygon", nil), ""},
		{"multipolygon_polygon_number", "2dsphere", geo("MultiPolygon", bson.A{5}), ""},
		{"multipolygon_ring_number", "2dsphere", geo("MultiPolygon", bson.A{bson.A{5}}), ""},
		{"multipolygon_position_number", "2dsphere", geo("MultiPolygon", bson.A{bson.A{bson.A{5}}}), ""},
		{"polygon_unclosed_long", "2dsphere", geo("Polygon", bson.A{circle(500, false)}), ""},
		{"linestring_one_vertex_long", "2dsphere", geo("LineString", func() bson.A {
			var line bson.A
			for i := 0; i < 400; i++ {
				line = append(line, bson.A{1.25, 2.5})
			}
			return line
		}()), ""},
		{"long_string_beside_point", "2dsphere", geo("Point", bson.A{500, 0}), strings.Repeat("x", 1000)},
		{"long_multibyte_string_beside_point", "2dsphere", geo("Point", bson.A{500, 0}), strings.Repeat("\u00e9", 400)},
		{"long_string_beside_2d_point", "2d", bson.A{500, 0}, strings.Repeat("x", 1000)},
		{"symbol_loc", "2dsphere", primitive.Symbol("x"), nil},
		{"javascript_loc", "2dsphere", primitive.JavaScript("f"), nil},
		{"long_symbol_beside_point", "2dsphere", geo("Point", bson.A{500, 0}), primitive.Symbol(strings.Repeat("x", 1000))},
		{"long_javascript_beside_point", "2dsphere", geo("Point", bson.A{500, 0}), primitive.JavaScript(strings.Repeat("x", 1000))},
	} {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "GeoErrorMessages_" + c.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: d("loc", c.index)})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					doc := d("_id", 1, "loc", c.loc)
					if c.pad != nil {
						doc = append(doc, bson.E{Key: "pad", Value: c.pad})
					}
					_, err := col.InsertOne(ctx, doc)
					var we mongo.WriteException
					if !errors.As(err, &we) || len(we.WriteErrors) == 0 {
						return bson.D{{Key: "err", Value: errCode(err)}}, nil
					}
					return d("code", we.WriteErrors[0].Code, "message", we.WriteErrors[0].Message), nil
				},
			})
		})
	}
}
